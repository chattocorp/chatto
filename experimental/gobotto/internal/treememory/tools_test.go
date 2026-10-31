package treememory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/memorytree"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestDisclosureAuthorizationAndReceipts(t *testing.T) {
	s, err := memorytree.Open(filepath.Join(t.TempDir(), "tree.sqlite"), "Root personality")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defineTestNote(t, s)
	m := &Manager{Store: s}
	ctx := context.Background()
	tools := m.Tools(false)
	call := func(tools []agent.Tool, name, args string) string {
		t.Helper()
		for _, tool := range tools {
			if tool.Name == name {
				v, e := tool.Execute(ctx, json.RawMessage(args))
				if e != nil {
					t.Fatal(e)
				}
				return v
			}
		}
		t.Fatal("tool missing")
		return ""
	}
	args := `{"change":{"operation":"create","node":{"parent":"root","title":"Projects","summary":"Useful entry point","type":"note","version":1,"data":{},"body":"DEEP_CONTENT"}}}`
	first := call(tools, "memory_tree_change", args)
	second := call(tools, "memory_tree_change", args)
	if first != second {
		t.Fatal("repeated create did not return receipt")
	}
	if strings.Contains(first, "DEEP_CONTENT") {
		t.Fatal("mutation receipt repeated document body")
	}
	state, _ := s.Snapshot(ctx)
	if len(state.Nodes) != 2 {
		t.Fatal("duplicate create")
	}
	prompt, err := m.Prompt(ctx)
	if err != nil || !strings.Contains(prompt, "Root personality") || !strings.Contains(prompt, "Useful entry point") || strings.Contains(prompt, "DEEP_CONTENT") {
		t.Fatal("progressive disclosure leaked content or lost index")
	}
	root, _ := state.Find("root")
	root.Body = "Changed personality"
	raw := encoded(map[string]any{"change": memorytree.Change{Operation: "update", Node: root}})
	if result := call(tools, "memory_tree_change", raw); !strings.Contains(result, "rejected") {
		t.Fatal("guest changed personality")
	}
	if result := call(m.Tools(true), "memory_tree_change", raw); !strings.Contains(result, "complete") {
		t.Fatal(result)
	}
	prompt, _ = m.Prompt(ctx)
	if !strings.Contains(prompt, "Changed personality") {
		t.Fatal("stale root")
	}
	bad := strings.Replace(args, `"body":"DEEP_CONTENT"`, `"body":null`, 1)
	if result := call(m.Tools(false), "memory_tree_change", bad); !strings.Contains(result, "rejected") {
		t.Fatal("null body accepted")
	}
	if result := call(tools, "memory_tree_read", `{"id":"root","unknown":true}`); !strings.Contains(result, "rejected") {
		t.Fatal("unknown arguments accepted")
	}
}

func TestMutationSchemaGuidesCreationAndMinimalMoves(t *testing.T) {
	raw, err := json.Marshal(changeSchema())
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"create instructions", `{"operation":"create","node":{"parent":"root","title":"Places","summary":"Research places","type":"note","version":1,"data":{},"body":"Procedure","instructions":true}}`, true},
		{"minimal move", `{"operation":"move","node":{"id":"city","revision":1,"parent":"country","position":0}}`, true},
		{"explicit migration", `{"operation":"migrate","node":{"id":"city","revision":1,"title":"Place","summary":"","type":"place","version":2,"data":{"country":"DE"},"body":"Facts","instructions":false}}`, true},
		{"move repeats body", `{"operation":"move","node":{"id":"city","revision":1,"parent":"country","position":0,"body":"Unnecessary copy"}}`, false},
		{"nested operation", `{"operation":"create","node":{"operation":"create","parent":"root","title":"Place","summary":"","type":"note","version":1,"data":{},"body":"Facts"}}`, false},
		{"missing create body", `{"operation":"create","node":{"parent":"root","title":"Place","summary":"","type":"note","version":1,"data":{}}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value any
			if err := json.Unmarshal([]byte(tc.input), &value); err != nil {
				t.Fatal(err)
			}
			if err := resolved.Validate(value); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestIndexDistinguishesOmittedContentAndChildren(t *testing.T) {
	s, err := memorytree.Open(filepath.Join(t.TempDir(), "tree.sqlite"), "Personality")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	defineTestNote(t, s)
	ctx := context.Background()
	parent, err := s.Apply(ctx, memorytree.Change{Operation: "create", Node: memorytree.Node{Parent: "root", Title: "Places", Type: "note", Version: 1, Data: json.RawMessage(`{}`), Body: "PROCEDURE", Instructions: true}}, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(ctx, memorytree.Change{Operation: "create", Node: memorytree.Node{Parent: parent.(memorytree.Node).ID, Title: "Wedel", Type: "note", Version: 1, Data: json.RawMessage(`{}`), Body: "FACTS"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: s}
	for _, tool := range m.Tools(false) {
		if tool.Name != "memory_tree_children" && tool.Name != "memory_tree_search" {
			continue
		}
		out, err := tool.Execute(ctx, json.RawMessage(`{"id":"root"}`))
		if err != nil {
			t.Fatal(err)
		}
		var result struct{ Items []map[string]any }
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, item := range result.Items {
			if _, exists := item["body"]; exists {
				t.Fatal("index misrepresents omitted body")
			}
			if _, exists := item["data"]; exists {
				t.Fatal("index misrepresents omitted data")
			}
			if item["title"] == "Places" {
				found = true
				if item["child_count"] != float64(1) || item["instructions"] != true {
					t.Fatal("index lost descendants or authority", item)
				}
			}
		}
		if !found {
			t.Fatal("parent missing from index")
		}
	}
	prompt, err := m.Prompt(ctx)
	if err != nil || strings.Contains(prompt, "PROCEDURE") || strings.Contains(prompt, "FACTS") || !strings.Contains(prompt, `"child_count":1`) {
		t.Fatal("prompt did not preserve progressive disclosure", err)
	}
}

func defineTestNote(t *testing.T, s *memorytree.Store) {
	t.Helper()
	_, err := s.Apply(context.Background(), memorytree.Change{Operation: "type", Type: memorytree.Type{Name: "note", Version: 1, Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}, true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMigrationToolAndVersionDiscovery(t *testing.T) {
	s, err := memorytree.Open(filepath.Join(t.TempDir(), "memory.sqlite"), "Personality")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := &Manager{Store: s}
	tools := m.Tools(true)
	call := func(name string, args any) map[string]any {
		t.Helper()
		for _, tool := range tools {
			if tool.Name != name {
				continue
			}
			out, err := tool.Execute(context.Background(), json.RawMessage(encoded(args)))
			if err != nil {
				t.Fatal(err)
			}
			var result map[string]any
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			return result
		}
		t.Fatal("missing tool", name)
		return nil
	}
	for version := 1; version <= 2; version++ {
		result := call("memory_tree_change", map[string]any{"change": memorytree.Change{Operation: "type", Type: memorytree.Type{Name: "place", Version: version, Schema: json.RawMessage(`{"type":"object","properties":{"country":{"type":"string"}},"required":["country"]}`)}}})
		if result["status"] != "complete" {
			t.Fatal(result)
		}
	}
	created := call("memory_tree_change", map[string]any{"change": memorytree.Change{Operation: "create", Node: memorytree.Node{Parent: "root", Title: "Place", Type: "place", Version: 1, Data: json.RawMessage(`{"country":"DE"}`)}}})
	if created["status"] != "complete" {
		t.Fatal(created)
	}
	state, _ := s.Snapshot(context.Background())
	n, _ := state.Find(created["result"].(map[string]any)["id"].(string))
	n.Version = 2
	if result := call("memory_tree_change", map[string]any{"change": memorytree.Change{Operation: "migrate", Node: n}}); result["status"] != "complete" || result["result"].(map[string]any)["version"] != float64(2) {
		t.Fatal("migration receipt missing target version", result)
	}
	for version := 1; version <= 2; version++ {
		result := call("memory_tree_search", map[string]any{"id": "root", "type": "place", "version": version, "field": "country", "value": "DE"})
		if result["total"] != float64(version-1) {
			t.Fatal("version filtering returned wrong records", result)
		}
		result = call("memory_tree_types", map[string]any{"type": "place", "version": version})
		if result["total"] != float64(1) || result["items"].([]any)[0].(map[string]any)["version"] != float64(version) {
			t.Fatal("schema lookup returned wrong version", result)
		}
	}
}
