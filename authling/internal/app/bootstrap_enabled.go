//go:build bootstrap

package app

import (
	"context"

	"github.com/chattocorp/chatto/authling/internal/config"
)

// bootstrapDevelopment runs fixtures after replay and before opening HTTP.
func bootstrapDevelopment(ctx context.Context, cfg config.Config, runtime *Runtime) error {
	if !cfg.Bootstrap.Enabled {
		return nil
	}
	return runtime.Accounts.BootstrapDevelopmentUsers(ctx)
}
