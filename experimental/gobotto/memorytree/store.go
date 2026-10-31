// Package memorytree stores a typed, revisioned memory tree in pure-Go SQLite.
// It has no model, Chatto, or terminal dependency. Writes and their audit records
// commit together. Nodes are archived, never deleted; type versions are immutable.
package memorytree

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	_ "modernc.org/sqlite"
)

// Node identity survives renames and moves. Archived hides the complete subtree.
// Revision is required for writes; Position orders siblings, with ID as tie-breaker.
type Node struct {
	ID       string          `json:"id"`
	Parent   string          `json:"parent"`
	Position int             `json:"position"`
	Revision int             `json:"revision"`
	Archived bool            `json:"archived"`
	Title    string          `json:"title"`
	Summary  string          `json:"summary"`
	Type     string          `json:"type"`
	Version  int             `json:"version"`
	Data     json.RawMessage `json:"data"`
	Body     string          `json:"body"`
	// Instructions marks owner-approved operating guidance. It does not grant
	// authority to override host rules or change runtime permissions.
	Instructions bool `json:"instructions"`
}

// Type defines an immutable schema version for a node's structured Data.
type Type struct {
	Name    string          `json:"name"`
	Version int             `json:"version"`
	Schema  json.RawMessage `json:"schema"`
}

// Snapshot is a consistent view of nodes and schemas, including archived nodes.
type Snapshot struct {
	Nodes []Node `json:"nodes"`
	Types []Type `json:"types"`
	index map[string]Node
}

// Store owns one SQLite connection. Separate Store instances coordinate through
// SQLite transactions; contention fails instead of replaying a mutation.
type Store struct{ db *sql.DB }

// Open creates a private database and seeds its root once. Existing personality
// is never overwritten by startup configuration. No remote schema loaders run.
func Open(path, personality string) (*Store, error) {
	if len(personality) > 4000 {
		return nil, errors.New("initial personality exceeds 4000 bytes")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	_, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=3000;
CREATE TABLE IF NOT EXISTS nodes (id TEXT PRIMARY KEY, parent TEXT REFERENCES nodes(id), payload BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS types (name TEXT NOT NULL, version INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY(name,version));
CREATE TABLE IF NOT EXISTS identity (singleton INTEGER PRIMARY KEY CHECK(singleton=1), binding TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS journal (sequence INTEGER PRIMARY KEY, at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP, operation TEXT NOT NULL, before BLOB, after BLOB NOT NULL);`)
	if err == nil {
		tx, e := db.Begin()
		err = e
		if e == nil {
			defer tx.Rollback()
			// Existing versions are never replaced. Fresh stores have only the
			// bootstrap personality type; the agent defines use-case types.
			b, _ := json.Marshal(Type{Name: "agent", Version: 1, Schema: json.RawMessage(`{"type":"object","properties":{"purpose":{"type":"string","minLength":1}},"required":["purpose"],"additionalProperties":false}`)})
			_, err = tx.Exec("INSERT OR IGNORE INTO types VALUES(?,?,?)", "agent", 1, b)
			if err == nil {
				b, _ := json.Marshal(Node{ID: "root", Title: "Agent", Summary: "Personality and working preferences", Type: "agent", Version: 1, Revision: 1, Data: json.RawMessage(`{"purpose":"Assist users and develop useful knowledge and ways of working"}`), Body: personality, Instructions: true})
				_, err = tx.Exec("INSERT OR IGNORE INTO nodes VALUES('root',NULL,?)", b)
			}
			if err == nil {
				err = tx.Commit()
			}
		}
	}
	if err != nil {
		db.Close()
		return nil, errors.New("cannot initialize memory database")
	}
	return s, nil
}

// Close must follow cancellation and completion of all callers.
func (s *Store) Close() error { return s.db.Close() }

// BindIdentity prevents accidental sharing when the configured Chatto bot or
// server changes. Supply an opaque hash; this binding contains no credentials.
func (s *Store) BindIdentity(ctx context.Context, binding string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO identity VALUES(1,?)", binding); err != nil {
		return err
	}
	var current string
	if err = tx.QueryRowContext(ctx, "SELECT binding FROM identity WHERE singleton=1").Scan(&current); err != nil {
		return err
	}
	if current != binding {
		return rejection("memory database belongs to another bot or server; configure a separate database")
	}
	return tx.Commit()
}

func snapshot(ctx context.Context, tx *sql.Tx) (Snapshot, error) {
	var out Snapshot
	for _, table := range []string{"nodes", "types"} {
		rows, err := tx.QueryContext(ctx, "SELECT payload FROM "+table)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var b []byte
			if err = rows.Scan(&b); err != nil {
				break
			}
			if table == "nodes" {
				var n Node
				err = json.Unmarshal(b, &n)
				out.Nodes = append(out.Nodes, n)
			} else {
				var t Type
				err = json.Unmarshal(b, &t)
				out.Types = append(out.Types, t)
			}
			if err != nil {
				break
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	sort.Slice(out.Nodes, func(i, j int) bool {
		a, b := out.Nodes[i], out.Nodes[j]
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		return a.ID < b.ID
	})
	sort.Slice(out.Types, func(i, j int) bool {
		a, b := out.Types[i], out.Types[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Version < b.Version
	})
	out.index = make(map[string]Node, len(out.Nodes))
	for _, n := range out.Nodes {
		out.index[n.ID] = n
	}
	return out, nil
}

// Snapshot reads a consistent tree for browsing and progressive disclosure.
func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	return snapshot(ctx, tx)
}

// Find returns a node by stable ID, including archived nodes.
func (s Snapshot) Find(id string) (Node, bool) {
	if s.index != nil {
		n, ok := s.index[id]
		return n, ok
	}
	for _, n := range s.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return Node{}, false
}

// Visible checks inherited archival, not only the node's local flag.
func (s Snapshot) Visible(id string) bool {
	for range len(s.Nodes) + 1 {
		n, ok := s.Find(id)
		if !ok || n.Archived {
			return false
		}
		if n.ID == "root" {
			return true
		}
		id = n.Parent
	}
	return false
}

// Children returns ordered immediate children. Archived trees remain browsable
// only when the caller explicitly requests them.
func (s Snapshot) Children(id string, archived bool) []Node {
	out := []Node{}
	for _, n := range s.Nodes {
		if n.Parent == id && (archived || s.Visible(n.ID)) {
			out = append(out, n)
		}
	}
	return out
}

var typeName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)

type rejection string

func (e rejection) Error() string { return string(e) }

// IsRejection identifies safe host-authored validation feedback.
func IsRejection(err error) bool { var r rejection; return errors.As(err, &r) }

func resolve(raw json.RawMessage) (*jsonschema.Resolved, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "true" && trimmed != "false" && !strings.HasPrefix(trimmed, "{") {
		return nil, rejection("schema must be an object or boolean")
	}
	if len(raw) > 8192 {
		return nil, rejection("schema exceeds 8192 bytes")
	}
	var schema jsonschema.Schema
	if json.Unmarshal(raw, &schema) != nil {
		return nil, rejection("invalid JSON Schema")
	}
	r, err := schema.Resolve(nil)
	if err != nil {
		return nil, rejection("invalid or external schema reference")
	}
	return r, nil
}

func validate(n Node, state Snapshot) error {
	if !typeName.MatchString(n.Type) || n.Version < 1 {
		return rejection("every node requires a type ID and positive schema version")
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(n.Data, &object) != nil || object == nil {
		return rejection("every node requires a structured data object validated by its type schema")
	}
	if strings.TrimSpace(n.Title) == "" || len(n.Title) > 120 || len(n.Summary) > 240 || len(n.Body) > 12000 || len(n.Data) > 12000 {
		return rejection("node limits: title 1–120, summary 240, body/data 12000 bytes")
	}
	if n.ID == "root" && (len(n.Body) > 4000 || len(n.Data) > 2000 || n.Type != "agent") {
		return rejection("root must use agent type; body limit 4000 and data limit 2000 bytes")
	}
	for _, t := range state.Types {
		if t.Name == n.Type && t.Version == n.Version {
			r, err := resolve(t.Schema)
			if err != nil {
				return err
			}
			var data any
			if json.Unmarshal(n.Data, &data) != nil || r.Validate(data) != nil {
				return rejection("node data does not match its type schema")
			}
			return nil
		}
	}
	return rejection("unknown type version; read types first")
}

// Change describes one atomic mutation. Update replaces content; move changes
// only parent/order; migrate explicitly replaces content and its type/version.
// Archive/restore preserve all descendants and content.
type Change struct {
	Operation string `json:"operation"`
	Node      Node   `json:"node"`
	Type      Type   `json:"type"`
}

// Apply enforces authorization and invariants independently of tool exposure.
// Non-owners can create ordinary notes and update their content. Owner-only
// batches control personality, schemas, and structural or archival changes.
func (s *Store) Apply(ctx context.Context, c Change, owner bool) (any, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	state, err := snapshot(ctx, tx)
	if err != nil {
		return nil, err
	}
	var before, after any
	if c.Operation == "type" {
		if !owner {
			return nil, rejection("only owners may define types")
		}
		if !typeName.MatchString(c.Type.Name) || c.Type.Version < 1 || len(state.Types) >= 128 {
			return nil, rejection("invalid type name/version or type limit reached")
		}
		if _, err = resolve(c.Type.Schema); err != nil {
			return nil, err
		}
		latest := 0
		for _, t := range state.Types {
			if t.Name == c.Type.Name && t.Version == c.Type.Version {
				return nil, rejection("type versions are immutable; create a new version")
			}
			if t.Name == c.Type.Name && t.Version > latest {
				latest = t.Version
			}
		}
		if c.Type.Version != latest+1 {
			return nil, rejection("new types start at version 1; existing types require the next consecutive version")
		}
		after = c.Type
		b, _ := json.Marshal(after)
		_, err = tx.ExecContext(ctx, "INSERT INTO types VALUES(?,?,?)", c.Type.Name, c.Type.Version, b)
	} else {
		n := c.Node
		if c.Operation == "create" {
			if len(state.Nodes) >= 4096 {
				return nil, rejection("node limit reached (4096)")
			}
			if (n.Type == "agent" || n.Instructions) && !owner {
				return nil, rejection("agent and instruction nodes require an owner")
			}
			n.ID = rand.Text()
			n.Revision = 1
			n.Archived = false
		} else {
			old, ok := state.Find(n.ID)
			if !ok {
				return nil, rejection("node not found")
			}
			if n.Revision != old.Revision {
				return nil, rejection("stale revision; read the node again")
			}
			if (old.ID == "root" || old.Type == "agent" || old.Instructions || n.Instructions || c.Operation != "update") && !owner {
				return nil, rejection("this change requires an owner-only batch")
			}
			if c.Operation != "restore" && !state.Visible(old.ID) {
				return nil, rejection("restore the archived subtree first")
			}
			before = old
			switch c.Operation {
			case "update", "migrate":
				if c.Operation == "update" && (n.Type != old.Type || n.Version != old.Version) {
					return nil, rejection("update cannot change type or schema version; an owner must use migrate with transformed data")
				}
				if c.Operation == "migrate" && n.Type == old.Type && n.Version == old.Version {
					return nil, rejection("migrate requires a different type or schema version; use update for ordinary content changes")
				}
				n.Parent = old.Parent
				n.Position = old.Position
				n.Archived = old.Archived
			case "move":
				parent, pos := n.Parent, n.Position
				n = old
				n.Parent = parent
				n.Position = pos
			case "archive", "restore":
				n = old
				n.Archived = c.Operation == "archive"
			default:
				return nil, rejection("unknown memory operation")
			}
			n.Revision = old.Revision + 1
		}
		if n.ID == "root" {
			if n.Parent != "" || n.Archived || c.Operation == "move" {
				return nil, rejection("root cannot be moved or archived")
			}
		} else {
			if !state.Visible(n.Parent) {
				return nil, rejection("parent must be an existing active node")
			}
			for parent := n.Parent; parent != ""; {
				if parent == n.ID {
					return nil, rejection("move would create a cycle")
				}
				p, ok := state.Find(parent)
				if !ok {
					return nil, rejection("missing ancestor")
				}
				parent = p.Parent
			}
			// Bound nesting for safe recursive browsing and predictable scans.
			depth := 1
			for parent := n.Parent; parent != ""; {
				p, _ := state.Find(parent)
				parent = p.Parent
				depth++
			}
			for _, desc := range state.Nodes {
				distance := 0
				for id := desc.ID; id != ""; {
					if id == n.ID {
						if depth+distance > 64 {
							return nil, rejection("tree depth limit is 64")
						}
						break
					}
					p, _ := state.Find(id)
					id = p.Parent
					distance++
				}
			}
			if depth > 64 {
				return nil, rejection("tree depth limit is 64")
			}
			count := 0
			for _, sibling := range state.Children(n.Parent, false) {
				if sibling.ID != n.ID {
					count++
				}
			}
			if count >= 64 && !n.Archived {
				return nil, rejection("parent has 64 children; group memories before adding more")
			}
		}
		if n.Type == "agent" && !owner {
			return nil, rejection("agent type requires an owner")
		}
		if err = validate(n, state); err != nil {
			return nil, err
		}
		after = n
		b, _ := json.Marshal(n)
		var parent any = n.Parent
		if n.ID == "root" {
			parent = nil
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO nodes VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET parent=excluded.parent,payload=excluded.payload", n.ID, parent, b)
	}
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	if _, err = tx.ExecContext(ctx, "INSERT INTO journal(operation,before,after) VALUES(?,?,?)", c.Operation, b, a); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return after, nil
}

// History returns one committed change, newest first. Before and After
// retain full content so an operator can recover any prior document revision.
func (s *Store) History(ctx context.Context, offset int) ([]json.RawMessage, error) {
	if offset < 0 {
		return nil, rejection("offset must not be negative")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT sequence,at,operation,before,after FROM journal ORDER BY sequence DESC LIMIT 1 OFFSET ?", offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var seq int
		var at, op string
		var before, after json.RawMessage
		if err = rows.Scan(&seq, &at, &op, &before, &after); err != nil {
			return nil, err
		}
		b, _ := json.Marshal(map[string]any{"sequence": seq, "at": at, "operation": op, "before": before, "after": after})
		out = append(out, b)
	}
	return out, rows.Err()
}

// Search scans a subtree, with optional type and top-level data-field equality.
// It returns summaries only; callers read matching nodes for their full content.
func (s Snapshot) Search(root, query, kind, field string, value json.RawMessage, archived bool) []Node {
	out := []Node{}
	for _, n := range s.Nodes {
		if !archived && !s.Visible(n.ID) {
			continue
		}
		if kind != "" && n.Type != kind {
			continue
		}
		inside := false
		for id := n.ID; id != ""; {
			if id == root {
				inside = true
				break
			}
			p, ok := s.Find(id)
			if !ok {
				break
			}
			id = p.Parent
		}
		if !inside {
			continue
		}
		if !strings.Contains(strings.ToLower(n.Title+" "+n.Summary+" "+n.Body+" "+string(n.Data)), strings.ToLower(query)) {
			continue
		}
		if field != "" {
			var data map[string]json.RawMessage
			_ = json.Unmarshal(n.Data, &data)
			var a, b any
			if json.Unmarshal(data[field], &a) != nil || json.Unmarshal(value, &b) != nil {
				continue
			}
			aa, _ := json.Marshal(a)
			bb, _ := json.Marshal(b)
			if string(aa) != string(bb) {
				continue
			}
		}
		n.Body = ""
		n.Data = nil
		out = append(out, n)
	}
	return out
}
