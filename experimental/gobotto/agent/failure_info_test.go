package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type syntheticTimeout struct{}

func (syntheticTimeout) Error() string { return "private URL and credentials" }
func (syntheticTimeout) Timeout() bool { return true }

func TestRequestFailureClassificationAndPrivacy(t *testing.T) {
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	for _, tc := range []struct {
		status int
		retry  bool
	}{{400, false}, {401, false}, {402, false}, {429, true}, {503, true}} {
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("private provider error"))}, nil
		})
		_, err := OpenRouter("private", "model", "session", nil)(context.Background(), nil)
		info := ModelFailureInfo(err)
		if info.Retryable != tc.retry || strings.Contains(info.Detail, "private") {
			t.Fatal(info)
		}
	}
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, syntheticTimeout{} })
	_, err := OpenRouter("private", "model", "session", nil)(context.Background(), nil)
	info := ModelFailureInfo(err)
	if info.Code != "request_timeout" || !info.Retryable || strings.Contains(info.Detail, "private") {
		t.Fatal(info)
	}
	if ModelFailureInfo(context.Canceled).Retryable || ModelFailureInfo(ErrOutputLimit).Retryable {
		t.Fatal("nonretryable failure classified as transient")
	}
}

func TestRequestTimeoutOverride(t *testing.T) {
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	for _, duration := range []time.Duration{0, 3 * time.Minute} {
		http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			if duration == 0 && ok {
				t.Error("zero did not disable HTTP deadline")
			}
			if duration > 0 && (!ok || time.Until(deadline) < duration-time.Second) {
				t.Error("request deadline not applied")
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Done"}}]}`))}, nil
		})
		if _, err := OpenRouter("test", "model", "session", nil)(WithRequestTimeout(context.Background(), duration), nil); err != nil {
			t.Fatal(err)
		}
	}
}
