//go:build bootstrap

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevelopmentDesignSystem(t *testing.T) {
	r := httptest.NewRecorder()
	Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/dev/ui", nil))
	if r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "Example users") {
		t.Fatal("development component preview is unavailable")
	}
	if r.Header().Get("Content-Security-Policy") != contentSecurityPolicy("") {
		t.Fatal("component preview must retain the shared content security policy")
	}
}
