package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestReasoningWireOptions(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	no := false
	budget := 2048
	for _, tc := range []struct {
		options *ReasoningOptions
		want    string
	}{
		{nil, ""},
		{&ReasoningOptions{Enabled: &no, Exclude: &no}, `{"enabled":false,"exclude":false}`},
		{&ReasoningOptions{Effort: "medium"}, `{"effort":"medium"}`},
		{&ReasoningOptions{MaxTokens: &budget}, `{"max_tokens":2048}`},
	} {
		http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if string(body["reasoning"]) != tc.want {
				t.Fatalf("reasoning=%s want=%s", body["reasoning"], tc.want)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`))}, nil
		})
		if _, err := OpenRouter("test", "test/model", "test", nil, CompletionOptions{Reasoning: tc.options})(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReasoningUsagePresence(t *testing.T) {
	for _, tc := range []struct {
		details string
		known   bool
		tokens  int64
	}{
		{"", false, 0}, {`,"completion_tokens_details":null`, false, 0}, {`,"completion_tokens_details":{}`, false, 0},
		{`,"completion_tokens_details":{"reasoning_tokens":null}`, false, 0},
		{`,"completion_tokens_details":{"reasoning_tokens":0}`, true, 0},
		{`,"completion_tokens_details":{"reasoning_tokens":75}`, true, 75},
	} {
		var wire wireUsage
		if err := json.Unmarshal([]byte(`{"completion_tokens":100`+tc.details+`}`), &wire); err != nil {
			t.Fatal(err)
		}
		u := wire.report("provider", "request")
		if u.ReasoningKnown != tc.known || u.Reasoning != tc.tokens || u.Output != 100 {
			t.Fatalf("incorrect reasoning accounting: %+v", u)
		}
	}
}
