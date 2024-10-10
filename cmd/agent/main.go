// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package main contains the entrypoint for the Talos agent.
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

	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/agent"
	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/debug"
	"github.com/siderolabs/omni-infra-provider-bare-metal/internal/version"
)

var rootCmdArgs struct {
	listenAddress string
	debug         bool
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:     version.Name,
	Short:   "Run the Talos agent",
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
	server := agent.NewServer(rootCmdArgs.listenAddress, logger)

	if err := server.Run(ctx); err != nil {
		return fmt.Errorf("failed to run server: %w", err)
	}

	return nil
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

func init() {
	rootCmd.Flags().StringVar(&rootCmdArgs.listenAddress, "listen-address", ":50010", "the address to listen on.")
	rootCmd.Flags().BoolVar(&rootCmdArgs.debug, "debug", false, "enable debug mode & logs.")
}
