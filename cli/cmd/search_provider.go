package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
To rebuild the index from EVT and keep the current index, stop every process
that uses search_provider.directory, run "chatto search-provider recover",
then start the provider again.
Never remove the NATS data directory.

Instructions: https://docs.chatto.run/guides/operations/search/#rebuild-the-search-index`,
	Run: func(cmd *cobra.Command, args []string) {
		runSearchProvider(searchProviderConfigFile)
	},
}

var searchProviderRecoverCmd = &cobra.Command{
	Use:   "recover",
	Short: "Prepare a rebuild of the search index from EVT and keep the current index",
	Long: `Prepare a rebuild of the search index from EVT and keep the current index.

Stop every process that uses search_provider.directory first: chatto run for
the bundled provider, or chatto search-provider. The command moves the current
index to search_provider.directory/previous. It replaces an earlier previous
index. At the next start, the provider builds a new index from EVT. Search is
ready again after the replay and a validation query complete. A restart
during the replay continues from the last checkpoint.

The command refuses when the volume does not have free space for 1.5 times
the current index. When an earlier recovery is not complete, the command
changes nothing; start the provider to continue it.

The command does not need NATS and does not change messages or accounts.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		directory, err := searchProviderDirectory(searchProviderConfigFile)
		if err != nil {
			return err
		}
		result, err := searchbleve.RecoverIndex(directory)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if result.InProgress {
			fmt.Fprintln(out, "A search index recovery is already in progress. Start the provider to continue it.")
			return nil
		}
		if result.DiscardedPreviousBytes > 0 {
			fmt.Fprintf(out, "Deleted the earlier previous index (%d bytes).\n", result.DiscardedPreviousBytes)
		}
		fmt.Fprintf(out, "Kept the current index (%d bytes) as %s.\n", result.IndexBytes, filepath.Join(directory, "previous"))
		fmt.Fprintln(out, "Start the provider to build a new index from EVT.")
		return nil
	},
}

var searchProviderDiscardPreviousCmd = &cobra.Command{
	Use:   "discard-previous",
	Short: "Delete the previous search index that a recovery kept",
	Long: `Delete the previous search index that a recovery kept.

The previous index contains message text. Later retractions, room deletions,
and key shredding do not change it. Delete it when you no longer need it.
Stop every process that uses search_provider.directory first. The command
refuses while a recovery is not complete.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		directory, err := searchProviderDirectory(searchProviderConfigFile)
		if err != nil {
			return err
		}
		size, err := searchbleve.DiscardPreviousIndex(directory)
		if err != nil {
			return err
		}
		if size == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No previous search index exists.")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted the previous search index (%d bytes).\n", size)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchProviderCmd)
	searchProviderCmd.PersistentFlags().StringVarP(&searchProviderConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	searchProviderCmd.AddCommand(searchProviderRecoverCmd, searchProviderDiscardPreviousCmd)
}

// searchProviderDirectory reads search_provider.directory. The maintenance
// commands use it for the bundled and the standalone provider.
func searchProviderDirectory(configPath string) (string, error) {
	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		return "", fmt.Errorf("read configuration: %w", err)
	}
	return cfg.SearchProvider.DirectoryOrDefault(), nil
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
