package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set by main package at startup
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "chatto",
	Short:   "Run and manage a Chatto server",
	Long:    rootBanner(Version),
	Version: Version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// SetVersion sets the version for the CLI
func SetVersion(v string) {
	Version = v
	rootCmd.Version = v
	rootCmd.Long = rootBanner(v)
}

func rootBanner(version string) string {
	return fmt.Sprintf(`Chatto is a self-hostable chat server for teams and communities.
Version: %s | Self-hosting docs: https://docs.chatto.run`, version)
}
