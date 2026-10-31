package agent

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelFailuresPreserveSafeDiagnostics(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		status     int
		body, want string
	}{
		{400, `{"error":{"message":"private request content"}}`, "Model HTTP status 400 (Bad Request)"},
		{402, `private account details`, "Model HTTP status 402 (Payment Required)"},
		{429, `private account details`, "Model HTTP status 429 (Too Many Requests)"},
		{503, `private provider details`, "Model HTTP status 503 (Service Unavailable)"},
		{200, `private invalid JSON`, "Invalid model JSON response"},
		{200, `{"error":{"code":400,"message":"private request content"}}`, "Model API error 400"},
		{200, `{"error":{"code":"private code"}}`, "Model API returned an error"},
		{200, `{"choices":[{"finish_reason":"content_filter"}]}`, "Model completion stopped by content filter"},
	} {
		calls := 0
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		_, err := OpenRouter("private-token", "test/model", "session", nil)(context.Background(), nil)
		if err == nil || ModelFailureDetail(err) != tc.want || calls != 1 {
			t.Fatalf("wrong diagnostic or unexpected retry: %v", err)
		}
		if strings.Contains(err.Error(), "private") {
			t.Fatal("private contents leaked")
		}
	}
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private transport details") })
	_, err := OpenRouter("private-token", "test/model", "session", nil)(context.Background(), nil)
	if !strings.Contains(ModelFailureDetail(err), "failed before response headers") {
		t.Fatal("transport classification lost")
	}
	if ModelFailureDetail(errors.New("private adapter error")) != "Model request failed" {
		t.Fatal("unknown adapter error leaked")
	}
}

type brokenResponseBody struct {
	err  error
	read bool
}

func (b *brokenResponseBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, b.err
	}
	b.read = true
	return copy(p, []byte("private-payload")), b.err
}
func (*brokenResponseBody) Close() error { return nil }

func TestBodyReadFailuresKeepSafeEvidence(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		err  error
		want string
	}{
		{io.ErrUnexpectedEOF, "truncated body (unexpected EOF)"},
		{gzip.ErrChecksum, "invalid gzip checksum"},
		{errors.New("private error with token and IP"), "unclassified transport error"},
	} {
		calls := 0
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, ProtoMajor: 2, ProtoMinor: 0, ContentLength: 100, Header: http.Header{"Content-Encoding": []string{"private-header"}}, Body: &brokenResponseBody{err: tc.err}}, nil
		})
		_, err := OpenRouter("private-token", "test/model", "session", nil)(context.Background(), nil)
		detail := ModelFailureDetail(err)
		for _, want := range []string{tc.want, "HTTP 200", "protocol 2.0", "bytes read 15", "content length 100", "encoding other"} {
			if !strings.Contains(detail, want) {
				t.Fatalf("missing %q in %q", want, detail)
			}
		}
		if calls != 1 || strings.Contains(detail, "private") {
			t.Fatal("retry or private data leak")
		}
	}
}

func TestRealHTTPResponseInterruptions(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, http2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "truncated HTTP1", true: "reset HTTP2"}[http2], func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !http2 {
					w.Header().Set("Content-Length", "100")
				}
				w.WriteHeader(200)
				w.Write([]byte("partial"))
				w.(http.Flusher).Flush()
				if http2 {
					panic(http.ErrAbortHandler)
				}
			}))
			server.EnableHTTP2 = http2
			server.StartTLS()
			defer server.Close()
			http.DefaultTransport = server.Client().Transport
			var output any
			err := postJSON(context.Background(), server.URL, "test", map[string]string{}, &output)
			detail := ModelFailureDetail(err)
			want := "truncated body (unexpected EOF)"
			if http2 {
				want = "HTTP/2 stream reset (code 2)"
			}
			if !strings.Contains(detail, want) || !strings.Contains(detail, "HTTP 200") || strings.Contains(detail, server.URL) {
				t.Fatalf("lost real transport diagnostic: %s", detail)
			}
		})
	}
}
