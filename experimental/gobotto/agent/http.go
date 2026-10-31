package agent

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"syscall"
	"time"
)

// modelFailure contains only host-authored diagnostics, never provider text,
// credentials, request URLs, or a wrapped transport error.
type modelFailure string

func (e modelFailure) Error() string { return string(e) }

// transportClass inspects error identities and numeric protocol codes only.
// Error() strings can contain credentials, hostnames, IPs, and response data.
func transportClass(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline exceeded"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "truncated body (unexpected EOF)"
	case errors.Is(err, io.EOF):
		return "connection closed (EOF)"
	case errors.Is(err, gzip.ErrHeader):
		return "invalid gzip header"
	case errors.Is(err, gzip.ErrChecksum):
		return "invalid gzip checksum"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection reset by peer"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused"
	case errors.Is(err, syscall.EPIPE):
		return "broken pipe"
	case errors.Is(err, net.ErrClosed):
		return "connection closed locally"
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	var corrupt flate.CorruptInputError
	if errors.As(err, &corrupt) {
		return "invalid compressed data"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "DNS lookup failed"
	}
	// net/http bundles its own unexported HTTP/2 error types. Match only known
	// library types and read numeric codes, never their potentially private Cause.
	for e, depth := err, 0; e != nil && depth < 8; e, depth = errors.Unwrap(e), depth+1 {
		v := reflect.ValueOf(e)
		if v.Kind() == reflect.Pointer {
			v = v.Elem()
		}
		if !v.IsValid() {
			continue
		}
		t := v.Type()
		if t.PkgPath() != "net/http" && t.PkgPath() != "net/http/internal/http2" && t.PkgPath() != "golang.org/x/net/http2" {
			continue
		}
		switch t.Name() {
		case "http2StreamError", "StreamError":
			code := v.FieldByName("Code")
			if code.IsValid() && code.Kind() == reflect.Uint32 {
				return fmt.Sprintf("HTTP/2 stream reset (code %d)", code.Uint())
			}
		case "http2ConnectionError", "ConnectionError":
			if v.Kind() == reflect.Uint32 {
				return fmt.Sprintf("HTTP/2 connection error (code %d)", v.Uint())
			}
		case "http2GoAwayError", "GoAwayError":
			code := v.FieldByName("ErrCode")
			if code.IsValid() && code.Kind() == reflect.Uint32 {
				return fmt.Sprintf("HTTP/2 GOAWAY (code %d)", code.Uint())
			}
		}
	}
	return fmt.Sprintf("unclassified transport error (%T)", err)
}

func responseFailure(res *http.Response, bytesRead int, reason string) error {
	encoding := res.Header.Get("Content-Encoding")
	switch encoding {
	case "", "gzip", "br", "deflate", "zstd":
	default:
		encoding = "other"
	}
	if encoding == "" {
		encoding = "none"
	}
	return modelFailure(fmt.Sprintf("Model response read failed: %s; HTTP %d; protocol %d.%d; bytes read %d; content length %d; encoding %s; decompressed %t", reason, res.StatusCode, res.ProtoMajor, res.ProtoMinor, bytesRead, res.ContentLength, encoding, res.Uncompressed))
}

// ModelFailureDetail returns a diagnostic safe for operational logs and private
// inspection. Unknown errors from injected adapters retain a generic message.
func ModelFailureDetail(err error) string {
	if errors.Is(err, ErrOutputLimit) {
		return ErrOutputLimit.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Model request deadline exceeded"
	}
	if errors.Is(err, context.Canceled) {
		return "Model request cancelled"
	}
	var failure modelFailure
	if errors.As(err, &failure) {
		return failure.Error()
	}
	return "Model request failed"
}

// postJSON sends one model request without retries or redirects. It omits
// private provider response bodies and URLs from errors that callers may log.
func postJSON(ctx context.Context, endpoint, token string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return modelFailure("Cannot encode model request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return modelFailure("Invalid model request URL")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return modelFailure("Model HTTP request failed before response headers: " + transportClass(err))
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return modelFailure(fmt.Sprintf("Model HTTP status %d (%s)", res.StatusCode, http.StatusText(res.StatusCode)))
	}
	const maxBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return responseFailure(res, len(data), transportClass(err))
	}
	if len(data) > maxBytes {
		return modelFailure("Model response exceeds size limit")
	}
	if json.Unmarshal(data, output) != nil {
		return modelFailure("Invalid model JSON response")
	}
	return nil
}
