package agent

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

// postJSON sends one model request without retries or redirects. It omits
// private provider response bodies and URLs from errors that callers may log.
func postJSON(ctx context.Context, endpoint, token string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return errors.New("encode model request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid model request URL")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("model HTTP request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("model HTTP status %d", res.StatusCode)
	}
	const maxBytes = 4 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil {
		return errors.New("read model response")
	}
	if len(data) > maxBytes {
		return errors.New("model response exceeds size limit")
	}
	if json.Unmarshal(data, output) != nil {
		return errors.New("invalid model JSON response")
	}
	return nil
}
