package bot

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

// inboxState commits the cursor and accepted work together. Started entries are
// never automatically retried: their external delivery may already have happened.
// Recent completed IDs suppress duplicates across reconnects and process restarts.
type inboxState struct {
	Version int
	Cursor  string
	Entries []inboxEntry
	Done    []string
}
type inboxEntry struct {
	Event   chatto.MessageEvent
	Started bool
}

func eventID(e chatto.MessageEvent) string {
	if e.RecoveryGap {
		return "recovery-gap"
	}
	if e.ID != "" {
		return e.ID
	}
	return e.Message.ID
}

func (s *Store) loadInbox() error {
	s.inbox.Version = 1
	data, err := os.ReadFile(filepath.Join(s.dir, ".realtime"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || json.Unmarshal(data, &s.inbox) != nil || s.inbox.Version != 1 {
		return errStore
	}
	ids := map[string]bool{}
	for _, e := range s.inbox.Entries {
		id := eventID(e.Event)
		if id == "" || ids[id] {
			return errStore
		}
		ids[id] = true
	}
	return nil
}

// updateInbox publishes state only after a synced atomic replacement. On failure
// the source stops with the old cursor, so replay can recover unaccepted work.
func (s *Store) updateInbox(change func(*inboxState)) error {
	if s == nil {
		return nil
	}
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	v := s.inbox
	v.Entries = append([]inboxEntry(nil), v.Entries...)
	v.Done = append([]string(nil), v.Done...)
	change(&v)
	if len(v.Entries) > 4096 {
		return errors.New("durable realtime inbox is full")
	}
	if len(v.Done) > 4096 {
		v.Done = v.Done[len(v.Done)-4096:]
	}
	data, err := json.Marshal(v)
	if err != nil || len(data) > 16<<20 {
		return errStore
	}
	f, err := os.CreateTemp(s.dir, ".inbox-*")
	if err != nil {
		return errStore
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errStore
	}
	if os.Rename(f.Name(), filepath.Join(s.dir, ".realtime")) != nil {
		return errStore
	}
	d, err := os.Open(s.dir)
	if err != nil {
		return errStore
	}
	err = d.Sync()
	d.Close()
	if err != nil {
		return errStore
	}
	s.inbox = v
	return nil
}

func (s *Store) checkpoint(cursor string, event *chatto.MessageEvent) error {
	return s.updateInbox(func(v *inboxState) {
		v.Cursor = cursor
		if event == nil {
			return
		}
		id := eventID(*event)
		for _, e := range v.Entries {
			if eventID(e.Event) == id {
				return
			}
		}
		if !event.RecoveryGap {
			for _, done := range v.Done {
				if done == id {
					return
				}
			}
		}
		retained := *event
		retained.Message = messageForInbox(event.Message)
		v.Entries = append(v.Entries, inboxEntry{Event: retained})
	})
}

func (s *Store) pendingInbox() (string, []chatto.MessageEvent, int) {
	if s == nil {
		return "", nil, 0
	}
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	var events []chatto.MessageEvent
	interrupted := 0
	for _, e := range s.inbox.Entries {
		if e.Started {
			interrupted++
			continue
		}
		events = append(events, e.Event)
	}
	return s.inbox.Cursor, events, interrupted
}

func (s *Store) inboxPending(event chatto.MessageEvent) bool {
	if s == nil {
		return true
	}
	s.inboxMu.Lock()
	defer s.inboxMu.Unlock()
	for _, e := range s.inbox.Entries {
		if eventID(e.Event) == eventID(event) {
			return !e.Started
		}
	}
	return false
}

func (s *Store) claim(events []chatto.MessageEvent) error {
	return s.updateInbox(func(v *inboxState) {
		for i := range v.Entries {
			for _, e := range events {
				if eventID(e) == eventID(v.Entries[i].Event) {
					v.Entries[i].Started = true
				}
			}
		}
	})
}

func (s *Store) finish(events []chatto.MessageEvent) error {
	return s.updateInbox(func(v *inboxState) {
		kept := v.Entries[:0]
		for _, entry := range v.Entries {
			remove := false
			for _, e := range events {
				if eventID(e) == eventID(entry.Event) {
					remove = true
					break
				}
			}
			if remove {
				v.Done = append(v.Done, eventID(entry.Event))
			} else {
				kept = append(kept, entry)
			}
		}
		v.Entries = kept
	})
}
