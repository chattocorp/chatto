package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestObservedRequestIncludesFinalToolsAndControls(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"done"}}]}`))}, nil
	})
	var captured string
	ctx := WithRequestObserver(context.Background(), func(s string) { captured = s })
	ctx = WithTools(ctx, []Tool{{Name: "schedule_list", Description: "List all schedules", Parameters: map[string]any{"type": "object"}}})
	ctx = context.WithValue(ctx, textOnlyKey{}, true)
	messages := []Message{{"role": []byte(`"system"`), "content": []byte(`"injected instructions"`)}}
	_, err := OpenRouter("private-api-key", "model", "session", nil)(ctx, messages)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"injected instructions", "schedule_list", "read_thread", `"tool_choice": "none"`} {
		if !strings.Contains(captured, want) {
			t.Fatal("missing captured request field", want)
		}
	}
	if strings.Contains(captured, "private-api-key") {
		t.Fatal("credential captured")
	}
}

func TestInspectionOmitsImagesAndProviderReasoning(t *testing.T) {
	input := map[string]any{"messages": []any{map[string]any{"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64,PRIVATE"}}}, "reasoning": "PRIVATE reasoning", "reasoning_details": []any{"PRIVATE details"}}}, "reasoning": map[string]any{"effort": "low"}}
	got := InspectionJSON(input)
	if strings.Contains(got, "PRIVATE") || !strings.Contains(got, "image omitted") || !strings.Contains(got, `"effort": "low"`) {
		t.Fatal("incorrect redaction", got)
	}
	if input["messages"].([]any)[0].(map[string]any)["reasoning"] != "PRIVATE reasoning" {
		t.Fatal("capture mutated request")
	}
}
