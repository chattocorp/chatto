//go:build !test_endpoints

package http_server

import (
	"net/http"
	"testing"
)

func TestAuthTestEndpointsUnavailableWithoutTag(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)

	if mockMailer.LastMessage() != nil {
		t.Fatal("expected test mailer to start empty")
	}

	resp, err := client.Get(ts.URL + "/auth/test/last-email")
	if err != nil {
		t.Fatalf("Failed to get last email: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", resp.StatusCode)
	}

	deadlineResp, err := client.Post(ts.URL+"/auth/test/privileged-mode-deadline", "application/json", nil)
	if err != nil {
		t.Fatalf("post privileged-mode test endpoint: %v", err)
	}
	defer deadlineResp.Body.Close()
	if deadlineResp.StatusCode != http.StatusNotFound {
		t.Fatalf("privileged-mode test endpoint status = %d, want 404", deadlineResp.StatusCode)
	}
}
