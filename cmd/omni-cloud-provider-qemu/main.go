// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package main contains the entrypoint.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/siderolabs/omni-cloud-provider-qemu/internal/debug"
	"github.com/siderolabs/omni-cloud-provider-qemu/internal/meta"
	"github.com/siderolabs/omni-cloud-provider-qemu/internal/provider"
	"github.com/siderolabs/omni-cloud-provider-qemu/internal/version"
)

var rootCmdArgs struct {
	omniAPIEndpoint    string
	talosctlPath       string
	imageFactoryPXEURL string
	subnetCIDR         string

	nameservers []string

	ipxeServerPort int
	numMachines    int

	clearState bool
	debug      bool
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     version.Name,
	Short:   "Provision QEMU VMs for Omni.",
	Version: version.Tag,
	Args:    cobra.NoArgs,
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		cmd.SilenceUsage = true // if the args are parsed fine, no need to show usage
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger, err := initLogger()
		if err != nil {
			return fmt.Errorf("failed to create logger: %w", err)
		}

		return run(cmd.Context(), logger)
	},
}

func run(ctx context.Context, logger *zap.Logger) error {
	omniServiceAccountKey := os.Getenv("OMNI_SERVICE_ACCOUNT_KEY")

	prov, err := provider.New(rootCmdArgs.omniAPIEndpoint, omniServiceAccountKey, rootCmdArgs.talosctlPath, rootCmdArgs.imageFactoryPXEURL,
		rootCmdArgs.subnetCIDR, rootCmdArgs.nameservers, rootCmdArgs.numMachines, rootCmdArgs.ipxeServerPort, rootCmdArgs.clearState, logger)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	if err = prov.Run(ctx); err != nil {
		return fmt.Errorf("failed to run provider: %w", err)
	}

	return nil
}

func initLogger() (*zap.Logger, error) {
	var loggerConfig zap.Config

	if debug.Enabled {
		loggerConfig = zap.NewDevelopmentConfig()
		loggerConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		loggerConfig = zap.NewProductionConfig()
	}

	if !rootCmdArgs.debug {
		loggerConfig.Level.SetLevel(zap.InfoLevel)
	} else {
		loggerConfig.Level.SetLevel(zap.DebugLevel)
	}

	return loggerConfig.Build(zap.AddStacktrace(zapcore.ErrorLevel))
}

func main() {
	if err := runCmd(); err != nil {
		log.Fatalf("failed to run: %v", err)
	}
}

func runCmd() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()

	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.Flags().StringVar(&rootCmdArgs.omniAPIEndpoint, "omni-api-endpoint", os.Getenv("OMNI_ENDPOINT"),
		"the endpoint of the Omni API, if not set, defaults to OMNI_ENDPOINT env var.")
	rootCmd.Flags().StringVar(&meta.ProviderID, "id", meta.ProviderID, "the id of the cloud provider, it is used to match the resources with the cloud provider label.")
	rootCmd.Flags().StringVar(&rootCmdArgs.talosctlPath, "talosctl-path", "", "the path to the talosctl binary, when unset, "+
		"it is searched in the current working directory and in the $PATH.")
	rootCmd.Flags().StringVar(&rootCmdArgs.subnetCIDR, "subnet-cidr", "172.42.0.0/24", "the CIDR of the subnet to use for the QEMU VMs.")
	rootCmd.Flags().StringSliceVar(&rootCmdArgs.nameservers, "nameservers", []string{"1.1.1.1", "1.0.0.1"}, "the nameservers to use for the QEMU VMs.")
	rootCmd.Flags().StringVar(&rootCmdArgs.imageFactoryPXEURL, "image-factory-pxe-url", "https://pxe.factory.talos.dev", "the URL of the image factory PXE server.")
	rootCmd.Flags().IntVar(&rootCmdArgs.ipxeServerPort, "ipxe-server-port", 42420, "the port the local (chaining) iPXE server should run on.")
	rootCmd.Flags().IntVar(&rootCmdArgs.numMachines, "num-machines", 8, "the number of machines to provision.")
	rootCmd.Flags().BoolVar(&rootCmdArgs.clearState, "clear-state", false, "clear the state of the provider (for debugging purposes).")
	rootCmd.Flags().BoolVar(&rootCmdArgs.debug, "debug", false, "enable debug mode & logs.")
}
