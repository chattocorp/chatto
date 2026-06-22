package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/exporter"
	"hmans.de/chatto/internal/runtimeunit"
)

var exporterConfigFile string

var exporterCmd = &cobra.Command{
	Use:   "exporter",
	Short: "Run the deployment-wide Prometheus exporter",
	Run: func(cmd *cobra.Command, args []string) {
		runExporter(exporterConfigFile)
	},
}

func init() {
	rootCmd.AddCommand(exporterCmd)
	exporterCmd.Flags().StringVarP(&exporterConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
}

func runExporter(configPath string) {
	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}
	configureLogging(cfg.General)

	if err := runtimeunit.RequireStandaloneNATSClientURL(cfg, "exporter"); err != nil {
		log.Fatal(err)
	}

	ctx, stop := runtimeunit.NotifyContext(context.Background())
	defer stop()

	nc, err := runtimeunit.ConnectToNATS(ctx, cfg, nil)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer runtimeunit.CloseNATSConnection(nc)

	env, err := runtimeunit.NewEnv(ctx, cfg, nc, log.WithPrefix("exporter"), Version)
	if err != nil {
		log.Fatal("Failed to create exporter environment", "error", err)
	}
	if err := runtimeunit.Run(ctx, env, exporter.Unit{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
