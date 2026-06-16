package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/pkg/natsauth"
)

var (
	resetConfigFile string
	resetYes        bool
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset deployment subsystems",
	Long:  "Operator-level reset commands. These are destructive — read each subcommand's --help carefully.",
}

var resetRBACCmd = &cobra.Command{
	Use:   "rbac",
	Short: "Reset and re-seed the RBAC aggregate",
	Long: `Appends reset facts to the event-sourced RBAC aggregate, re-creates
the system roles, re-seeds default permission grants for non-owner roles, and
assigns the 'owner' role to every user whose verified email matches the
'owners.emails' list in chatto.toml. Owner permissions are effective
automatically and are not stored as editable grants.

This is the repair tool for the event-sourced RBAC layout and the
operator escape hatch for misconfigured / drifted RBAC state.

DESTRUCTIVE: custom roles, explicit role assignments, and permission overrides
are cleared by reset events. Rebuild those after the reset.`,
	Run: runResetRBAC,
}

func init() {
	rootCmd.AddCommand(resetCmd)
	resetCmd.AddCommand(resetRBACCmd)

	resetRBACCmd.Flags().StringVarP(&resetConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	resetRBACCmd.Flags().BoolVar(&resetYes, "yes", false, "skip the interactive confirmation prompt")
}

func runResetRBAC(cmd *cobra.Command, args []string) {
	cfg, err := config.ReadConfig(resetConfigFile)
	if err != nil {
		log.Fatal("Failed to read configuration", "error", err)
	}

	if !resetYes {
		fmt.Fprintln(os.Stderr, "This will reset the event-sourced RBAC aggregate and re-seed it from code.")
		fmt.Fprintln(os.Stderr, "Custom roles, role assignments, and room-level overrides will be lost.")
		fmt.Fprint(os.Stderr, "Type 'reset' to continue: ")
		var reply string
		_, _ = fmt.Scanln(&reply)
		if strings.TrimSpace(reply) != "reset" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			os.Exit(1)
		}
	}

	nc, err := connectForReset(cfg)
	if err != nil {
		log.Fatal("Failed to connect to NATS", "error", err)
	}
	defer nc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg.Core.AuthTokenTTL = cfg.Auth.TokenTTLOrDefault()
	cfg.Core.EmailOTP = cfg.Auth.EmailOTP
	cfg.Core.Replicas = cfg.NATS.ReplicasOrDefault()
	cfg.Core.Limits = cfg.Limits
	cfg.Core.Owners = cfg.Owners

	chattoCore, err := core.NewChattoCore(ctx, nc, cfg.Core)
	if err != nil {
		log.Fatal("Failed to initialize core", "error", err)
	}

	if err := chattoCore.ResetRBAC(ctx, cfg.Owners); err != nil {
		log.Fatal("Failed to reset RBAC", "error", err)
	}

	log.Info("RBAC reset complete")
}

// connectForReset opens a NATS connection using the configured client auth.
// Mirrors `connectForKeys` so operator commands share the same auth pattern.
func connectForReset(cfg config.ChattoConfig) (*nats.Conn, error) {
	authOpts, err := natsauth.ConnectOptions(cfg.NATS.Client.NATSAuthConfig())
	if err != nil {
		return nil, fmt.Errorf("get NATS auth options: %w", err)
	}
	nc, err := nats.Connect(cfg.NATS.Client.URL, authOpts...)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}
	return nc, nil
}
