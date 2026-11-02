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

func TestBrainPathsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	m := &Manager{directory: root, run: func(context.Context, string, []string, []byte) ([]byte, error) {
		t.Fatal("unsafe request reached means")
		return nil, nil
	}}
	for _, name := range []string{"../outside", "/absolute", "linked", ".archive"} {
		for _, op := range []string{"create", "archive", "schema_set"} {
			args := `{"name":"` + name + `"}`
			if op == "create" {
				args = `{"name":"` + name + `","title":"Notes","description":"Shared notes","schema":{},"instructions":"Anyone may read."}`
			}
			if op == "schema_set" {
				args = `{"name":"` + name + `","schema":{}}`
			}
			result, err := toolCall(t, m.Tools(true), "memory_brain_"+op, args)
			if err != nil || !strings.Contains(result, "error") {
				t.Fatal("unsafe lifecycle request accepted", name, op)
			}
		}
	}
	bindings, err := m.bindings()
	if err != nil || len(bindings) != 0 {
		t.Fatal("symlink discovered as brain")
	}
}

func TestSchemaUpdateBoundaries(t *testing.T) {
	m := &Manager{directory: t.TempDir()}
	path := filepath.Join(m.directory, "notes")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "MEANS.md"), []byte("placeholder"), 0600); err != nil {
		t.Fatal(err)
	}
	mutations := 0
	m.run = func(_ context.Context, _ string, args []string, _ []byte) ([]byte, error) {
		if args[len(args)-1] == "info" {
			return []byte(`{"schema":{}}`), nil
		}
		mutations++
		return nil, errors.New("private provider detail")
	}
	for _, tool := range m.Tools(false) {
		if tool.Name == "memory_brain_schema_set" {
			t.Fatal("non-owner batch received schema tool")
		}
	}
	tools := m.Tools(true)
	args := `{"name":"notes","schema":{}}`
	if result, _ := toolCall(t, tools, "memory_brain_schema_set", args); !strings.Contains(result, "Read memory_info") {
		t.Fatal("schema change did not require current guidance")
	}
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	for _, invalid := range []string{`null`, `42`, `"string"`, `[]`} {
		if result, err := toolCall(t, tools, "memory_brain_schema_set", `{"name":"notes","schema":`+invalid+`}`); err != nil || !strings.Contains(result, "error") {
			t.Fatal("invalid schema argument accepted", invalid)
		}
	}
	if mutations != 0 {
		t.Fatal("rejected request reached means")
	}
	if _, err := toolCall(t, tools, "memory_brain_schema_set", args); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("CLI failure not safely reported")
	}
	if result, _ := toolCall(t, tools, "memory_brain_schema_set", args); !strings.Contains(result, "uncertain") || mutations != 1 {
		t.Fatal("uncertain mutation retried")
	}
	if result, _ := toolCall(t, tools, "memory_create", `{"collection":"notes","title":"Test","body":"","data":{}}`); !strings.Contains(result, "uncertain") || mutations != 1 {
		t.Fatal("document write allowed after uncertain schema mutation")
	}
	m.collections = []config.Collection{{Name: "notes", Path: path, Writable: true}}
	m.directory = t.TempDir()
	tools = m.Tools(true)
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	if result, _ := toolCall(t, tools, "memory_brain_schema_set", args); !strings.Contains(result, "fixed collection") || mutations != 1 {
		t.Fatal("fixed binding schema mutation allowed")
	}
}

func TestBrainCreationValidationAndUncertainWrites(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty reservation", true: "partial write"}[partial], func(t *testing.T) {
			m := &Manager{directory: t.TempDir()}
			calls := 0
			m.run = func(_ context.Context, dir string, _ []string, input []byte) ([]byte, error) {
				calls++
				if !strings.HasPrefix(string(input), "---\n{") || !strings.Contains(string(input), "\n---\n") {
					t.Fatal("missing generated frontmatter")
				}
				if partial {
					if err := os.WriteFile(filepath.Join(dir, "MEANS.md"), input, 0600); err != nil {
						t.Fatal(err)
					}
				}
				return nil, errors.New("private CLI failure")
			}
			tools := m.Tools(true)
			args := `{"name":"notes","title":"Notes","description":"Shared notes","schema":{},"instructions":"Anyone may read."}`
			result, err := toolCall(t, tools, "memory_brain_create", args)
			if partial {
				if err == nil {
					t.Fatal("partial write classified as safe")
				}
				result, _ = toolCall(t, tools, "memory_brain_create", strings.Replace(args, `"notes"`, `"other"`, 1))
				if !strings.Contains(result, "uncertain") || calls != 1 {
					t.Fatal("uncertain write allowed another mutation")
				}
			} else {
				if err != nil || !strings.Contains(result, `"retryable":true`) {
					t.Fatal("empty failure did not allow correction")
				}
				toolCall(t, tools, "memory_brain_create", args)
				if calls != 2 {
					t.Fatal("safe correction was blocked")
				}
			}
		})
	}
}

func TestBrainDefinitionEscapesFrontmatter(t *testing.T) {
	a := brainArguments{Name: "notes", Title: "A title\n---\nInjected: value", Description: "A \"quoted\" description", Schema: json.RawMessage(`false`), Instructions: "# Guidance\n\n---\nBody"}
	definition, err := a.definition()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(definition), "\n---\n", 2)
	var metadata struct {
		Title       string
		Description string
		Schema      bool
	}
	if len(parts) != 2 || json.Unmarshal([]byte(strings.TrimPrefix(parts[0], "---\n")), &metadata) != nil || metadata.Title != a.Title || metadata.Description != a.Description || parts[1] != a.Instructions+"\n" {
		t.Fatal("metadata escaped its frontmatter envelope")
	}
}

func TestGuidanceRequiredBeforeDocumentAccess(t *testing.T) {
	m := &Manager{directory: t.TempDir()}
	path := filepath.Join(m.directory, "notes")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "MEANS.md"), []byte("placeholder"), 0600); err != nil {
		t.Fatal(err)
	}
	m.run = func(context.Context, string, []string, []byte) ([]byte, error) {
		t.Fatal("document accessed without guidance")
		return nil, nil
	}
	for _, op := range []string{"search", "read"} {
		arg := `{"collection":"notes","query":"anything"}`
		if op == "read" {
			arg = `{"collection":"notes","id":"abcde"}`
		}
		result, err := toolCall(t, m.Tools(false), "memory_"+op, arg)
		if err != nil || !strings.Contains(result, "Read memory_info") {
			t.Fatal("guidance requirement missing")
		}
	}
}
