package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestLoginPageAutofocusesEmail(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := response.Body.String(); !strings.Contains(body, `name="email" autocomplete="email" autofocus`) {
		t.Fatalf("login page email input does not have autofocus: %q", body)
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

func TestHandlerDoesNotExposeAccountDataSync(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/data/sync", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	response := httptest.NewRecorder()
	setSessionCookie(response, "opaque", true)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != secureSessionCookieName || cookie.Value != "opaque" || cookie.Path != "/" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Expires.Unix() > 0 || cookie.MaxAge != 0 {
		t.Fatalf("session cookie = %+v", cookie)
	}
}

func TestSessionCookieRejectsDuplicateValues(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://auth.example/account", nil)
	request.Header.Set("Cookie", secureSessionCookieName+"=first; "+secureSessionCookieName+"=second")
	if _, err := sessionCookie(request, true); err != errAmbiguousSessionCookie {
		t.Fatalf("sessionCookie error = %v, want %v", err, errAmbiguousSessionCookie)
	}
}

func TestSameOriginRejectsMissingAndCrossSiteSignals(t *testing.T) {
	expected, err := url.Parse("https://auth.example")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, requestURL, origin, fetchSite string
		want                                bool
	}{
		{name: "matching origin", origin: "https://auth.example", want: true},
		{name: "different origin", origin: "https://evil.example", want: false},
		{name: "scheme mismatch", origin: "http://auth.example", want: false},
		{name: "opaque origin", origin: "null", fetchSite: "same-origin", want: false},
		{name: "origin with path", origin: "https://auth.example/forged", fetchSite: "same-origin", want: false},
		{name: "same-origin fetch metadata supplements origin", origin: "https://auth.example", fetchSite: "same-origin", want: true},
		{name: "same-origin metadata without origin", fetchSite: "same-origin", want: false},
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
			if got := sameOrigin(request, expected); got != test.want {
				t.Fatalf("sameOrigin = %v, want %v", got, test.want)
			}
		})
	}
}

func TestHandlerRejectsNonCanonicalHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://alias.example/", nil)
	response := httptest.NewRecorder()

	Handler(Dependencies{PublicURL: "https://auth.example"}).ServeHTTP(response, request)

	if response.Code != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMisdirectedRequest)
	}
}

func TestHandlerAcceptsCanonicalHostWithImplicitDefaultPort(t *testing.T) {
	tests := []struct {
		name, publicURL, requestURL string
	}{
		{name: "HTTPS", publicURL: "https://auth.example", requestURL: "https://auth.example/"},
		{name: "HTTP", publicURL: "http://localhost", requestURL: "http://localhost/"},
		{name: "explicit HTTPS default", publicURL: "https://auth.example:443", requestURL: "https://auth.example/"},
		{name: "explicit HTTP default", publicURL: "http://localhost:80", requestURL: "http://localhost/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.requestURL, nil)
			response := httptest.NewRecorder()

			Handler(Dependencies{PublicURL: test.publicURL}).ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
		})
	}
}
