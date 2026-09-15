package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/email"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/web"
)

func TestRecoveryAdmissionSurvivesDeliveryFailureAndRestart(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "existing"}[exists], func(t *testing.T) {
			cfg := embeddedTestConfig(t)
			sender := &inspectingSender{inspect: func(email.Message) error { return errors.New("injected SMTP outage") }}
			runtime, cancel, runErrors := startTestRuntime(t, cfg, sender)
			if exists {
				if _, err := runtime.Accounts.CreateLocal(t.Context(), "review@example.invalid", "a deliberately uncommon review password"); err != nil {
					t.Fatal(err)
				}
			}
			before := eventCount(t, runtime)
			for range 5 {
				if _, err := runtime.PasswordReset.Start(t.Context(), "review@example.invalid"); err == nil || errors.Is(err, storage.ErrAdmissionLimited) {
					t.Fatalf("initial delivery: %v", err)
				}
			}
			stopTestRuntime(t, runtime, cancel, runErrors)
			runtime, cancel, runErrors = startTestRuntime(t, cfg, sender)
			defer stopTestRuntime(t, runtime, cancel, runErrors)
			for range 5 {
				if _, err := runtime.PasswordReset.Start(t.Context(), "review@example.invalid"); err == nil || errors.Is(err, storage.ErrAdmissionLimited) {
					t.Fatalf("delivery after restart: %v", err)
				}
			}
			for range 5 {
				if _, err := runtime.PasswordReset.Start(t.Context(), "review@example.invalid"); !errors.Is(err, storage.ErrAdmissionLimited) {
					t.Fatalf("failed deliveries refunded admission: %v", err)
				}
			}
			want := uint64(0)
			if exists {
				want = 10
			}
			if added := eventCount(t, runtime) - before; added != want {
				t.Fatalf("added %d events, want %d", added, want)
			}
		})
	}
}

func betaOIDCConfig(t *testing.T) config.Config {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	cfg.OIDC.Clients = []config.OIDCClientConfig{{ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"}}}
	return cfg
}

func betaHandler(runtime *Runtime, cfg config.Config) http.Handler {
	return web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, Authorizations: runtime.Authorizations, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURLOrDefault()})
}

func betaAuthorizationURL() string {
	return "http://localhost:8080/oauth/authorize?" + url.Values{
		"client_id": {"test-client"}, "redirect_uri": {"http://localhost:9999/callback"},
		"response_type": {"code"}, "scope": {"openid"}, "state": {"review-state"},
		"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"},
	}.Encode()
}

func TestOIDCAdmissionLimitsStateAcrossRestart(t *testing.T) {
	cfg := betaOIDCConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	js, err := jetstream.New(runtime.connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.KeyValue(t.Context(), storage.RuntimeStateBucket)
	if err != nil {
		t.Fatal(err)
	}
	// Exhaust all but one admission without creating 999 irrelevant requests.
	if _, err := kv.Create(t.Context(), "oidc.admission.global", []byte(`{"count":999}`), jetstream.KeyTTL(10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	betaHandler(runtime, cfg).ServeHTTP(response, httptest.NewRequest(http.MethodGet, betaAuthorizationURL(), nil))
	if !strings.HasPrefix(response.Header().Get("Location"), "/oidc/consent?id=") {
		t.Fatalf("last admission did not create pending request: %d", response.Code)
	}
	pending := response.Header().Get("Location")
	stopTestRuntime(t, runtime, cancel, runErrors)
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	for range 3 {
		response = httptest.NewRecorder()
		betaHandler(runtime, cfg).ServeHTTP(response, httptest.NewRequest(http.MethodGet, betaAuthorizationURL(), nil))
		if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "600" {
			t.Fatalf("exhausted budget status: %d", response.Code)
		}
	}
	response = httptest.NewRecorder()
	betaHandler(runtime, cfg).ServeHTTP(response, httptest.NewRequest(http.MethodGet, cfg.HTTP.PublicURLOrDefault()+pending, nil))
	if response.Code != http.StatusSeeOther || !strings.HasPrefix(response.Header().Get("Location"), "/login?id=") {
		t.Fatal("admission exhaustion blocked an existing request")
	}
	js, _ = jetstream.New(runtime.connection.NATS)
	kv, _ = js.KeyValue(t.Context(), storage.RuntimeStateBucket)
	keys, err := kv.Keys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	for _, key := range keys {
		if strings.HasPrefix(key, "oidc.request.") {
			requests++
		}
	}
	if requests != 1 {
		t.Fatalf("stored %d requests, want 1", requests)
	}
	if _, err := kv.Put(t.Context(), "oidc.admission.global", []byte(`{"count":-1}`)); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	betaHandler(runtime, cfg).ServeHTTP(response, httptest.NewRequest(http.MethodGet, strings.Replace(betaAuthorizationURL(), "test-client", "unknown-client", 1), nil))
	if response.Code != http.StatusServiceUnavailable || response.Header().Get("Location") != "" {
		t.Fatal("unavailable admission state did not reject before client lookup")
	}
}

func TestOIDCSilentAuthorizationNeverShowsInteractivePages(t *testing.T) {
	cfg := betaOIDCConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	account, err := runtime.Accounts.CreateLocal(t.Context(), "silent@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := runtime.Sessions.Create(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "authling_session", Value: token}
	check := func(start string, cookie *http.Cookie, want string) {
		t.Helper()
		handler := betaHandler(runtime, cfg)
		target := start
		for range 5 {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			if cookie != nil {
				req.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code < 300 || response.Code >= 400 {
				t.Fatalf("silent request rendered a page: %d", response.Code)
			}
			target = response.Header().Get("Location")
			parsed, err := url.Parse(target)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Host == "localhost:9999" {
				if parsed.Query().Get("state") != "review-state" {
					t.Fatal("lost state")
				}
				if want == "code" {
					if parsed.Query().Get("code") == "" || parsed.Query().Get("error") != "" {
						t.Fatal("silent grant reuse failed")
					}
				} else if parsed.Query().Get("error") != want {
					t.Fatalf("error = %q, want %q", parsed.Query().Get("error"), want)
				}
				return
			}
			if strings.HasPrefix(parsed.Path, "/login") {
				t.Fatal("silent request redirected to login")
			}
		}
		t.Fatal("silent request did not finish")
	}
	silent := betaAuthorizationURL() + "&prompt=none"
	check(silent, nil, "login_required")
	check(silent, cookie, "consent_required")
	grant, err := runtime.Authorizations.Authorize(t.Context(), account.ID, authorizations.Client{ID: "test-client", Name: "Test Client", Host: "configured by this Authling operator"}, []string{"openid"})
	if err != nil {
		t.Fatal(err)
	}
	check(silent, cookie, "code")
	check(silent+"&max_age=0", cookie, "login_required")
	// Persist a pending silent request before restarting its serving process.
	response := httptest.NewRecorder()
	betaHandler(runtime, cfg).ServeHTTP(response, httptest.NewRequest(http.MethodGet, silent, nil))
	pending := response.Header().Get("Location")
	stopTestRuntime(t, runtime, cancel, runErrors)
	runtime, cancel, runErrors = startTestRuntime(t, cfg)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	check(pending, cookie, "code")
	if err := runtime.Authorizations.Revoke(t.Context(), account.ID, grant.ID); err != nil {
		t.Fatal(err)
	}
	check(silent, cookie, "consent_required")
	check(strings.Replace(silent, "prompt=none", "prompt=none+login", 1), cookie, "invalid_request")
}

func TestSignupRejectsUnavailableOIDCRequestBeforeSideEffects(t *testing.T) {
	cfg := betaOIDCConfig(t)
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, cfg, sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	handler := web.Handler(web.Dependencies{Registration: runtime.Registration, Accounts: runtime.Accounts, Sessions: runtime.Sessions, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURLOrDefault()})
	before := eventCount(t, runtime)
	for _, path := range []string{"/signup", "/signup/verify", "/signup/complete"} {
		body := url.Values{"oidc_request": {"missing"}, "email": {"signup-review@example.invalid"}, "flow": {"invalid"}, "code": {"000000"}, "password": {"a deliberately uncommon password"}, "password_confirmation": {"a deliberately uncommon password"}}
		req := httptest.NewRequest(http.MethodPost, "http://localhost:8080"+path, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://localhost:8080")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", path, response.Code)
		}
	}
	if sender.count() != 0 || eventCount(t, runtime) != before {
		t.Fatal("invalid OIDC request caused signup effects")
	}
}
