package cmd

import (
	"encoding/hex"
	"os"
	"path/filepath"
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
}
