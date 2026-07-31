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

func TestSameOriginRejectsMissingAndCrossSiteSignals(t *testing.T) {
	tests := []struct {
		name, requestURL, origin, fetchSite string
		want                                bool
	}{
		{name: "matching origin", origin: "https://auth.example", want: true},
		{name: "different origin", origin: "https://evil.example", want: false},
		{name: "scheme mismatch", origin: "http://auth.example", want: false},
		{name: "same-origin fetch metadata", fetchSite: "same-origin", want: true},
		{name: "same-origin metadata survives proxy host rewrite", requestURL: "https://internal.example/signup", origin: "https://auth.example", fetchSite: "same-origin", want: true},
		{name: "missing browser evidence", want: false},
		{name: "cross-site fetch metadata", fetchSite: "cross-site", want: false},
		{name: "cross-site metadata overrides a matching origin", origin: "https://auth.example", fetchSite: "cross-site", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestURL := test.requestURL
			if requestURL == "" {
				requestURL = "https://auth.example/signup"
			}
			request := httptest.NewRequest(http.MethodPost, requestURL, nil)
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Sec-Fetch-Site", test.fetchSite)
			if got := sameOrigin(request); got != test.want {
				t.Fatalf("sameOrigin = %v, want %v", got, test.want)
			}
		})
	}
}
