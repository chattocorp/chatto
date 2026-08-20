package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/email"
	"hmans.de/authling/internal/emailchange"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/passwordreset"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/authling/internal/registration"
	"hmans.de/authling/internal/sessions"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/web"
)

func TestRuntimeCreatesAccountWithReadYourWrites(t *testing.T) {
	cfg := embeddedTestConfig(t)
	runtime, cancel, runErrors := startTestRuntime(t, cfg)

	account, err := runtime.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if account.ID == "" {
		t.Fatal("created account ID is empty")
	}
	if account.CreatedAt.IsZero() {
		t.Fatal("created account timestamp is zero")
	}
	if got, ok := runtime.Accounts.Get(account.ID); !ok || got != account {
		t.Fatalf("projected account = %+v, %v; want %+v, true", got, ok, account)
	}

	stopTestRuntime(t, runtime, cancel, runErrors)
}

func TestRuntimeAppliesConfiguredPasswordMinimumLength(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.Authentication.PasswordMinimumLength = 12
	runtime, cancel, runErrors := startTestRuntime(t, cfg)

	if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", "12345678901"); !errors.Is(err, accounts.ErrInvalidPassword) || err.Error() != "password must contain at least 12 characters and at most 1024 bytes" {
		t.Fatalf("eleven-character password error = %v, want configured policy error", err)
	}
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", "123456789012"); err != nil {
		t.Fatalf("create account with twelve-character password: %v", err)
	}

	stopTestRuntime(t, runtime, cancel, runErrors)
}

func TestRuntimeRejectsCommonPasswords(t *testing.T) {
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t))

	for _, password := range []string{"password123", "Password123", "1234567890"} {
		if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", password); !errors.Is(err, accounts.ErrInvalidPassword) || err.Error() != "password is too common; choose a less predictable password" {
			t.Fatalf("CreateLocal password %q error = %v, want common-password policy error", password, err)
		}
	}
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "person@example.com", "password123 is only part of this passphrase"); err != nil {
		t.Fatalf("create account with non-blocklisted passphrase: %v", err)
	}

	stopTestRuntime(t, runtime, cancel, runErrors)
}

func TestRuntimeReplaysAccountsAfterFullRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg)
	account, err := first.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatalf("create account before restart: %v", err)
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, cancelRestarted, restartedErrors := startTestRuntime(t, cfg)
	if got := restarted.Accounts.Count(); got != 1 {
		t.Fatalf("replayed account count = %d, want 1", got)
	}
	if got, ok := restarted.Accounts.Get(account.ID); !ok || got != account {
		t.Fatalf("replayed account = %+v, %v; want %+v, true", got, ok, account)
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)
}

func TestOIDCIssuerCannotDriftAfterInitialization(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg)
	firstKey, ok := first.issuer.SigningKey()
	if !ok {
		t.Fatal("first runtime has no signing key")
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, err := New(testContext(t), cfg, logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	runContext, cancel := context.WithCancel(context.Background())
	runErrors := make(chan error, 1)
	go func() { runErrors <- restarted.Run(runContext) }()
	if err := restarted.WaitReady(testContext(t)); err != nil {
		t.Fatalf("restart with stable issuer: %v", err)
	}
	restartedKey, ok := restarted.issuer.SigningKey()
	if !ok || restartedKey.ID != firstKey.ID {
		t.Fatalf("restarted signing key = %q, want %q", restartedKey.ID, firstKey.ID)
	}
	cancel()
	<-runErrors
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}

	drifted := cfg
	drifted.HTTP.PublicURL = "http://localhost:8081"
	invalid, err := New(testContext(t), drifted, logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	invalidContext, invalidCancel := context.WithCancel(context.Background())
	invalidErrors := make(chan error, 1)
	go func() { invalidErrors <- invalid.Run(invalidContext) }()
	err = invalid.WaitReady(testContext(t))
	if err == nil || !strings.Contains(err.Error(), "does not match immutable OIDC issuer") {
		t.Fatalf("issuer drift error = %v", err)
	}
	invalidCancel()
	<-invalidErrors
	if err := invalid.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOIDCAuthorizationCodeFlowAndSingleUseCode(t *testing.T) {
	cfg := embeddedTestConfig(t)
	cfg.HTTP = config.HTTPConfig{BindAddress: "127.0.0.1:8080", PublicURL: "http://localhost:8080"}
	cfg.OIDC.Clients = []config.OIDCClientConfig{{ID: "test-client", Name: "Test Client", RedirectURIs: []string{"http://localhost:9999/callback"}}}
	runtime, cancel, runErrors := startTestRuntime(t, cfg)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "oidc@example.com", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	handler := web.Handler(web.Dependencies{Accounts: runtime.Accounts, Authentication: runtime.Authentication, Registration: runtime.Registration, Sessions: runtime.Sessions, OIDC: runtime.OIDC, PublicURL: cfg.HTTP.PublicURLOrDefault()})

	discovery := requestHandler(t, handler, http.MethodGet, "http://localhost:8080/.well-known/openid-configuration", "", nil)
	if discovery.Code != http.StatusOK || !strings.Contains(discovery.Body.String(), `"issuer":"http://localhost:8080"`) {
		t.Fatalf("discovery status/body = %d %s", discovery.Code, discovery.Body.String())
	}

	verifier := strings.Repeat("v", 43)
	code := completeAuthorization(t, handler, verifier, nil)
	wrongVerifier := strings.Repeat("w", 43)
	wrong := redeemCode(t, handler, code, wrongVerifier)
	if wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong verifier status/body = %d %s", wrong.Code, wrong.Body.String())
	}
	tokenResponse := redeemCode(t, handler, code, verifier)
	if tokenResponse.Code != http.StatusOK {
		t.Fatalf("token status/body = %d %s", tokenResponse.Code, tokenResponse.Body.String())
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(tokenResponse.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" || tokens.IDToken == "" || tokens.TokenType != "Bearer" {
		t.Fatalf("token response = %+v", tokens)
	}
	claims := verifyIDToken(t, runtime, tokens.IDToken)
	if claims["iss"] != "http://localhost:8080" || claims["sub"] != account.ID || claims["azp"] != "test-client" {
		t.Fatalf("ID token claims = %+v", claims)
	}

	userinfoRequest := httptest.NewRequest(http.MethodGet, "http://localhost:8080/oauth/userinfo", nil)
	userinfoRequest.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	userinfo := httptest.NewRecorder()
	handler.ServeHTTP(userinfo, userinfoRequest)
	if userinfo.Code != http.StatusOK || !strings.Contains(userinfo.Body.String(), `"sub":"`+account.ID+`"`) {
		t.Fatalf("userinfo status/body = %d %s", userinfo.Code, userinfo.Body.String())
	}

	reused := redeemCode(t, handler, code, verifier)
	if reused.Code == http.StatusOK {
		t.Fatalf("authorization code was reusable: %s", reused.Body.String())
	}

	raceCode := completeAuthorization(t, handler, verifier, nil)
	start := make(chan struct{})
	responses := make(chan int, 2)
	for range 2 {
		go func() { <-start; responses <- redeemCode(t, handler, raceCode, verifier).Code }()
	}
	close(start)
	successes, rejected := 0, 0
	for range 2 {
		switch <-responses {
		case http.StatusOK:
			successes++
		case http.StatusBadRequest:
			rejected++
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("concurrent code redemption successes/rejections = %d/%d, want 1/1", successes, rejected)
	}
}

func completeAuthorization(t *testing.T, handler http.Handler, verifier string, cookie *http.Cookie) string {
	return completeAuthorizationForScopes(t, handler, verifier, cookie, "openid")
}

func completeAuthorizationForScopes(t *testing.T, handler http.Handler, verifier string, cookie *http.Cookie, scopes string) string {
	t.Helper()
	challenge := "7w_YNF9DSfIdPf_pRjSq646_kPr-2-o9NAl16JGghdM"
	if verifier != strings.Repeat("v", 43) {
		t.Fatal("test verifier and challenge fixture diverged")
	}
	query := url.Values{"client_id": {"test-client"}, "redirect_uri": {"http://localhost:9999/callback"}, "response_type": {"code"}, "scope": {scopes}, "state": {"state-value"}, "nonce": {"nonce-value"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	authorize := requestHandler(t, handler, http.MethodGet, "http://localhost:8080/oauth/authorize?"+query.Encode(), "", cookie)
	location := authorize.Header().Get("Location")
	if authorize.Code < 300 || authorize.Code >= 400 || !strings.HasPrefix(location, "/oidc/consent?id=") {
		t.Fatalf("authorize status/location/body = %d %q %s", authorize.Code, location, authorize.Body.String())
	}
	parsed, _ := url.Parse(location)
	requestID := parsed.Query().Get("id")
	if cookie == nil {
		login := requestHandler(t, handler, http.MethodPost, "http://localhost:8080/login", url.Values{"email": {"oidc@example.com"}, "password": {"a deliberately uncommon password"}, "oidc_request": {requestID}}.Encode(), nil)
		if login.Code != http.StatusSeeOther {
			t.Fatalf("login status/body = %d %s", login.Code, login.Body.String())
		}
		cookies := login.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatalf("login cookies = %d", len(cookies))
		}
		cookie = cookies[0]
	}
	consent := requestHandler(t, handler, http.MethodPost, "http://localhost:8080/oidc/consent", url.Values{"id": {requestID}, "decision": {"allow"}}.Encode(), cookie)
	if consent.Code != http.StatusSeeOther {
		t.Fatalf("consent status/body = %d %s", consent.Code, consent.Body.String())
	}
	callback := requestHandler(t, handler, http.MethodGet, consent.Header().Get("Location"), "", cookie)
	redirect, err := url.Parse(callback.Header().Get("Location"))
	if err != nil || redirect.Host != "localhost:9999" {
		t.Fatalf("callback status/redirect/body = %d %q %s, error = %v", callback.Code, callback.Header().Get("Location"), callback.Body.String(), err)
	}
	if redirect.Query().Get("state") != "state-value" || redirect.Query().Get("code") == "" {
		t.Fatalf("callback query = %v", redirect.Query())
	}
	return redirect.Query().Get("code")
}

func redeemCode(t *testing.T, handler http.Handler, code, verifier string) *httptest.ResponseRecorder {
	t.Helper()
	return requestHandler(t, handler, http.MethodPost, "http://localhost:8080/oauth/token", url.Values{"grant_type": {"authorization_code"}, "client_id": {"test-client"}, "redirect_uri": {"http://localhost:9999/callback"}, "code": {code}, "code_verifier": {verifier}}.Encode(), nil)
}

func requestHandler(t *testing.T, handler http.Handler, method, target, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	if strings.HasPrefix(target, "/") {
		target = "http://localhost:8080" + target
	}
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Origin", "http://localhost:8080")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func verifyIDToken(t *testing.T, runtime *Runtime, raw string) map[string]any {
	t.Helper()
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("ID token has %d segments", len(parts))
	}
	key, ok := runtime.issuer.SigningKey()
	if !ok {
		t.Fatal("missing issuer signing key")
	}
	parsed, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := parsed.Claims(&key.Private.PublicKey, &claims); err != nil {
		t.Fatalf("verify ID token: %v", err)
	}
	return claims
}

func TestVerifiedEmailRegistrationCreatesAccountOnlyAfterConfirmation(t *testing.T) {
	cfg := embeddedTestConfig(t)
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, cfg, sender)

	flow, err := runtime.Registration.Start(testContext(t), " Person@Example.com ")
	if err != nil {
		t.Fatalf("start registration: %v", err)
	}
	if runtime.Accounts.Count() != 0 {
		t.Fatal("account exists before email confirmation")
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if code == "" {
		t.Fatal("verification email has no six-digit code")
	}
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "999999"
	}
	if err := runtime.Registration.Verify(testContext(t), flow, wrongCode); !errors.Is(err, registration.ErrInvalidCode) {
		t.Fatalf("wrong code error = %v, want ErrInvalidCode", err)
	}
	if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
		t.Fatalf("verify code: %v", err)
	}
	if runtime.Accounts.Count() != 0 {
		t.Fatal("account exists before password completion")
	}
	account, err := runtime.Registration.Complete(testContext(t), flow, "a long secure passphrase")
	if err != nil {
		t.Fatalf("complete registration: %v", err)
	}
	if account.ID == "" || runtime.Accounts.Count() != 1 {
		t.Fatalf("created account = %+v, count = %d", account, runtime.Accounts.Count())
	}
	authenticated, err := runtime.Authentication.Login(testContext(t), " PERSON@example.COM ", "a long secure passphrase")
	if err != nil || authenticated != account {
		t.Fatalf("authenticated account = %+v, error = %v; want %+v", authenticated, err, account)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "person@example.com", "wrong password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("wrong-password error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "absent@example.com", "wrong password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("absent-account error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := runtime.Registration.Complete(testContext(t), flow, "a long secure passphrase"); !errors.Is(err, registration.ErrInvalidFlow) {
		t.Fatalf("reused flow error = %v, want ErrInvalidFlow", err)
	}
	stopTestRuntime(t, runtime, cancel, runErrors)

	restarted, cancelRestarted, restartErrors := startTestRuntime(t, cfg, sender)
	if !restarted.Accounts.HasEmail("person@example.com") {
		t.Fatal("replayed account does not claim normalized email")
	}
	messageCount := sender.count()
	duplicateFlow, err := restarted.Registration.Start(testContext(t), "person@example.com")
	if err != nil {
		t.Fatalf("start duplicate registration: %v", err)
	}
	if sender.count() != messageCount+1 {
		t.Fatal("duplicate registration did not follow the same email-delivery path")
	}
	duplicateCode := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := restarted.Registration.Verify(testContext(t), duplicateFlow, duplicateCode); err != nil {
		t.Fatalf("verify duplicate flow: %v", err)
	}
	if _, err := restarted.Registration.Complete(testContext(t), duplicateFlow, "another sufficiently long password"); !errors.Is(err, accounts.ErrEmailClaimed) {
		t.Fatalf("duplicate completion error = %v, want ErrEmailClaimed", err)
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartErrors)
}

func TestBrowserSessionSurvivesRestartAndCanBeRevoked(t *testing.T) {
	cfg := embeddedTestConfig(t)
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg)
	account, err := first.Accounts.Create(testContext(t))
	if err != nil {
		t.Fatal(err)
	}
	token, created, err := first.Sessions.Create(testContext(t), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := first.Sessions.Validate(testContext(t), token); err != nil || got != created {
		t.Fatalf("validated session = %+v, error = %v; want %+v", got, err, created)
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, cancelRestarted, restartedErrors := startTestRuntime(t, cfg)
	if got, err := restarted.Sessions.Validate(testContext(t), token); err != nil || got.AccountID != account.ID {
		t.Fatalf("restarted session = %+v, error = %v; want account %q", got, err, account.ID)
	}
	if err := restarted.Sessions.Revoke(testContext(t), token); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Sessions.Validate(testContext(t), token); err == nil {
		t.Fatal("revoked session still validates")
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)
}

func TestPasswordResetChangesCredentialAndInvalidatesOlderSessionsAcrossRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	sender := &capturingSender{}
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg, sender)
	account, err := first.Accounts.CreateLocal(testContext(t), "recover@example.com", "the original uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	oldSession, _, err := first.Sessions.Create(testContext(t), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := first.PasswordReset.Start(testContext(t), " Recover@Example.com ")
	if err != nil {
		t.Fatalf("start password reset: %v", err)
	}
	if sender.last().Subject != "Your Authling password reset code" {
		t.Fatalf("password reset subject = %q", sender.last().Subject)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	requestEvent, requestRecord := lastAccountEvent(t, first, account.ID)
	request := requestEvent.GetPasswordResetRequested()
	if request == nil || request.GetAccountId() != account.ID || request.GetCredentialEventId() == "" {
		t.Fatalf("password reset request audit event = %+v", requestEvent)
	}
	if bytes.Contains(requestRecord, []byte("recover@example.com")) || bytes.Contains(requestRecord, []byte(code)) {
		t.Fatal("password reset request audit event contains email or recovery code")
	}
	if err := first.PasswordReset.Verify(testContext(t), flow, code); err != nil {
		t.Fatalf("verify password reset: %v", err)
	}
	if _, err := first.PasswordReset.Complete(testContext(t), flow, "short"); !errors.Is(err, accounts.ErrInvalidPassword) {
		t.Fatalf("short replacement password error = %v, want ErrInvalidPassword", err)
	}
	changed, err := first.PasswordReset.Complete(testContext(t), flow, "the replacement uncommon password")
	if err != nil {
		t.Fatalf("complete password reset: %v", err)
	}
	if changed.ID != account.ID || changed.AuthenticationVersion != account.AuthenticationVersion+1 {
		t.Fatalf("changed account = %+v, original = %+v", changed, account)
	}
	changeEvent, _ := lastAccountEvent(t, first, account.ID)
	if got := changeEvent.GetPasswordChanged().GetPasswordResetRequestEventId(); got != requestEvent.GetId() {
		t.Fatalf("password change request event ID = %q, want %q", got, requestEvent.GetId())
	}
	if _, err := first.Authentication.Login(testContext(t), "recover@example.com", "the original uncommon password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("old password login error = %v, want ErrInvalidCredentials", err)
	}
	if authenticated, err := first.Authentication.Login(testContext(t), "recover@example.com", "the replacement uncommon password"); err != nil || authenticated != changed {
		t.Fatalf("new password login = %+v, %v; want %+v", authenticated, err, changed)
	}
	if _, err := first.Sessions.Validate(testContext(t), oldSession); !errors.Is(err, sessions.ErrNotFound) {
		t.Fatalf("older session validation error = %v, want ErrNotFound", err)
	}
	if _, err := first.PasswordReset.Complete(testContext(t), flow, "another replacement password"); !errors.Is(err, passwordreset.ErrInvalidFlow) {
		t.Fatalf("reused reset error = %v, want ErrInvalidFlow", err)
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, cancelRestarted, restartedErrors := startTestRuntime(t, cfg, sender)
	defer stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)
	if authenticated, err := restarted.Authentication.Login(testContext(t), "recover@example.com", "the replacement uncommon password"); err != nil || authenticated.ID != account.ID || authenticated.AuthenticationVersion != 1 {
		t.Fatalf("restarted login = %+v, %v", authenticated, err)
	}
	if _, err := restarted.Sessions.Validate(testContext(t), oldSession); !errors.Is(err, sessions.ErrNotFound) {
		t.Fatalf("restarted older session validation error = %v, want ErrNotFound", err)
	}
}

func TestPasswordResetHidesAbsentAccountsAndRejectsStaleConcurrentFlows(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	if _, err := runtime.Accounts.CreateLocal(testContext(t), "race-reset@example.com", "the original reset race password"); err != nil {
		t.Fatal(err)
	}

	flows := make([]string, 2)
	for i := range flows {
		flow, err := runtime.PasswordReset.Start(testContext(t), "race-reset@example.com")
		if err != nil {
			t.Fatalf("start reset %d: %v", i, err)
		}
		flows[i] = flow
	}
	for i, message := range sender.all() {
		code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(message.Body)
		if err := runtime.PasswordReset.Verify(testContext(t), flows[i], code); err != nil {
			t.Fatalf("verify reset %d: %v", i, err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, flow := range flows {
		go func() {
			<-start
			_, err := runtime.PasswordReset.Complete(testContext(t), flow, fmt.Sprintf("replacement reset race password %d", i))
			results <- err
		}()
	}
	close(start)
	var successes, stale int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, passwordreset.ErrInvalidFlow) {
			stale++
		} else {
			t.Fatalf("concurrent reset error = %v", err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent reset successes=%d stale=%d, want 1/1", successes, stale)
	}

	beforeAbsent := sender.count()
	eventsBeforeAbsent := eventCount(t, runtime)
	absentFlow, err := runtime.PasswordReset.Start(testContext(t), "absent-reset@example.com")
	if err != nil {
		t.Fatalf("start absent reset: %v", err)
	}
	if sender.count() != beforeAbsent+1 {
		t.Fatal("absent account did not follow the email-delivery path")
	}
	if got := eventCount(t, runtime); got != eventsBeforeAbsent {
		t.Fatalf("absent account added %d durable audit events", got-eventsBeforeAbsent)
	}
	absentCode := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.PasswordReset.Verify(testContext(t), absentFlow, absentCode); err != nil {
		t.Fatalf("verify absent reset: %v", err)
	}
	if _, err := runtime.PasswordReset.Complete(testContext(t), absentFlow, "a sufficiently long absent password"); !errors.Is(err, passwordreset.ErrInvalidFlow) {
		t.Fatalf("complete absent reset error = %v, want ErrInvalidFlow", err)
	}
}

func TestEmailChangePreservesAccountAndInvalidatesSessionsAcrossRestart(t *testing.T) {
	cfg := embeddedTestConfig(t)
	sender := &capturingSender{}
	first, cancelFirst, firstErrors := startTestRuntime(t, cfg, sender)
	account, err := first.Accounts.CreateLocal(testContext(t), "before@example.com", "the original uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	oldSession, _, err := first.Sessions.Create(testContext(t), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := first.EmailChange.Start(testContext(t), account.ID, "the original uncommon password", " After@Example.com ")
	if err != nil {
		t.Fatalf("start email change: %v", err)
	}
	codeMessage := sender.last()
	if codeMessage.To != "after@example.com" || codeMessage.Subject != "Your Authling email change code" {
		t.Fatalf("email change code message = %+v", codeMessage)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(codeMessage.Body)
	requestEvent, requestRecord := lastAccountEvent(t, first, account.ID)
	request := requestEvent.GetEmailChangeRequested()
	if request == nil || request.GetAccountId() != account.ID || request.GetCredentialEventId() == "" {
		t.Fatalf("email change request event = %+v", requestEvent)
	}
	if bytes.Contains(requestRecord, []byte("before@example.com")) || bytes.Contains(requestRecord, []byte("after@example.com")) || bytes.Contains(requestRecord, []byte(code)) {
		t.Fatal("email change request event contains an address or verification code")
	}
	if err := first.EmailChange.Verify(testContext(t), account.ID, flow, code); err != nil {
		t.Fatalf("verify email change: %v", err)
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)

	restarted, cancelRestarted, restartedErrors := startTestRuntime(t, cfg, sender)
	completion, err := restarted.EmailChange.Complete(testContext(t), account.ID, flow)
	if err != nil {
		t.Fatalf("complete email change after restart: %v", err)
	}
	if completion.OldAddressNotificationFailed || completion.Account.ID != account.ID || completion.Account.AuthenticationVersion != account.AuthenticationVersion+1 {
		t.Fatalf("email change completion = %+v, original = %+v", completion, account)
	}
	recoveryTarget := accounts.EmailChangeTarget{AccountID: account.ID, CredentialEventID: request.GetCredentialEventId(), RequestEventID: requestEvent.GetId()}
	if recovered, ok := restarted.Accounts.CompletedEmailChange(recoveryTarget, "after@example.com"); !ok || recovered != completion.Account {
		t.Fatalf("recover committed email change = %+v, %v; want %+v", recovered, ok, completion.Account)
	}
	notice := sender.last()
	if notice.To != "before@example.com" || notice.Subject != "Your Authling email address changed" || strings.Contains(notice.Body, "after@example.com") {
		t.Fatalf("old-address notice = %+v", notice)
	}
	changeEvent, changeRecord := lastAccountEvent(t, restarted, account.ID)
	change := changeEvent.GetEmailChanged()
	if change == nil || change.GetEmailChangeRequestEventId() != requestEvent.GetId() || change.GetPriorCredentialEventId() != request.GetCredentialEventId() {
		t.Fatalf("email change event = %+v", changeEvent)
	}
	if bytes.Contains(changeRecord, []byte("before@example.com")) || bytes.Contains(changeRecord, []byte("after@example.com")) || bytes.Contains(changeRecord, []byte(code)) {
		t.Fatal("email change event contains plaintext identity or verification material")
	}
	stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)

	replayed, cancelReplayed, replayedErrors := startTestRuntime(t, cfg, sender)
	defer stopTestRuntime(t, replayed, cancelReplayed, replayedErrors)
	if _, err := replayed.Authentication.Login(testContext(t), "before@example.com", "the original uncommon password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("old-address login error = %v, want ErrInvalidCredentials", err)
	}
	if authenticated, err := replayed.Authentication.Login(testContext(t), "after@example.com", "the original uncommon password"); err != nil || authenticated != completion.Account {
		t.Fatalf("new-address login = %+v, %v; want %+v", authenticated, err, completion.Account)
	}
	if _, err := replayed.Sessions.Validate(testContext(t), oldSession); !errors.Is(err, sessions.ErrNotFound) {
		t.Fatalf("older session validation error = %v, want ErrNotFound", err)
	}
	if _, err := replayed.EmailChange.Complete(testContext(t), account.ID, flow); !errors.Is(err, emailchange.ErrInvalidFlow) {
		t.Fatalf("reused email change error = %v, want ErrInvalidFlow", err)
	}
}

func TestEmailChangeHidesClaimedAddressAndRejectsStaleFlows(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	first, err := runtime.Accounts.CreateLocal(testContext(t), "first@example.com", "the first uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.Accounts.CreateLocal(testContext(t), "claimed@example.com", "the claimed uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	claimedFlow, err := runtime.EmailChange.Start(testContext(t), first.ID, "the first uncommon password", "claimed@example.com")
	if err != nil {
		t.Fatalf("start change to claimed address: %v", err)
	}
	claimedCode := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.EmailChange.Verify(testContext(t), second.ID, claimedFlow, claimedCode); !errors.Is(err, emailchange.ErrInvalidCode) {
		t.Fatalf("cross-account flow verification error = %v, want ErrInvalidCode", err)
	}
	if err := runtime.EmailChange.Verify(testContext(t), first.ID, claimedFlow, claimedCode); err != nil {
		t.Fatalf("verify claimed-address flow: %v", err)
	}
	if _, err := runtime.EmailChange.Complete(testContext(t), first.ID, claimedFlow); !errors.Is(err, emailchange.ErrInvalidFlow) {
		t.Fatalf("complete claimed-address flow error = %v, want ErrInvalidFlow", err)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "first@example.com", "the first uncommon password"); err != nil {
		t.Fatalf("original address changed after claimed failure: %v", err)
	}

	flows := make([]string, 2)
	codes := make([]string, 2)
	for i, address := range []string{"new-one@example.com", "new-two@example.com"} {
		flow, err := runtime.EmailChange.Start(testContext(t), first.ID, "the first uncommon password", address)
		if err != nil {
			t.Fatalf("start email change %d: %v", i, err)
		}
		flows[i] = flow
		codes[i] = regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
		if err := runtime.EmailChange.Verify(testContext(t), first.ID, flow, codes[i]); err != nil {
			t.Fatalf("verify email change %d: %v", i, err)
		}
	}
	if _, err := runtime.EmailChange.Complete(testContext(t), first.ID, flows[0]); err != nil {
		t.Fatalf("complete first email change: %v", err)
	}
	if _, err := runtime.EmailChange.Complete(testContext(t), first.ID, flows[1]); !errors.Is(err, emailchange.ErrInvalidFlow) {
		t.Fatalf("complete stale email change error = %v, want ErrInvalidFlow", err)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "new-one@example.com", "the first uncommon password"); err != nil {
		t.Fatalf("winning changed address login: %v", err)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "new-two@example.com", "the first uncommon password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("stale changed address login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestEmailChangeReauthenticationThrottlesWithoutConsumingDeliveryBudget(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "reauth-limit@example.com", "the correct reauthentication password")
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := runtime.EmailChange.Start(testContext(t), account.ID, "the wrong reauthentication password", "reauth-target@example.com"); !errors.Is(err, accounts.ErrInvalidCredentials) {
			t.Fatalf("failed email change reauthentication error = %v", err)
		}
	}
	if sender.count() != 0 {
		t.Fatalf("wrong-password email deliveries = %d, want 0", sender.count())
	}
	if _, err := runtime.EmailChange.Start(testContext(t), account.ID, "the correct reauthentication password", "reauth-target@example.com"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("throttled email change reauthentication error = %v", err)
	}
	if sender.count() != 0 {
		t.Fatalf("throttled email deliveries = %d, want 0", sender.count())
	}
}

func TestConcurrentEmailChangesCannotClaimSameAddress(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	accountsByIndex := make([]accounts.Account, 2)
	passwords := []string{"the first concurrent email password", "the second concurrent email password"}
	for i, address := range []string{"concurrent-one@example.com", "concurrent-two@example.com"} {
		account, err := runtime.Accounts.CreateLocal(testContext(t), address, passwords[i])
		if err != nil {
			t.Fatal(err)
		}
		accountsByIndex[i] = account
	}
	flows := make([]string, 2)
	for i := range flows {
		flow, err := runtime.EmailChange.Start(testContext(t), accountsByIndex[i].ID, passwords[i], "shared-new@example.com")
		if err != nil {
			t.Fatalf("start concurrent email change %d: %v", i, err)
		}
		flows[i] = flow
		code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
		if err := runtime.EmailChange.Verify(testContext(t), accountsByIndex[i].ID, flow, code); err != nil {
			t.Fatalf("verify concurrent email change %d: %v", i, err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i := range flows {
		go func() {
			<-start
			_, err := runtime.EmailChange.Complete(testContext(t), accountsByIndex[i].ID, flows[i])
			results <- err
		}()
	}
	close(start)
	var successes, claimed int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, emailchange.ErrInvalidFlow):
			claimed++
		default:
			t.Fatalf("concurrent email change error = %v", err)
		}
	}
	if successes != 1 || claimed != 1 {
		t.Fatalf("concurrent email change successes=%d claimed=%d, want 1/1", successes, claimed)
	}
	winningPasswords := 0
	for _, password := range passwords {
		if _, err := runtime.Authentication.Login(testContext(t), "shared-new@example.com", password); err == nil {
			winningPasswords++
		} else if !errors.Is(err, accounts.ErrInvalidCredentials) {
			t.Fatalf("shared-address login error = %v", err)
		}
	}
	if winningPasswords != 1 {
		t.Fatalf("shared-address winning passwords = %d, want 1", winningPasswords)
	}
}

func TestEmailChangeKeepsCommittedIdentityWhenOldAddressNoticeFails(t *testing.T) {
	sender := &failingNotificationSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "notify-old@example.com", "the notification failure password")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the notification failure password", "notify-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.EmailChange.Verify(testContext(t), account.ID, flow, code); err != nil {
		t.Fatal(err)
	}
	completion, err := runtime.EmailChange.Complete(testContext(t), account.ID, flow)
	if err != nil || !completion.OldAddressNotificationFailed {
		t.Fatalf("completion = %+v, %v; want committed notification warning", completion, err)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "notify-new@example.com", "the notification failure password"); err != nil {
		t.Fatalf("changed identity after notification failure: %v", err)
	}
}

func TestCommittedEmailChangeRecoveryDoesNotCrossPasswordReset(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "recovery-old@example.com", "the recovery original password")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the recovery original password", "recovery-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	requestEvent, _ := lastAccountEvent(t, runtime, account.ID)
	target := accounts.EmailChangeTarget{
		AccountID: account.ID, CredentialEventID: requestEvent.GetEmailChangeRequested().GetCredentialEventId(), RequestEventID: requestEvent.GetId(),
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.EmailChange.Verify(testContext(t), account.ID, flow, code); err != nil {
		t.Fatal(err)
	}
	completion, err := runtime.EmailChange.Complete(testContext(t), account.ID, flow)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := runtime.Accounts.CompletedEmailChange(target, "recovery-new@example.com"); !ok {
		t.Fatal("committed email change was not recoverable before password reset")
	}
	resetFlow, err := runtime.PasswordReset.Start(testContext(t), "recovery-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	resetCode := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.PasswordReset.Verify(testContext(t), resetFlow, resetCode); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.PasswordReset.Complete(testContext(t), resetFlow, "the recovery replacement password"); err != nil {
		t.Fatal(err)
	}
	if _, ok := runtime.Accounts.CompletedEmailChange(target, "recovery-new@example.com"); ok {
		t.Fatal("email change recovery crossed the later password-reset generation")
	}
	if _, _, err := runtime.Sessions.CreateAtAuthenticationVersion(testContext(t), account.ID, completion.AuthenticationVersion); err == nil {
		t.Fatal("email change completion established a session across the later password-reset generation")
	}
}

func TestEmailChangeRecoversCommittedCompletionAfterLeaseExpires(t *testing.T) {
	cfg := embeddedTestConfig(t)
	clock := &mutableClock{now: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)}
	crashingSender := newCrashAfterCommitSender()
	first, cancelFirst, firstErrors := startTestRuntimeWithOptions(t, cfg, crashingSender, []emailchange.Option{emailchange.WithClock(clock.Now)})
	account, err := first.Accounts.CreateLocal(testContext(t), "crash-old@example.com", "the post commit recovery password")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := first.EmailChange.Start(testContext(t), account.ID, "the post commit recovery password", "crash-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(crashingSender.last().Body)
	if err := first.EmailChange.Verify(testContext(t), account.ID, flow, code); err != nil {
		t.Fatal(err)
	}
	firstCompletion := make(chan error, 1)
	go func() {
		_, err := first.EmailChange.Complete(context.Background(), account.ID, flow)
		firstCompletion <- err
	}()
	select {
	case <-crashingSender.notificationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("email change did not commit before simulated crash")
	}
	stopTestRuntime(t, first, cancelFirst, firstErrors)
	close(crashingSender.releaseNotification)
	select {
	case <-firstCompletion:
	case <-time.After(5 * time.Second):
		t.Fatal("interrupted completion did not return")
	}

	recoverySender := &capturingSender{}
	restarted, cancelRestarted, restartedErrors := startTestRuntimeWithOptions(t, cfg, recoverySender, []emailchange.Option{emailchange.WithClock(clock.Now)})
	defer stopTestRuntime(t, restarted, cancelRestarted, restartedErrors)
	if _, err := restarted.EmailChange.Complete(testContext(t), account.ID, flow); err == nil {
		t.Fatal("recovery ignored the still-active completion lease")
	}
	clock.Advance(46 * time.Second)
	completion, err := restarted.EmailChange.Complete(testContext(t), account.ID, flow)
	if err != nil {
		t.Fatalf("recover committed email change after lease expiry: %v", err)
	}
	if completion.Account.ID != account.ID || completion.AuthenticationVersion != account.AuthenticationVersion+1 {
		t.Fatalf("recovered completion = %+v, original account = %+v", completion, account)
	}
	if notice := recoverySender.last(); notice.To != "crash-old@example.com" || notice.Subject != "Your Authling email address changed" {
		t.Fatalf("recovered old-address notice = %+v", notice)
	}
	if _, err := restarted.EmailChange.Complete(testContext(t), account.ID, flow); !errors.Is(err, emailchange.ErrInvalidFlow) {
		t.Fatalf("reused recovered flow error = %v, want ErrInvalidFlow", err)
	}
}

func TestVerifiedEmailChangeBecomesStaleAfterPasswordReset(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "stale-old@example.com", "the stale email change password")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the stale email change password", "stale-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.EmailChange.Verify(testContext(t), account.ID, flow, code); err != nil {
		t.Fatal(err)
	}
	resetFlow, err := runtime.PasswordReset.Start(testContext(t), "stale-old@example.com")
	if err != nil {
		t.Fatal(err)
	}
	resetCode := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.PasswordReset.Verify(testContext(t), resetFlow, resetCode); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.PasswordReset.Complete(testContext(t), resetFlow, "the replacement password after stale flow"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EmailChange.Complete(testContext(t), account.ID, flow); !errors.Is(err, emailchange.ErrInvalidFlow) {
		t.Fatalf("stale email change completion error = %v, want ErrInvalidFlow", err)
	}
	if runtime.Accounts.HasEmail("stale-new@example.com") {
		t.Fatal("stale email change claimed its replacement address")
	}
}

func TestEmailChangeWrongCodesExhaustAndFlowsExpire(t *testing.T) {
	clock := &mutableClock{now: time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)}
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntimeWithOptions(t, embeddedTestConfig(t), sender, []emailchange.Option{emailchange.WithClock(clock.Now)})
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "otp-old@example.com", "the email change otp password")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the email change otp password", "otp-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for range 5 {
		if err := runtime.EmailChange.Verify(testContext(t), account.ID, flow, wrong); !errors.Is(err, emailchange.ErrInvalidCode) {
			t.Fatalf("wrong email change code error = %v, want ErrInvalidCode", err)
		}
	}
	if err := runtime.EmailChange.Verify(testContext(t), account.ID, flow, code); !errors.Is(err, emailchange.ErrInvalidCode) {
		t.Fatalf("correct code after exhaustion error = %v, want ErrInvalidCode", err)
	}
	expiringFlow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the email change otp password", "expired-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	expiringCode := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	clock.Advance(emailchange.FlowTTL)
	if err := runtime.EmailChange.Verify(testContext(t), account.ID, expiringFlow, expiringCode); !errors.Is(err, emailchange.ErrInvalidCode) {
		t.Fatalf("expired email change code error = %v, want ErrInvalidCode", err)
	}
	if runtime.Accounts.HasEmail("otp-new@example.com") || runtime.Accounts.HasEmail("expired-new@example.com") {
		t.Fatal("exhausted or expired email change mutated the active identity")
	}
}

func TestConcurrentEmailChangeCompletionHasOneActiveLease(t *testing.T) {
	sender := newBlockingNotificationSender()
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "lease-old@example.com", "the completion lease password")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the completion lease password", "lease-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.EmailChange.Verify(testContext(t), account.ID, flow, code); err != nil {
		t.Fatal(err)
	}
	firstResult := make(chan error, 1)
	completionContext := testContext(t)
	go func() {
		_, err := runtime.EmailChange.Complete(completionContext, account.ID, flow)
		firstResult <- err
	}()
	select {
	case <-sender.notificationStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first completion did not reach old-address notification")
	}
	if _, err := runtime.EmailChange.Complete(testContext(t), account.ID, flow); err == nil {
		t.Fatal("concurrent email change completion bypassed the active lease")
	}
	close(sender.releaseNotification)
	if err := <-firstResult; err != nil {
		t.Fatalf("leased email change completion: %v", err)
	}
	var notices int
	for _, message := range sender.all() {
		if message.Subject == "Your Authling email address changed" {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("old-address notices = %d, want 1", notices)
	}
}

func TestEmailChangeDeliveryFailureLeavesIdentityUnchangedAndCanRetry(t *testing.T) {
	sender := &failingFirstEmailChangeCodeSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "delivery-old@example.com", "the delivery failure password")
	if err != nil {
		t.Fatal(err)
	}
	before := eventCount(t, runtime)
	if _, err := runtime.EmailChange.Start(testContext(t), account.ID, "the delivery failure password", "delivery-new@example.com"); err == nil {
		t.Fatal("email change with failed code delivery succeeded")
	}
	request, _ := lastAccountEvent(t, runtime, account.ID)
	if request.GetEmailChangeRequested() == nil || eventCount(t, runtime) != before+1 {
		t.Fatalf("failed-delivery audit event = %+v", request)
	}
	if _, err := runtime.Authentication.Login(testContext(t), "delivery-old@example.com", "the delivery failure password"); err != nil {
		t.Fatalf("old identity after delivery failure: %v", err)
	}
	if runtime.Accounts.HasEmail("delivery-new@example.com") {
		t.Fatal("failed code delivery changed the active identity")
	}
	if _, err := runtime.EmailChange.Start(testContext(t), account.ID, "the delivery failure password", "delivery-new@example.com"); err != nil {
		t.Fatalf("retry after failed code delivery: %v", err)
	}
}

func TestEmailChangeCommitsDurableFactsBeforeDeliveryEffects(t *testing.T) {
	sender := &inspectingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "ordered-old@example.com", "the delivery ordering password")
	if err != nil {
		t.Fatal(err)
	}
	var requestInspected bool
	sender.inspect = func(message email.Message) error {
		if message.Subject != "Your Authling email change code" {
			return nil
		}
		requestInspected = true
		event, record := lastAccountEvent(t, runtime, account.ID)
		if event.GetEmailChangeRequested() == nil {
			return fmt.Errorf("latest account event before code delivery is not EmailChangeRequested")
		}
		code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(message.Body)
		for _, secret := range []string{"ordered-old@example.com", "ordered-new@example.com", code} {
			if bytes.Contains(record, []byte(secret)) {
				return fmt.Errorf("request event exposed %q before code delivery", secret)
			}
		}
		return nil
	}
	flow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the delivery ordering password", "ordered-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !requestInspected {
		t.Fatal("code delivery did not inspect its preceding audit event")
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.EmailChange.Verify(testContext(t), account.ID, flow, code); err != nil {
		t.Fatal(err)
	}
	var mutationInspected bool
	sender.inspect = func(message email.Message) error {
		if message.Subject != "Your Authling email address changed" {
			return nil
		}
		mutationInspected = true
		if !runtime.Accounts.HasEmail("ordered-new@example.com") || runtime.Accounts.HasEmail("ordered-old@example.com") {
			return fmt.Errorf("identity mutation was not projected before old-address notification")
		}
		event, _ := lastAccountEvent(t, runtime, account.ID)
		if event.GetEmailChanged() == nil {
			return fmt.Errorf("latest account event before old-address notification is not EmailChanged")
		}
		return nil
	}
	if _, err := runtime.EmailChange.Complete(testContext(t), account.ID, flow); err != nil {
		t.Fatal(err)
	}
	if !mutationInspected {
		t.Fatal("old-address notification did not inspect its preceding identity mutation")
	}
}

func TestEmailChangeRuntimeStateDoesNotExposeIdentityOrVerificationMaterial(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	account, err := runtime.Accounts.CreateLocal(testContext(t), "sealed-old@example.com", "the runtime secrecy password")
	if err != nil {
		t.Fatal(err)
	}
	flow, err := runtime.EmailChange.Start(testContext(t), account.ID, "the runtime secrecy password", "sealed-new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	js, err := jetstream.New(runtime.connection.NATS)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := storage.OpenStores(testContext(t), js, 1)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := stores.RuntimeState.Keys(testContext(t))
	if err != nil {
		t.Fatal(err)
	}
	secrets := [][]byte{[]byte("sealed-old@example.com"), []byte("sealed-new@example.com"), []byte(account.ID), []byte(flow), []byte(code)}
	for _, key := range keys {
		for _, secret := range secrets {
			if bytes.Contains([]byte(key), secret) {
				t.Fatalf("runtime key %q exposes sensitive workflow material", key)
			}
		}
		entry, err := stores.RuntimeState.Get(testContext(t), key)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range secrets {
			if bytes.Contains(entry.Value(), secret) {
				t.Fatalf("runtime value for %q exposes sensitive workflow material", key)
			}
		}
	}
}

func TestLoginThrottlesAfterTenFailedAttempts(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	flow, err := runtime.Registration.Start(testContext(t), "limited@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Registration.Complete(testContext(t), flow, "a sufficiently long throttle password"); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := runtime.Authentication.Login(testContext(t), "limited@example.com", "wrong password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
			t.Fatalf("failed login error = %v, want ErrInvalidCredentials", err)
		}
	}
	if _, err := runtime.Authentication.Login(testContext(t), "limited@example.com", "a sufficiently long throttle password"); !errors.Is(err, accounts.ErrInvalidCredentials) {
		t.Fatalf("throttled valid login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestRegistrationExhaustsFlowAfterFiveWrongCodes(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	flow, err := runtime.Registration.Start(testContext(t), "attempts@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	wrong := "000000"
	if code == wrong {
		wrong = "999999"
	}
	for range 5 {
		if err := runtime.Registration.Verify(testContext(t), flow, wrong); !errors.Is(err, registration.ErrInvalidCode) {
			t.Fatalf("wrong-code error = %v", err)
		}
	}
	if err := runtime.Registration.Verify(testContext(t), flow, code); !errors.Is(err, registration.ErrInvalidCode) {
		t.Fatalf("exhausted flow error = %v", err)
	}
	if runtime.Accounts.Count() != 0 {
		t.Fatal("exhausted flow created an account")
	}
}

func TestConcurrentVerifiedFlowsCannotClaimSameEmailTwice(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)

	flows := make([]string, 2)
	for i := range flows {
		flow, err := runtime.Registration.Start(testContext(t), "race@example.com")
		if err != nil {
			t.Fatalf("start flow %d: %v", i, err)
		}
		flows[i] = flow
	}
	messages := sender.all()
	for i, flow := range flows {
		code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(messages[i].Body)
		if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
			t.Fatalf("verify flow %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	errorsByFlow := make(chan error, 2)
	for _, flow := range flows {
		go func() {
			<-start
			_, err := runtime.Registration.Complete(testContext(t), flow, "a sufficiently long race password")
			errorsByFlow <- err
		}()
	}
	close(start)
	var successes, claimed int
	for range 2 {
		err := <-errorsByFlow
		switch {
		case err == nil:
			successes++
		case errors.Is(err, accounts.ErrEmailClaimed):
			claimed++
		default:
			t.Fatalf("concurrent completion error = %v", err)
		}
	}
	if successes != 1 || claimed != 1 || runtime.Accounts.Count() != 1 {
		t.Fatalf("successes=%d claimed=%d accounts=%d, want 1/1/1", successes, claimed, runtime.Accounts.Count())
	}
}

func TestVerifiedFlowAllowsOnlyOneConcurrentCompletion(t *testing.T) {
	sender := &capturingSender{}
	runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
	defer stopTestRuntime(t, runtime, cancel, runErrors)
	flow, err := runtime.Registration.Start(testContext(t), "single-flow@example.com")
	if err != nil {
		t.Fatal(err)
	}
	code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
	if err := runtime.Registration.Verify(testContext(t), flow, code); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := runtime.Registration.Complete(testContext(t), flow, "one deliberately long completion password")
			results <- err
		}()
	}
	close(start)
	var successes, rejected int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, registration.ErrInvalidFlow) {
			rejected++
		} else {
			t.Fatalf("completion error = %v", err)
		}
	}
	if successes != 1 || rejected != 1 || runtime.Accounts.Count() != 1 {
		t.Fatalf("successes=%d rejected=%d accounts=%d", successes, rejected, runtime.Accounts.Count())
	}
}

func embeddedTestConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		NATS: config.NATSConfig{
			Embedded: config.EmbeddedNATSConfig{
				Enabled: true,
				DataDir: t.TempDir(),
			},
		},
	}
}

func startTestRuntime(
	t *testing.T,
	cfg config.Config,
	senders ...email.Sender,
) (*Runtime, context.CancelFunc, <-chan error) {
	t.Helper()
	if len(senders) == 0 {
		return startTestRuntimeWithOptions(t, cfg, nil, nil)
	}
	return startTestRuntimeWithOptions(t, cfg, senders[0], nil)
}

func startTestRuntimeWithOptions(
	t *testing.T,
	cfg config.Config,
	sender email.Sender,
	emailChangeOptions []emailchange.Option,
) (*Runtime, context.CancelFunc, <-chan error) {
	t.Helper()
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	var runtime *Runtime
	var err error
	if sender == nil {
		runtime, err = New(testContext(t), cfg, logger)
	} else {
		runtime, err = newRuntimeWithEmailChangeOptions(testContext(t), cfg, logger, sender, emailChangeOptions...)
	}
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	runContext, cancel := context.WithCancel(context.Background())
	runErrors := make(chan error, 1)
	go func() {
		runErrors <- runtime.Run(runContext)
	}()
	if err := runtime.WaitReady(testContext(t)); err != nil {
		cancel()
		<-runErrors
		runtime.Close()
		t.Fatalf("wait for runtime readiness: %v", err)
	}
	return runtime, cancel, runErrors
}

type capturingSender struct {
	mu       sync.Mutex
	messages []email.Message
}

type failingNotificationSender struct{ capturingSender }

type failingFirstEmailChangeCodeSender struct {
	capturingSender
	failed bool
}

type inspectingSender struct {
	capturingSender
	inspect func(email.Message) error
}

func (s *inspectingSender) SendContext(ctx context.Context, message email.Message) error {
	if s.inspect != nil {
		if err := s.inspect(message); err != nil {
			return err
		}
	}
	return s.capturingSender.SendContext(ctx, message)
}

type crashAfterCommitSender struct {
	capturingSender
	notificationStarted chan struct{}
	releaseNotification chan struct{}
	startOnce           sync.Once
}

func newCrashAfterCommitSender() *crashAfterCommitSender {
	return &crashAfterCommitSender{notificationStarted: make(chan struct{}), releaseNotification: make(chan struct{})}
}

func (s *crashAfterCommitSender) SendContext(ctx context.Context, message email.Message) error {
	if message.Subject == "Your Authling email address changed" {
		s.startOnce.Do(func() { close(s.notificationStarted) })
		select {
		case <-s.releaseNotification:
			return errors.New("simulated process loss after identity commit")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.capturingSender.SendContext(ctx, message)
}

type mutableClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mutableClock) Advance(duration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(duration)
}

type blockingNotificationSender struct {
	capturingSender
	notificationStarted chan struct{}
	releaseNotification chan struct{}
	startOnce           sync.Once
}

func newBlockingNotificationSender() *blockingNotificationSender {
	return &blockingNotificationSender{notificationStarted: make(chan struct{}), releaseNotification: make(chan struct{})}
}

func (s *blockingNotificationSender) SendContext(ctx context.Context, message email.Message) error {
	if message.Subject == "Your Authling email address changed" {
		s.startOnce.Do(func() { close(s.notificationStarted) })
		select {
		case <-s.releaseNotification:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.capturingSender.SendContext(ctx, message)
}

func (s *failingFirstEmailChangeCodeSender) SendContext(ctx context.Context, message email.Message) error {
	s.mu.Lock()
	if message.Subject == "Your Authling email change code" && !s.failed {
		s.failed = true
		s.mu.Unlock()
		return errors.New("email change code delivery failed")
	}
	s.mu.Unlock()
	return s.capturingSender.SendContext(ctx, message)
}

func (s *failingNotificationSender) SendContext(ctx context.Context, message email.Message) error {
	if message.Subject == "Your Authling email address changed" {
		return errors.New("notification delivery failed")
	}
	return s.capturingSender.SendContext(ctx, message)
}

func (s *capturingSender) SendContext(_ context.Context, message email.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, message)
	return nil
}
func (s *capturingSender) last() email.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messages[len(s.messages)-1]
}
func (s *capturingSender) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.messages) }
func (s *capturingSender) all() []email.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]email.Message(nil), s.messages...)
}

func stopTestRuntime(
	t *testing.T,
	runtime *Runtime,
	cancel context.CancelFunc,
	runErrors <-chan error,
) {
	t.Helper()
	cancel()
	select {
	case err := <-runErrors:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("runtime shutdown error = %v, want context cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runtime did not stop")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func lastAccountEvent(t *testing.T, runtime *Runtime, accountID string) (*corev1.Event, []byte) {
	t.Helper()
	js, err := runtime.connection.NATS.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	subject, err := evtstream.AccountSubject(accountID)
	if err != nil {
		t.Fatal(err)
	}
	record, err := js.GetLastMsg(storage.EventStreamName, subject)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := evtstream.Decode(record.Data)
	if err != nil {
		t.Fatal(err)
	}
	return decoded.Event, record.Data
}

func eventCount(t *testing.T, runtime *Runtime) uint64 {
	t.Helper()
	js, err := runtime.connection.NATS.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	info, err := js.StreamInfo(storage.EventStreamName)
	if err != nil {
		t.Fatal(err)
	}
	return info.State.Msgs
}
