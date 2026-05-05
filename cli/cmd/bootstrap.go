//go:build bootstrap

package cmd

import (
	"context"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

func init() {
	// Register the bootstrap hook. Reads the [bootstrap] section from
	// chatto.toml and applies it on every startup.
	devStartupHook = func(ctx context.Context, c *core.ChattoCore, cfg config.ChattoConfig) {
		applyBootstrap(ctx, c, cfg.Bootstrap, cfg.Server.PrimarySpaceID)
	}
}
