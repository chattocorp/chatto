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
	body := response.Body.String()
	if !strings.Contains(body, `name="email" autocomplete="email" autofocus`) {
		t.Fatalf("login page email input does not have autofocus: %q", body)
	}
	if !strings.Contains(body, `href="/password-reset"`) || !strings.Contains(body, "Forgot your password?") {
		t.Fatalf("login page does not link to password reset: %q", body)
	}
}

func TestPasswordResetPageRendersWithoutAccountDisclosure(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/password-reset", nil)
	response := httptest.NewRecorder()

	Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "Reset your password") || !strings.Contains(body, `action="/password-reset"`) {
		t.Fatalf("body does not contain the password reset form: %q", body)
	}
	if strings.Contains(body, "account exists") || strings.Contains(body, "account not found") || strings.Contains(body, "<script") {
		t.Fatalf("password reset page has disclosure or script content: %q", body)
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

func TestHandlerUsesForwardedOriginOnlyWhenExplicitlyTrusted(t *testing.T) {
	for _, test := range []struct {
		name  string
		trust bool
		want  int
	}{
		{name: "disabled", want: http.StatusMisdirectedRequest},
		{name: "enabled", trust: true, want: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
			request.Header.Set("X-Forwarded-Host", "auth.example:42443")
			request.Header.Set("X-Forwarded-Proto", "https")
			response := httptest.NewRecorder()

			Handler(Dependencies{
				PublicURL: "https://auth.example:42443", TrustProxyHeaders: test.trust,
			}).ServeHTTP(response, request)

			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestHandlerRejectsMalformedTrustedProxyOrigins(t *testing.T) {
	tests := []struct {
		name   string
		hosts  []string
		protos []string
	}{
		{name: "missing host", protos: []string{"https"}},
		{name: "missing protocol", hosts: []string{"auth.example:42443"}},
		{name: "unsupported protocol", hosts: []string{"auth.example:42443"}, protos: []string{"ftp"}},
		{name: "forwarded host chain", hosts: []string{"attacker.example", "auth.example:42443"}, protos: []string{"https"}},
		{name: "forwarded protocol chain", hosts: []string{"auth.example:42443"}, protos: []string{"http", "https"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)
			for _, host := range test.hosts {
				request.Header.Add("X-Forwarded-Host", host)
			}
			for _, protocol := range test.protos {
				request.Header.Add("X-Forwarded-Proto", protocol)
			}
			response := httptest.NewRecorder()

			Handler(Dependencies{
				PublicURL: "https://auth.example:42443", TrustProxyHeaders: true,
			}).ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
		})
	}
}
