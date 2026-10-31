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
