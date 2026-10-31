package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func TestTruncatedToolCallNeverExecutes(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	requests := 0
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"usage":{"completion_tokens":2048},"choices":[{"finish_reason":"length","message":{"role":"assistant","tool_calls":[{"id":"call","type":"function","function":{"name":"memory_brain_create","arguments":"{}"}}]}}]}`))}, nil
	})
	executed := false
	ctx := WithTools(context.Background(), []Tool{{Name: "memory_brain_create", Execute: func(context.Context, json.RawMessage) (string, error) { executed = true; return "{}", nil }}})
	var output int64
	complete := OpenRouter("test", "test/model", "session", func(u Usage) { output = u.Output })
	_, err := NewSession("context").Turn(ctx, complete, nil, "create a brain")
	if !errors.Is(err, ErrOutputLimit) || executed || requests != 1 || output != 2048 {
		t.Fatal("truncation executed or retried a tool, lost usage, or lost its failure reason")
	}
}

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenRouterWireContract(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	finish := "stop"
	cost := `,"cost":0.00125`
	cache := `,"prompt_tokens_details":{"cached_tokens":80}`
	expectedModel := "test/model"
	expectedTokens := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://openrouter.ai/api/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("incorrect endpoint or credential")
		}
		var body struct {
			Model        string            `json:"model"`
			Messages     []Message         `json:"messages"`
			MaxTokens    *int              `json:"max_tokens"`
			SessionID    string            `json:"session_id"`
			CacheControl map[string]string `json:"cache_control"`
			Tools        []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if (expectedTokens == 0 && body.MaxTokens != nil) || (expectedTokens > 0 && (body.MaxTokens == nil || *body.MaxTokens != expectedTokens)) {
			t.Error("output cap must be omitted unless explicitly configured")
		}
		if body.Model != expectedModel || body.SessionID != "stable-session" || len(body.Messages) != 1 || len(body.Tools) != 2 || body.Tools[0].Function.Name != "read_thread" || body.Tools[1].Function.Name != "react_to_ping" {
			t.Error("incorrect completion request")
		}
		if (body.CacheControl["type"] == "ephemeral") != strings.HasPrefix(expectedModel, "anthropic/") {
			t.Error("wrong model cache hint")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"gen-test","provider":"Test provider","usage":{"prompt_tokens":100,"completion_tokens":10` + cache + cost + `},"choices":[{"finish_reason":"` + finish + `","message":{"role":"assistant","content":"hello"}}]}`)), Header: make(http.Header)}, nil
	})
	var usage Usage
	complete := OpenRouter("test-key", "test/model", "stable-session", func(u Usage) { usage = u })
	if _, err := complete(context.Background(), []Message{textMessage("user", "hi")}); err != nil {
		t.Fatal(err)
	}
	if usage.Input != 100 || usage.CachedInput != 80 || !usage.CacheKnown || usage.Output != 10 || usage.Provider != "Test provider" || usage.RequestID != "gen-test" {
		t.Fatalf("usage=%+v", usage)
	}
	if usage.Cost == nil || *usage.Cost != 0.00125 {
		t.Fatalf("missing account charge: %+v", usage)
	}
	for _, field := range []string{"", `,"prompt_tokens_details":null`, `,"prompt_tokens_details":{}`, `,"prompt_tokens_details":{"cached_tokens":null}`, `,"prompt_tokens_details":{"cached_tokens":0}`} {
		cache = field
		if _, err := complete(context.Background(), []Message{textMessage("user", "hi")}); err != nil {
			t.Fatal(err)
		}
		if usage.CachedInput != 0 || usage.CacheKnown != strings.Contains(field, `"cached_tokens":0`) {
			t.Fatalf("cache presence lost for %s: %+v", field, usage)
		}
	}
	for _, value := range []string{`,"cost":0`, "", `,"cost":null`} {
		cost = value
		if _, err := complete(context.Background(), []Message{textMessage("user", "hi")}); err != nil {
			t.Fatal(err)
		}
		if (usage.Cost != nil) != (value == `,"cost":0`) {
			t.Fatalf("incorrect cost presence for %q", value)
		}
	}
	finish = "length"
	if _, err := complete(context.Background(), []Message{textMessage("user", "hi")}); !errors.Is(err, ErrOutputLimit) {
		t.Fatal("truncated completion did not report output limit", err)
	}
	finish = "stop"
	expectedTokens = 16384
	complete = OpenRouter("test-key", expectedModel, "stable-session", nil, CompletionOptions{MaxOutputTokens: expectedTokens})
	if _, err := complete(context.Background(), []Message{textMessage("user", "hi")}); err != nil {
		t.Fatal(err)
	}
	expectedTokens = 0
	expectedModel = "anthropic/test-model"
	complete = OpenRouter("test-key", expectedModel, "stable-session", nil)
	if _, err := complete(context.Background(), []Message{textMessage("user", "hi")}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRouterCapabilitiesAreTurnScoped(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	want := 3
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Tools) != want {
			t.Fatal("tools leaked across turns")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)), Header: make(http.Header)}, nil
	})
	complete := OpenRouter("test", "test/model", "session", nil)
	ctx := WithTools(context.Background(), []Tool{{Name: "memory_create", Parameters: map[string]any{"type": "object"}}})
	if _, err := complete(ctx, nil); err != nil {
		t.Fatal(err)
	}
	want = 2
	if _, err := complete(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
