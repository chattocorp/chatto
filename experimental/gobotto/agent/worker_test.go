package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWorkerCannotUseChattoOrParentTools(t *testing.T) {
	for _, name := range []string{"read_thread", "react_to_ping", "delegate", "memory_create"} {
		calls := 0
		ctx := WithTools(context.Background(), nil)
		out, err := NewWorker("Investigate", 2).Turn(ctx, func(_ context.Context, messages []Message) (Message, error) {
			calls++
			if calls == 1 {
				arguments := `{}`
				if name == "react_to_ping" {
					arguments = `{"emoji":"eyes"}`
				}
				encoded, _ := json.Marshal(arguments)
				return decode(t, `{"role":"assistant","tool_calls":[{"id":"x","type":"function","function":{"name":"`+name+`","arguments":`+string(encoded)+`}}]}`), nil
			}
			if !strings.Contains(string(messages[len(messages)-1]["content"]), "Unknown tool") {
				t.Fatal("forbidden capability was not rejected", name)
			}
			return textMessage("assistant", "Cannot perform that operation."), nil
		}, nil, "task")
		if err != nil || out.Emoji != "" || out.Body == "" {
			t.Fatal(name, out, err)
		}
	}
}

func TestWorkerUsesItsOwnToolBudget(t *testing.T) {
	executed := 0
	ctx := WithTools(context.Background(), []Tool{{Name: "read_file", Execute: func(context.Context, json.RawMessage) (string, error) { executed++; return "{}", nil }}})
	out, err := NewWorker("Investigate", 10).Turn(ctx, func(ctx context.Context, _ []Message) (Message, error) {
		if final, _ := ctx.Value(textOnlyKey{}).(bool); final {
			return textMessage("assistant", "Investigation is partial."), nil
		}
		return decode(t, `{"role":"assistant","tool_calls":[{"id":"read","type":"function","function":{"name":"read_file","arguments":"{}"}}]}`), nil
	}, nil, "task")
	if err != nil || executed != 10 || out.Body != "Investigation is partial." {
		t.Fatal("child budget not enforced", executed, out, err)
	}
}

func TestWorkerWireContainsOnlyAssignedTools(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var input struct {
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if len(input.Tools) != 1 || input.Tools[0].Function.Name != "read_file" {
			t.Fatal("worker exposed parent tools", input)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Done"}}]}`))}, nil
	})
	ctx := WithTools(context.Background(), []Tool{{Name: "read_file"}})
	if _, err := NewWorker("Investigate", 2).Turn(ctx, OpenRouter("test", "test/model", "child", nil), nil, "task"); err != nil {
		t.Fatal(err)
	}
}
