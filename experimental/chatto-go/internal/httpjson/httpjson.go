// Package httpjson sends bounded Connect JSON requests without exposing private response bodies.
package httpjson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// StatusError retains the response status without its potentially private body.
type StatusError struct{ Code int }

func (e StatusError) Error() string { return fmt.Sprintf("HTTP status %d", e.Code) }

// Post sends one request. It does not retry requests or follow redirects.
// Redirects could disclose conversation data or credentials to another service.
func Post(ctx context.Context, endpoint, token string, input, output any) error {
	return PostAtCursor(ctx, endpoint, token, "", input, output)
}

// PostAtCursor fences a resource read at an accepted realtime boundary.
func PostAtCursor(ctx context.Context, endpoint, token, cursor string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return errors.New("encode request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid request URL")
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Connect-Protocol-Version", "1")
	if cursor != "" {
		req.Header.Set("Chatto-Realtime-Minimum-Cursor", cursor)
	}
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("HTTP request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return StatusError{Code: res.StatusCode}
	}
	const maxBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return errors.New("read response")
	}
	if len(data) > maxBytes {
		return errors.New("response exceeds size limit")
	}
	if json.Unmarshal(data, output) != nil {
		return errors.New("invalid JSON response")
	}
	return nil
}
