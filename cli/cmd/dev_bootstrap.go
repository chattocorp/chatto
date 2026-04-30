//go:build dev

package cmd

import (
	"context"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

func init() {
	// Register dev bootstrap hook. Reads the [dev_bootstrap] section from
	// chatto.toml and applies it on every dev-build startup.
	devStartupHook = func(ctx context.Context, c *core.ChattoCore, cfg config.ChattoConfig) {
		devBootstrapFromConfig(ctx, c, cfg.DevBootstrap)
	}
}
