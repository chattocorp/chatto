package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConversationExecutionLimits(t *testing.T) {
	for _, tc := range []struct {
		settings string
		valid    bool
	}{
		{"", true}, {"timeout='0s'\nmax_tool_calls=0\nmax_model_steps=0\nmax_delegations=0", true},
		{"timeout='1h'\nmax_tool_calls=1000\nmax_model_steps=2000\nmax_delegations=500", true},
		{"timeout='-1s'", false}, {"timeout='bad'", false}, {"max_tool_calls=-1", false}, {"max_model_steps=-1", false}, {"max_delegations=-1", false},
	} {
		file := filepath.Join(t.TempDir(), "gobotto.toml")
		if err := os.WriteFile(file, []byte("[conversation]\nmodel='test/model'\n"+tc.settings), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(file)
		if (err == nil) != tc.valid {
			t.Fatal(tc.settings, err)
		}
		if tc.settings == "" && (cfg.Conversation.TimeoutDuration != 0 || cfg.Conversation.MaxToolCalls != 0 || cfg.Conversation.MaxModelSteps != 0 || cfg.Conversation.MaxDelegations != 0) {
			t.Fatal("implicit execution limits")
		}
		if cfg.Conversation.MaxToolCalls == 1000 && (cfg.Conversation.TimeoutDuration != time.Hour || cfg.Conversation.MaxModelSteps != 2000 || cfg.Conversation.MaxDelegations != 500) {
			t.Fatal("configured limits lost")
		}
	}
}
