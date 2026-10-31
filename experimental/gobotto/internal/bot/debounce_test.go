package bot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestConfiguredDebounceResetsAfterEveryMessage(t *testing.T) {
	for _, configured := range []time.Duration{0, 750 * time.Millisecond, 6 * time.Second} {
		t.Run(configured.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				window := configured
				if window == 0 {
					window = 4 * time.Second
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				f := newFake()
				requests := make(chan string, 8)
				done := make(chan error, 1)
				go func() {
					done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
						return func(_ context.Context, messages []agent.Message) (agent.Message, error) {
							data, _ := json.Marshal(messages)
							requests <- string(data)
							return answer(), nil
						}
					}, func(tui.Event) {}, Options{Debounce: configured})
				}()
				<-f.ready
				f.events <- input("one", "first-fragment", true)
				synctest.Wait()
				time.Sleep(window - time.Millisecond)
				synctest.Wait()
				if len(requests) != 0 {
					t.Fatal("turn started before configured quiet period")
				}
				f.events <- input("two", "second-fragment", false)
				synctest.Wait()
				time.Sleep(window - time.Millisecond)
				synctest.Wait()
				if len(requests) != 0 {
					t.Fatal("new message did not reset debounce")
				}
				time.Sleep(time.Millisecond)
				synctest.Wait()
				select {
				case request := <-requests:
					if !strings.Contains(request, "first-fragment") || !strings.Contains(request, "second-fragment") {
						t.Fatal("fragments were not batched")
					}
				default:
					t.Fatal("turn did not start at the configured deadline")
				}
				cancel()
				<-done
			})
		})
	}
}
