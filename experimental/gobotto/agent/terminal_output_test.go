package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWrittenAnswerWinsOverTerminalReaction(t *testing.T) {
	s := NewSession("thread")
	out, err := s.Turn(context.Background(), func(context.Context, []Message) (Message, error) {
		return decode(t, `{"role":"assistant","content":"Here are the findings.","tool_calls":[{"id":"eyes","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"eyes\"}"}}]}`), nil
	}, nil, "Investigate")
	if err != nil || out.Body != "Here are the findings." || out.Emoji != "" {
		t.Fatal(out, err)
	}
	s.RecordDelivery(nil)
	if s.LastDeliveredReply() != out.Body {
		t.Fatal("written delivery was not recorded")
	}
	data, _ := json.Marshal(s.Snapshot())
	if !strings.Contains(string(data), "Reaction skipped") {
		t.Fatal("missing explicit reaction receipt")
	}
}

func TestReactionReceiptDoesNotClaimTextDelivery(t *testing.T) {
	s := NewSession("thread")
	_, err := s.Turn(context.Background(), func(context.Context, []Message) (Message, error) {
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"eyes","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"eyes\"}"}}]}`), nil
	}, nil, "Thanks")
	if err != nil {
		t.Fatal(err)
	}
	s.RecordDelivery(nil)
	data, _ := json.Marshal(s.Snapshot())
	if !strings.Contains(string(data), "No accompanying text was posted") || s.LastDeliveredReply() != "" {
		t.Fatal(string(data))
	}
}

func TestRestoreClarifiesLegacyReactionReceipt(t *testing.T) {
	s := Restore(Snapshot{Messages: []Message{
		decode(t, `{"role":"assistant","content":"Never delivered text","tool_calls":[{"id":"eyes","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"eyes\"}"}}]}`),
		decode(t, `{"role":"tool","tool_call_id":"eyes","content":"Output delivered to Chatto."}`),
	}})
	data, _ := json.Marshal(s.Snapshot())
	if !strings.Contains(string(data), "No accompanying text was posted") || s.LastDeliveredReply() != "" {
		t.Fatal(string(data))
	}
}
