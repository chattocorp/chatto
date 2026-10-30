//go:build !bootstrap

package app

import (
	"testing"

	"github.com/chattocorp/chatto/authling/internal/config"
)

func TestReleaseRejectsBootstrapBeforeRuntimeAccess(t *testing.T) {
	if err := bootstrapDevelopment(t.Context(), config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Bootstrap: config.BootstrapConfig{Enabled: true}}
	if err := bootstrapDevelopment(t.Context(), cfg, nil); err == nil {
		t.Fatal("release accepted bootstrap")
	}
}
