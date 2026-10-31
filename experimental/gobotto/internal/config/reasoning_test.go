package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReasoningConfiguration(t *testing.T) {
	for _, tc := range []struct {
		settings string
		valid    bool
	}{
		{"", true}, {"enabled=false", true}, {"effort='medium'", true}, {"max_tokens=2048", true}, {"exclude=false", true},
		{"effort='typo'", false}, {"effort='high'\nmax_tokens=1024", false}, {"max_tokens=0", false}, {"max_tokens=8192", true},
		{"enabled=false\neffort='high'", false}, {"enabled=true\neffort='none'", false}, {"enabled=false\nmax_tokens=1024", false},
		{"effort='none'\nenabled=false", true}, {"unknown=true", false},
	} {
		path := filepath.Join(t.TempDir(), "gobotto.toml")
		if err := os.WriteFile(path, []byte("[conversation]\nmodel='test/model'\n[conversation.reasoning]\n"+tc.settings+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if (err == nil) != tc.valid {
			t.Fatalf("settings %q: %v", tc.settings, err)
		}
		if tc.settings == "enabled=false" && (cfg.Conversation.Reasoning.Enabled == nil || *cfg.Conversation.Reasoning.Enabled) {
			t.Fatal("explicit false lost")
		}
	}
}
