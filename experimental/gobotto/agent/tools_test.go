package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCapabilityReceiptSurvivesLaterModelFailure(t *testing.T) {
	s := NewSession("context")
	writes := 0
	ctx := WithTools(context.Background(), []Tool{{Name: "memory_create", Execute: func(context.Context, json.RawMessage) (string, error) {
		writes++
		return `{"id":"abcde","hash":"saved"}`, nil
	}}})
	calls := 0
	_, err := s.Turn(ctx, func(context.Context, []Message) (Message, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("model unavailable")
		}
		var m Message
		json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"memory_create","arguments":"{}"}}]}`), &m)
		return m, nil
	}, nil, "remember this")
	if err == nil || writes != 1 {
		t.Fatal("test did not execute mutation then fail")
	}
	data, _ := json.Marshal(s.Snapshot())
	if !strings.Contains(string(data), "abcde") || !strings.Contains(string(data), "saved") {
		t.Fatal("durable mutation receipt rolled back")
	}
}

func TestCapabilityRejectionRemainsActionable(t *testing.T) {
	result := `{"status":"rejected","error":"Read memory_info first","retryable":true,"mutated":false}`
	ctx := WithTools(context.Background(), []Tool{{Name: "memory_search", Execute: func(context.Context, json.RawMessage) (string, error) { return result, nil }}})
	calls := 0
	var states []string
	_, err := NewSession("context").Turn(ctx, func(_ context.Context, messages []Message) (Message, error) {
		calls++
		if calls == 1 {
			var m Message
			json.Unmarshal([]byte(`{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"memory_search","arguments":"{}"}}]}`), &m)
			return m, nil
		}
		var feedback string
		if json.Unmarshal(messages[len(messages)-1]["content"], &feedback) != nil || feedback != result {
			t.Fatal("rejection feedback was replaced with uncertain failure")
		}
		return textMessage("assistant", "I'll read the guidance first."), nil
	}, nil, "recall", func(e ToolEvent) { states = append(states, e.State) })
	if err != nil || strings.Join(states, ",") != "running,rejected" {
		t.Fatalf("incorrect rejection state: %v %v", states, err)
	}
}

func TestBatchedRejectionPreservesBudgetAndAllowsWrapUp(t *testing.T) {
	executed, requests := 0, 0
	ctx := WithTools(context.Background(), []Tool{{Name: "work", Execute: func(context.Context, json.RawMessage) (string, error) {
		executed++
		return `{"status":"complete"}`, nil
	}}})
	s := NewSession("context")
	outcome, err := s.Turn(ctx, func(ctx context.Context, messages []Message) (Message, error) {
		requests++
		if requests == 1 {
			return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"work","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"work","arguments":"{}"}}]}`), nil
		}
		if requests == 2 {
			if executed != 0 {
				t.Fatal("batched calls executed")
			}
			for _, m := range messages[len(messages)-2:] {
				var feedback string
				_ = json.Unmarshal(m["content"], &feedback)
				if !strings.Contains(feedback, "None of these calls ran") {
					t.Fatal("misleading rejection feedback")
				}
			}
		}
		if final, _ := ctx.Value(textOnlyKey{}).(bool); final {
			if executed != maxTurnTools {
				t.Fatal("rejected batch consumed execution budget")
			}
			return textMessage("assistant", "Eight actions completed; verification is still pending."), nil
		}
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"work","type":"function","function":{"name":"work","arguments":"{}"}}]}`), nil
	}, nil, "work")
	if err != nil || outcome.Body != "Eight actions completed; verification is still pending." || requests != 10 || executed != 8 {
		t.Fatalf("bad wrap-up: %v, requests=%d, executed=%d", err, requests, executed)
	}
}

func TestRepeatedRejectionsHaveFiniteWrapUp(t *testing.T) {
	requests := 0
	outcome, err := NewSession("context").Turn(context.Background(), func(ctx context.Context, _ []Message) (Message, error) {
		requests++
		if final, _ := ctx.Value(textOnlyKey{}).(bool); final {
			return textMessage("assistant", "I could not perform the requested operation."), nil
		}
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"unknown","type":"function","function":{"name":"missing_tool","arguments":"{}"}}]}`), nil
	}, nil, "work")
	if err != nil || outcome.Body == "" || requests != maxTurnSteps+1 {
		t.Fatal("rejection loop did not terminate with a report", err, requests)
	}
}

func TestDeliveredReplyRecoveryRequiresTerminalReceipt(t *testing.T) {
	s := NewSession("context")
	s.messages = append(s.messages, textMessage("assistant", "saved reply"))
	if s.LastDeliveredReply() != "" {
		t.Fatal("inferred delivery before receipt")
	}
	s.RecordDelivery(nil)
	if s.LastDeliveredReply() != "saved reply" {
		t.Fatal("lost confirmed reply")
	}
	s.messages = append(s.messages, textMessage("user", "next turn"))
	if s.LastDeliveredReply() != "" {
		t.Fatal("borrowed an earlier turn's reply")
	}
	s.messages = append(s.messages, textMessage("assistant", "uncertain reply"))
	s.RecordDelivery(errors.New("uncertain"))
	if s.LastDeliveredReply() != "" {
		t.Fatal("uncertain receipt treated as successful")
	}
}
