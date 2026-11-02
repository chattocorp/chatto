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

func TestSilentTurnRetainsReceiptAndResumes(t *testing.T) {
	s := NewSession("thread")
	out, err := s.Turn(context.Background(), func(context.Context, []Message) (Message, error) {
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"quiet","type":"function","function":{"name":"stay_silent","arguments":"{}"}}]}`), nil
	}, nil, "People talking to each other")
	if err != nil || !out.Silent || out.Body != "" || out.Emoji != "" || out.Waiting {
		t.Fatal(out, err)
	}
	s = Restore(s.Snapshot())
	if s.Snapshot().PendingTool != "" || s.LastDeliveredReply() != "" {
		t.Fatal("silence left pending delivery or invented a reply")
	}
	out, err = s.Turn(context.Background(), func(_ context.Context, messages []Message) (Message, error) {
		data, _ := json.Marshal(messages)
		if !strings.Contains(string(data), "Turn ended silently") {
			t.Fatal("silent turn receipt lost")
		}
		return textMessage("assistant", "I'm back."), nil
	}, nil, "Please help again")
	if err != nil || out.Body != "I'm back." || out.Silent {
		t.Fatal(out, err)
	}
}

func TestSilentToolRejectsInvalidArguments(t *testing.T) {
	for _, args := range []string{"null", `{"unexpected":true}`, "[]"} {
		calls := 0
		out, err := NewSession("thread").Turn(context.Background(), func(_ context.Context, messages []Message) (Message, error) {
			calls++
			if calls == 1 {
				encoded, _ := json.Marshal(args)
				return decode(t, `{"role":"assistant","tool_calls":[{"id":"quiet","type":"function","function":{"name":"stay_silent","arguments":`+string(encoded)+`}}]}`), nil
			}
			if !strings.Contains(string(messages[len(messages)-1]["content"]), "rejected") {
				t.Fatal("invalid silence accepted")
			}
			return textMessage("assistant", "Answer"), nil
		}, nil, "input")
		if err != nil || out.Silent || calls != 2 {
			t.Fatal(out, err, calls)
		}
	}
}

func TestWrittenAnswerWinsOverSilence(t *testing.T) {
	out, err := NewSession("thread").Turn(context.Background(), func(context.Context, []Message) (Message, error) {
		return decode(t, `{"role":"assistant","content":"Findings","tool_calls":[{"id":"quiet","type":"function","function":{"name":"stay_silent","arguments":"{}"}}]}`), nil
	}, nil, "input")
	if err != nil || out.Silent || out.Body != "Findings" {
		t.Fatal(out, err)
	}
}
