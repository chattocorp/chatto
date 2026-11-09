package brainview

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/hmans/means/collection"
	"go.yaml.in/yaml/v3"
)

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[path] = string(b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBrowserListsReadsAndNeverWritesOrEmbeds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	for _, name := range []string{"memory", "conversations", "schedules", "apps/tasks", "apps/.archived"} {
		if _, err := collection.Init(filepath.Join(root, name), nil); err != nil {
			t.Fatal(err)
		}
	}
	c, err := collection.Load(filepath.Join(root, "memory"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < PageSize+1; i++ {
		if _, err := c.Create(ctx, collection.CreateOptions{Title: fmt.Sprintf("Fact %02d", i), Body: "Remember this synthetic fact."}); err != nil {
			t.Fatal(err)
		}
	}
	// A journal's short stored ID differs from its long host-selected filename.
	if err := os.WriteFile(filepath.Join(root, "conversations", "host-scope-digest.md"), []byte("---\n{\"id\":\"abc123\",\"title\":\"Conversation journal\",\"data\":{\"scope\":{\"room\":\"room-a\",\"thread\":\"thread-a\"}}}\n---\nPrivate synthetic note"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "memory"), filepath.Join(root, "apps", "alias")); err != nil {
		t.Fatal(err)
	}
	fixed := filepath.Join(root, "external")
	if _, err := collection.Init(fixed, nil); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	t.Setenv("MEANS_EMBEDDING_BASE_URL", server.URL)
	t.Setenv("MEANS_EMBEDDING_API_KEY", "synthetic-key")
	b := New(config.Memory{Directory: root, Collections: []config.Collection{{Name: "external", Path: fixed}}, Embeddings: &config.Embeddings{Provider: "openai"}})
	before := snapshot(t, root)
	catalog, err := b.Catalog(ctx)
	if err != nil || len(catalog) != 5 || catalog[0].Key != "memory" || catalog[1].Key != "conversations" || catalog[2].Key != "schedules" {
		t.Fatal("catalog", catalog, err)
	}
	page, err := b.List(ctx, "memory", 0)
	if err != nil || len(page.Entries) != PageSize || !page.More {
		t.Fatal("first page", err)
	}
	second, err := b.List(ctx, "memory", PageSize)
	if err != nil || len(second.Entries) != 1 || second.More {
		t.Fatal("second page", err)
	}
	doc, err := b.Read(ctx, "memory", page.Entries[0].ID)
	if err != nil || doc.Body != "Remember this synthetic fact." {
		t.Fatal("document", err)
	}
	journals, err := b.List(ctx, "conversations", 0)
	if err != nil || len(journals.Entries) != 1 || journals.Entries[0].Title != "Thread · abc123" {
		t.Fatal("journal list", err)
	}
	doc, err = b.Read(ctx, "conversations", "abc123")
	if err != nil || doc.Body != "Private synthetic note" {
		t.Fatal("journal lookup", err)
	}
	if _, err = b.Read(ctx, "../memory", page.Entries[0].ID); err == nil {
		t.Fatal("accepted arbitrary collection path")
	}
	if _, err = b.Read(ctx, "memory", "../private"); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("unsafe document error", err)
	}
	if requests.Load() != 0 || !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("browser wrote files or contacted embedding provider")
	}
}

func TestBrowserFreshDiscoveryCancellationAndBounds(t *testing.T) {
	root := t.TempDir()
	b := New(config.Memory{Directory: root})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Catalog(ctx); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := collection.Init(filepath.Join(root, "apps", "new-app"), nil); err != nil {
		t.Fatal(err)
	}
	list, err := b.Catalog(context.Background())
	if err != nil || list[len(list)-1].Key != "apps/new-app" {
		t.Fatal("fresh app missing", err)
	}
	text := bounded(strings.Repeat("界", maxText))
	if len(text) > maxText+100 || !strings.Contains(text, "display truncated") {
		t.Fatal("unbounded display")
	}
	label := titleLabel(strings.Repeat("界", 200))
	if !utf8.ValidString(text) || !utf8.ValidString(label) || utf8.RuneCountInString(label) != 161 {
		t.Fatal("display limits broke Unicode or title bounds")
	}
}

func TestMetadataYAMLPreservesSchemaAndExactValues(t *testing.T) {
	text, err := metadataYAML(map[string]any{
		"schema": json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"}}}`),
		"data":   map[string]any{"count": json.Number("9007199254740993"), "enabled": true, "label": "true"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "schema:\n") || !strings.Contains(text, "count: 9007199254740993") || strings.Contains(text, "{") {
		t.Fatal("not readable block YAML", text)
	}
	var decoded struct {
		Data struct {
			Count   uint64
			Enabled bool
			Label   string
		}
	}
	if err := yaml.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Data.Count != 9007199254740993 || !decoded.Data.Enabled || decoded.Data.Label != "true" {
		t.Fatal("YAML changed metadata types", text)
	}
}
