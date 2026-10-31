package memory

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
)

func toolCall(t *testing.T, tools []agent.Tool, name, args string) (string, error) {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool.Execute(context.Background(), json.RawMessage(args))
		}
	}
	t.Fatalf("missing tool %s", name)
	return "", nil
}

func TestCollectionMetadataComesFromMeans(t *testing.T) {
	title := "Team memory"
	instructions := "# Procedures\n\nKeep **complete** Markdown.\n"
	m := &Manager{collections: []config.Collection{{Name: "notes", Path: "/fixed"}}}
	m.run = func(_ context.Context, dir string, args []string, _ []byte) ([]byte, error) {
		if dir != "/fixed" || strings.Join(args, " ") != "--path /fixed --json info" {
			t.Fatal("metadata did not use configured collection")
		}
		return json.Marshal(map[string]any{"path": "/fixed", "title": title, "description": "Shared procedures", "instructions": instructions, "schema": map[string]any{}})
	}
	tools := m.Tools(false)
	for _, current := range []string{"Team memory", "Updated title"} {
		title = current
		result, err := toolCall(t, tools, "memory_catalog", `{}`)
		var entries []struct{ Name, Title, Description string }
		if err != nil || json.Unmarshal([]byte(result), &entries) != nil || len(entries) != 1 || entries[0].Name != "notes" || entries[0].Title != current || entries[0].Description != "Shared procedures" {
			t.Fatal("catalog did not load current means metadata")
		}
	}
	result, err := toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	var info struct{ Title, Description, Instructions string }
	if err != nil || json.Unmarshal([]byte(result), &info) != nil || info.Instructions != instructions || info.Title != title || info.Description != "Shared procedures" || strings.Contains(result, "/fixed") {
		t.Fatal("info did not preserve collection metadata and Markdown without its path")
	}
}

func TestReadOnlyAndCollectionAllowlist(t *testing.T) {
	calls := 0
	m := &Manager{collections: []config.Collection{{Name: "known", Path: "/fixed", Writable: false}}, run: func(context.Context, string, []string, []byte) ([]byte, error) { calls++; return nil, nil }}
	for _, tool := range m.Tools(false) {
		if strings.HasPrefix(tool.Name, "memory_brain_") {
			t.Fatal("lifecycle exposed to non-owner")
		}
	}
	for _, args := range []string{`{"collection":"../../outside"}`, `{"collection":"known","path":"/outside"}`} {
		result, err := toolCall(t, m.Tools(true), "memory_info", args)
		if err != nil || !strings.Contains(result, "error") {
			t.Fatal("invalid binding accepted")
		}
	}
	result, _ := toolCall(t, m.Tools(true), "memory_create", `{"collection":"known","title":"test","body":"text","data":{}}`)
	if !strings.Contains(result, "read-only") || calls != 0 {
		t.Fatal("host did not enforce collection permissions before subprocess")
	}
}

func TestMeansArgumentsReceiptsAndWritePrerequisites(t *testing.T) {
	writes := 0
	m := &Manager{collections: []config.Collection{{Name: "notes", Path: "/fixed", Writable: true}}}
	m.run = func(_ context.Context, dir string, args []string, input []byte) ([]byte, error) {
		if dir != "/fixed" || strings.Join(args[:3], " ") != "--path /fixed --json" {
			t.Fatal("path escaped binding")
		}
		switch args[3] {
		case "info":
			return []byte(`{"path":"/fixed","schema":{},"instructions":"workflow"}`), nil
		case "search":
			if args[len(args)-2] != "--" || args[len(args)-1] != "--path=/outside" {
				t.Fatal("query treated as flags")
			}
			return []byte(`{"documents":[]}`), nil
		case "show":
			return []byte(`{"id":"abcde","hash":"current","body":"old"}`), nil
		case "create", "update":
			writes++
			var envelope map[string]json.RawMessage
			if json.Unmarshal(input, &envelope) != nil || envelope["frontmatter"] == nil {
				t.Fatal("write payload is not stdin JSON")
			}
			if args[3] == "update" && string(envelope["hash"]) != `"current"` {
				t.Fatal("update lost concurrency guard")
			}
			return []byte(`{"id":"abcde","hash":"next","body":"saved"}`), nil
		}
		return nil, errors.New("unexpected command")
	}
	tools := m.Tools(true)
	create := `{"collection":"notes","title":"Preference","body":"short answers","data":{}}`
	if result, _ := toolCall(t, tools, "memory_create", `{"collection":"notes","title":"Preference","body":null,"data":{}}`); !strings.Contains(result, "cannot be null") {
		t.Fatal("null body accepted as an empty replacement")
	}
	if result, _ := toolCall(t, tools, "memory_create", create); !strings.Contains(result, "Read memory_info") {
		t.Fatal("write skipped schema read")
	}
	if result, err := toolCall(t, tools, "memory_info", `{"collection":"notes"}`); err != nil || strings.Contains(result, "/fixed") {
		t.Fatal("info leaked configured path")
	}
	toolCall(t, tools, "memory_search", `{"collection":"notes","query":"--path=/outside"}`)
	for range 2 {
		if _, err := toolCall(t, tools, "memory_create", create); err != nil {
			t.Fatal(err)
		}
	}
	if writes != 1 {
		t.Fatal("duplicate create executed twice")
	}
	update := `{"collection":"notes","id":"abcde","hash":"current","title":"Preference","body":"new","data":{}}`
	if result, _ := toolCall(t, tools, "memory_update", update); !strings.Contains(result, "Read this document") {
		t.Fatal("update skipped document read")
	}
	toolCall(t, tools, "memory_read", `{"collection":"notes","id":"abcde"}`)
	if _, err := toolCall(t, tools, "memory_update", update); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatal("update did not execute")
	}
}

func TestUncertainWriteBlocksFurtherWrites(t *testing.T) {
	calls := 0
	m := &Manager{collections: []config.Collection{{Name: "notes", Writable: true}}, run: func(_ context.Context, _ string, args []string, _ []byte) ([]byte, error) {
		if args[3] == "info" {
			return []byte(`{"schema":{}}`), nil
		}
		calls++
		return nil, errors.New("private error")
	}}
	tools := m.Tools(true)
	toolCall(t, tools, "memory_info", `{"collection":"notes"}`)
	args := `{"collection":"notes","title":"A","body":"B","data":{}}`
	if _, err := toolCall(t, tools, "memory_create", args); err == nil {
		t.Fatal("write failure hidden")
	}
	result, err := toolCall(t, tools, "memory_create", args)
	if err != nil || !strings.Contains(result, "uncertain") || calls != 1 {
		t.Fatal("uncertain mutation retried")
	}
}

func TestOutputLimitCannotBeBypassedByCopy(t *testing.T) {
	out := &boundedOutput{}
	if _, err := io.Copy(out, strings.NewReader(strings.Repeat("x", resultLimit+1))); err == nil || len(out.Bytes()) > resultLimit {
		t.Fatal("unbounded subprocess output")
	}
}
