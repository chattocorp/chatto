//go:build !dev

package cmd

import (
	"context"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

func init() {
	// No-op in production builds. The [dev_bootstrap] section in chatto.toml
	// (if present) is parsed but ignored.
	devStartupHook = func(context.Context, *core.ChattoCore, config.ChattoConfig) {}
}
