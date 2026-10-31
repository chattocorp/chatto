package bot

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestGracefulQuitDrainsOnlyActiveTurn(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "finish", true: "force"}[force], func(t *testing.T) {
			store, err := OpenStore(t.TempDir(), "server", "bot", "model")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				shutdown := make(chan struct{})
				started := make(chan context.Context, 4)
				release := make(chan struct{})
				f := newFake()
				done := make(chan error, 1)
				go func() {
					done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
						return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
							started <- ctx
							select {
							case <-release:
								return answer(), nil
							case <-ctx.Done():
								return nil, ctx.Err()
							}
						}
					}, func(tui.Event) {}, Options{Store: store, Shutdown: shutdown})
				}()
				<-f.ready
				f.events <- input("first", "first", true)
				turnCtx := <-started
				f.events <- input("queued", "queued", false)
				synctest.Wait()
				time.Sleep(batchWindow + time.Second)
				close(shutdown)
				synctest.Wait()
				if turnCtx.Err() != nil {
					t.Fatal("normal quit cancelled active turn")
				}
				select {
				case <-done:
					t.Fatal("quit before active turn finished")
				default:
				}
				if force {
					cancel()
				} else {
					close(release)
				}
				err := <-done
				if force && !errors.Is(err, context.Canceled) {
					t.Fatal("force quit did not cancel", err)
				}
				if !force && err != nil {
					t.Fatal(err)
				}
				if len(started) != 0 {
					t.Fatal("queued turn started while quitting")
				}
				if force && len(f.posts) != 0 {
					t.Fatal("cancelled turn delivered")
				}
				if !force {
					if len(f.posts) != 1 || <-f.posts != "first" {
						t.Fatal("active turn did not finish delivery")
					}
				}
				_, pending, _ := store.pendingInbox()
				if len(pending) != 1 || pending[0].Message.ID != "queued" {
					t.Fatal("queued message was lost on shutdown")
				}
			})
		})
	}
}

func TestGracefulQuitWhileIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		shutdown := make(chan struct{})
		close(shutdown)
		f := newFake()
		err := Run(context.Background(), f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete { t.Fatal("idle shutdown started model"); return nil }, func(tui.Event) {}, Options{Shutdown: shutdown})
		if err != nil {
			t.Fatal(err)
		}
	})
}
