package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/runtimeunit"
	"hmans.de/chatto/internal/video"
)

var assetProcessingConfigFile string

var assetProcessingCmd = &cobra.Command{
	Use:   "asset-processing",
	Short: "Run durable asset-processing workers",
	Run: func(cmd *cobra.Command, args []string) {
		runAssetProcessing(assetProcessingConfigFile)
	},
}

func init() {
	rootCmd.AddCommand(assetProcessingCmd)
	assetProcessingCmd.Flags().StringVarP(&assetProcessingConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
}

func runAssetProcessing(configPath string) {
	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}
	configureLogging(cfg.General)
	if err := runtimeunit.RequireStandaloneNATSClientURL(cfg, "asset-processing"); err != nil {
		log.Fatal(err)
	}

	ctx, stop := runtimeunit.NotifyContext(context.Background())
	defer stop()
	unit := video.Unit{}
	nc, err := runtimeunit.ConnectToNATS(ctx, cfg, nil)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer runtimeunit.CloseNATSConnection(nc)
	env, err := runtimeunit.NewEnv(ctx, cfg, nc, log.WithPrefix(unit.Name()), Version)
	if err != nil {
		log.Fatal("Failed to create asset-processing environment", "error", err)
	}
	if err := runtimeunit.Run(ctx, env, unit); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
