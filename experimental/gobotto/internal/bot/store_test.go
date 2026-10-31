package bot

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestLegacySnapshotGetsStableHumanID(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "original-provider"}
	old := savedConversation{Version: 1, ID: c.id, Room: c.key.room, Root: c.key.root, Provider: c.provider}
	data, _ := json.Marshal(old)
	if err := os.WriteFile(s.legacyPath(c), data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.load()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("migration: %v", err)
	}
	r := loaded[0]
	id := r.humanID
	if !validHumanID.MatchString(id) || filepath.Base(s.path(r)) != id+".json" {
		t.Fatal("missing readable ID")
	}
	if _, err := os.Stat(s.legacyPath(c)); !os.IsNotExist(err) {
		t.Fatal("legacy file remains")
	}
	// A crash after rename but before checkpoint must still load the same ID.
	again, err := s.load()
	if err != nil || again[0].humanID != id {
		t.Fatalf("rename recovery: %v", err)
	}
	r.provider = "replacement-provider"
	if err := s.save(r, false); err != nil {
		t.Fatal(err)
	}
	again, err = s.load()
	if err != nil || again[0].humanID != id {
		t.Fatalf("ID changed after context reset: %v", err)
	}
	data, _ = os.ReadFile(s.path(r))
	var stored savedConversation
	if json.Unmarshal(data, &stored) != nil || stored.HumanID != id {
		t.Fatal("ID not persisted")
	}
	other := &conversation{id: 2, key: threadKey{"room", "different-root"}, provider: "other", humanID: id}
	if err := s.save(other, false); err == nil {
		t.Fatal("ID collision overwrote history")
	}
}

type unavailableThread struct{ *fakeClient }

func (f unavailableThread) Thread(context.Context, string, string, string) (chatto.Page, error) {
	return chatto.Page{}, errors.New("access denied")
}

func TestRestoredContextRechecksAccessAndOfflineChanges(t *testing.T) {
	for _, denied := range []bool{false, true} {
		f := newFake()
		var client Client = f
		if denied {
			client = unavailableThread{f}
		}
		old := chatto.Message{ID: "first", RoomID: "room", ThreadRootEventID: "root", ActorID: "human", Body: "removed-private-content"}
		c := &conversation{model: agent.NewSession(old.Body), key: threadKey{"room", "root"}, resumed: true, current: map[string]string{"first": fingerprint(old)}}
		calls := 0
		c.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
			calls++
			data, _ := json.Marshal(messages)
			if strings.Contains(string(data), old.Body) {
				t.Fatal("offline edit leaked stale context")
			}
			return answer(), nil
		}
		err := respond(context.Background(), client, c, []chatto.MessageEvent{input("new", "followup", true)}, nil)
		if denied {
			if err == nil || calls != 0 {
				t.Fatal("used stored context without access")
			}
		} else if err != nil || calls != 1 {
			t.Fatalf("refresh failed: %v", err)
		}
	}
}

func TestStorageFailurePreventsDelivery(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f := newFake()
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "stable", store: s}
	c.complete = func(context.Context, []agent.Message) (agent.Message, error) {
		// Fail the checkpoint immediately before the proposed reply is sent.
		s.dir += "/missing"
		return answer(), nil
	}
	if err := respond(context.Background(), f, c, []chatto.MessageEvent{input("first", "initial-private-text", true)}, nil); !errors.Is(err, errStore) {
		t.Fatalf("got %v", err)
	}
	select {
	case <-f.posts:
		t.Fatal("sent reply without checkpoint")
	default:
	}
}

func TestStoreRestoresConversationAndUsage(t *testing.T) {
	base := t.TempDir()
	s, err := OpenStore(base, "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	f := newFake()
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "stable", started: time.Now(), store: s, turns: 1, usage: totals{Input: 100, Output: 20, Cost: 0.001, Known: true, CostKnown: true}}
	c.complete = func(context.Context, []agent.Message) (agent.Message, error) { return answer(), nil }
	if err := respond(context.Background(), f, c, []chatto.MessageEvent{input("first", "initial-private-text", true)}, nil); err != nil {
		t.Fatal(err)
	}
	<-f.posts
	c.title = "Cache performance"
	c.titleAttempted = true
	if err := s.save(c, false); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(c.model.Snapshot())
	info, err := os.Stat(s.path(c))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("snapshot is not owner-only")
	}
	if _, err := OpenStore(base, "server", "bot", "model"); err == nil {
		t.Fatal("second writer acquired lock")
	}
	s.Close()
	s, err = OpenStore(base, "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	loaded, err := s.load()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("load: %v", err)
	}
	r := loaded[0]
	if r.title != "Cache performance" || !r.titleAttempted {
		t.Fatal("title did not survive restart")
	}
	after, _ := json.Marshal(r.model.Snapshot())
	if string(before) != string(after) || r.provider != "stable" || r.usage.Cost != 0.001 || r.usage.Input != 100 || r.turns != 1 {
		t.Fatal("restart lost state")
	}
	r.store = s
	r.complete = func(_ context.Context, messages []agent.Message) (agent.Message, error) {
		prior := c.model.Snapshot().Messages
		for i := range prior {
			a, _ := json.Marshal(prior[i])
			b, _ := json.Marshal(messages[i])
			if string(a) != string(b) {
				t.Fatal("restart changed retained prefix")
			}
		}
		return answer(), nil
	}
	if err := respond(context.Background(), f, r, []chatto.MessageEvent{input("new", "followup", true)}, nil); err != nil {
		t.Fatal(err)
	}
	<-f.posts
	if f.readCount() != 2 {
		t.Fatal("restored session did not reauthorize and refresh")
	}
	other, err := OpenStore(base, "other-server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	isolated, err := other.load()
	if err != nil || len(isolated) != 0 {
		t.Fatal("server histories mixed")
	}
}

func TestInterruptedReactionIsClosedWithoutReplay(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "stable", model: agent.NewSession("context")}
	_, err = c.model.Turn(context.Background(), func(context.Context, []agent.Message) (agent.Message, error) {
		var m agent.Message
		json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"reaction","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"eyes\"}"}}]}`), &m)
		return m, nil
	}, nil, "ping")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.save(c, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	state := loaded[0].model.Snapshot()
	last := state.Messages[len(state.Messages)-1]
	if state.PendingTool != "" || string(last["tool_call_id"]) != `"reaction"` || !strings.Contains(string(last["content"]), "uncertain") {
		t.Fatal("unclosed interrupted tool exchange")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFake()
	err = Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
		return func(context.Context, []agent.Message) (agent.Message, error) {
			t.Error("replayed model turn")
			return answer(), nil
		}
	}, func(e tui.Event) {
		if e.Kind == tui.Failed {
			cancel()
		}
	}, Options{Store: s})
	if err != context.Canceled {
		t.Fatal(err)
	}
	select {
	case <-f.posts:
		t.Fatal("replayed delivery")
	default:
	}
}

func TestCorruptSnapshotFailsWithoutOverwriting(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "stable"}
	if err := s.save(c, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path(c), []byte(`{"Version":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.load(); err == nil {
		t.Fatal("accepted partial JSON")
	}
	data, _ := os.ReadFile(s.path(c))
	if string(data) != `{"Version":` {
		t.Fatal("overwrote corrupt snapshot")
	}
}
