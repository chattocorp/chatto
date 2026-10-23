//go:build !bootstrap

package cmd

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
)

func init() {
	// No-op in production builds. The [bootstrap] section in chatto.toml
	// (if present) is parsed but ignored.
	devStartupHook = func(context.Context, *core.ChattoCore, config.ChattoConfig) {}
}
