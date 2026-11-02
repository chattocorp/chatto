package bot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
)

// sessionIndex contains no transcript, message bodies, or child ledgers. It is
// the first JSON field in a snapshot, so startup reads a bounded prefix.
type sessionIndex struct {
	Epoch                                string
	Version                              int
	ID                                   uint64
	HumanID, Title, Room, Root, Provider string
	Started, Used                        time.Time
	Turns                                int
	Usage                                totals
	Interrupted, Blocked                 bool
}

// trimRuntime bounds idle hydrated conversations. Active work, queued input,
// and unacknowledged child results pin a conversation until safely checkpointed.
func (s *Store) trimRuntime(sessions map[threadKey]*conversation, active *conversation) error {
	if s == nil {
		return nil
	}
	var candidates []*conversation
	count := 0
	for _, c := range sessions {
		if c.unloaded {
			continue
		}
		count++
		if c != active && len(c.pending) == 0 && len(c.childReady) == 0 && c.deadline.IsZero() && c.jobs.CanEvict() {
			candidates = append(candidates, c)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].used.Before(candidates[j].used) })
	for _, c := range candidates {
		if count <= 8 {
			break
		}
		if err := s.save(c, false); err != nil {
			return err
		}
		c.jobs.Close()
		sessions[c.key] = indexFor(c, false).stub()
		count--
	}
	return nil
}

func indexFor(c *conversation, interrupted bool) *sessionIndex {
	return &sessionIndex{Version: 1, ID: c.id, HumanID: c.humanID, Title: c.title, Room: c.key.room, Root: c.key.root, Provider: c.provider, Started: c.started, Used: c.used, Turns: c.turns, Usage: c.usage, Interrupted: interrupted, Blocked: c.blocked}
}

func (v *sessionIndex) stub() *conversation {
	return &conversation{unloaded: true, id: v.ID, humanID: v.HumanID, title: v.Title, key: threadKey{v.Room, v.Root}, provider: v.Provider, started: v.Started, used: v.Used, turns: v.Turns, usage: v.Usage, interrupted: v.Interrupted, blocked: v.Blocked, resumed: true}
}

// readIndex stops before inspection and transcript data. LimitReader rejects
// oversized/corrupt index prefixes without buffering an unbounded file.
func readIndex(r io.Reader) (*sessionIndex, error) {
	d := json.NewDecoder(io.LimitReader(r, 64<<10))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errStore
	}
	field, err := d.Token()
	if err != nil || field != "Index" {
		return nil, errors.New("session index missing; use a new session directory (legacy files are unchanged)")
	}
	var v *sessionIndex
	if d.Decode(&v) != nil || v == nil || v.Version != 1 || v.ID == 0 || !validHumanID.MatchString(v.HumanID) || v.Room == "" || v.Root == "" || v.Provider == "" {
		return nil, errStore
	}
	return v, nil
}

func (s *Store) readStub(path string) (*conversation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errStore
	}
	defer f.Close()
	v, err := readIndex(f)
	if err != nil {
		return nil, err
	}
	c := v.stub()
	if s.path(c) != path {
		return nil, errStore
	}
	return c, nil
}

func (s *Store) loadIndex(ctx context.Context) ([]*conversation, error) {
	if s == nil {
		return nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, errStore
	}
	var result []*conversation
	ids := map[uint64]bool{}
	threads := map[threadKey]bool{}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c, err := s.readStub(path)
		if err != nil {
			return nil, err
		}
		if ids[c.id] || threads[c.key] {
			return nil, errStore
		}
		ids[c.id] = true
		threads[c.key] = true
		result = append(result, c)
	}
	if len(result) > sessionLimit {
		return nil, errors.New("too many stored sessions")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result, nil
}

func (s *Store) hydrate(c *conversation) (*conversation, error) {
	if !c.unloaded {
		return c, nil
	}
	loaded, err := s.loadPaths([]string{s.path(c)})
	if err != nil {
		return nil, err
	}
	if len(loaded) != 1 || loaded[0].id != c.id || loaded[0].key != c.key {
		return nil, errStore
	}
	loaded[0].used = c.used
	return loaded[0], nil
}

// inspectionPage reads only the snapshot prefix. Atomic file replacement keeps
// every page internally consistent while a worker checkpoints a newer version.
// Transcripts and child ledgers are never decoded by UI readers.
func (s *Store) inspectionPage(ctx context.Context, human string, before int) (inspection.Page, error) {
	var page inspection.Page
	if s == nil || !validHumanID.MatchString(human) {
		return page, errStore
	}
	s.readerMu.Lock()
	if s.closed {
		s.readerMu.Unlock()
		return page, errStore
	}
	s.readers.Add(1)
	s.readerMu.Unlock()
	defer s.readers.Done()
	if err := ctx.Err(); err != nil {
		return page, err
	}
	f, err := os.Open(filepath.Join(s.dir, human+".json"))
	if err != nil {
		return page, errStore
	}
	defer f.Close()
	d := json.NewDecoder(f)
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return page, errStore
	}
	field, err := d.Token()
	if err != nil || field != "Index" {
		return page, errStore
	}
	var index sessionIndex
	if d.Decode(&index) != nil || index.HumanID != human {
		return page, errStore
	}
	field, err = d.Token()
	if err != nil || field != "Records" {
		return page, errStore
	}
	start, err = d.Token()
	if err != nil {
		return page, errStore
	}
	if start == json.Delim('[') {
		for d.More() {
			if err := ctx.Err(); err != nil {
				return page, err
			}
			var t inspection.Turn
			if d.Decode(&t) != nil {
				return page, errStore
			}
			if before > 0 && t.Number >= before {
				continue
			}
			// Retain a bounded window, even if an on-disk file has more turns.
			page.Turns = append(page.Turns, t)
			if len(page.Turns) > 20 {
				page.Turns = page.Turns[1:]
				page.Before = page.Turns[0].Number
			}
		}
		if _, err := d.Token(); err != nil {
			return page, errStore
		}
	} else if start != nil {
		return page, errStore
	}
	field, err = d.Token()
	if err != nil || field != "Logs" {
		return page, errStore
	}
	if before == 0 && d.Decode(&page.Logs) != nil {
		return page, errStore
	}
	if index.Epoch != s.epoch {
		for i := range page.Turns {
			t := &page.Turns[i]
			if t.State == "running" {
				t.State = "interrupted"
				if t.Response != "" || t.Emoji != "" {
					t.State = "uncertain"
				}
				t.Failure = "Process stopped; no automatic retry"
			}
			for j := range t.Steps {
				if t.Steps[j].State == "running" {
					t.Steps[j].State = "interrupted"
				}
			}
		}
	}
	return page, ctx.Err()
}
