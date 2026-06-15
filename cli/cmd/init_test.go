package cmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestInitGeneratesCoreSecret(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	originalConfigFile := initConfigFile
	initConfigFile = ""
	t.Cleanup(func() { initConfigFile = originalConfigFile })

	initCmd.Run(initCmd, nil)

	cfg, err := config.ReadConfig(filepath.Join(tmpDir, "chatto.toml"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if len(cfg.Core.SecretKey) != 64 {
		t.Fatalf("generated core secret length = %d, want 64", len(cfg.Core.SecretKey))
	}
	if _, err := hex.DecodeString(cfg.Core.SecretKey); err != nil {
		t.Fatalf("generated core secret should be hex: %v", err)
	}
	if cfg.NATS.Client.URL != "nats://127.0.0.1:4222" {
		t.Fatalf("derived embedded NATS client URL = %q", cfg.NATS.Client.URL)
	}
	if cfg.NATS.Client.Token != cfg.NATS.Embedded.AuthToken {
		t.Fatalf("derived embedded NATS client token should match embedded auth token")
	}
	raw, err := os.ReadFile(filepath.Join(tmpDir, "chatto.toml"))
	if err != nil {
		t.Fatalf("read generated raw config: %v", err)
	}
	if strings.Contains(string(raw), "[nats.client]") {
		t.Fatal("generated embedded config should not include a duplicate [nats.client] table")
	}
}
