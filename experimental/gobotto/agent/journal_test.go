package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestJournalCompletionHasNoToolsOrParentContext(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var request struct {
			Tools    []any     `json:"tools"`
			Choice   string    `json:"tool_choice"`
			Messages []Message `json:"messages"`
			Max      int       `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Tools) != 0 || request.Choice != "none" || request.Max != 4096 || len(request.Messages) != 2 {
			t.Fatal("summary inherited parent tools or uncapped controls")
		}
		b, _ := json.Marshal(request)
		if strings.Contains(string(b), "PARENT_SECRET") {
			t.Fatal("parent context leaked")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}]}`))}, nil
	})
	ctx := WithTools(context.Background(), []Tool{{Name: "PARENT_SECRET"}})
	_, err := SummarizeConversation(ctx, OpenRouter("key", "model", "session", nil), `{"entries":[]}`)
	if err != nil {
		t.Fatal(err)
	}
}
