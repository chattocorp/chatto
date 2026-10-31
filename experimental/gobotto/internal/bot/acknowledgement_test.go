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

type acknowledgementClient struct {
	*fakeClient
	acknowledge func(context.Context, string, string) error
}

func (c *acknowledgementClient) React(ctx context.Context, room, target, emoji string) error {
	if emoji == "eyes" {
		return c.acknowledge(ctx, room, target)
	}
	return c.fakeClient.React(ctx, room, target, emoji)
}

func TestThreadAcknowledgementIsImmediateAndIndependent(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "timeout"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				acks := make(chan string, 8)
				client := &acknowledgementClient{fakeClient: newFake(), acknowledge: func(ctx context.Context, room, target string) error {
					acks <- room + "/" + target
					if outcome == "timeout" {
						<-ctx.Done()
						return ctx.Err()
					}
					if outcome == "failure" {
						return errors.New("reaction rejected")
					}
					return nil
				}}
				done := make(chan error, 1)
				go func() {
					done <- Run(ctx, client, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
						return func(context.Context, []agent.Message) (agent.Message, error) { return answer(), nil }
					}, func(tui.Event) {})
				}()
				<-client.ready
				client.events <- input("ignored", "ordinary message", false)
				synctest.Wait()
				if len(acks) != 0 {
					t.Fatal("acknowledged a message without joining")
				}
				client.events <- input("opening-reply", "hello", true)
				synctest.Wait()
				select {
				case target := <-acks:
					if target != "room/opening-reply" {
						t.Fatal("acknowledgement targeted the root instead of the trigger")
					}
				default:
					t.Fatal("acknowledgement waited for debounce")
				}
				client.events <- input("opening-reply", "duplicate", true)
				client.events <- input("second", "follow up", true)
				synctest.Wait()
				time.Sleep(batchWindow + time.Second)
				synctest.Wait()
				select {
				case target := <-client.posts:
					if target != "second" {
						t.Fatal("wrong final reply target")
					}
				default:
					t.Fatal("acknowledgement blocked the model reply")
				}
				time.Sleep(10 * time.Second)
				synctest.Wait()
				if len(acks) != 0 {
					t.Fatal("repeated acknowledgement")
				}
				cancel()
				<-done
			})
		})
	}
}

func TestRestoredThreadDoesNotRepeatAcknowledgement(t *testing.T) {
	store, err := OpenStore(t.TempDir(), "server", "bot", "model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.save(&conversation{id: 1, key: threadKey{"room", "root"}, provider: "stable"}, false); err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		client := &acknowledgementClient{fakeClient: newFake(), acknowledge: func(context.Context, string, string) error {
			t.Error("restored thread repeated acknowledgement")
			return nil
		}}
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, client, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
				return func(context.Context, []agent.Message) (agent.Message, error) { return answer(), nil }
			}, func(tui.Event) {}, Options{Store: store})
		}()
		<-client.ready
		client.events <- input("new", "hello again", true)
		<-client.posts
		cancel()
		<-done
	})
}
