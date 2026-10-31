package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTitleRequestIsBoundedAndIndependent(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if string(body["model"]) != `"test/title-model"` || string(body["max_tokens"]) != "64" || body["tools"] != nil {
			t.Fatal("incorrect title request")
		}
		if !strings.Contains(string(body["messages"]), "Cache performance") || strings.Contains(string(body["messages"]), "PRIVATE_INSTRUCTIONS") || strings.Contains(string(body["messages"]), "PRIVATE_REASONING") {
			t.Fatal("wrong title context")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"content":"  \"Understanding cache hits\"\n"}}],"usage":{"prompt_tokens":40,"completion_tokens":4,"cost":0.00001}}`))}, nil
	})
	s := NewSession(`{"messages":[{"body":"Cache performance"}]}`)
	s.SetInstructions(Identity{ID: "bot"}, "PRIVATE_INSTRUCTIONS")
	s.messages = append(s.messages, Message{"role": json.RawMessage(`"assistant"`), "content": json.RawMessage(`"Here is how it works"`), "reasoning": json.RawMessage(`"PRIVATE_REASONING"`)})
	result, err := OpenRouterTitles("test-key", "test/title-model", "Be specific")(context.Background(), s.Snapshot())
	if err != nil || result.Title != "Understanding cache hits" || result.Usage == nil || result.Usage.Input != 40 || result.Usage.Cost == nil || *result.Usage.Cost != 0.00001 {
		t.Fatal("title or billing data missing")
	}
}

func TestTitleSanitization(t *testing.T) {
	result := CleanTitle("\x00\u202eHello\n\tworld")
	if result != "Hello world" {
		t.Fatal("unsafe title characters remain")
	}
	if len([]rune(CleanTitle(strings.Repeat("界", 100)))) != 60 {
		t.Fatal("title is not bounded")
	}
}
