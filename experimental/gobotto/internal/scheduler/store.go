// Package scheduler owns owner-authorized recurring tasks and durable claims.
// Definitions are means documents; the private ledger is the execution authority.
package scheduler

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxSchedules = 100

var errStorage = errors.New("schedule storage failed; inspect the schedules directory before retrying")

// Definition is the saved task. Destination IDs and owner are host supplied;
// the owner selects room or thread delivery when creating the schedule.
type Definition struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Task     string `json:"task"`
	Schedule string `json:"schedule"`
	// Destination selects a top-level room post or a reply in the saved thread.
	Destination Destination `json:"destination"`
	CreatedFrom Origin      `json:"created_from"`
	Owner       string      `json:"created_by"`
	// AuthorizedBy is the configured admin who approved this revision.
	AuthorizedBy string `json:"authorized_by"`
	Enabled      bool   `json:"enabled"`
	Revision     int64  `json:"revision"`
}

// documentMetadata omits task prose from frontmatter. The shadow field detects
// obsolete documents that still put task text in metadata instead of the body.
type documentMetadata struct {
	Definition
	Task *string `json:"task,omitempty"`
}

// Destination is a host-resolved delivery target, separate from creation context.
type Destination struct {
	Kind   string `json:"kind"`
	Room   string `json:"room_id"`
	Thread string `json:"thread_id,omitempty"`
}

// Origin records where a schedule was created; it does not restrict management.
type Origin struct {
	Room   string `json:"room_id"`
	Thread string `json:"thread_id"`
}

// Entry is separate execution state. Hash and Revision bind authorization to
// structured definition data, so manual edits cannot change approved work.
type Entry struct {
	Hash     string    `json:"hash"`
	Next     time.Time `json:"next"`
	Started  time.Time `json:"started,omitempty"`
	Status   string    `json:"status"`
	Revision int64     `json:"revision"`
	// RunningRevision prevents overlap even if a claimed revision is paused.
	RunningRevision int64 `json:"running_revision,omitempty"`
}

type ledger struct {
	Version int              `json:"version"`
	Scope   string           `json:"scope"`
	Entries map[string]Entry `json:"entries"`
}

// Manager serializes management and claims and holds an exclusive directory lock.
// A failed durable write poisons this instance: no further effects are allowed.
type Manager struct {
	mu     sync.Mutex
	dir    string
	lock   *os.File
	state  ledger
	failed bool
	now    func() time.Time
}

// Open binds this brain to one server/bot scope, regardless of model selection.
// Overdue runs are skipped at startup; running claims become interrupted.
func Open(dir, scope string) (*Manager, error) {
	if scope == "" {
		return nil, errors.New("scheduler requires a server/bot scope")
	}
	scope = digest([]byte(scope))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errStorage
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil, errStorage
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errStorage
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, errors.New("schedules directory is already in use")
	}
	m := &Manager{dir: dir, lock: f, now: time.Now, state: ledger{Version: 1, Scope: scope, Entries: map[string]Entry{}}}
	ok := false
	defer func() {
		if !ok {
			m.Close()
		}
	}()
	definition := scheduleBrainDefinition()
	path := filepath.Join(dir, "MEANS.md")
	data, err := os.ReadFile(path)
	refreshDefinition := err == nil && string(data) != string(definition)
	if os.IsNotExist(err) {
		// Do not adopt an existing collection or unregistered documents.
		entries, e := os.ReadDir(dir)
		if e != nil {
			return nil, errStorage
		}
		for _, entry := range entries {
			if entry.Name() != ".lock" {
				return nil, errors.New("schedules directory is not an empty reserved brain")
			}
		}
		if atomicWrite(path, definition) != nil {
			return nil, errStorage
		}
	} else if err != nil {
		return nil, errStorage
	}
	data, err = os.ReadFile(filepath.Join(dir, ".runtime.json"))
	if err == nil {
		if json.Unmarshal(data, &m.state) != nil || m.state.Version != 1 || m.state.Scope != scope || m.state.Entries == nil || len(m.state.Entries) > maxSchedules {
			return nil, errors.New("invalid schedule ledger or different server/bot scope")
		}
	} else if !os.IsNotExist(err) {
		return nil, errStorage
	}
	now := m.now()
	for id, e := range m.state.Entries {
		d, err := m.read(id, e)
		if err != nil {
			e.Status = "invalid"
			m.state.Entries[id] = e
			continue
		}
		if e.RunningRevision != 0 {
			e.RunningRevision = 0
			if d.Enabled {
				e.Status = "interrupted"
			}
		}
		if d.Enabled && !e.Next.After(now) {
			e.Next = d.next(now)
			if e.Next.IsZero() {
				e.Status = "invalid"
			}
		}
		m.state.Entries[id] = e
	}
	if err := m.save(m.state.Entries); err != nil {
		return nil, err
	}
	// Collection metadata is code-owned, not an execution grant. Refresh it only
	// after checking the ledger scope; task bytes and authorization stay unchanged.
	if refreshDefinition && atomicWrite(path, definition) != nil {
		return nil, errStorage
	}
	ok = true
	return m, nil
}

// Authorized rechecks a claimed definition immediately before execution or
// delivery. Deleting a claimed task revokes it even while it waits for the worker.
func (m *Manager) Authorized(d Definition) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failed {
		return false
	}
	e, ok := m.state.Entries[d.ID]
	if !ok || e.RunningRevision != d.Revision || e.Revision != d.Revision || !d.Enabled {
		return false
	}
	current, err := m.read(d.ID, e)
	return err == nil && current == d
}

// Close releases the process lock after all users have stopped.
func (m *Manager) Close() {
	if m != nil && m.lock != nil {
		_ = syscall.Flock(int(m.lock.Fd()), syscall.LOCK_UN)
		_ = m.lock.Close()
		m.lock = nil
	}
}

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func validID(id string) bool {
	if len(id) != 12 && len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func (m *Manager) read(id string, e Entry) (Definition, error) {
	var d Definition
	if !validID(id) {
		return d, errStorage
	}
	p := filepath.Join(m.dir, id+".md")
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<10 {
		return d, errStorage
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return d, errStorage
	}
	var envelope struct {
		Data documentMetadata `json:"data"`
	}
	if len(b) < 9 || string(b[:4]) != "---\n" {
		return d, errStorage
	}
	header, body, ok := bytes.Cut(b[4:], []byte("\n---\n"))
	if !ok || json.Unmarshal(header, &envelope) != nil || envelope.Data.Task != nil {
		return d, errStorage
	}
	d = envelope.Data.Definition
	d.Task = strings.TrimSpace(string(body))
	if d.ID != id || d.CreatedFrom.Room == "" || d.CreatedFrom.Thread == "" || d.Owner == "" || d.AuthorizedBy == "" || d.Title == "" || d.Task == "" || d.Revision < 1 || d.Revision != e.Revision || definitionHash(d) != e.Hash {
		return d, errStorage
	}
	if _, _, err := parseSchedule(d.Schedule); err != nil {
		return d, errStorage
	}
	if d.Destination.Room == "" || (d.Destination.Kind != "room" && d.Destination.Kind != "thread") || (d.Destination.Kind == "room" && d.Destination.Thread != "") || (d.Destination.Kind == "thread" && d.Destination.Thread == "") {
		return d, errStorage
	}
	return d, nil
}

func (m *Manager) save(entries map[string]Entry) error {
	if m.failed {
		return errStorage
	}
	v := m.state
	v.Entries = entries
	b, err := json.Marshal(v)
	if err != nil || atomicWrite(filepath.Join(m.dir, ".runtime.json"), b) != nil {
		m.failed = true
		return errStorage
	}
	m.state = v
	return nil
}

func clone(entries map[string]Entry) map[string]Entry {
	out := make(map[string]Entry, len(entries))
	for id, e := range entries {
		out[id] = e
	}
	return out
}

// atomicWrite syncs the file and directory before acknowledging state changes.
func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".schedule-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// Claim durably starts at most one due occurrence. Only current configured
// owners authorize execution. The next due time is advanced before any effect.
func (m *Manager) Claim(now time.Time, owners []string) (*Definition, error) {
	if m == nil {
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failed {
		return nil, errStorage
	}
	ids := make([]string, 0, len(m.state.Entries))
	for id := range m.state.Entries {
		ids = append(ids, id)
	}
	// Oldest due work wins. ID ordering alone can starve a task behind a
	// frequently due schedule when execution takes longer than its interval.
	slices.SortFunc(ids, func(a, b string) int {
		if order := m.state.Entries[a].Next.Compare(m.state.Entries[b].Next); order != 0 {
			return order
		}
		return cmp.Compare(a, b)
	})
	for _, id := range ids {
		e := m.state.Entries[id]
		if e.RunningRevision != 0 || e.Status == "paused" || e.Status == "invalid" || e.Status == "revoked" || e.Next.After(now) {
			continue
		}
		d, err := m.read(id, e)
		if err != nil {
			e.Status = "invalid"
		} else if !d.Enabled {
			e.Status = "paused"
		} else if !slices.Contains(owners, d.AuthorizedBy) {
			e.Status = "revoked"
		} else {
			e.Status = "running"
			e.RunningRevision = d.Revision
			e.Started = now
			e.Next = d.next(now)
			if e.Next.IsZero() {
				e.Status = "invalid"
				e.RunningRevision = 0
			}
		}
		entries := clone(m.state.Entries)
		entries[id] = e
		if err := m.save(entries); err != nil {
			return nil, err
		}
		if e.Status == "running" {
			return &d, nil
		}
	}
	return nil, nil
}

// Finish records a terminal result. It never retries an effect. Advancing an
// overdue next time skips intervals missed during a long execution.
func (m *Manager) Finish(claim Definition, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := claim.ID
	e, ok := m.state.Entries[id]
	if !ok {
		return nil
	}
	if e.RunningRevision != claim.Revision {
		return nil
	}
	e.RunningRevision = 0
	if e.Revision != claim.Revision {
		entries := clone(m.state.Entries)
		entries[id] = e
		return m.save(entries)
	}
	d, err := m.read(id, e)
	if err != nil {
		e.Status = "invalid"
	} else {
		e.Status = status
		now := m.now()
		if !e.Next.After(now) {
			e.Next = d.next(now)
			if e.Next.IsZero() {
				e.Status = "invalid"
			}
		}
	}
	entries := clone(m.state.Entries)
	entries[id] = e
	return m.save(entries)
}

func newID() string { var id [6]byte; _, _ = rand.Read(id[:]); return fmt.Sprintf("%x", id) }

// definitionHash binds approval to structured task data, not Markdown formatting.
func definitionHash(d Definition) string { b, _ := json.Marshal(d); return digest(b) }

// writeDefinition writes task data before granting its revision in the ledger.
// A partial write fails closed; it cannot authorize a new task after restart.
func (m *Manager) writeDefinition(d Definition, e Entry) error {
	b, _ := json.Marshal(map[string]any{"id": d.ID, "title": d.Title, "data": documentMetadata{Definition: d}})
	if atomicWrite(filepath.Join(m.dir, d.ID+".md"), []byte("---\n"+string(b)+"\n---\n\n"+d.Task+"\n")) != nil {
		m.failed = true
		return errStorage
	}
	e.Hash, e.Revision = definitionHash(d), d.Revision
	entries := clone(m.state.Entries)
	entries[d.ID] = e
	return m.save(entries)
}
