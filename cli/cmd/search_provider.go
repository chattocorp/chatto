package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/runtimeunit"
	searchbleve "hmans.de/chatto/internal/search/bleve"
)

var searchProviderConfigFile string

var searchProviderCmd = &cobra.Command{
	Use:   "search-provider",
	Short: "Run the bundled Bleve message search provider",
	Run: func(cmd *cobra.Command, args []string) {
		runSearchProvider(searchProviderConfigFile)
	},
}

func init() {
	rootCmd.AddCommand(searchProviderCmd)
	searchProviderCmd.Flags().StringVarP(&searchProviderConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
}

func runSearchProvider(configPath string) {
	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}
	configureLogging(cfg.General)
	if err := runtimeunit.RequireStandaloneNATSClientURL(cfg, "search-provider"); err != nil {
		log.Fatal(err)
	}

	ctx, stop := runtimeunit.NotifyContext(context.Background())
	defer stop()
	unit := searchbleve.Unit{}
	nc, err := runtimeunit.ConnectToNATS(ctx, cfg, nil)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer runtimeunit.CloseNATSConnection(nc)
	env, err := runtimeunit.NewEnv(ctx, cfg, nc, log.WithPrefix(unit.Name()), Version)
	if err != nil {
		log.Fatal("Failed to create search provider environment", "error", err)
	}
	if err := runtimeunit.Run(ctx, env, unit); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
