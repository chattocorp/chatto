package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebounceConfig(t *testing.T) {
	for _, tc := range []struct {
		setting string
		want    time.Duration
	}{
		{"", 4 * time.Second},
		{"debounce = '750ms'", 750 * time.Millisecond},
		{"debounce = '6s'", 6 * time.Second},
		{"debounce = ''", 0},
		{"debounce = '0s'", 0},
		{"debounce = '-1s'", 0},
		{"debounce = 'private-marker'", 0},
		{"debounce = 4", 0},
	} {
		path := filepath.Join(t.TempDir(), "gobotto.toml")
		if err := os.WriteFile(path, []byte("[conversation]\nmodel='test/model'\n"+tc.setting), 0600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if tc.want == 0 {
			if err == nil || strings.Contains(err.Error(), "private-marker") {
				t.Fatal("invalid debounce accepted or exposed")
			}
		} else if err != nil || cfg.Conversation.DebounceDuration != tc.want {
			t.Fatalf("setting %q: got %s, error %v", tc.setting, cfg.Conversation.DebounceDuration, err)
		}
	}
}

func TestConversationConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gobotto.toml")
	text := "[conversation]\nmodel = ' test/model '\nsystem_prompt = '''\nAnswer in German.\nKeep answers short.\n'''\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Conversation.Model != "test/model" || cfg.Conversation.SystemPrompt != "Answer in German.\nKeep answers short.\n" {
		t.Fatal("model or multiline instructions changed")
	}
	if cfg.Conversation.MaxOutputTokens != nil {
		t.Fatal("incorrect default output budget")
	}
	if err := os.WriteFile(path, []byte(text+"max_output_tokens = 16384\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil || cfg.Conversation.MaxOutputTokens == nil || *cfg.Conversation.MaxOutputTokens != 16384 {
		t.Fatal("configured output budget lost", err)
	}
}

func TestInvalidConfigFailsWithoutLeakingContents(t *testing.T) {
	for _, text := range []string{
		"[conversation]\nmodel='test/model'\nmax_output_tokens=0\n",
		"[conversation]\nmodel='test/model'\nmax_output_tokens=-1\n",
		"owner_user_ids = ['']\n[conversation]\nmodel = 'test/model'\n",
		"owner_user_ids = [' private-marker ']\n[conversation]\nmodel = 'test/model'\n",
		"owner_user_ids = [42]\n[conversation]\nmodel = 'test/model'\n",
		"[conversation]\nmodel = 'test/model'\n[titles]\nmodel = ''\n",
		"[conversation]\nmodel = ' '\n",
		"[conversation]\nmodel = 'test/model'\nsystem_promt = 'private-marker'\n",
		"[conversation]\nmodel = 42\n",
		"[conversation]\nmodel = 'test/model'\nsystem_prompt = private-marker\n",
		"[unknown]\nvalue = 'private-marker'\n",
	} {
		path := filepath.Join(t.TempDir(), "gobotto.toml")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil || strings.Contains(err.Error(), "private-marker") {
			t.Fatal("invalid config accepted or private contents exposed")
		}
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing configuration accepted")
	}
}

func TestOwnersAreExactAndCanonical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gobotto.toml")
	if err := os.WriteFile(path, []byte("owner_user_ids = ['user-b', 'user-A', 'user-b']\n[conversation]\nmodel = 'test/model'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || len(cfg.OwnerUserIDs) != 2 || cfg.OwnerUserIDs[0] != "user-A" || cfg.OwnerUserIDs[1] != "user-b" {
		t.Fatal("owner IDs not canonical or case changed", err)
	}
}

func TestMemoryCollectionsResolveAgainstConfigFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "gobotto.toml")
	base := "[conversation]\nmodel='test/model'\n[memory]\ndirectory='./brains'\n[[memory.collections]]\nname='notes'\npath='./notes'\n"
	if err := os.WriteFile(path, []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err == nil && cfg.Memory.Directory != filepath.Join(root, "brains") {
		t.Fatal("brain directory did not resolve relative to config")
	}
	if err != nil || len(cfg.Memory.Collections) != 1 || cfg.Memory.Collections[0].Path != filepath.Join(root, "notes") || cfg.Memory.Collections[0].Writable {
		t.Fatal("incorrect memory defaults/path", err)
	}
	for _, suffix := range []string{"[[memory.collections]]\nname='notes'\npath='./other'\n", "[[memory.collections]]\nname='../escape'\npath='./other'\n"} {
		if err := os.WriteFile(path, []byte(base+suffix), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("invalid memory binding accepted")
		}
	}
}

func TestBundledConfig(t *testing.T) {
	if _, err := Load("../../gobotto.toml"); err != nil {
		t.Fatal(err)
	}
}
