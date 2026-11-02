package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/inspection"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func lazyFixture(t *testing.T) (*Store, *conversation) {
	t.Helper()
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "provider", model: agent.NewSession("private transcript"), usage: totals{Input: 100, Known: true}, turns: 45}
	for n := 1; n <= 45; n++ {
		c.records = append(c.records, inspection.Turn{Number: n, State: "delivered", Response: fmt.Sprint(n)})
	}
	if err := s.save(c, false); err != nil {
		t.Fatal(err)
	}
	return s, c
}

func TestIndexAndPagesDoNotDecodeTranscript(t *testing.T) {
	s, c := lazyFixture(t)
	data, err := os.ReadFile(s.path(c))
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(data, []byte(`"History":`))
	if start < 0 {
		t.Fatal("missing history field")
	}
	broken := append(append([]byte(nil), data[:start]...), []byte(`"History":invalid transcript`)...)
	if err := os.WriteFile(s.path(c), broken, 0600); err != nil {
		t.Fatal(err)
	}
	index, err := s.loadIndex(context.Background())
	if err != nil || len(index) != 1 {
		t.Fatal("metadata read decoded transcript", err)
	}
	stub := index[0]
	if !stub.unloaded || stub.model != nil || stub.records != nil || stub.seen != nil || stub.usage.Input != 100 {
		t.Fatal("startup hydrated private data or lost summary")
	}
	before := 0
	seen := map[int]bool{}
	for _, want := range []int{20, 20, 5} {
		page, err := s.inspectionPage(context.Background(), c.humanID, before)
		if err != nil || len(page.Turns) != want {
			t.Fatal("page decoded transcript or lost turns", len(page.Turns), err)
		}
		for _, turn := range page.Turns {
			if seen[turn.Number] {
				t.Fatal("duplicate page turn")
			}
			seen[turn.Number] = true
		}
		before = page.Before
	}
	if before != 0 || len(seen) != 45 {
		t.Fatal("page boundaries incorrect")
	}
	if _, err := s.hydrate(stub); err == nil {
		t.Fatal("runtime accepted corrupt transcript")
	}
	if err := s.save(stub, false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.path(c))
	if !bytes.Equal(after, broken) {
		t.Fatal("stub save overwrote unloaded history")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.inspectionPage(ctx, c.humanID, 0); err == nil {
		t.Fatal("page ignored cancellation")
	}
}

func TestRuntimeCachePreservesPinnedWorkAndRoundTrips(t *testing.T) {
	s, _ := lazyFixture(t)
	sessions := map[threadKey]*conversation{}
	for n := 0; n < 12; n++ {
		v := &conversation{id: uint64(n + 1), key: threadKey{"room", fmt.Sprint(n)}, provider: fmt.Sprint(n), model: agent.NewSession(fmt.Sprintf("history %d", n)), used: time.Unix(int64(n), 0)}
		if err := s.save(v, false); err != nil {
			t.Fatal(err)
		}
		sessions[v.key] = v
	}
	active := sessions[threadKey{"room", "0"}]
	pending := sessions[threadKey{"room", "1"}]
	pending.pending = []chatto.MessageEvent{input("queued", "input", true)}
	if err := s.trimRuntime(sessions, active); err != nil {
		t.Fatal(err)
	}
	loaded := 0
	for _, v := range sessions {
		if !v.unloaded {
			loaded++
		}
	}
	if loaded != 8 || sessions[active.key].unloaded || sessions[pending.key].unloaded {
		t.Fatal("cache failed to bound idle history or evicted pinned work")
	}
	stub := sessions[threadKey{"room", "2"}]
	if !stub.unloaded {
		t.Fatal("oldest eligible history was not unloaded")
	}
	v, err := s.hydrate(stub)
	if err != nil || v.model == nil || !v.resumed {
		t.Fatal("rehydration failed", err)
	}
	b, _ := json.Marshal(v.model.Snapshot())
	if !bytes.Contains(b, []byte("history 2")) {
		t.Fatal("cached transcript lost")
	}
}

func TestIncomingMessageHydratesOnlyItsThread(t *testing.T) {
	s, c := lazyFixture(t)
	other := &conversation{id: 2, key: threadKey{"other", "other"}, provider: "other", model: agent.NewSession("other history")}
	if err := s.save(other, false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f := newFake()
	bound := make(chan string, 3)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(provider string, _ func(agent.Usage)) agent.Complete {
			bound <- provider
			return func(context.Context, []agent.Message) (agent.Message, error) { return answer(), nil }
		}, func(tui.Event) {}, Options{Store: s, Debounce: time.Millisecond})
	}()
	<-f.ready
	select {
	case <-bound:
		t.Fatal("startup bound a transcript")
	default:
	}
	f.events <- input("followup", "new message", false)
	select {
	case provider := <-bound:
		if provider != c.provider {
			t.Error("wrong transcript bound")
		}
	case <-ctx.Done():
		t.Fatal("lazy thread did not resume")
	}
	select {
	case <-f.posts:
	case <-ctx.Done():
		t.Fatal("lazy thread did not respond")
	}
	cancel()
	<-done
	if len(bound) != 0 {
		t.Fatal("unrelated history was hydrated")
	}
}

func TestStartupDoesNotBindUnopenedHistories(t *testing.T) {
	s, c := lazyFixture(t)
	data, _ := os.ReadFile(s.path(c))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := false
	err := Run(ctx, newFake(), chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
		t.Error("startup bound an unopened history")
		return nil
	}, func(e tui.Event) {
		if e.Kind == tui.Started {
			started = true
			if e.LoadDetails == nil || len(e.Records) > 0 {
				t.Error("startup emitted eager history")
			}
		}
		if e.Kind == tui.Completed {
			cancel()
		}
	}, Options{Store: s})
	if err != context.Canceled || !started {
		t.Fatal("startup did not publish metadata", err)
	}
	after, _ := os.ReadFile(s.path(c))
	if !bytes.Equal(data, after) {
		t.Fatal("shutdown rewrote unopened history")
	}
}

func TestNewStorageGenerationLeavesLegacyDataUntouched(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	old := filepath.Join(filepath.Dir(s.dir), "old.json")
	data := []byte(`{"private":"old transcript"}`)
	if err := os.WriteFile(old, data, 0600); err != nil {
		t.Fatal(err)
	}
	index, err := s.loadIndex(context.Background())
	if err != nil || len(index) != 0 {
		t.Fatal("legacy generation was loaded", err)
	}
	after, _ := os.ReadFile(old)
	if !bytes.Equal(data, after) {
		t.Fatal("legacy data changed")
	}
}

func TestInspectionDistinguishesLiveCheckpointFromPreviousProcess(t *testing.T) {
	base := t.TempDir()
	s, err := OpenStore(base, "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	c := &conversation{id: 1, key: threadKey{"room", "root"}, provider: "provider", records: []inspection.Turn{{Number: 1, State: "running", Response: "proposed reply", Steps: []inspection.Step{{Name: "Post reply", State: "running"}}}}}
	if err := s.save(c, true); err != nil {
		t.Fatal(err)
	}
	page, err := s.inspectionPage(context.Background(), c.humanID, 0)
	if err != nil || page.Turns[0].State != "running" || page.Turns[0].Steps[0].State != "running" {
		t.Fatal("live checkpoint marked interrupted", err)
	}
	s.Close()
	s, err = OpenStore(base, "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err = s.inspectionPage(context.Background(), c.humanID, 0)
	if err != nil || page.Turns[0].State != "uncertain" || page.Turns[0].Steps[0].State != "interrupted" {
		t.Fatal("previous process checkpoint appeared live", err)
	}
}
