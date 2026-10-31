package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		for _, op := range []string{"create", "archive"} {
			args := `{"name":"` + name + `"}`
			if op == "create" {
				args = `{"name":"` + name + `","title":"Notes","description":"Shared notes","schema":{},"instructions":"Anyone may read."}`
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
