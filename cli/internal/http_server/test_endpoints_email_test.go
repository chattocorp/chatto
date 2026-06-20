//go:build test_endpoints

package http_server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func TestAuthRoutes_TestEmailEndpoint(t *testing.T) {
	ts, client, _, mockMailer := setupTestHTTPServerWithMailer(t)

	// Trigger a registration email
	reqBody := map[string]string{"email": "testendpoint@example.com"}
	body, _ := json.Marshal(reqBody)

	resp, err := client.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to send register request: %v", err)
	}
	resp.Body.Close()

	// Verify email was captured
	if mockMailer.LastMessage() == nil {
		t.Fatal("Expected email to be captured")
	}

	// Test the /auth/test/last-email endpoint
	emailResp, err := client.Get(ts.URL + "/auth/test/last-email")
	if err != nil {
		t.Fatalf("Failed to get last email: %v", err)
	}
	defer emailResp.Body.Close()

	if emailResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", emailResp.StatusCode)
	}

	var emailResult map[string]interface{}
	if err := json.NewDecoder(emailResp.Body).Decode(&emailResult); err != nil {
		t.Fatalf("Failed to decode email response: %v", err)
	}

	if emailResult["to"] != "testendpoint@example.com" {
		t.Errorf("Expected to: testendpoint@example.com, got %v", emailResult["to"])
	}
	if emailResult["subject"] != "Complete your registration for Chatto" {
		t.Errorf("Expected subject: 'Complete your registration for Chatto', got %v", emailResult["subject"])
	}

	// Test DELETE /auth/test/emails
	req, _ := http.NewRequest("DELETE", ts.URL+"/auth/test/emails", nil)
	deleteResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete emails: %v", err)
	}
	deleteResp.Body.Close()

	if deleteResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", deleteResp.StatusCode)
	}

	if mockMailer.LastMessage() != nil {
		t.Error("Expected emails to be cleared")
	}
}
