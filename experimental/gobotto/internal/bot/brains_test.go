package bot

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/memory"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

// Uses local means info only; no model requests or embedding operations occur.
func TestBrainCatalogReachesFreshAndRestoredSessions(t *testing.T) {
	if _, err := exec.LookPath("means"); err != nil {
		t.Skip("means is not installed")
	}
	root := t.TempDir()
	m, err := memory.New(nil, root)
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	complete := func(_ context.Context, messages []agent.Message) (agent.Message, error) {
		var content string
		if err := json.Unmarshal(messages[len(messages)-1]["content"], &content); err != nil {
			t.Fatal(err)
		}
		var batch struct {
			Catalog []struct{ Name string } `json:"brainCatalog"`
		}
		if err := json.Unmarshal([]byte(content), &batch); err != nil {
			t.Fatal(err)
		}
		if want == "" && len(batch.Catalog) != 0 {
			t.Fatal("stale catalog")
		}
		if want != "" && (len(batch.Catalog) != 1 || batch.Catalog[0].Name != want) {
			t.Fatal("current catalog missing from model request")
		}
		return answer(), nil
	}
	c := &conversation{key: threadKey{"room", "root"}, memory: m, complete: complete}
	f := newFake()
	if err := respond(context.Background(), f, c, []chatto.MessageEvent{input("first", "hello", true)}, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "notes")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "MEANS.md"), []byte("---\ntitle: Notes\ndescription: Shared notes\nschema:\n  type: object\n---\nAnyone may read and write.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	want = "notes"
	c.model = agent.Restore(c.model.Snapshot())
	if err := respond(context.Background(), f, c, []chatto.MessageEvent{input("second", "recall", true)}, nil); err != nil {
		t.Fatal(err)
	}
	fresh := &conversation{key: threadKey{"room", "new-root"}, memory: m, complete: complete}
	if err := respond(context.Background(), f, fresh, []chatto.MessageEvent{input("third", "hello", true)}, nil); err != nil {
		t.Fatal(err)
	}
}
