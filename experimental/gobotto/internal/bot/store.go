package bot

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

var errStore = errors.New("session storage failed; check the session directory")

// Store owns private, atomic JSON snapshots in one server/bot/model namespace.
// Its process lock prevents two bot processes from overwriting the same history.
type Store struct {
	dir     string
	lock    *os.File
	inboxMu sync.Mutex
	inbox   inboxState
}

type totals struct {
	Input, Cached, Output         int64
	Cost                          float64
	Known, CostKnown, CostMissing bool
}

type savedConversation struct {
	Version              int
	ID                   uint64
	HumanID              string
	Title                string
	TitleAttempted       bool
	Room, Root, Provider string
	Started              time.Time
	Turns                int
	History              *agent.Snapshot
	Seen                 map[string]bool
	Current              map[string]string
	Usage                totals
	Interrupted          bool
	Blocked              bool
	Records              []inspection.Turn
	Logs                 []inspection.Log
}

// OpenStore isolates credentials by identity, not by secret token. The namespace
// includes the model because retained provider metadata is model-specific.
func OpenStore(base, server, viewer, model string) (*Store, error) {
	key, _ := json.Marshal([]string{server, viewer, model})
	dir := filepath.Join(base, fmt.Sprintf("%x", sha256.Sum256(key)))
	if os.MkdirAll(dir, 0700) != nil {
		return nil, errStore
	}
	if os.Chmod(dir, 0700) != nil {
		return nil, errStore
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errStore
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, errors.New("session directory is already in use")
	}
	s := &Store{dir: dir, lock: f}
	if err := s.loadInbox(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the process lock after all session workers have stopped.
func (s *Store) Close() error { return s.lock.Close() }

func (s *Store) path(c *conversation) string {
	return filepath.Join(s.dir, c.humanID+".json")
}

func (s *Store) legacyPath(c *conversation) string {
	key, _ := json.Marshal([]string{c.key.room, c.key.root})
	return filepath.Join(s.dir, fmt.Sprintf("%x.json", sha256.Sum256(key)))
}

// save is called only by the conversation's current owner. Pending realtime
// messages live in the separate atomic inbox; started batches are not retried.
func (s *Store) save(c *conversation, interrupted bool) error {
	if s == nil {
		return nil
	}
	if c.humanID == "" {
		c.humanID = humanID(c.provider)
	}
	if !validHumanID.MatchString(c.humanID) {
		return errStore
	}
	// Refuse to overwrite a different conversation, even if an ID collides.
	if data, err := os.ReadFile(s.path(c)); err == nil {
		var previous savedConversation
		if json.Unmarshal(data, &previous) != nil || previous.Room != c.key.room || previous.Root != c.key.root {
			return errStore
		}
	} else if !os.IsNotExist(err) {
		return errStore
	}
	v := savedConversation{Version: 1, ID: c.id, Room: c.key.room, Root: c.key.root, Provider: c.provider, Started: c.started, Turns: c.turns, Seen: c.seen, Usage: c.usage, Interrupted: interrupted}
	v.HumanID = c.humanID
	v.Title = c.title
	v.TitleAttempted = c.titleAttempted
	v.Current = c.current
	v.Blocked = c.blocked
	v.Records = c.records
	v.Logs = c.storedLogs()
	if c.model != nil {
		state := c.model.Snapshot()
		v.History = &state
	}
	f, err := os.CreateTemp(s.dir, ".snapshot-*")
	if err != nil {
		return errStore
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(v)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errStore
	}
	if os.Rename(f.Name(), s.path(c)) != nil {
		return errStore
	}
	d, err := os.Open(s.dir)
	if err != nil {
		return errStore
	}
	defer d.Close()
	if d.Sync() != nil {
		return errStore
	}
	return nil
}

func (s *Store) remove(c *conversation) error {
	if s == nil {
		return nil
	}
	if err := os.Remove(s.path(c)); err != nil && !os.IsNotExist(err) {
		return errStore
	}
	return nil
}

func (s *Store) load() ([]*conversation, error) {
	if s == nil {
		return nil, nil
	}
	paths, err := filepath.Glob(filepath.Join(s.dir, "*.json"))
	if err != nil {
		return nil, errStore
	}
	var result []*conversation
	ids := map[uint64]bool{}
	names := map[string]bool{}
	legacy := map[*conversation]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errStore
		}
		var v savedConversation
		if json.Unmarshal(data, &v) != nil || v.Version != 1 || v.ID == 0 || ids[v.ID] || v.Room == "" || v.Root == "" || v.Provider == "" {
			return nil, errors.New("invalid session snapshot; restore or move the session directory before restarting")
		}
		ids[v.ID] = true
		c := &conversation{id: v.ID, key: threadKey{v.Room, v.Root}, provider: v.Provider, started: v.Started, used: v.Started, turns: v.Turns, seen: v.Seen, usage: v.Usage, resumed: true, interrupted: v.Interrupted}
		c.humanID = v.HumanID
		c.title = agent.CleanTitle(v.Title)
		c.titleAttempted = v.TitleAttempted
		c.records = inspection.Retain(v.Records)
		c.logs = v.Logs
		if len(c.logs) > 200 {
			c.logs = c.logs[len(c.logs)-200:]
		}
		for i := range c.records {
			t := &c.records[i]
			if t.State == "running" {
				t.State = "interrupted"
				t.Failure = "Process stopped; no automatic retry"
				if t.Response != "" || t.Emoji != "" {
					t.State = "uncertain"
					t.Failure = "Delivery may have occurred before restart; no automatic retry"
				}
			}
			for j := range t.Steps {
				if t.Steps[j].State == "running" {
					t.Steps[j].State = "interrupted"
				}
			}
		}
		if c.humanID == "" {
			c.humanID = humanID(c.provider)
		}
		if !validHumanID.MatchString(c.humanID) || names[c.humanID] {
			return nil, errStore
		}
		names[c.humanID] = true
		c.current = v.Current
		c.blocked = v.Blocked
		if s.path(c) != path {
			if v.HumanID != "" || s.legacyPath(c) != path {
				return nil, errStore
			}
			legacy[c] = path
		}
		if c.seen == nil {
			c.seen = map[string]bool{}
		}
		if v.History != nil {
			c.model = agent.Restore(*v.History)
			if v.Interrupted {
				c.model.RecordDelivery(errors.New("interrupted"))
			}
		}
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	if len(result) > sessionLimit {
		return nil, errors.New("too many stored sessions")
	}
	// Rename only after all snapshots validate. The old payload can still be
	// loaded at its new name if the process exits before its next checkpoint.
	for c, path := range legacy {
		if os.Rename(path, s.path(c)) != nil {
			return nil, errStore
		}
	}
	return result, nil
}

var validHumanID = regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9a-f]{12}$`)

// humanID derives a readable label from the opaque random provider ID. Keep the
// stored result across provider resets so operators can track one conversation.
func humanID(seed string) string {
	adjectives := [...]string{"calm", "bright", "gentle", "quiet", "swift", "brave", "merry", "keen", "bold", "warm", "cool", "fair", "kind", "wise", "lively", "sunny"}
	animals := [...]string{"otter", "fox", "owl", "lynx", "panda", "robin", "badger", "falcon", "koala", "finch", "heron", "seal", "tiger", "raven", "wren", "yak"}
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("%s-%s-%x", adjectives[sum[0]>>4], animals[sum[0]&15], sum[1:7])
}

func fingerprint(m chatto.Message) string {
	data, _ := json.Marshal(m)
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
