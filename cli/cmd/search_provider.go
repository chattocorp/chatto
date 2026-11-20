package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	searchbleve "github.com/chattocorp/chatto/cli/internal/search/bleve"
)

var searchProviderConfigFile string

var searchProviderCmd = &cobra.Command{
	Use:   "search-provider",
	Short: "Run the bundled Bleve message search provider",
	Long: `Run the bundled Bleve message search provider.

Known index-format and language changes trigger an automatic rebuild from EVT.
For manual recovery, stop every process using its configured
search_provider.directory, move that directory aside, then restart the provider.
For a bundled provider, stop and restart chatto run instead.
Never remove the NATS data directory. A missing search index is rebuilt from EVT.

Instructions: https://docs.chatto.run/guides/operations/search/#rebuild-the-search-index`,
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
	unit := searchbleve.Unit{Health: searchbleve.NewHealth()}
	logger := log.WithPrefix(unit.Name())
	// Start the health listener before NATS and index startup, so liveness
	// and diagnostics answer while the provider waits, replays, or rebuilds.
	if cfg.SearchProvider.Health.Enabled {
		healthServer, err := searchbleve.StartHealthServer(cfg.SearchProvider.Health, unit.Health, logger)
		if err != nil {
			log.Fatal("Failed to start search provider health listener", "error", err)
		}
		defer func() {
			if err := healthServer.Close(); err != nil {
				logger.Warn("Failed to stop search provider health listener", "error", err)
			}
		}()
	}
	if err := runSearchProviderUnit(ctx, cfg, unit, logger); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSearchProviderUnit(ctx context.Context, cfg config.ChattoConfig, unit searchbleve.Unit, logger *log.Logger) error {
	nc, err := runtimeunit.ConnectToNATS(ctx, cfg, nil)
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}
	defer runtimeunit.CloseNATSConnection(nc)
	env, err := runtimeunit.NewEnv(ctx, cfg, nc, logger, Version)
	if err != nil {
		return fmt.Errorf("create search provider environment: %w", err)
	}
	return runtimeunit.Run(ctx, env, unit)
}
