package bot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestInboxRetainsPendingButNeverReplaysStartedWork(t *testing.T) {
	base := t.TempDir()
	s, err := OpenStore(base, "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	a, b := input("a", "pending", true), input("b", "started", true)
	if err := s.checkpoint("cursor-a", &a); err != nil {
		t.Fatal(err)
	}
	if err := s.checkpoint("cursor-b", &b); err != nil {
		t.Fatal(err)
	}
	if err := s.claim([]chatto.MessageEvent{b}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenStore(base, "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cursor, pending, started := s.pendingInbox()
	if cursor != "cursor-b" || len(pending) != 1 || pending[0].Message.ID != "a" || started != 1 {
		t.Fatal("inbox recovery lost pending or repeated started work")
	}
	if err := s.finish([]chatto.MessageEvent{a}); err != nil {
		t.Fatal(err)
	}
	if err := s.checkpoint("duplicate", &a); err != nil {
		t.Fatal(err)
	}
	if s.inboxPending(a) {
		t.Fatal("completed event was queued again")
	}
	if _, err := s.load(); err != nil {
		t.Fatal("inbox confused session loader", err)
	}
}

func TestInboxWriteFailureDoesNotAdvanceCursor(t *testing.T) {
	s, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a := input("a", "accepted", true)
	if err := s.checkpoint("accepted", &a); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.dir, ".realtime")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	b := input("b", "not accepted", true)
	if err := s.checkpoint("not accepted", &b); !errors.Is(err, errStore) {
		t.Fatal("expected storage failure", err)
	}
	cursor, pending, _ := s.pendingInbox()
	if cursor != "accepted" || len(pending) != 1 || pending[0].Message.ID != "a" {
		t.Fatal("failed write advanced in-memory checkpoint")
	}
}

func TestRestartDuringDebounceProcessesSavedBatch(t *testing.T) {
	base := t.TempDir()
	s, err := OpenStore(base, "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		f := newFake()
		done := make(chan error, 1)
		factory := func(string, func(agent.Usage)) agent.Complete {
			return func(context.Context, []agent.Message) (agent.Message, error) { return answer(), nil }
		}
		go func() { done <- Run(ctx, f, chatto.Profile{ID: "bot"}, factory, func(tui.Event) {}, Options{Store: s}) }()
		<-f.ready
		f.events <- input("first", "one", true)
		f.events <- input("second", "two", false)
		synctest.Wait()
		_, pending, _ := s.pendingInbox()
		if len(pending) != 2 {
			t.Fatal("debounced messages not durable")
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		s.Close()
		s, err = OpenStore(base, "server", "bot", "model")
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		ctx, cancel = context.WithCancel(context.Background())
		defer cancel()
		f = newFake()
		go func() { done <- Run(ctx, f, chatto.Profile{ID: "bot"}, factory, func(tui.Event) {}, Options{Store: s}) }()
		<-f.ready
		time.Sleep(batchWindow)
		synctest.Wait()
		select {
		case target := <-f.posts:
			if target != "second" {
				t.Fatal("wrong batch target")
			}
		default:
			t.Fatal("restored batch did not run")
		}
		_, pending, _ = s.pendingInbox()
		if len(pending) != 0 {
			t.Fatal("completed batch remained pending")
		}
		cancel()
		<-done
	})
}

type recoveryFake struct {
	*fakeClient
	observe func(context.Context, chatto.RecoveryOptions, func(), func(context.Context, chatto.MessageEvent) error) error
}

func (f *recoveryFake) ObserveResumable(ctx context.Context, _ string, opts chatto.RecoveryOptions, ready func(), handle func(context.Context, chatto.MessageEvent) error) error {
	return f.observe(ctx, opts, ready, handle)
}

func TestReplayWaitsForCaughtUpBeforeStartingTurns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := &recoveryFake{fakeClient: newFake()}
		f.observe = func(ctx context.Context, _ chatto.RecoveryOptions, ready func(), handle func(context.Context, chatto.MessageEvent) error) error {
			if err := handle(ctx, input("first", "one", true)); err != nil {
				return err
			}
			time.Sleep(batchWindow + time.Second)
			if len(f.posts) > 0 {
				t.Error("model ran before replay completed")
			}
			if err := handle(ctx, input("second", "two", false)); err != nil {
				return err
			}
			ready()
			close(f.ready)
			<-ctx.Done()
			return ctx.Err()
		}
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
				return func(context.Context, []agent.Message) (agent.Message, error) { return answer(), nil }
			}, func(tui.Event) {})
		}()
		<-f.ready
		time.Sleep(batchWindow)
		synctest.Wait()
		select {
		case target := <-f.posts:
			if target != "second" {
				t.Fatal("replayed batch split")
			}
		default:
			t.Fatal("replayed batch did not run")
		}
		if len(f.posts) > 0 {
			t.Fatal("multiple replies to replay batch")
		}
		cancel()
		<-done
	})
}
