package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
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

func TestAccountSyncAuthorizationMonitorExpiresIdleConnection(t *testing.T) {
	expired := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go monitorAccountSyncAuthorization(ctx, time.Millisecond, func(context.Context, bool) bool {
		return false
	}, func() {
		close(expired)
	})
	select {
	case <-expired:
	case <-time.After(time.Second):
		t.Fatal("idle connection authorization was not checked")
	}
}

func TestAccountSyncAuthenticationAdmissionIsFairAcrossNetworkSources(t *testing.T) {
	admission := newAccountSyncAuthenticationAdmission()
	for attempt := range maxPendingAccountSyncAuthPerSource {
		if !admission.acquire("192.0.2.1") {
			t.Fatalf("source admission %d was rejected", attempt)
		}
	}
	if admission.acquire("192.0.2.1") {
		t.Fatal("source above its pending limit was accepted")
	}
	if !admission.acquire("198.51.100.2") {
		t.Fatal("one saturated source excluded another source")
	}
	admission.release("192.0.2.1")
	if !admission.acquire("192.0.2.1") {
		t.Fatal("source did not recover after one authentication ended")
	}
}

func TestAccountSyncNetworkSourceUsesForwardingOnlyFromTrustedProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://auth.example/data/sync", nil)
	request.RemoteAddr = "192.0.2.10:54321"
	request.Header.Set("X-Forwarded-For", "198.51.100.20")
	if got := accountSyncNetworkSource(request, nil); got != "192.0.2.10" {
		t.Fatalf("network source = %q, want direct peer address", got)
	}
	trusted := []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
	if got := accountSyncNetworkSource(request, trusted); got != "198.51.100.20" {
		t.Fatalf("trusted-proxy source = %q, want forwarded client address", got)
	}
	request.Header.Set("X-Forwarded-For", "198.51.100.20, 192.0.2.10")
	if got := accountSyncNetworkSource(request, trusted); got != "192.0.2.10" {
		t.Fatalf("ambiguous forwarded source = %q, want direct peer address", got)
	}
}

func TestTrustedProxyClientsReceiveIndependentAuthenticationAdmission(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}
	request := func(client string) *http.Request {
		candidate := httptest.NewRequest(http.MethodGet, "https://auth.example/data/sync", nil)
		candidate.RemoteAddr = "192.0.2.10:54321"
		candidate.Header.Set("X-Forwarded-For", client)
		return candidate
	}
	admission := newAccountSyncAuthenticationAdmission()
	attacker := accountSyncNetworkSource(request("198.51.100.1"), trusted)
	for range maxPendingAccountSyncAuthPerSource {
		if !admission.acquire(attacker) {
			t.Fatal("attacker allowance ended early")
		}
	}
	legitimate := accountSyncNetworkSource(request("203.0.113.2"), trusted)
	if !admission.acquire(legitimate) {
		t.Fatal("one forwarded client excluded another forwarded client")
	}
}

func TestAccountSyncRequestOriginAllowsHTTPSAndLoopbackDevelopment(t *testing.T) {
	tests := []struct {
		origin string
		want   bool
	}{
		{origin: "https://client.example", want: true},
		{origin: "http://localhost:5173", want: true},
		{origin: "http://app.localhost:5173", want: true},
		{origin: "http://127.0.0.1:5173", want: true},
		{origin: "http://client.example"},
		{origin: "https://client.example/path"},
		{origin: "null"},
		{origin: "file://client"},
	}
	for _, test := range tests {
		t.Run(test.origin, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://auth.example/data/sync", nil)
			request.Header.Set("Origin", test.origin)
			got, ok := accountSyncRequestOrigin(request)
			if ok != test.want || ok && got != test.origin {
				t.Fatalf("accountSyncRequestOrigin = %q, %v; want %q, %v", got, ok, test.origin, test.want)
			}
		})
	}
}

func TestDecodeAccountSyncAuthenticationRejectsInvalidEnvelopes(t *testing.T) {
	valid := `{"type":"authenticate","access_token":"opaque"}`
	if got, ok := decodeAccountSyncAuthentication([]byte(valid)); !ok || got.AccessToken != "opaque" {
		t.Fatalf("valid authentication = %+v, %v", got, ok)
	}
	for _, invalid := range []string{
		`{}`,
		`{"type":"other","access_token":"opaque"}`,
		`{"type":"authenticate","access_token":""}`,
		`{"type":"authenticate","access_token":"opaque","extra":true}`,
		valid + `{}`,
		`["authenticate","opaque"]`,
	} {
		if _, ok := decodeAccountSyncAuthentication([]byte(invalid)); ok {
			t.Fatalf("invalid authentication %s was accepted", invalid)
		}
	}
}
