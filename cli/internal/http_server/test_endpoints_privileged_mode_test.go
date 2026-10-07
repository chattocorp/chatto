//go:build test_endpoints

package http_server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/internal/core"
)

func TestPrivilegedModeDeadlineEndpoint(t *testing.T) {
	env := setupWebSocketTestServer(t)
	registerPrivilegedModeDeadlineEndpoint(env.httpServer.router.Group("/auth"), env.httpServer)
	post := func(body string, wantStatus int) time.Time {
		t.Helper()
		response, err := env.client.Post(env.server.URL+"/auth/test/privileged-mode-deadline", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("post deadline: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != wantStatus {
			t.Fatalf("deadline status = %d, want %d", response.StatusCode, wantStatus)
		}
		if wantStatus != http.StatusOK {
			return time.Time{}
		}
		var result struct {
			ExpiresAt time.Time `json:"expiresAt"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatalf("decode deadline: %v", err)
		}
		return result.ExpiresAt
	}
	post(`{"remainingMs":5000}`, http.StatusUnauthorized)
	for _, body := range []string{`{}`, `{"remainingMs":0}`, `{"remainingMs":60001}`, `{"remainingMs":-1}`, `{`} {
		post(body, http.StatusBadRequest)
	}
	if _, err := env.core.CreateUser(env.ctx, core.SystemActorID, "deadline-test", "Deadline Test", "password123"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	env.login(t, "deadline-test", "password123")
	var sessionID string
	for _, cookie := range env.cookieJar.Cookies(mustParseURL(env.server.URL)) {
		if isBrowserSessionCookieName(cookie.Name) {
			sessionID = cookie.Value
			break
		}
	}
	if sessionID == "" {
		t.Fatal("login did not return a cookie session")
	}
	post(`{"remainingMs":5000}`, http.StatusConflict)
	if _, err := env.core.SetCookiePrivilegedMode(env.ctx, sessionID, true); err != nil {
		t.Fatalf("activate: %v", err)
	}
	deadline := post(`{"remainingMs":5000}`, http.StatusOK)
	if remaining := time.Until(deadline); remaining <= 0 || remaining > 5*time.Second {
		t.Fatalf("remaining = %v, want within (0, 5s]", remaining)
	}
	record, err := env.core.ValidateCookieCredential(env.ctx, sessionID)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if !record.GetPrivilegedModeExpiresAt().AsTime().Equal(deadline) {
		t.Fatal("stored deadline does not match the endpoint response")
	}
	post(`{"remainingMs":60000}`, http.StatusConflict)
	if _, err := env.core.SetCookiePrivilegedMode(env.ctx, sessionID, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	post(`{"remainingMs":5000}`, http.StatusConflict)
}
