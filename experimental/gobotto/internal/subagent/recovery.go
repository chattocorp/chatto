package subagent

import (
	"context"
	"fmt"
	"time"

	"github.com/chatto-chat/chatto/experimental/gobotto/agent"
)

// recoverRequest retries only a model request; tool execution remains outside
// this loop, so completed reads or writes cannot be replayed by transport recovery.
func recoverRequest(ctx context.Context, retries int, run *Run, request func() (agent.Message, error), publish func() error) (agent.Message, error) {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		msg, err := request()
		if err == nil {
			run.LastFailure = nil
			return msg, nil
		}
		info := agent.ModelFailureInfo(err)
		run.LastFailure = &info
		if ctx.Err() != nil || !info.Retryable || attempt >= retries {
			return nil, err
		}
		run.Recoveries++
		run.phase("retry_backoff")
		run.note("recovery", fmt.Sprintf("Model request failed (%s). Retrying the same request with retained evidence; no tools are replayed. %s", info.Code, info.Detail))
		if err := publish(); err != nil {
			return nil, err
		}
		timer := time.NewTimer(time.Second * time.Duration(1<<min(attempt, 5)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
