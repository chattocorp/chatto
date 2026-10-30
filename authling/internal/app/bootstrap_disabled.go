//go:build !bootstrap

package app

import (
	"context"
	"fmt"

	"github.com/chattocorp/chatto/authling/internal/config"
)

// bootstrapDevelopment rejects enabled fixtures even when called without the
// normal configuration validation. Release binaries contain no fixture code.
func bootstrapDevelopment(_ context.Context, cfg config.Config, _ *Runtime) error {
	if cfg.Bootstrap.Enabled {
		return fmt.Errorf("bootstrap requires a development build with the bootstrap tag")
	}
	return nil
}
