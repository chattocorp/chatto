package memory

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltInMemoryPersistenceAndRecall(t *testing.T) {
	if _, err := exec.LookPath("means"); err != nil {
		t.Skip("means is not installed")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	if err := os.MkdirAll(filepath.Join(root, "config", "means"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "means", "config.yaml"), []byte("embeddings: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "brains")
	m, err := New(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.builtin != filepath.Join(resolved, "memory") || m.directory != filepath.Join(resolved, "apps") {
		t.Fatal("memory and apps do not use separate directories")
	}
	tools := m.Tools(false)
	toolCall(t, tools, "memory_info", `{"collection":"memory"}`)
	for _, fact := range []struct{ kind, user, summary string }{
		{"general_fact", "", "Project uses Go"},
		{"user_fact", "user-a", "Prefers concise answers"},
		{"user_fact", "user-b", "Other participant fact"},
	} {
		data := map[string]any{"kind": fact.kind, "summary": fact.summary}
		if fact.user != "" {
			data["user_id"] = fact.user
		}
		args, _ := json.Marshal(map[string]any{"collection": "memory", "title": fact.summary, "body": "Detail omitted from index", "data": data})
		result, err := toolCall(t, tools, "memory_create", string(args))
		if err != nil || !strings.Contains(result, `"id"`) {
			t.Fatal("could not store fact", err, result)
		}
	}
	// Restart preserves documents and the immutable definition.
	m, err = New(nil, dir)
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.KnowledgeIndex(context.Background(), []string{"user-a", "user-a", ""})
	if err != nil || !strings.Contains(string(index), "Project uses Go") || !strings.Contains(string(index), "Prefers concise answers") || strings.Contains(string(index), "Other participant fact") || strings.Contains(string(index), "Detail omitted") {
		t.Fatal("incorrect discovery index", err, string(index))
	}
	for _, op := range []string{"create", "archive", "schema_set"} {
		result, err := toolCall(t, m.Tools(true), "memory_brain_"+op, `{"name":"memory"}`)
		if err != nil || !strings.Contains(result, "built-in") {
			t.Fatal("built-in definition was not protected", op, err, result)
		}
	}
	tools = m.Tools(false)
	toolCall(t, tools, "memory_info", `{"collection":"memory"}`)
	if _, err := toolCall(t, tools, "memory_create", `{"collection":"memory","title":"Invalid","body":"No author","data":{"kind":"user_fact","summary":"Missing ID"}}`); err == nil {
		t.Fatal("user fact accepted without a user ID")
	}
}

func TestRejectLegacyLayoutAndLinkedApps(t *testing.T) {
	if _, err := exec.LookPath("means"); err != nil {
		t.Skip("means is not installed")
	}
	for _, layout := range []string{".memory", "old_app", "linked_apps"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			if layout == "linked_apps" {
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "apps")); err != nil {
					t.Fatal(err)
				}
			} else {
				path := filepath.Join(root, layout)
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "MEANS.md"), []byte("existing data"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := New(nil, root); err == nil {
				t.Fatal("unsafe layout accepted")
			}
			if _, err := os.Stat(filepath.Join(root, "memory")); !os.IsNotExist(err) {
				t.Fatal("created replacement memory despite invalid layout")
			}
		})
	}
}

func TestKnowledgeIndexBoundsAndDiagnostics(t *testing.T) {
	m := &Manager{builtin: "/memory"}
	calls := 0
	m.run = func(_ context.Context, _ string, args []string, _ []byte) ([]byte, error) {
		calls++
		if strings.Contains(strings.Join(args, " "), "--semantic") {
			t.Fatal("index must not contact an embedding provider")
		}
		return json.Marshal(map[string]any{"documents": []any{map[string]any{"id": "fact", "frontmatter": map[string]any{"title": "A fact", "data": map[string]any{"kind": "general_fact", "summary": strings.Repeat("x", 17000)}}}}, "has_more": true, "diagnostics": []string{"ignored private detail"}})
	}
	index, err := m.KnowledgeIndex(context.Background(), []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"})
	if err != nil || len(index) > 16<<10 || calls != 9 || !strings.Contains(string(index), `"truncated":true`) || !strings.Contains(string(index), `"hasDiagnostics":true`) || strings.Contains(string(index), "private detail") {
		t.Fatal("index bounds or diagnostics incorrect", err, string(index), calls)
	}
}
