// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package provisioner implements the Omni cloud provider QEMU provisioner.
package provisioner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	sideronet "github.com/siderolabs/net"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/provision"
	"github.com/siderolabs/talos/pkg/provision/providers"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapio"
)

const (
	providerName = "qemu"
	diskSize     = 6 * 1024 * 1024 * 1024
	diskDriver   = "virtio"
	memSize      = 3072 * 1024 * 1024
	mtu          = 1500
	cniBundleURL = "https://github.com/siderolabs/talos/releases/latest/download/talosctl-cni-bundle-amd64.tar.gz"
)

// Provisioner is the Omni cloud provider QEMU provisioner.
//
// It watches the MachineRequest resources and provisions them as QEMU VMs as needed.
type Provisioner struct {
	logger *zap.Logger

	subnetCIDR netip.Prefix

	talosctlPath string
	stateDir     string
	cniDir       string
	clusterName  string

	nameservers []netip.Addr

	numMachines    int
	ipxeServerPort int
}

// New creates a new QEMU provisioner.
func New(talosctlPath, clusterName, subnetCIDR string, nameservers []string, numMachines, ipxeServerPort int, logger *zap.Logger) (*Provisioner, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	talosDir, err := config.GetTalosDirectory()
	if err != nil {
		return nil, fmt.Errorf("failed to get Talos directory: %w", err)
	}

	stateDir := filepath.Join(talosDir, "clusters")
	cniDir := filepath.Join(talosDir, "cni")

	logger = logger.With(zap.String("cluster_name", clusterName), zap.String("state_dir", stateDir))

	if talosctlPath == "" {
		if talosctlPath, err = findTalosctl(); err != nil {
			return nil, fmt.Errorf("failed to find talosctl binary: %w", err)
		}
	}

	cidr, err := netip.ParsePrefix(subnetCIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subnet CIDR: %w", err)
	}

	nss, err := parseNameservers(nameservers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse nameservers: %w", err)
	}

	return &Provisioner{
		talosctlPath:   talosctlPath,
		stateDir:       stateDir,
		cniDir:         cniDir,
		clusterName:    clusterName,
		subnetCIDR:     cidr,
		nameservers:    nss,
		ipxeServerPort: ipxeServerPort,
		numMachines:    numMachines,
		logger:         logger,
	}, nil
}

// Run starts the provisioner by provisioning a QEMU cluster with the given number of machines. If the cluster already exists, it is loaded instead.
func (provisioner *Provisioner) Run(ctx context.Context) error {
	qemuProvisioner, err := providers.Factory(ctx, providerName)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	gatewayAddr, err := sideronet.NthIPInNetwork(provisioner.subnetCIDR, 1)
	if err != nil {
		return fmt.Errorf("failed to get gateway address: %w", err)
	}

	ipxeBootScript := fmt.Sprintf("http://%s/ipxe?uuid=${uuid}", net.JoinHostPort(gatewayAddr.String(), strconv.Itoa(provisioner.ipxeServerPort)))

	existingCluster, err := qemuProvisioner.Reflect(ctx, provisioner.clusterName, provisioner.stateDir)
	if err == nil {
		provisioner.logger.Info("loaded existing cluster")

		if len(existingCluster.Info().Nodes) != provisioner.numMachines {
			provisioner.logger.Warn("number of nodes in the existing cluster does not match the requested number of machines",
				zap.Int("requested", provisioner.numMachines),
				zap.Int("existing", len(existingCluster.Info().Nodes)))
		}

		return nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to load existing cluster: %w", err)
	}

	provisioner.logger.Info("create a new cluster")

	nodes := make([]provision.NodeRequest, 0, provisioner.numMachines)

	nanoCPUs, err := parseCPUShare("3")
	if err != nil {
		return fmt.Errorf("failed to parse CPU share: %w", err)
	}

	for i := range provisioner.numMachines {
		nodeUUID := uuid.New()

		provisioner.logger.Info("generated node UUID", zap.String("uuid", nodeUUID.String()))

		var ip netip.Addr

		ip, err = sideronet.NthIPInNetwork(provisioner.subnetCIDR, i+2)
		if err != nil {
			return fmt.Errorf("failed to calculate offset %d from CIDR %s: %w", i+2, provisioner.subnetCIDR, err)
		}

		nodes = append(nodes, provision.NodeRequest{
			Name: nodeUUID.String(),
			Type: machine.TypeWorker,

			IPs:      []netip.Addr{ip},
			Memory:   memSize,
			NanoCPUs: nanoCPUs,
			Disks: []*provision.Disk{
				{
					Size:   diskSize,
					Driver: diskDriver,
				},
			},
			SkipInjectingConfig: true,
			UUID:                &nodeUUID,
		})
	}

	request := provision.ClusterRequest{
		Name: provisioner.clusterName,

		Network: provision.NetworkRequest{
			Name:         provisioner.clusterName,
			CIDRs:        []netip.Prefix{provisioner.subnetCIDR},
			GatewayAddrs: []netip.Addr{gatewayAddr},
			MTU:          mtu,
			Nameservers:  provisioner.nameservers,
			CNI: provision.CNIConfig{
				BinPath:   []string{filepath.Join(provisioner.cniDir, "bin")},
				ConfDir:   filepath.Join(provisioner.cniDir, "conf.d"),
				CacheDir:  filepath.Join(provisioner.cniDir, "cache"),
				BundleURL: cniBundleURL,
			},
		},

		IPXEBootScript: ipxeBootScript,
		SelfExecutable: provisioner.talosctlPath,
		StateDirectory: provisioner.stateDir,

		Nodes: nodes,
	}

	logWriter := zapio.Writer{
		Log:   provisioner.logger,
		Level: zapcore.InfoLevel,
	}
	defer logWriter.Close() //nolint:errcheck

	if _, err = qemuProvisioner.Create(ctx, request,
		provision.WithBootlader(true),
		provision.WithUEFI(true),
		provision.WithLogWriter(&logWriter),
	); err != nil {
		return fmt.Errorf("failed to create cluster: %w", err)
	}

	<-ctx.Done()

	return nil
}

// ClearState clears the state of the provisioner.
func (provisioner *Provisioner) ClearState(ctx context.Context) error {
	qemuProvisioner, err := providers.Factory(ctx, providerName)
	if err != nil {
		return fmt.Errorf("failed to create provisioner: %w", err)
	}

	// attempt to load existing cluster
	cluster, err := qemuProvisioner.Reflect(ctx, provisioner.clusterName, provisioner.stateDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to load existing cluster while clearing state: %w", err)
	}

	logWriter := zapio.Writer{
		Log:   provisioner.logger,
		Level: zapcore.InfoLevel,
	}
	defer logWriter.Close() //nolint:errcheck

	if err = qemuProvisioner.Destroy(ctx, cluster, provision.WithDeleteOnErr(true), provision.WithLogWriter(&logWriter)); err != nil {
		if strings.Contains(err.Error(), "no such network interface") {
			return nil
		}

		return fmt.Errorf("failed to destroy cluster: %w", err)
	}

	return nil
}

// ResetMachine resets the machine with the given UUID by wiping its disk and rebooting it.
func (provisioner *Provisioner) ResetMachine(ctx context.Context, uuid string) error {
	logger := provisioner.logger.With(zap.String("uuid", uuid), zap.String("op", "reset"))

	launchConf, err := provisioner.readMachineLaunchConfig(uuid)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Warn("machine launch config not found, assume it is already reset")

			return nil
		}

		return err
	}

	if len(launchConf.DiskPaths) != 1 {
		logger.Warn("unexpected number of disk paths", zap.Int("num_disk_paths", len(launchConf.DiskPaths)))
	}

	if len(launchConf.DiskPaths) > 0 {
		diskFile := launchConf.DiskPaths[0]

		logger.Info("wipe the disk file", zap.String("disk_file", diskFile))

		if err = os.Truncate(diskFile, 0); err != nil {
			return err
		}

		if err = os.Truncate(diskFile, diskSize); err != nil {
			return err
		}
	}

	if len(launchConf.GatewayAddrs) == 0 {
		logger.Warn("no gateway address found")

		return nil
	}

	gatewayAddr := launchConf.GatewayAddrs[0]
	rebootEndpoint := "http://" + net.JoinHostPort(gatewayAddr.String(), strconv.Itoa(launchConf.APIPort)) + "/reboot"

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rebootEndpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create reboot request: %w", err)
	}

	logger.Info("rebooting machine", zap.String("reboot_endpoint", rebootEndpoint))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make reboot request: %w", err)
	}

	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code while resetting machine: %d", resp.StatusCode)
	}

	return nil
}

// readMachineLaunchConfig reads the machine launch config JSON of the machine with the given UUID from the cluster state directory of this provisioner.
//
// It is used to get the disk paths and gateway address of the machine, to be used to reset the machine disk and to reboot the machine.
func (provisioner *Provisioner) readMachineLaunchConfig(uuid string) (launchConfig, error) {
	configPath := filepath.Join(provisioner.stateDir, provisioner.clusterName, uuid+".config")

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return launchConfig{}, fmt.Errorf("failed to read machine launch config JSON: %w", err)
	}

	var conf launchConfig

	if err = json.Unmarshal(configData, &conf); err != nil {
		return launchConfig{}, fmt.Errorf("failed to unmarshal machine launch config JSON: %w", err)
	}

	return conf, nil
}

// launchConfig is the JSON structure of the machine launch config, containing only the fields needed by this provisioner.
type launchConfig struct {
	DiskPaths    []string
	GatewayAddrs []netip.Addr
	APIPort      int
}

func parseCPUShare(cpus string) (int64, error) {
	cpu, ok := new(big.Rat).SetString(cpus)
	if !ok {
		return 0, fmt.Errorf("failed to parsing as a rational number: %s", cpus)
	}

	nano := cpu.Mul(cpu, big.NewRat(1e9, 1))
	if !nano.IsInt() {
		return 0, errors.New("value is too precise")
	}

	return nano.Num().Int64(), nil
}

func findTalosctl() (string, error) {
	name := "talosctl"

	// check the current working directory
	if stat, err := os.Stat(name); err == nil && !stat.IsDir() {
		return name, nil
	}

	// check the PATH
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("failed to find talosctl binary: %w", err)
	}

	return path, nil
}

func parseNameservers(nameservers []string) ([]netip.Addr, error) {
	nss := make([]netip.Addr, 0, len(nameservers))

	for _, ns := range nameservers {
		addr, err := netip.ParseAddr(ns)
		if err != nil {
			return nil, fmt.Errorf("failed to parse nameserver %q: %w", ns, err)
		}

		nss = append(nss, addr)
	}

	return nss, nil
}
