package cmd

import (
	"github.com/spf13/cobra"
	"hmans.de/chatto/internal/config"
)

// newConfigCommand creates the offline configuration checker without shared
// flag state, so callers can test the same command operators execute.
func newConfigCommand() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Inspect server configuration"}
	var path string
	check := &cobra.Command{
		Use: "check", Short: "Check configuration and reject unknown or deprecated settings",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.CheckConfig(path); err != nil {
				return err
			}
			cmd.Println("Configuration check passed. External services were not contacted.")
			return nil
		},
	}
	check.Flags().StringVarP(&path, "config", "c", "", "configuration file (default: chatto.toml; absent default allows ENV-only configuration)")
	command.AddCommand(check)
	return command
}

func init() { rootCmd.AddCommand(newConfigCommand()) }
