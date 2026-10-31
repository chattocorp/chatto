package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIdentityUpdatesRestoredPromptWithoutChangingHistory(t *testing.T) {
	s := NewSession("thread context")
	s.messages = append(s.messages, textMessage("assistant", "Earlier mistaken identity claim"))
	before, _ := json.Marshal(s.messages[1:])
	s = Restore(s.Snapshot())
	identity := Identity{ID: "bot", Login: "test-bot", DisplayName: "Test Bot"}
	s.SetIdentity(identity)
	first, _ := json.Marshal(s.messages[0])
	s.SetIdentity(identity)
	second, _ := json.Marshal(s.messages[0])
	if string(first) != string(second) {
		t.Fatal("unchanged identity changed prompt prefix")
	}
	identity.Login = "renamed-bot"
	s.SetIdentity(identity)
	var prompt string
	json.Unmarshal(s.messages[0]["content"], &prompt)
	if !strings.Contains(prompt, `"login":"renamed-bot"`) || strings.Contains(prompt, `"login":"test-bot"`) || !strings.Contains(prompt, "replaces any earlier identity claims") {
		t.Fatal("restored identity was not replaced")
	}
	after, _ := json.Marshal(s.messages[1:])
	if string(before) != string(after) {
		t.Fatal("identity refresh discarded conversation history")
	}
}

func TestOperatorPromptReplacesOldPreferencesOnRestore(t *testing.T) {
	s := NewSession("thread context")
	identity := Identity{ID: "bot", Login: "test-bot"}
	s.SetInstructions(identity, "Original operator preferences")
	s.messages = append(s.messages, textMessage("assistant", "previous answer"))
	before, _ := json.Marshal(s.messages[1:])
	s = Restore(s.Snapshot())
	s.SetInstructions(identity, "New operator preferences")
	var prompt string
	json.Unmarshal(s.messages[0]["content"], &prompt)
	if strings.Contains(prompt, "Original operator preferences") || !strings.Contains(prompt, "New operator preferences") || !strings.Contains(prompt, "do not call read_thread just to start a turn") || !strings.Contains(prompt, `"login":"test-bot"`) {
		t.Fatal("operator preferences replaced internal rules or identity, or failed to refresh")
	}
	first, _ := json.Marshal(s.messages[0])
	s.SetInstructions(identity, "New operator preferences")
	second, _ := json.Marshal(s.messages[0])
	after, _ := json.Marshal(s.messages[1:])
	if string(first) != string(second) || string(before) != string(after) {
		t.Fatal("unchanged configuration changed prefix or history")
	}
	s.SetInstructions(identity, "")
	if strings.Contains(string(s.messages[0]["content"]), "New operator preferences") {
		t.Fatal("cleared preferences survived")
	}
}

func TestReactionToolExchangeSurvivesNextTurn(t *testing.T) {
	s := NewSession("initial snapshot")
	complete := func(context.Context, []Message) (Message, error) {
		return decode(t, `{"role":"assistant","reasoning_details":[{"data":"opaque"}],"tool_calls":[{"id":"r","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"heart\"}"}}]}`), nil
	}
	read := func(context.Context) (string, error) { t.Fatal("unexpected thread reload"); return "", nil }
	if _, err := s.Turn(context.Background(), complete, read, "first"); err != nil {
		t.Fatal(err)
	}
	s.RecordDelivery(nil)
	_, err := s.Turn(context.Background(), func(_ context.Context, m []Message) (Message, error) {
		if string(m[3]["reasoning_details"]) != `[{"data":"opaque"}]` || string(m[4]["tool_call_id"]) != `"r"` || string(m[4]["role"]) != `"tool"` {
			t.Fatal("reaction transcript was not retained")
		}
		return textMessage("assistant", "hello"), nil
	}, read, "second")
	if err != nil {
		t.Fatal(err)
	}
}

func TestFailedToolTurnDoesNotLeaveUnmatchedCalls(t *testing.T) {
	s := NewSession("initial")
	_, err := s.Turn(context.Background(), func(context.Context, []Message) (Message, error) {
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"r","type":"function","function":{"name":"read_thread","arguments":"{}"}}]}`), nil
	}, func(context.Context) (string, error) { return "", errors.New("unavailable") }, "first")
	if err == nil {
		t.Fatal("expected failure")
	}
	_, err = s.Turn(context.Background(), func(_ context.Context, m []Message) (Message, error) {
		data, _ := json.Marshal(m)
		if strings.Contains(string(data), "tool_calls") {
			t.Fatal("unmatched tool call survived failed turn")
		}
		return textMessage("assistant", "ok"), nil
	}, nil, "second")
	if err != nil {
		t.Fatal(err)
	}
}
