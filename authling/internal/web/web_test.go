package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRendersHomePageWithoutScripts(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Identity, under your control.") {
		t.Fatalf("body does not contain the Authling heading: %q", body)
	}
	if strings.Contains(body, "<script") {
		t.Fatalf("body unexpectedly contains a script: %q", body)
	}
	if got := response.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") {
		t.Fatalf("Content-Security-Policy = %q, want fail-closed default", got)
	}
}

func TestHandlerServesEmbeddedStylesheet(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
		t.Fatalf("Content-Type = %q, want text/css", got)
	}
	if response.Body.Len() == 0 {
		t.Fatal("embedded stylesheet is empty")
	}
}
