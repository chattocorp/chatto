package memorytree

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openTest(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path, "Initial personality")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	state, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, typ := range state.Types {
		if typ.Name == "note" {
			return s
		}
	}
	if _, err := s.Apply(context.Background(), Change{Operation: "type", Type: Type{Name: "note", Version: 1, Schema: json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string"}},"additionalProperties":false}`)}}, true); err != nil {
		t.Fatal(err)
	}
	return s
}
func snap(t *testing.T, s *Store) Snapshot {
	t.Helper()
	v, err := s.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func create(t *testing.T, s *Store, parent, title string) Node {
	t.Helper()
	v, err := s.Apply(context.Background(), Change{Operation: "create", Node: Node{Parent: parent, Title: title, Summary: "summary", Type: "note", Version: 1, Data: json.RawMessage(`{"topic":"testing"}`), Body: "remember this"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	return v.(Node)
}

func TestInstructionAuthoritySurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.sqlite")
	s := openTest(t, path)
	ctx := context.Background()
	n := Node{Parent: "root", Title: "Places", Type: "note", Version: 1, Data: json.RawMessage(`{}`), Body: "Research and remember places", Instructions: true}
	if _, err := s.Apply(ctx, Change{Operation: "create", Node: n}, false); !IsRejection(err) {
		t.Fatal("guest created instructions", err)
	}
	v, err := s.Apply(ctx, Change{Operation: "create", Node: n}, true)
	if err != nil {
		t.Fatal(err)
	}
	n = v.(Node)
	s.Close()
	s = openTest(t, path)
	n, _ = snap(t, s).Find(n.ID)
	if !n.Instructions {
		t.Fatal("instruction authority lost on reload")
	}
	// A guest cannot strip the flag to bypass the protected-node check.
	n.Instructions = false
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: n}, false); !IsRejection(err) {
		t.Fatal("guest demoted instructions", err)
	}
	data := create(t, s, n.ID, "Wedel")
	data.Instructions = true
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: data}, false); !IsRejection(err) {
		t.Fatal("guest promoted data", err)
	}
	data.Instructions = false
	data.Body = "Additional facts"
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: data}, false); err != nil {
		t.Fatal("guest could not contribute data", err)
	}
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: n}, true); err != nil {
		t.Fatal("owner could not change instruction authority", err)
	}
}

func TestTreeIntegrityAndRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.sqlite")
	s := openTest(t, path)
	ctx := context.Background()
	a := create(t, s, "root", "Projects")
	b := create(t, s, a.ID, "Nested fact")
	for _, change := range []Change{
		{Operation: "move", Node: Node{ID: a.ID, Revision: a.Revision, Parent: b.ID}},
		{Operation: "move", Node: Node{ID: a.ID, Revision: a.Revision, Parent: "missing"}},
		{Operation: "move", Node: Node{ID: "root", Revision: 1, Parent: a.ID}},
		{Operation: "archive", Node: Node{ID: "root", Revision: 1}},
	} {
		if _, err := s.Apply(ctx, change, true); !IsRejection(err) {
			t.Fatalf("unsafe change accepted: %v", err)
		}
	}
	history, err := s.History(ctx, 0)
	if err != nil || len(history) != 1 {
		t.Fatal("rejected writes changed journal", err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM journal").Scan(&count); err != nil || count != 3 {
		t.Fatal("rejected writes journaled", err)
	}
	archived, err := s.Apply(ctx, Change{Operation: "archive", Node: a}, true)
	if err != nil {
		t.Fatal(err)
	}
	state := snap(t, s)
	if state.Visible(b.ID) || len(state.Nodes) != 3 || len(state.Search("root", "", "note", "", nil, false)) != 0 {
		t.Fatal("archive lost nodes or exposed descendants")
	}
	reopened := openTest(t, path)
	state = snap(t, reopened)
	if state.Visible(b.ID) {
		t.Fatal("archive lost on restart")
	}
	root, _ := state.Find("root")
	if root.Body != "Initial personality" {
		t.Fatal("startup overwrote personality")
	}
	if _, err := reopened.Apply(ctx, Change{Operation: "restore", Node: archived.(Node)}, true); err != nil {
		t.Fatal(err)
	}
	state = snap(t, reopened)
	if !state.Visible(b.ID) || len(state.Search(a.ID, "remember", "note", "topic", json.RawMessage(`"testing"`), false)) != 2 {
		t.Fatal("subtree recovery/search failed")
	}
	old := b
	b.Body = "replacement"
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: b}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: old}, false); !IsRejection(err) {
		t.Fatal("stale update accepted")
	}
	history, err = s.History(ctx, 0)
	if err != nil || !strings.Contains(string(history[0]), "remember this") {
		t.Fatal("previous content not recoverable from journal")
	}
}

func TestTypesAndPermissions(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), "memory.sqlite"))
	ctx := context.Background()
	state := snap(t, s)
	root, _ := state.Find("root")
	typeDef := Type{Name: "fact", Version: 1, Schema: json.RawMessage(`{"type":"object","required":["source"],"properties":{"source":{"type":"string"}},"additionalProperties":false}`)}
	if _, err := s.Apply(ctx, Change{Operation: "type", Type: typeDef}, false); !IsRejection(err) {
		t.Fatal("guest defined schema")
	}
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: root}, false); !IsRejection(err) {
		t.Fatal("guest changed root")
	}
	if _, err := s.Apply(ctx, Change{Operation: "type", Type: typeDef}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, Change{Operation: "type", Type: typeDef}, true); !IsRejection(err) {
		t.Fatal("schema overwritten")
	}
	n := Node{Parent: "root", Title: "Fact", Type: "fact", Version: 1, Data: json.RawMessage(`{}`)}
	if _, err := s.Apply(ctx, Change{Operation: "create", Node: n}, false); !IsRejection(err) {
		t.Fatal("invalid document accepted")
	}
	n.Data = json.RawMessage(`{"source":"a book"}`)
	v, err := s.Apply(ctx, Change{Operation: "create", Node: n}, false)
	if err != nil {
		t.Fatal(err)
	}
	n = v.(Node)
	typeDef.Version = 2
	typeDef.Schema = json.RawMessage(`false`)
	if _, err = s.Apply(ctx, Change{Operation: "type", Type: typeDef}, true); err != nil {
		t.Fatal(err)
	}
	n.Body = "valid under original schema"
	if _, err = s.Apply(ctx, Change{Operation: "update", Node: n}, false); err != nil {
		t.Fatal("new version changed old node validation", err)
	}
	n, _ = snap(t, s).Find(n.ID)
	for _, op := range []string{"move", "archive", "restore"} {
		if _, err = s.Apply(ctx, Change{Operation: op, Node: n}, false); err == nil {
			t.Fatal("guest structural change", op)
		}
	}
	if _, err = s.Apply(ctx, Change{Operation: "type", Type: Type{Name: "remote", Version: 1, Schema: json.RawMessage(`{"$ref":"https://example.com/schema"}`)}}, true); !IsRejection(err) {
		t.Fatal("external schema reference accepted")
	}
}

func TestIdentityBinding(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), "memory.sqlite"))
	ctx := context.Background()
	for range 2 {
		if err := s.BindIdentity(ctx, "one-bot-hash"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.BindIdentity(ctx, "other-bot-hash"); !IsRejection(err) {
		t.Fatal("database reused across identities")
	}
}

func TestCompetingWritersAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.sqlite")
	a := openTest(t, path)
	b := openTest(t, path)
	node := create(t, a, "root", "Shared")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, s := range []*Store{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.Apply(context.Background(), Change{Operation: "update", Node: node}, false)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("competing writes did not preserve revision guard", success)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Apply(ctx, Change{Operation: "archive", Node: node}, true); err == nil {
		t.Fatal("cancelled mutation committed")
	}
	state := snap(t, a)
	current, _ := state.Find(node.ID)
	if current.Revision != 2 || current.Archived {
		t.Fatal("lost update or partial cancelled write")
	}
}

func TestMoveOrderAndRootBudget(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), "memory.sqlite"))
	ctx := context.Background()
	a := create(t, s, "root", "A")
	b := create(t, s, "root", "B")
	b.Position = -1
	if _, err := s.Apply(ctx, Change{Operation: "move", Node: b}, true); err != nil {
		t.Fatal(err)
	}
	children := snap(t, s).Children("root", false)
	if children[0].ID != b.ID {
		t.Fatal("sibling order not changed")
	}
	root, _ := snap(t, s).Find("root")
	root.Body = strings.Repeat("x", 4001)
	if _, err := s.Apply(ctx, Change{Operation: "update", Node: root}, true); !IsRejection(err) {
		t.Fatal("unbounded root accepted")
	}
	a.Parent = b.ID
	if _, err := s.Apply(ctx, Change{Operation: "move", Node: a}, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := snap(t, s).Find(a.ID); got.ID != a.ID || got.Parent != b.ID {
		t.Fatal("move changed identity")
	}
}
