package bot

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestMessageInvalidationScope(t *testing.T) {
	c := &conversation{key: threadKey{"room", "root"}, pending: []chatto.MessageEvent{input("queued", "text", true)}}
	c.contextIDs.Store("known", true)
	c.contextReady.Store(true)
	for _, tc := range []struct {
		room, id          string
		all, active, want bool
	}{
		{"room", "unrelated", false, true, false}, {"room", "known", false, true, true},
		{"room", "root", false, false, true}, {"room", "queued", false, false, true},
		{"room", "in-flight", false, true, true}, {"other", "known", false, true, false},
		{"room", "", false, true, true}, {"", "", true, true, true},
	} {
		e := chatto.MessageEvent{InvalidatedRoom: tc.room, InvalidatedMessage: tc.id, InvalidateAll: tc.all, InvalidationReason: chatto.MessageEdited}
		if c.affectedBy(e, tc.active, []chatto.MessageEvent{input("in-flight", "text", true)}) != tc.want {
			t.Fatalf("wrong scope for %+v", tc)
		}
	}
	c.contextReady.Store(false)
	if !c.affectedBy(chatto.MessageEvent{InvalidatedRoom: "room", InvalidatedMessage: "unknown", InvalidationReason: chatto.MessageEdited}, true, nil) {
		t.Fatal("context read race must invalidate conservatively")
	}
}

func TestInvalidationDuringModelRequest(t *testing.T) {
	for _, affected := range []bool{false, true} {
		t.Run(map[bool]string{false: "unrelated edit continues", true: "context retraction cancels"}[affected], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f := newFake()
				started := make(chan context.Context, 1)
				release := make(chan struct{})
				events := make(chan tui.Event, 128)
				done := make(chan error, 1)
				go func() {
					done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
						return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
							started <- ctx
							select {
							case <-ctx.Done():
								return nil, ctx.Err()
							case <-release:
								return answer(), nil
							}
						}
					}, func(e tui.Event) { events <- e })
				}()
				<-f.ready
				f.events <- input("first", "start", true)
				synctest.Wait()
				time.Sleep(batchWindow)
				synctest.Wait()
				modelCtx := <-started
				target := "unrelated-thread-message"
				if affected {
					target = "first"
				}
				f.events <- chatto.MessageEvent{InvalidatedRoom: "room", InvalidatedMessage: target, InvalidationReason: chatto.MessageRetracted}
				synctest.Wait()
				if affected {
					if modelCtx.Err() == nil {
						t.Fatal("affected model request not cancelled")
					}
					select {
					case <-f.posts:
						t.Fatal("invalidated turn posted")
					default:
					}
					found := false
					for len(events) > 0 {
						e := <-events
						if e.Inspection != nil && e.Inspection.State == "cancelled" && strings.Contains(e.Inspection.Failure, "retracted") {
							found = true
						}
					}
					if !found {
						t.Fatal("cancellation reason missing from inspection")
					}
				} else {
					if modelCtx.Err() != nil {
						t.Fatal("unrelated message cancelled active turn")
					}
					close(release)
					synctest.Wait()
					<-f.posts
				}
				cancel()
				<-done
			})
		})
	}
}
