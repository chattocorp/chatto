package bot

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
	"github.com/chatto-chat/chatto/experimental/gobotto/internal/tui"
	chatto "github.com/chattocorp/chatto/experimental/chatto-go"
)

func TestConfiguredTurnDeadline(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Second} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newFake()
			started := make(chan context.Context, 1)
			done := make(chan error, 1)
			go func() {
				done <- Run(ctx, f, chatto.Profile{ID: "bot"}, func(string, func(agent.Usage)) agent.Complete {
					return func(ctx context.Context, _ []agent.Message) (agent.Message, error) {
						started <- ctx
						<-ctx.Done()
						return nil, ctx.Err()
					}
				}, func(tui.Event) {}, Options{TurnTimeout: timeout})
			}()
			<-f.ready
			f.events <- input("first", "investigate", true)
			synctest.Wait()
			time.Sleep(batchWindow)
			synctest.Wait()
			turnCtx := <-started
			time.Sleep(4 * time.Minute)
			synctest.Wait()
			if timeout == 0 {
				if turnCtx.Err() != nil {
					t.Fatal("implicit parent deadline", turnCtx.Err())
				}
			} else if turnCtx.Err() != context.DeadlineExceeded {
				t.Fatal("configured deadline ignored", turnCtx.Err())
			}
			cancel()
			<-done
			if turnCtx.Err() == nil {
				t.Fatal("unlimited turn ignored cancellation")
			}
		})
	}
}
