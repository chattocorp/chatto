package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTreeBackendConfig(t *testing.T) {
	for _, backend := range []string{"tree", "means", "disabled", "invalid"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "gobotto.toml")
		if err := os.WriteFile(path, []byte("[conversation]\nmodel='test'\n[memory]\nbackend='"+backend+"'\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if backend == "invalid" {
			if err == nil {
				t.Fatal("invalid backend accepted")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if backend == "tree" && cfg.Memory.Database != filepath.Join(dir, ".gobotto", "memory.sqlite") {
			t.Fatal("database path not relative to config")
		}
	}
}
