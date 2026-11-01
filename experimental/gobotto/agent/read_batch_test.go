package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestWorkerCancellationClosesReadBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	executed := 0
	ctx = WithTools(ctx, []Tool{{Name: "read", ReadOnly: true, Execute: func(context.Context, json.RawMessage) (string, error) {
		executed++
		cancel()
		return `{"text":"completed before cancellation"}`, nil
	}}})
	s := NewWorker("Investigate", 0)
	_, err := s.Turn(ctx, func(context.Context, []Message) (Message, error) {
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"read","arguments":"{}"}}]}`), nil
	}, nil, "task")
	encoded, _ := json.Marshal(s.Snapshot())
	if !errors.Is(err, context.Canceled) || executed != 1 || !strings.Contains(string(encoded), `"tool_call_id":"b"`) || !strings.Contains(string(encoded), "completed before cancellation") {
		t.Fatal("lost or incomplete receipts", err, executed, string(encoded))
	}
}

func TestWorkerReadBatchLimitAndReceipts(t *testing.T) {
	for _, limit := range []int{0, 1} {
		executed, calls := 0, 0
		ctx := WithTools(context.Background(), []Tool{{Name: "read", ReadOnly: true, Execute: func(context.Context, json.RawMessage) (string, error) { executed++; return `{"text":"evidence"}`, nil }}})
		s := NewWorker("Investigate", limit)
		out, err := s.Turn(ctx, func(_ context.Context, msgs []Message) (Message, error) {
			calls++
			if calls == 1 {
				return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"read","arguments":"{}"}}]}`), nil
			}
			encoded, _ := json.Marshal(msgs)
			if !strings.Contains(string(encoded), `"tool_call_id":"a"`) || !strings.Contains(string(encoded), `"tool_call_id":"b"`) {
				t.Fatal("incomplete batch transcript", string(encoded))
			}
			return textMessage("assistant", "Evidence summary"), nil
		}, nil, "task")
		want := 2
		if limit == 1 {
			want = 1
		}
		if err != nil || executed != want || out.Limited != (limit == 1) {
			t.Fatal(out, err, executed)
		}
	}
}

func TestWorkerMixedBatchExecutesNothing(t *testing.T) {
	executed, calls := 0, 0
	var tools []Tool
	for _, name := range []string{"read", "write"} {
		tools = append(tools, Tool{Name: name, ReadOnly: name == "read", Execute: func(context.Context, json.RawMessage) (string, error) { executed++; return "{}", nil }})
	}
	_, err := NewWorker("Investigate", 0).Turn(WithTools(context.Background(), tools), func(_ context.Context, _ []Message) (Message, error) {
		calls++
		if calls == 1 {
			return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"write","arguments":"{}"}}]}`), nil
		}
		return textMessage("assistant", "Stopped"), nil
	}, nil, "task")
	if err != nil || executed != 0 {
		t.Fatal(err, executed)
	}
}
