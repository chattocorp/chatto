package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func TestInstructionUpdateGuards(t *testing.T) {
	for _, result := range []string{"failure", `{}`, `{"hash":"new","instructions":"wrong"}`} {
		t.Run(result, func(t *testing.T) {
			m := &Manager{directory: t.TempDir()}
			path := filepath.Join(m.directory, "tasks")
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "MEANS.md"), []byte("placeholder"), 0600); err != nil {
				t.Fatal(err)
			}
			writes := 0
			m.run = func(_ context.Context, _ string, args []string, input []byte) ([]byte, error) {
				if args[3] == "info" {
					return []byte(`{"hash":"old","schema":{},"instructions":"Original"}`), nil
				}
				writes++
				if strings.Join(args[3:], " ") != "instructions set --input -" {
					t.Fatal("wrong command")
				}
				var payload struct{ Hash, Instructions string }
				if json.Unmarshal(input, &payload) != nil || payload.Hash != "old" || payload.Instructions != "Use tables." {
					t.Fatal("incorrect CLI payload")
				}
				if result == "failure" {
					return nil, errors.New("private CLI failure")
				}
				return []byte(result), nil
			}
			for _, tool := range m.Tools(false) {
				if tool.Name == "memory_brain_instructions_set" {
					t.Fatal("non-owner received tool")
				}
			}
			tools := m.Tools(true)
			args := `{"name":"tasks","hash":"old","instructions":"Use tables."}`
			if out, _ := toolCall(t, tools, "memory_brain_instructions_set", args); !strings.Contains(out, "Read memory_info") {
				t.Fatal("missing read prerequisite")
			}
			toolCall(t, tools, "memory_info", `{"collection":"tasks"}`)
			if out, _ := toolCall(t, tools, "memory_brain_instructions_set", strings.Replace(args, `"old"`, `"stale"`, 1)); !strings.Contains(out, "exact hash") {
				t.Fatal("stale host hash accepted")
			}
			if out, _ := toolCall(t, tools, "memory_brain_instructions_set", `{"name":"memory","hash":"old","instructions":"Replace policy"}`); !strings.Contains(out, "built-in") {
				t.Fatal("built-in memory not protected")
			}
			if writes != 0 {
				t.Fatal("rejected request reached CLI")
			}
			if _, err := toolCall(t, tools, "memory_brain_instructions_set", args); err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal("invalid receipt or error accepted")
			}
			if out, _ := toolCall(t, tools, "memory_brain_instructions_set", args); !strings.Contains(out, "uncertain") || writes != 1 {
				t.Fatal("uncertain write replayed")
			}
			m.collections = []config.Collection{{Name: "tasks", Path: path, Writable: true}}
			m.directory = t.TempDir()
			tools = m.Tools(true)
			toolCall(t, tools, "memory_info", `{"collection":"tasks"}`)
			if out, _ := toolCall(t, tools, "memory_brain_instructions_set", args); !strings.Contains(out, "fixed collection") || writes != 1 {
				t.Fatal("fixed collection instructions changed")
			}
		})
	}
}

func TestEmbeddedMeansInstructionsUpdate(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root := t.TempDir()
	m, err := New(nil, root)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := toolCall(t, m.Tools(true), "memory_brain_create", `{"name":"tasks","title":"Tasks","description":"Project tasks","schema":{},"instructions":"Anyone may read.\n"}`); err != nil || !strings.Contains(out, "complete") {
		t.Fatal("app creation failed", err, out)
	}
	path := filepath.Join(root, "apps", "tasks", "MEANS.md")
	docPath := filepath.Join(root, "apps", "tasks", "task.md")
	doc := []byte("---\nid: abcde\ntitle: Example task\ndata: {}\n---\nKeep this task.\n")
	if err := os.WriteFile(docPath, doc, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tools := m.Tools(true)
	info, err := toolCall(t, tools, "memory_info", `{"collection":"tasks"}`)
	var metadata struct{ Hash string }
	if err != nil || json.Unmarshal([]byte(info), &metadata) != nil || metadata.Hash == "" {
		t.Fatal("missing definition hash", err)
	}
	// Another turn holds the old hash; means must reject it after the first edit.
	stale := m.Tools(true)
	toolCall(t, stale, "memory_info", `{"collection":"tasks"}`)
	instructions := "Anyone may read.\n\nShow tasks as a table with priority and author.\n---\nKeep this body delimiter.\n"
	args, _ := json.Marshal(map[string]string{"name": "tasks", "hash": metadata.Hash, "instructions": instructions})
	for range 2 {
		out, err := toolCall(t, tools, "memory_brain_instructions_set", string(args))
		if err != nil || !strings.Contains(out, "complete") {
			t.Fatal("update or receipt replay failed", err, out)
		}
	}
	if out, _ := toolCall(t, tools, "memory_search", `{"collection":"tasks","query":""}`); !strings.Contains(out, "Read memory_info") {
		t.Fatal("old guidance still usable")
	}
	after, err := os.ReadFile(path)
	if err != nil || strings.SplitN(string(before), "\n---\n", 2)[0] != strings.SplitN(string(after), "\n---\n", 2)[0] {
		t.Fatal("frontmatter changed", err)
	}
	info, err = toolCall(t, tools, "memory_info", `{"collection":"tasks"}`)
	var updated struct{ Instructions string }
	if err != nil || json.Unmarshal([]byte(info), &updated) != nil || updated.Instructions != instructions {
		t.Fatal("body was not preserved exactly", err)
	}
	if _, err := toolCall(t, stale, "memory_brain_instructions_set", string(args)); err == nil {
		t.Fatal("means accepted stale definition hash")
	}
	final, err := os.ReadFile(path)
	if err != nil || string(final) != string(after) {
		t.Fatal("stale edit changed definition", err)
	}
	stored, err := os.ReadFile(docPath)
	if err != nil || string(stored) != string(doc) {
		t.Fatal("instruction edit changed a document", err)
	}
}
