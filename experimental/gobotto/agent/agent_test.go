package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func decode(t *testing.T, value string) Message {
	t.Helper()
	var m Message
	if err := json.Unmarshal([]byte(value), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestToolRoundTripPreservesProviderFields(t *testing.T) {
	step, reads := 0, 0
	call := decode(t, `{"role":"assistant","content":null,"reasoning_details":[{"data":"opaque"}],"tool_calls":[{"id":"call1","type":"function","function":{"name":"read_thread","arguments":"{}"}}]}`)
	answer, err := Run(context.Background(), func(_ context.Context, messages []Message) (Message, error) {
		step++
		if step == 1 {
			return call, nil
		}
		if string(messages[3]["reasoning_details"]) != string(call["reasoning_details"]) {
			t.Fatal("provider state lost")
		}
		if string(messages[4]["tool_call_id"]) != `"call1"` || string(messages[4]["content"]) != `"thread context"` {
			t.Fatal("tool result not correlated")
		}
		return textMessage("assistant", "Hello"), nil
	}, func(context.Context) (string, error) { reads++; return "thread context", nil }, "Hi")
	if err != nil || answer.Body != "Hello" || answer.Emoji != "" || reads != 2 || step != 2 {
		t.Fatalf("answer=%q reads=%d steps=%d err=%v", answer, reads, step, err)
	}
}

func TestToolCannotSelectAnotherThread(t *testing.T) {
	step := 0
	reads := 0
	_, err := Run(context.Background(), func(_ context.Context, messages []Message) (Message, error) {
		step++
		if step == 1 {
			return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read_thread","arguments":"{\"roomId\":\"elsewhere\"}"}}]}`), nil
		}
		return textMessage("assistant", "Unavailable"), nil
	}, func(context.Context) (string, error) { reads++; return "context", nil }, "Hi")
	if err != nil || reads != 1 {
		t.Fatal(err)
	}
}

func TestReactionFinishesAfterReadingThread(t *testing.T) {
	read := false
	outcome, err := Run(context.Background(), func(_ context.Context, messages []Message) (Message, error) {
		if !read {
			t.Fatal("model called before reading thread")
		}
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"thumbsup\"}"}}]}`), nil
	}, func(context.Context) (string, error) { read = true; return "thread", nil }, "thanks")
	if err != nil || outcome.Emoji != "thumbsup" || outcome.Body != "" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestReactionRejectsDestinationArguments(t *testing.T) {
	step := 0
	outcome, err := Run(context.Background(), func(context.Context, []Message) (Message, error) {
		step++
		if step == 1 {
			return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"react_to_ping","arguments":"{\"emoji\":\"thumbsup\",\"messageId\":\"other\"}"}}]}`), nil
		}
		return textMessage("assistant", "Thanks"), nil
	}, func(context.Context) (string, error) { return "thread", nil }, "thanks")
	if err != nil || outcome.Emoji != "" || outcome.Body != "Thanks" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestLoopLimitAndCancellation(t *testing.T) {
	calls := 0
	complete := func(context.Context, []Message) (Message, error) {
		calls++
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"read_thread","arguments":"{}"}}]}`), nil
	}
	read := func(context.Context) (string, error) { return "{}", nil }
	if outcome, err := Run(context.Background(), complete, read, "Hi"); err != nil || calls != 9 || !strings.Contains(outcome.Body, "work limit") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, complete, read, "Hi"); !errors.Is(err, context.Canceled) || calls != 9 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
