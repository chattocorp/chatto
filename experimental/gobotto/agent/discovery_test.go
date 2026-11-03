package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDiscoveryChangesProviderSchemasWithoutGrantingAuthority(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"done"}}]}`))}, nil
	})
	var captured string
	base := WithRequestObserver(context.Background(), func(s string) { captured = s })
	base = WithTools(base, []Tool{{Name: "tool_search"}, {Name: "schedule_list"}})
	ctx := WithToolDiscovery(base, "tool_search")
	check := func(ctx context.Context, present, absent string) {
		t.Helper()
		if _, err := OpenRouter("test", "model", "session", nil)(ctx, nil); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(captured, present) || strings.Contains(captured, absent) {
			t.Fatal("wrong schema exposure", captured)
		}
	}
	check(ctx, "tool_search", "schedule_list")
	ExposeTools(ctx, "schedule_list", "forbidden")
	check(ctx, "schedule_list", "forbidden")
	check(WithToolDiscovery(base, "tool_search"), "tool_search", "schedule_list")
	check(WithTools(ctx, []Tool{{Name: "child_read"}}), "child_read", "schedule_list")
	ctx = WithTools(ctx, []Tool{{Name: "tool_search"}})
	ExposeTools(ctx, "schedule_list")
	check(ctx, "tool_search", "schedule_list")
}

func TestPermanentPromptBudget(t *testing.T) {
	if size := len(chattoPrompt) + len(behaviorPrompt); size > 8000 {
		t.Fatalf("permanent prompt is %d bytes; put feature guidance in discovered tools", size)
	}
}
