package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapBuildBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authling.toml")
	if err := os.WriteFile(path, []byte("[nats.embedded]\nenabled = true\n[bootstrap]\nenabled = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if bootstrapAvailable && err != nil {
		t.Fatal(err)
	}
	if !bootstrapAvailable && (err == nil || !strings.Contains(err.Error(), "bootstrap tag")) {
		t.Fatal("release accepted TOML bootstrap")
	}
	if err := os.WriteFile(path, []byte("[nats.embedded]\nenabled = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTHLING_BOOTSTRAP_ENABLED", "true")
	cfg, err := Read(path)
	if bootstrapAvailable && (err != nil || !cfg.Bootstrap.Enabled) {
		t.Fatalf("tagged ENV bootstrap: %v", err)
	}
	if !bootstrapAvailable && (err == nil || !strings.Contains(err.Error(), "bootstrap tag")) {
		t.Fatal("release accepted ENV bootstrap")
	}
	t.Setenv("AUTHLING_BOOTSTRAP_ENABLED", "false")
	cfg, err = Read(path)
	if err != nil || cfg.Bootstrap.Enabled {
		t.Fatalf("disabled bootstrap: %v", err)
	}
}
