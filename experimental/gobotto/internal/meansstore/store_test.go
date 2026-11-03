package meansstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hmans/means/collection"
)

func TestEmbeddingOptInAndOfflineDefault(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		vector := make([]float64, 512)
		vector[0] = 1
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"index": 0, "embedding": vector}}})
	}))
	defer server.Close()
	t.Setenv("MEANS_EMBEDDING_BASE_URL", server.URL)
	t.Setenv("MEANS_EMBEDDING_API_KEY", "synthetic-test-key")
	t.Setenv("PATH", t.TempDir())
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	if err := os.Mkdir(filepath.Join(config, "means"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "means", "config.yaml"), []byte("embeddings:\n  provider: openai\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		dir := t.TempDir()
		if _, err := collection.Init(dir, nil); err != nil {
			t.Fatal(err)
		}
		runner := Runner{Limit: 64 << 10}
		if enabled {
			runner.Embeddings = &collection.EmbeddingConfig{Provider: "openai"}
		}
		_, err := runner.Run(context.Background(), dir, []string{"create", "--input", "-"}, []byte(`{"frontmatter":{"title":"Synthetic fact","data":{}},"body":"Useful fact"}`))
		if err != nil {
			t.Fatal(err)
		}
		if (requests.Load() > 0) != enabled {
			t.Fatal("embedding opt-in violated", enabled, requests.Load())
		}
	}
}

func TestBoundsCancellationAndSafeErrors(t *testing.T) {
	dir := t.TempDir()
	c, err := collection.Init(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := c.Create(context.Background(), collection.CreateOptions{Title: "private-marker", Body: strings.Repeat("x", 200)})
	if err != nil {
		t.Fatal(err)
	}
	r := Runner{Limit: 100}
	if out, err := r.Run(context.Background(), dir, []string{"show", "--", doc.ID}, nil); err == nil || out != nil {
		t.Fatal("unbounded result")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Run(ctx, dir, []string{"create"}, []byte(`{"frontmatter":{"title":"private-marker"},"body":"x"}`)); err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatal("unsafe cancellation", err)
	}
	result, err := c.List(context.Background(), collection.ListOptions{})
	if err != nil || len(result.Documents) != 1 {
		t.Fatal("cancelled write executed", err)
	}
	if _, err := r.Run(context.Background(), dir, []string{"update", "--", doc.ID}, []byte(`{"hash":"private-marker","body":"private-marker"}`)); err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatal("unsafe failure", err)
	}
}

func TestExactNumbersAndFreshDefinitions(t *testing.T) {
	dir := t.TempDir()
	if _, err := collection.Init(dir, []byte(`{"type":"object"}`)); err != nil {
		t.Fatal(err)
	}
	r := Runner{Limit: 64 << 10}
	out, err := r.Run(context.Background(), dir, []string{"create"}, []byte(`{"frontmatter":{"title":"Number","data":{"count":9007199254740993}},"body":"fact"}`))
	if err != nil || !strings.Contains(string(out), "9007199254740993") {
		t.Fatal("number lost precision", err)
	}
	out, err = r.Run(context.Background(), dir, []string{"search", "--filter", "count=9007199254740993"}, nil)
	if err != nil || !strings.Contains(string(out), "Number") {
		t.Fatal("filter lost precision", err)
	}
	c, err := collection.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetInstructions(context.Background(), c.Describe().Hash, "New guidance"); err != nil {
		t.Fatal(err)
	}
	out, err = r.Run(context.Background(), dir, []string{"info"}, nil)
	if err != nil || !strings.Contains(string(out), "New guidance") {
		t.Fatal("stale definition", err)
	}
}
