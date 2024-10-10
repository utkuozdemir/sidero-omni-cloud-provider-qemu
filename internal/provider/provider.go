// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package provider holds the main logic of the Omni cloud provider QEMU.
package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/cosi-project/runtime/pkg/controller/runtime"
	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/resource/protobuf"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/hashicorp/go-multierror"
	"github.com/siderolabs/omni/client/pkg/client"
	omniresources "github.com/siderolabs/omni/client/pkg/omni/resources"
	"github.com/siderolabs/omni/client/pkg/omni/resources/cloud"
	"github.com/siderolabs/omni/client/pkg/omni/resources/system"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/controller"
	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/ipxe"
	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/meta"
	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/provisioner"
	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/resources"
)

// Provider is the Omni cloud provider QEMU provider.
//
// It watches the MachineRequest resources and provisions them as QEMU VMs as needed.
type Provider struct {
	logger                *zap.Logger
	provisioner           *provisioner.Provisioner
	omniEndpoint          string
	omniServiceAccountKey string
	imageFactoryPXEURL    string
	ipxeServerPort        int

	clear bool
}

// New creates a new provider.
func New(omniEndpoint, omniServiceAccountKey, talosctlPath, imageFactoryPXEURL, subnetCIDR string, nameservers []string,
	numMachines, ipxeServerPort int, clearState bool, logger *zap.Logger,
) (*Provider, error) {
	if omniEndpoint == "" {
		return nil, fmt.Errorf("omniEndpoint is required")
	}

	if omniServiceAccountKey == "" {
		return nil, fmt.Errorf("omni service account key is required")
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	clusterName := "omni-cloud-provider-" + meta.ProviderID

	qemuProvisioner, err := provisioner.New(talosctlPath, clusterName, subnetCIDR, nameservers, numMachines, ipxeServerPort, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create provisioner: %w", err)
	}

	return &Provider{
		omniEndpoint:          omniEndpoint,
		omniServiceAccountKey: omniServiceAccountKey,
		imageFactoryPXEURL:    imageFactoryPXEURL,
		ipxeServerPort:        ipxeServerPort,
		clear:                 clearState,
		provisioner:           qemuProvisioner,
		logger:                logger,
	}, nil
}

// Run runs the provider.
func (provider *Provider) Run(ctx context.Context) (runErr error) {
	omniClient, err := provider.omniClient()
	if err != nil {
		return err
	}

	omniState := omniClient.Omni().State()

	if provider.clear {
		if err = provider.clearState(ctx, omniState); err != nil {
			return fmt.Errorf("failed to clear provider state: %w", err)
		}
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return provider.watchSysVersion(ctx, omniClient.Omni().State())
	})

	eg.Go(func() error {
		server := ipxe.NewServer(omniState, provider.imageFactoryPXEURL, provider.ipxeServerPort, provider.logger)

		ipxeServerErr := server.Run(ctx)
		if ipxeServerErr != nil {
			provider.logger.Error("failed to start iPXE server", zap.Error(ipxeServerErr))
		}

		return ipxeServerErr
	})

	eg.Go(func() error {
		provisionErr := provider.provisioner.Run(ctx)
		if provisionErr != nil {
			provider.logger.Error("failed to provision machines", zap.Error(provisionErr))
		}

		return provisionErr
	})

	defer func() {
		if closeErr := omniClient.Close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("failed to close Omni client: %w", closeErr))
		}
	}()

	if err = protobuf.RegisterResource(resources.QemuMachineType, &resources.QemuMachine{}); err != nil {
		return fmt.Errorf("failed to register resource: %w", err)
	}

	if err = protobuf.RegisterResource(resources.QemuMachineAllocationType, &resources.QemuMachineAllocation{}); err != nil {
		return fmt.Errorf("failed to register resource: %w", err)
	}

	cosiRuntime, err := runtime.NewRuntime(omniState, provider.logger)
	if err != nil {
		return fmt.Errorf("failed to create runtime: %w", err)
	}

	if err = cosiRuntime.RegisterQController(controller.NewMachineRequestStatusController(provider.provisioner)); err != nil {
		return fmt.Errorf("failed to register controller: %w", err)
	}

	if err = cosiRuntime.Run(ctx); err != nil {
		return fmt.Errorf("failed to run runtime: %w", err)
	}

	return eg.Wait()
}

// watchSysVersion serves as a health check - if the connection to the Omni API is lost (e.g., due to an Omni restart),
// this will return an error and the provider will crash.
func (provider *Provider) watchSysVersion(ctx context.Context, st state.State) error {
	eventCh := make(chan state.Event)

	if err := st.Watch(ctx, system.NewSysVersion(omniresources.EphemeralNamespace, system.SysVersionID).Metadata(), eventCh); err != nil {
		return fmt.Errorf("failed to watch system version: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-eventCh:
			if event.Type == state.Errored {
				return event.Error
			}
		}
	}
}

func (provider *Provider) omniClient() (*client.Client, error) {
	var cliOpts []client.Option

	if provider.omniServiceAccountKey != "" {
		cliOpts = append(cliOpts, client.WithServiceAccount(provider.omniServiceAccountKey))
	}

	omniClient, err := client.New(provider.omniEndpoint, cliOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create Omni client: %w", err)
	}

	return omniClient, nil
}

func (provider *Provider) clearState(ctx context.Context, st state.State) error {
	provider.logger.Info("clearing provider state")

	if err := provider.provisioner.ClearState(ctx); err != nil {
		return err
	}

	statusList, err := st.List(ctx, cloud.NewMachineRequestStatus("").Metadata())
	if err != nil {
		return err
	}

	qemuMachineList, err := st.List(ctx, resources.NewQemuMachine("").Metadata())
	if err != nil {
		return err
	}

	qemuMachineAllocationList, err := st.List(ctx, resources.NewQemuMachineAllocation("").Metadata())
	if err != nil {
		return err
	}

	var errs error

	for _, list := range []resource.List{statusList, qemuMachineList, qemuMachineAllocationList} {
		for _, item := range list.Items {
			res, getErr := st.Get(ctx, item.Metadata())
			if getErr != nil {
				errs = multierror.Append(errs, getErr)

				continue
			}

			if destroyErr := st.Destroy(ctx, item.Metadata(), state.WithDestroyOwner(res.Metadata().Owner())); destroyErr != nil {
				errs = multierror.Append(errs, destroyErr)

				continue
			}

			provider.logger.Info("destroyed resource", zap.String("id", item.Metadata().ID()))
		}
	}

	return errs
}
