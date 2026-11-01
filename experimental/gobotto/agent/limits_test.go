package agent

import (
	"context"
	"encoding/json"
	"testing"
)

func TestUnlimitedAndLargeConfiguredBudgets(t *testing.T) {
	for _, worker := range []bool{false, true} {
		for _, limit := range []int{0, 100} {
			executed := 0
			ctx := WithLimits(context.Background(), Limits{MaxToolCalls: limit, MaxModelSteps: limit})
			ctx = WithTools(ctx, []Tool{{Name: "work", Execute: func(context.Context, json.RawMessage) (string, error) { executed++; return "{}", nil }}})
			s := NewSession("context")
			if worker {
				s = NewWorker("Investigate", limit)
			}
			out, err := s.Turn(ctx, func(ctx context.Context, _ []Message) (Message, error) {
				if final, _ := ctx.Value(textOnlyKey{}).(bool); final {
					t.Fatal("old implicit cap remains")
				}
				if executed == 80 {
					return textMessage("assistant", "done"), nil
				}
				return decode(t, `{"role":"assistant","tool_calls":[{"id":"work","type":"function","function":{"name":"work","arguments":"{}"}}]}`), nil
			}, nil, "work")
			if err != nil || out.Body != "done" || executed != 80 {
				t.Fatal(out, err, executed)
			}
		}
	}
}

func TestUnlimitedLoopStillCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	_, err := NewWorker("Investigate", 0).Turn(ctx, func(context.Context, []Message) (Message, error) {
		calls++
		if calls == 20 {
			cancel()
		}
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"unknown","type":"function","function":{"name":"missing","arguments":"{}"}}]}`), nil
	}, nil, "work")
	if err != context.Canceled || calls != 20 {
		t.Fatal("unlimited work ignored cancellation", calls, err)
	}
}
