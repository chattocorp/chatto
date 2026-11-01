package bot

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/subagent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func wireMessage(text string) agent.Message {
	var m agent.Message
	_ = json.Unmarshal([]byte(text), &m)
	return m
}

type backgroundClient struct {
	*fakeClient
	replies chan string
}

func (f *backgroundClient) Reply(_ context.Context, room, root, target, body, _ string) error {
	if room != "room" || root != "root" || (target != "first" && target != "status") {
		return errors.New("invalid routing")
	}
	f.replies <- body
	return nil
}

func TestQuestionAnsweredWhileChildIsBlocked(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "router.go"), []byte("package router"), 0600); err != nil {
			t.Fatal(err)
		}
		blocked, release := make(chan struct{}), make(chan struct{})
		children, err := subagent.Open(map[string]config.Subagent{"code": {Dir: dir, Model: "test"}}, func(_, _ string, _ func(agent.Usage)) agent.Complete {
			calls := 0
			return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
				calls++
				if calls == 1 {
					return wireMessage(`{"role":"assistant","content":"Provisional: the router is the likely candidate; verification is unfinished.","tool_calls":[{"id":"read","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"router.go\"}"}}]}`), nil
				}
				close(blocked)
				select {
				case <-release:
					return wireMessage(`{"role":"assistant","content":"Verified final investigation report."}`), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer children.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := &backgroundClient{fakeClient: newFake(), replies: make(chan string, 8)}
		done := make(chan error, 1)
		go func() {
			done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(_ string, _ func(agent.Usage)) agent.Complete {
				calls := 0
				return func(_ context.Context, messages []agent.Message) (agent.Message, error) {
					calls++
					switch calls {
					case 1:
						return wireMessage(`{"role":"assistant","tool_calls":[{"id":"child","type":"function","function":{"name":"delegate","arguments":"{\"agent\":\"code\",\"task\":\"Investigate router\"}"}}]}`), nil
					case 2:
						return wireMessage(`{"role":"assistant","tool_calls":[{"id":"wait","type":"function","function":{"name":"defer_response","arguments":"{}"}}]}`), nil
					case 3:
						text := string(messages[len(messages)-1]["content"])
						if !strings.Contains(text, "Provisional: the router") || !strings.Contains(text, "how is it going") {
							return nil, errors.New("live buffer missing from question context")
						}
						return wireMessage(`{"role":"assistant","content":"Still investigating; the router is a provisional candidate."}`), nil
					case 4:
						text := string(messages[len(messages)-1]["content"])
						if !strings.Contains(text, "Verified final investigation report") || !strings.Contains(text, "completedInvestigations") {
							return nil, errors.New("completion context missing")
						}
						return wireMessage(`{"role":"assistant","content":"Here is the final report."}`), nil
					default:
						return nil, errors.New("unexpected repeated turn")
					}
				}
			}, func(tui.Event) {}, Options{Subagents: children})
		}()
		<-f.ready
		f.events <- input("first", "Investigate", true)
		time.Sleep(batchWindow)
		synctest.Wait()
		<-blocked
		<-f.replies // announcement
		f.events <- input("status", "how is it going", false)
		synctest.Wait()
		time.Sleep(batchWindow)
		synctest.Wait()
		select {
		case body := <-f.replies:
			if !strings.Contains(body, "Still investigating") {
				t.Fatal(body)
			}
		default:
			t.Fatal("status reply blocked behind child")
		}
		close(release)
		synctest.Wait()
		if body := <-f.replies; body != "Here is the final report." {
			t.Fatal(body)
		}
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}

func TestBackgroundLifecycle(t *testing.T) {
	for _, mode := range []string{"graceful", "invalidate", "force"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				started, release, cancelled := make(chan struct{}), make(chan struct{}), make(chan struct{})
				children, err := subagent.Open(map[string]config.Subagent{"code": {Dir: t.TempDir()}}, func(_, _ string, _ func(agent.Usage)) agent.Complete {
					return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
						close(started)
						select {
						case <-release:
							return wireMessage(`{"role":"assistant","content":"Finished investigation."}`), nil
						case <-ctx.Done():
							close(cancelled)
							return nil, ctx.Err()
						}
					}
				})
				if err != nil {
					t.Fatal(err)
				}
				defer children.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				shutdown := make(chan struct{})
				f := &backgroundClient{fakeClient: newFake(), replies: make(chan string, 8)}
				done := make(chan error, 1)
				go func() {
					done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(_ string, _ func(agent.Usage)) agent.Complete {
						calls := 0
						return func(context.Context, []agent.Message) (agent.Message, error) {
							calls++
							if calls == 1 {
								return wireMessage(`{"role":"assistant","tool_calls":[{"id":"child","type":"function","function":{"name":"delegate","arguments":"{\"agent\":\"code\",\"task\":\"Investigate\"}"}}]}`), nil
							}
							if calls == 2 {
								return wireMessage(`{"role":"assistant","tool_calls":[{"id":"wait","type":"function","function":{"name":"defer_response","arguments":"{}"}}]}`), nil
							}
							return wireMessage(`{"role":"assistant","content":"Final result."}`), nil
						}
					}, func(tui.Event) {}, Options{Subagents: children, Shutdown: shutdown})
				}()
				<-f.ready
				f.events <- input("first", "Investigate", true)
				time.Sleep(batchWindow)
				synctest.Wait()
				<-started
				<-f.replies
				if mode == "graceful" {
					close(shutdown)
					synctest.Wait()
					select {
					case <-done:
						t.Fatal("quit before child finished")
					default:
					}
					close(release)
					synctest.Wait()
					if body := <-f.replies; body != "Final result." {
						t.Fatal(body)
					}
					if err := <-done; err != nil {
						t.Fatal(err)
					}
				} else {
					if mode == "invalidate" {
						f.events <- chatto.MessageEvent{InvalidatedRoom: "room", InvalidatedMessage: "first", InvalidationReason: chatto.MessageRetracted}
					} else {
						cancel()
					}
					<-cancelled
					synctest.Wait()
					select {
					case body := <-f.replies:
						t.Fatal("stale child result posted", body)
					default:
					}
					cancel()
					if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
						t.Fatal(err)
					}
				}
			})
		})
	}
}
