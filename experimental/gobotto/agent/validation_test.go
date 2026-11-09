package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCorrectionPreservesReceiptsWithoutReplayingTools(t *testing.T) {
	for _, correction := range []string{`{"role":"assistant","content":"Saved."}`, `{"role":"assistant","content":null}`, `{"role":"assistant","tool_calls":[{"id":"again","type":"function","function":{"name":"save","arguments":"{}"}}]}`} {
		t.Run(correction, func(t *testing.T) {
			writes, requests, rejected := 0, 0, 0
			ctx := WithTools(context.Background(), []Tool{{Name: "save", Execute: func(context.Context, json.RawMessage) (string, error) {
				writes++
				return "saved receipt", nil
			}}})
			s := NewSession("thread")
			out, err := s.Turn(ctx, func(ctx context.Context, messages []Message) (Message, error) {
				requests++
				switch requests {
				case 1:
					return decode(t, `{"role":"assistant","tool_calls":[{"id":"save","type":"function","function":{"name":"save","arguments":"{}"}}]}`), nil
				case 2:
					return decode(t, `{"role":"assistant","content":null}`), nil
				case 3:
					data, _ := json.Marshal(messages)
					if ctx.Value(textOnlyKey{}) != true || !strings.Contains(string(data), "saved receipt") || !strings.Contains(string(data), "empty model answer") {
						t.Fatal("correction lost receipts, reason, or tool restriction")
					}
					return decode(t, correction), nil
				default:
					t.Fatal("unbounded correction")
					return nil, nil
				}
			}, nil, "save", func(e ToolEvent) {
				if e.Name == "model_validation" {
					rejected++
				}
			})
			if writes != 1 || requests != 3 || rejected == 0 {
				t.Fatal(writes, requests, rejected)
			}
			if strings.Contains(correction, "Saved.") {
				if err != nil || out.Body != "Saved." {
					t.Fatal(out, err)
				}
			} else {
				var validation ValidationError
				if !errors.As(err, &validation) {
					t.Fatal("missing safe reason", err)
				}
			}
			data, _ := json.Marshal(s.Snapshot())
			if !strings.Contains(string(data), "saved receipt") {
				t.Fatal("receipt lost after correction")
			}
		})
	}
}

func TestValidationCorrectionRespectsStepLimit(t *testing.T) {
	requests := 0
	_, err := NewSession("thread").Turn(WithLimits(context.Background(), Limits{MaxModelSteps: 1}), func(context.Context, []Message) (Message, error) {
		requests++
		return decode(t, `{"role":"assistant","content":null}`), nil
	}, nil, "input")
	if err == nil || requests != 2 {
		t.Fatal(requests, err)
	}
}

func TestResponseValidationReasons(t *testing.T) {
	for _, tc := range []struct{ response, reason string }{
		{`{"role":"user","content":"private text"}`, "invalid assistant role"},
		{`{"role":"assistant","tool_calls":"private text"}`, "invalid tool calls"},
		{`{"role":"assistant","tool_calls":[{"id":"a","type":"function"},{"id":"a","type":"function"}]}`, "invalid tool call identity"},
		{`{"role":"assistant","content":null}`, "empty model answer"},
	} {
		err := validateResponse(decode(t, tc.response))
		if err == nil || err.Error() != tc.reason {
			t.Fatal(err, tc.reason)
		}
	}
}

func TestProviderFailureDoesNotTriggerResponseCorrection(t *testing.T) {
	requests := 0
	want := errors.New("provider failure")
	_, err := NewSession("thread").Turn(context.Background(), func(context.Context, []Message) (Message, error) {
		requests++
		return nil, want
	}, nil, "input")
	if err != want || requests != 1 {
		t.Fatal(requests, err)
	}
}
