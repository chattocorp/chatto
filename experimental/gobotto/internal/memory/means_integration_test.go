package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/hmans/means/collection"
)

// The real embedded library must work without an installed CLI.
func TestEmbeddedMeansRoundTrip(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	if err := os.MkdirAll(filepath.Join(root, "config", "means"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "means", "config.yaml"), []byte("embeddings: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "collection")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := collection.Init(path, nil); err != nil {
		t.Fatal("means init failed", err)
	}
	m, err := New([]config.Collection{{Name: "notes", Path: path, Writable: true}})
	if err != nil {
		t.Fatal(err)
	}
	tools := m.Tools(true)
	if _, err := toolCall(t, tools, "memory_info", `{"collection":"notes"}`); err != nil {
		t.Fatal(err)
	}
	created, err := toolCall(t, tools, "memory_create", `{"collection":"notes","title":"Release tone","body":"Keep announcements short.","data":{}}`)
	if err != nil {
		t.Fatal("create failed", err)
	}
	var doc struct {
		ID   string `json:"id"`
		Hash string `json:"hash"`
	}
	if json.Unmarshal([]byte(created), &doc) != nil || doc.ID == "" {
		t.Fatal("missing creation receipt")
	}
	// A new turn has no model-local memory of the document, but finds it on disk.
	tools = m.Tools(true)
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	if found, err := toolCall(t, tools, "memory_search", `{"collection":"notes","query":"Release"}`); err != nil || !strings.Contains(found, doc.ID) {
		t.Fatal("cross-turn search failed", err)
	}
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	read, err := toolCall(t, tools, "memory_read", `{"collection":"notes","id":"`+doc.ID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal([]byte(read), &doc) != nil {
		t.Fatal("invalid read")
	}
	update, _ := json.Marshal(map[string]any{"collection": "notes", "id": doc.ID, "hash": doc.Hash, "title": "Release tone", "body": "Write in English.", "data": map[string]any{}})
	concurrent := m.Tools(true)
	toolCall(t, concurrent, "memory_info", `{"collection":"notes"}`)
	toolCall(t, concurrent, "memory_read", `{"collection":"notes","id":"`+doc.ID+`"}`)
	if _, err := toolCall(t, tools, "memory_update", string(update)); err != nil {
		t.Fatal("update failed", err)
	}
	// Both callers read the same hash. The library must reject the second writer,
	// even though its hash passes the host's current-turn read prerequisite.
	if _, err := toolCall(t, concurrent, "memory_update", string(update)); err == nil {
		t.Fatal("means accepted a conflicting update")
	}
	// A separate turn reads the replacement content and rejects a stale hash.
	tools = m.Tools(true)
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	read, err = toolCall(t, tools, "memory_read", `{"collection":"notes","id":"`+doc.ID+`"}`)
	if err != nil || !strings.Contains(read, "Write in English.") {
		t.Fatal("updated body not persisted", err)
	}
	if result, _ := toolCall(t, tools, "memory_update", string(update)); !strings.Contains(result, "Read this document") {
		t.Fatal("stale hash accepted")
	}

	// Managed brains survive manager recreation and are usable by non-owners.
	brains := filepath.Join(root, "brains")
	managed, err := New(nil, brains)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"name": "projects", "title": "Project notes", "description": "Shared project knowledge", "schema": map[string]any{"type": "object"}, "instructions": "Anyone may read and write project facts.\n\n---\nKeep **Markdown** intact."})
	ownerTools := managed.Tools(true)
	invalid := `{"name":"projects","title":"Project notes","description":"Shared project knowledge","schema":{"type":42},"instructions":"Anyone may read and write."}`
	rejected, err := toolCall(t, ownerTools, "memory_brain_create", invalid)
	if err != nil || !strings.Contains(rejected, `"retryable":true`) || !strings.Contains(rejected, `"mutated":false`) {
		t.Fatal("invalid schema not safely rejected", err, rejected)
	}
	if _, err := os.Stat(filepath.Join(brains, "apps", "projects")); !os.IsNotExist(err) {
		t.Fatal("invalid schema left a collection behind")
	}
	for range 2 {
		result, err := toolCall(t, ownerTools, "memory_brain_create", string(args))
		if err != nil || !strings.Contains(result, `"status":"complete"`) {
			t.Fatal("brain creation failed", err, result)
		}
	}
	managed, err = New(nil, brains)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := managed.Catalog(ctx)
	if err != nil || !strings.Contains(catalog, "Project notes") {
		t.Fatal("new session cannot discover brain", err)
	}
	info, err := toolCall(t, managed.Tools(false), "memory_info", `{"collection":"projects"}`)
	var metadata struct {
		Instructions string `json:"instructions"`
	}
	if err != nil || json.Unmarshal([]byte(info), &metadata) != nil || metadata.Instructions != "Anyone may read and write project facts.\n\n---\nKeep **Markdown** intact.\n" {
		t.Fatal("generated frontmatter damaged Markdown", err)
	}
	guestTools := managed.Tools(false)
	for _, tool := range guestTools {
		if strings.HasPrefix(tool.Name, "memory_brain_") {
			t.Fatal("guest received lifecycle tool")
		}
	}
	toolCall(t, guestTools, "memory_info", `{"collection":"projects"}`)
	written, err := toolCall(t, guestTools, "memory_create", `{"collection":"projects","title":"A fact","body":"Useful knowledge","data":{}}`)
	if err != nil || !strings.Contains(written, `"id"`) {
		t.Fatal("guest document write unavailable", err, written)
	}
	// Add a priority value without replacing documents or brain guidance.
	ownerTools = managed.Tools(true)
	toolCall(t, ownerTools, "memory_info", `{"collection":"projects"}`)
	schemaArgs := `{"name":"projects","schema":{"type":"object","properties":{"priority":{"enum":["low","medium","high","HIGHEST"]}}}}`
	for range 2 {
		result, err := toolCall(t, ownerTools, "memory_brain_schema_set", schemaArgs)
		if err != nil || !strings.Contains(result, `"status":"complete"`) {
			t.Fatal("schema update or receipt replay failed", err, result)
		}
	}
	if result, _ := toolCall(t, ownerTools, "memory_search", `{"collection":"projects","query":""}`); !strings.Contains(result, "Read memory_info") {
		t.Fatal("schema update did not invalidate guidance")
	}
	info, err = toolCall(t, ownerTools, "memory_info", `{"collection":"projects"}`)
	if err != nil || !strings.Contains(info, "HIGHEST") || json.Unmarshal([]byte(info), &metadata) != nil || metadata.Instructions != "Anyone may read and write project facts.\n\n---\nKeep **Markdown** intact.\n" {
		t.Fatal("schema update lost guidance or was not installed", err, info)
	}
	if json.Unmarshal([]byte(written), &doc) != nil {
		t.Fatal("invalid original document receipt")
	}
	read, err = toolCall(t, ownerTools, "memory_read", `{"collection":"projects","id":"`+doc.ID+`"}`)
	var preserved struct{ Hash string }
	if err != nil || json.Unmarshal([]byte(read), &preserved) != nil || preserved.Hash != doc.Hash {
		t.Fatal("schema change modified an existing document", err)
	}
	if result, err := toolCall(t, ownerTools, "memory_create", `{"collection":"projects","title":"Urgent","body":"A task","data":{"priority":"HIGHEST"}}`); err != nil || !strings.Contains(result, `"id"`) {
		t.Fatal("new enum value unavailable", err, result)
	}
	// A narrowing schema cannot invalidate the original document with empty data.
	before, err := os.ReadFile(filepath.Join(brains, "apps", "projects", "MEANS.md"))
	if err != nil {
		t.Fatal(err)
	}
	ownerTools = managed.Tools(true)
	toolCall(t, ownerTools, "memory_info", `{"collection":"projects"}`)
	if _, err := toolCall(t, ownerTools, "memory_brain_schema_set", `{"name":"projects","schema":{"type":"object","required":["priority"]}}`); err == nil {
		t.Fatal("schema incompatible with existing documents accepted")
	}
	after, err := os.ReadFile(filepath.Join(brains, "apps", "projects", "MEANS.md"))
	if err != nil || string(before) != string(after) {
		t.Fatal("rejected schema changed the definition", err)
	}
	result, err := toolCall(t, managed.Tools(true), "memory_brain_archive", `{"name":"projects"}`)
	if err != nil || !strings.Contains(result, `"status":"complete"`) {
		t.Fatal("archive failed", err, result)
	}
	catalog, err = managed.Catalog(ctx)
	if err != nil || strings.Contains(catalog, `"name":"projects"`) || !strings.Contains(catalog, `"kind":"memory"`) {
		t.Fatal("archived brain still discoverable", err, catalog)
	}
	if result, _ := toolCall(t, guestTools, "memory_read", `{"collection":"projects","id":"abcde"}`); !strings.Contains(result, "Unknown") {
		t.Fatal("old tools retained archived binding")
	}
	archived, err := filepath.Glob(filepath.Join(brains, "apps", ".archive", "projects-*", "MEANS.md"))
	if err != nil || len(archived) != 1 {
		t.Fatal("archive did not retain brain")
	}
}
