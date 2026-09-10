package http_server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestSetupBlocksRegistrationBeforeValidationAndMail(t *testing.T) {
	s := setupHTTPServerTestServer(t, config.AuthConfig{})
	s.setupAuthRoutes()
	for _, path := range []string{"/auth/register", "/auth/register/verify-code", "/auth/register/complete", "/auth/browser/register/complete"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			s.router.ServeHTTP(response, request)
			if response.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409: %s", response.Code, response.Body.String())
			}
		})
	}
}
