package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/keyvault"
	"hmans.de/authling/internal/sessions"
	"hmans.de/authling/internal/storage"
	"hmans.de/authling/internal/web"
)

func TestBrowserAuthenticationCannotUpgradeStaleGeneration(t *testing.T) {
	for _, flowKind := range []string{"login", "signup", "password reset"} {
		t.Run(flowKind, func(t *testing.T) {
			sender := &capturingSender{}
			runtime, cancel, runErrors := startTestRuntime(t, embeddedTestConfig(t), sender)
			defer stopTestRuntime(t, runtime, cancel, runErrors)
			ctx := testContext(t)
			const address = "generation@example.invalid"
			const initialPassword = "initial generation test password"
			const resetPassword = "reset generation test password"
			const laterPassword = "later generation test password"
			var flow string
			var err error
			if flowKind != "signup" {
				if _, err := runtime.Accounts.CreateLocal(ctx, address, initialPassword); err != nil {
					t.Fatal(err)
				}
			}
			path := "/login"
			form := url.Values{"email": {address}, "password": {initialPassword}}
			proofPassword := initialPassword
			switch flowKind {
			case "signup":
				flow, err = runtime.Registration.Start(ctx, address)
				if err != nil {
					t.Fatal(err)
				}
				code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
				if err := runtime.Registration.Verify(ctx, flow, code); err != nil {
					t.Fatal(err)
				}
				path = "/signup/complete"
				form = url.Values{"flow": {flow}, "password": {initialPassword}, "password_confirmation": {initialPassword}}
			case "password reset":
				flow, err = runtime.PasswordReset.Start(ctx, address)
				if err != nil {
					t.Fatal(err)
				}
				code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(sender.last().Body)
				if err := runtime.PasswordReset.Verify(ctx, flow, code); err != nil {
					t.Fatal(err)
				}
				path = "/password-reset/complete"
				proofPassword = resetPassword
				form = url.Values{"flow": {flow}, "password": {resetPassword}}
			}
			js, err := jetstream.New(runtime.connection.NATS)
			if err != nil {
				t.Fatal(err)
			}
			stores, err := storage.OpenStores(ctx, js, 1)
			if err != nil {
				t.Fatal(err)
			}
			key, err := keyvault.New(stores.Keys).WorkflowKey(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(key)
			changed := false
			// The first generation lookup is the session constructor's boundary,
			// after the HTTP handler has obtained its authentication result.
			sessionService := sessions.New(stores.RuntimeState, js, key, func(ctx context.Context, accountID string) (uint64, bool, error) {
				if !changed {
					changed = true
					target, err := runtime.Accounts.PreparePasswordChange(ctx, accountID, proofPassword, laterPassword)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := runtime.Accounts.ChangePassword(ctx, target); err != nil {
						t.Fatal(err)
					}
				}
				return runtime.Accounts.AuthenticationVersion(ctx, accountID)
			})
			handler := web.Handler(web.Dependencies{
				Accounts: runtime.Accounts, Authentication: runtime.Authentication, Registration: runtime.Registration,
				PasswordReset: runtime.PasswordReset, Sessions: sessionService, PublicURL: "http://localhost:8080",
			})
			request := httptest.NewRequest(http.MethodPost, "http://localhost:8080"+path, strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", "http://localhost:8080")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !changed {
				t.Fatal("request did not reach the session boundary")
			}
			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("stale authentication status = %d, want 503", response.Code)
			}
			if len(response.Result().Cookies()) != 0 {
				t.Fatal("stale authentication issued a browser cookie")
			}
			if _, err := runtime.Authentication.Login(ctx, address, laterPassword); err != nil {
				t.Fatalf("new credential did not survive: %v", err)
			}
		})
	}
}
