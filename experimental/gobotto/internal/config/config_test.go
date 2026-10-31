package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
}

func TestInvalidConfigFailsWithoutLeakingContents(t *testing.T) {
	for _, text := range []string{
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

func TestBundledConfig(t *testing.T) {
	if _, err := Load("../../gobotto.toml"); err != nil {
		t.Fatal(err)
	}
}
