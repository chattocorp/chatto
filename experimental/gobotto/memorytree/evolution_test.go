package memorytree

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestSchemaEvolutionAndAtomicMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.sqlite")
	s := openTest(t, path)
	ctx := context.Background()
	first := Type{Name: "profile", Version: 1, Schema: json.RawMessage(`{"type":"object","properties":{"user_id":{"type":"string"}},"required":["user_id"],"additionalProperties":false}`)}
	if _, err := s.Apply(ctx, Change{Operation: "type", Type: first}, true); err != nil {
		t.Fatal(err)
	}
	v, err := s.Apply(ctx, Change{Operation: "create", Node: Node{Parent: "root", Title: "Profile", Type: "profile", Version: 1, Data: json.RawMessage(`{"user_id":"user-a"}`)}}, false)
	if err != nil {
		t.Fatal(err)
	}
	original := v.(Node)
	second := first
	second.Schema = json.RawMessage(`{"type":"object","properties":{"actor_id":{"type":"string"}},"required":["actor_id"],"additionalProperties":false}`)
	if _, err := s.Apply(ctx, Change{Operation: "type", Type: second}, true); !IsRejection(err) {
		t.Fatal("in-place schema replacement accepted", err)
	}
	second.Version = 3
	if _, err := s.Apply(ctx, Change{Operation: "type", Type: second}, true); !IsRejection(err) {
		t.Fatal("non-consecutive schema version accepted", err)
	}
	second.Version = 2
	if _, err := s.Apply(ctx, Change{Operation: "type", Type: second}, true); err != nil {
		t.Fatal("isolated breaking version rejected", err)
	}
	old, _ := snap(t, s).Find(original.ID)
	if old.Version != 1 || validate(old, snap(t, s)) != nil {
		t.Fatal("new schema reinterpreted old record")
	}
	next := old
	next.Version = 2
	before, _ := s.History(ctx, 0)
	for _, tc := range []struct {
		op    string
		owner bool
	}{
		{"update", true}, {"update", false}, {"migrate", false}, {"migrate", true},
	} {
		if _, err := s.Apply(ctx, Change{Operation: tc.op, Node: next}, tc.owner); !IsRejection(err) {
			t.Fatalf("invalid migration accepted: %+v: %v", tc, err)
		}
	}
	unchanged, _ := snap(t, s).Find(old.ID)
	after, _ := s.History(ctx, 0)
	if unchanged.Revision != old.Revision || unchanged.Version != 1 || string(before[0]) != string(after[0]) {
		t.Fatal("rejected migration changed data or journal")
	}
	next.Data = json.RawMessage(`{"actor_id":"user-a"}`)
	if _, err := s.Apply(ctx, Change{Operation: "migrate", Node: next}, false); !IsRejection(err) {
		t.Fatal("guest migrated valid data", err)
	}
	v, err = s.Apply(ctx, Change{Operation: "migrate", Node: next}, true)
	if err != nil {
		t.Fatal(err)
	}
	migrated := v.(Node)
	if migrated.ID != old.ID || migrated.Parent != old.Parent || migrated.Position != old.Position || migrated.Revision != old.Revision+1 || migrated.Version != 2 {
		t.Fatal("migration lost identity, location, or revision")
	}
	if _, err := s.Apply(ctx, Change{Operation: "migrate", Node: next}, true); !IsRejection(err) {
		t.Fatal("stale migration accepted", err)
	}
	history, _ := s.History(ctx, 0)
	var entry struct {
		Operation     string
		Before, After Node
	}
	if err := json.Unmarshal(history[0], &entry); err != nil || entry.Operation != "migrate" || entry.Before.Version != 1 || entry.After.Version != 2 {
		t.Fatal("migration recovery journal missing", err)
	}
	s.Close()
	s = openTest(t, path)
	reloaded, _ := snap(t, s).Find(old.ID)
	if reloaded.Version != 2 || validate(reloaded, snap(t, s)) != nil {
		t.Fatal("migration did not survive reload")
	}
}

func TestFreshStoreHasNoGenericNoteAndAllNodesValidate(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "memory.sqlite"), "Personality")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state := snap(t, s)
	if len(state.Types) != 1 || state.Types[0].Name != "agent" {
		t.Fatal("fresh store offers generic fallback types")
	}
	for _, n := range state.Nodes {
		if err := validate(n, state); err != nil {
			t.Fatal("bootstrap node bypasses schema", err)
		}
	}
	for _, n := range []Node{
		{Parent: "root", Title: "Untyped", Data: json.RawMessage(`{}`)},
		{Parent: "root", Title: "Unknown", Type: "missing", Version: 1, Data: json.RawMessage(`{}`)},
		{Parent: "root", Title: "Null", Type: "agent", Version: 1, Data: json.RawMessage(`null`)},
		{Parent: "root", Title: "Invalid", Type: "agent", Version: 1, Data: json.RawMessage(`{}`)},
	} {
		if _, err := s.Apply(context.Background(), Change{Operation: "create", Node: n}, true); !IsRejection(err) {
			t.Fatal("invalid typed data accepted", err)
		}
	}
}

func TestOpenPreservesLegacySchemaAndContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.sqlite")
	s := openTest(t, path)
	legacy := Type{Name: "agent", Version: 1, Schema: json.RawMessage(`{"type":"object"}`)}
	b, _ := json.Marshal(legacy)
	if _, err := s.db.Exec("UPDATE types SET payload=? WHERE name='agent' AND version=1", b); err != nil {
		t.Fatal(err)
	}
	root, _ := snap(t, s).Find("root")
	root.Data = json.RawMessage(`{}`)
	root.Body = "Existing personality"
	if _, err := s.Apply(context.Background(), Change{Operation: "update", Node: root}, true); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = openTest(t, path)
	state := snap(t, s)
	root, _ = state.Find("root")
	if root.Body != "Existing personality" || string(root.Data) != `{}` || validate(root, state) != nil {
		t.Fatal("reopen changed legacy memory")
	}
	for _, typ := range state.Types {
		if typ.Name == "agent" && string(typ.Schema) != string(legacy.Schema) {
			t.Fatal("reopen replaced legacy schema")
		}
	}
}
