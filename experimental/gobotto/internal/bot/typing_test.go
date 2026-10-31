package bot

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

type typingClient struct {
	*fakeClient
	refresh func(context.Context, string, string) error
}

func (c *typingClient) RefreshTypingIndicator(ctx context.Context, room, root string) error {
	return c.refresh(ctx, room, root)
}

func TestTypingFollowsTurnLifetime(t *testing.T) {
	for _, outcome := range []string{"reply", "reaction", "failure", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				pulses := make(chan time.Time, 16)
				client := &typingClient{fakeClient: newFake(), refresh: func(_ context.Context, room, root string) error {
					if room != "room" || root != "root" {
						t.Error("typing escaped the active thread")
					}
					pulses <- time.Now()
					// Typing failures must not fail the model turn.
					return errors.New("typing unavailable")
				}}
				started := time.Now()
				c := &conversation{key: threadKey{"room", "root"}, complete: func(context.Context, []agent.Message) (agent.Message, error) {
					time.Sleep(5 * time.Second)
					switch outcome {
					case "failure":
						return nil, errors.New("model failed")
					case "cancelled":
						cancel()
						return nil, ctx.Err()
					case "reaction":
						var message agent.Message
						if err := json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"r","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"heart\"}"}}]}`), &message); err != nil {
							t.Fatal(err)
						}
						return message, nil
					default:
						return answer(), nil
					}
				}}
				err := respond(ctx, client, c, []chatto.MessageEvent{input("trigger", "hello", true)}, nil)
				if (err != nil) != (outcome == "failure" || outcome == "cancelled") {
					t.Fatalf("unexpected turn result: %v", err)
				}
				if len(pulses) != 3 {
					t.Fatalf("want immediate typing and two refreshes, got %d", len(pulses))
				}
				for i := range 3 {
					if got := (<-pulses).Sub(started); got != time.Duration(i)*2*time.Second {
						t.Fatalf("typing cadence: %s", got)
					}
				}
				time.Sleep(6 * time.Second)
				synctest.Wait()
				if len(pulses) != 0 {
					t.Fatal("typing continued after turn ended")
				}
			})
		})
	}
}

func TestStopTypingCancelsInFlightRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, exited := make(chan struct{}), make(chan struct{})
		client := &typingClient{fakeClient: newFake(), refresh: func(ctx context.Context, _, _ string) error {
			close(started)
			<-ctx.Done()
			close(exited)
			return ctx.Err()
		}}
		stop := startTyping(context.Background(), client, threadKey{"room", "root"})
		<-started
		stop()
		stop()
		select {
		case <-exited:
		default:
			t.Fatal("stop returned before in-flight typing request ended")
		}
	})
}
