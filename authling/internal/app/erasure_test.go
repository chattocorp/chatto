package app

import (
	"context"
	"errors"
	"hmans.de/authling/internal/logging"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/web"
)

func TestAccountErasureDeniesAccessReleasesEmailAndReplaysWithoutKeys(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	cfg.OIDC.Clients = []config.OIDCClientConfig{{ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"}}}
	first, cancel, runErrors := startTestRuntime(t, cfg)
	email := "oidc@example.com"
	const password = "a deliberately uncommon password"
	account, err := first.Accounts.CreateLocal(t.Context(), email, password)
	if err != nil {
		t.Fatal(err)
	}
	created, _ := lastAccountEvent(t, first, account.ID)
	refs := []string{created.GetAccountCreated().GetUserKeyRef(), created.GetAccountCreated().GetCredentialKeyRef()}
	if refs[0] == "" {
		t.Fatal("missing key refs")
	}
	emailTarget, err := first.Accounts.PrepareEmailChange(t.Context(), account.ID, password, "erasure-final@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	emailTarget, err = first.Accounts.RecordEmailChangeRequested(t.Context(), emailTarget)
	if err != nil {
		t.Fatal(err)
	}
	account, err = first.Accounts.ChangeEmail(t.Context(), emailTarget, "erasure-final@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	email = "erasure-final@example.invalid"

	if _, err := first.Accounts.UpdateProfile(t.Context(), account.ID, "erasure-person", "Erasure Person"); err != nil {
		t.Fatal(err)
	}
	reset, exists, err := first.Accounts.RecordPasswordResetRequested(t.Context(), email)
	if err != nil || !exists {
		t.Fatal(err)
	}
	change, err := first.Accounts.PreparePasswordChange(t.Context(), account.ID, password, "replacement uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := first.Sessions.Create(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "authling_session", Value: token}
	handler := web.Handler(web.Dependencies{Accounts: first.Accounts, Authentication: first.Authentication, Sessions: first.Sessions, Authorizations: first.Authorizations, OIDC: first.OIDC, PublicURL: cfg.HTTP.PublicURLOrDefault()})
	verifier := strings.Repeat("v", 43)
	code := completeAuthorization(t, handler, verifier, cookie)
	tokens := issueOIDCTokens(t, handler, code, verifier)
	pendingCode := completeAuthorization(t, handler, verifier, cookie)
	if err := first.Accounts.RequestErasure(t.Context(), account.ID, account.AuthenticationVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Sessions.Validate(t.Context(), token); err == nil {
		t.Fatal("deleted account session accepted")
	}
	if _, _, err := first.Sessions.CreateAtAuthenticationVersion(t.Context(), account.ID, account.AuthenticationVersion, time.Now()); err == nil {
		t.Fatal("stale proof created session")
	}
	if _, err := first.Accounts.Profile(t.Context(), account.ID); err == nil {
		t.Fatal("deleted profile readable")
	}
	if _, err := first.Accounts.UpdateProfile(t.Context(), account.ID, "resurrection", "Resurrection"); err == nil {
		t.Fatal("deleted profile updated")
	}
	if _, err := first.Accounts.ResetPassword(t.Context(), reset, "another uncommon password"); !errors.Is(err, accounts.ErrCredentialChanged) {
		t.Fatalf("reset: %v", err)
	}
	if _, err := first.Accounts.ChangePassword(t.Context(), change); !errors.Is(err, accounts.ErrCredentialChanged) {
		t.Fatalf("password change: %v", err)
	}
	if _, err := first.Authorizations.Authorize(t.Context(), account.ID, authorizations.Client{ID: "test-client", Name: "Test", Host: "localhost"}, []string{"openid"}); !errors.Is(err, authorizations.ErrNotFound) {
		t.Fatalf("authorize deleted: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/oauth/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code == http.StatusOK {
		t.Fatal("UserInfo accepted deleted account")
	}
	if response := redeemCode(t, handler, pendingCode, verifier); response.Code == http.StatusOK {
		t.Fatal("code exchange accepted deleted account")
	}
	// Calling completion concurrently with the durable worker is idempotent.
	if err := first.Erasure.Complete(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}
	js, _, err := storage.Open(t.Context(), first.connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if _, err := stores.Keys.Get(t.Context(), ref); !errors.Is(err, jetstream.ErrKeyNotFound) && !errors.Is(err, jetstream.ErrKeyDeleted) {
			t.Fatalf("key was not purged: %v", err)
		}
	}
	replacement, err := first.Accounts.CreateLocal(t.Context(), email, password)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.ID == account.ID {
		t.Fatal("deleted sub reused")
	}
	stopTestRuntime(t, first, cancel, runErrors)
	second, cancelSecond, secondErrors := startTestRuntime(t, cfg)
	defer stopTestRuntime(t, second, cancelSecond, secondErrors)
	if _, ok := second.Accounts.Get(account.ID); ok {
		t.Fatal("erased account replayed as active")
	}
	got, err := second.Accounts.AuthenticateLocal(t.Context(), email, password)
	if err != nil || got.ID != replacement.ID {
		t.Fatalf("replacement login after replay: %v", err)
	}
	if err := second.Erasure.Complete(t.Context(), account.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAccountDeletionHTTPRequiresPasswordConfirmationAndOrigin(t *testing.T) {
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t))
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	const password = "a deliberately uncommon password"
	account, err := runtime.Accounts.CreateLocal(t.Context(), "erase@example.invalid", password)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := runtime.Sessions.Create(t.Context(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "authling_session", Value: token}
	handler := web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Sessions: runtime.Sessions, PublicURL: "http://localhost:8080"})
	for _, tc := range []struct {
		name, origin, password, confirm string
		status                          int
	}{
		{"cross origin", "https://evil.example", password, "delete", http.StatusForbidden},
		{"no origin", "", password, "delete", http.StatusForbidden},
		{"no confirmation", "http://localhost:8080", password, "", http.StatusUnprocessableEntity},
		{"wrong password", "http://localhost:8080", "wrong password", "delete", http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/account/delete", strings.NewReader(url.Values{"password": {tc.password}, "confirm": {tc.confirm}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", tc.origin)
			req.AddCookie(cookie)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d", response.Code, tc.status)
			}
			if err := runtime.Accounts.RequireActive(t.Context(), account.ID); err != nil {
				t.Fatal("rejected form deleted account", err)
			}
		})
	}
	response := requestHandler(t, handler, http.MethodPost, "http://localhost:8080/account/delete", url.Values{"password": {password}, "confirm": {"delete"}}.Encode(), cookie)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Your account is closed") {
		t.Fatalf("delete response: %d %s", response.Code, response.Body.String())
	}
	if _, err := runtime.Sessions.Inspect(t.Context(), token); err == nil {
		t.Fatal("cookie remained valid")
	}
}

func TestErasureRejectsPasswordProofBeforeCredentialChange(t *testing.T) {
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t))
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(t.Context(), "stale@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	proof, err := runtime.Accounts.PreparePasswordChange(t.Context(), account.ID, "a deliberately uncommon password", "another uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Accounts.ChangePassword(t.Context(), proof); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Accounts.RequestErasure(context.Background(), account.ID, account.AuthenticationVersion); !errors.Is(err, accounts.ErrCredentialChanged) {
		t.Fatalf("stale erasure: %v", err)
	}
}

func TestActiveAccountKeyLossStillFailsStartup(t *testing.T) {
	cfg := embeddedTestConfig(t)
	first, cancel, runErrors := startTestRuntime(t, cfg)
	account, err := first.Accounts.CreateLocal(t.Context(), "key-loss@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	event, _ := lastAccountEvent(t, first, account.ID)
	js, _, err := storage.Open(t.Context(), first.connection.NATS, 1)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(t.Context(), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := stores.Keys.Purge(t.Context(), event.GetAccountCreated().GetUserKeyRef()); err != nil {
		t.Fatal(err)
	}
	stopTestRuntime(t, first, cancel, runErrors)
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	second, err := New(t.Context(), cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	errorsCh := make(chan error, 1)
	go func() { errorsCh <- second.Run(ctx) }()
	if err := second.WaitReady(ctx); err == nil {
		t.Fatal("active missing key treated as erasure")
	}
	stop()
	if err := <-errorsCh; err == nil {
		t.Fatal("projection did not fail on active key loss")
	}
}
