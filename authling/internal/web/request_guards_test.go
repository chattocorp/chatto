package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/authentication"
	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/emailchange"
	"hmans.de/authling/internal/oidcprovider"
	"hmans.de/authling/internal/passwordreset"
	"hmans.de/authling/internal/registration"
	"hmans.de/authling/internal/sessions"
)

func TestBrowserMutationsRejectOriginBeforeReadingBody(t *testing.T) {
	// Non-nil services pass availability checks. Rejected requests must never
	// reach these uninitialized services or parse the deliberately invalid form.
	handler := Handler(Dependencies{
		Accounts: &accounts.Service{}, Authentication: &authentication.Service{},
		Authorizations: &authorizations.Service{}, EmailChange: &emailchange.Service{},
		OIDC: &oidcprovider.Service{}, PasswordReset: &passwordreset.Service{},
		Registration: &registration.Service{}, Sessions: &sessions.Service{},
		PublicURL: "https://auth.example",
	})
	paths := []string{
		"/login", "/logout", "/signup", "/signup/verify", "/signup/complete",
		"/password-reset", "/password-reset/verify", "/password-reset/complete",
		"/oidc/consent", "/account/profile", "/account/password", "/account/delete",
		"/account/email", "/account/email/verify", "/account/email/complete",
		"/account/authorizations/revoke", "/account/sessions/revoke", "/account/sessions/revoke-others",
	}
	for _, path := range paths {
		for _, origin := range []string{"", "https://evil.example", "https://auth.example"} {
			t.Run(path+"/"+origin, func(t *testing.T) {
				body := strings.NewReader("%" + strings.Repeat("x", 64<<10))
				request := httptest.NewRequest(http.MethodPost, "https://auth.example"+path, body)
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				request.Header.Set("Origin", origin)
				if origin == "https://auth.example" {
					request.Header.Set("Sec-Fetch-Site", "cross-site")
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusForbidden || response.Body.String() != "cross-origin request rejected\n" {
					t.Fatalf("response = %d %q, want origin rejection", response.Code, response.Body.String())
				}
				if body.Len() != (64<<10)+1 {
					t.Fatal("rejected request body was read")
				}
			})
		}
	}
}

func TestBrowserFormBodyLimits(t *testing.T) {
	for _, test := range []struct {
		path, form, acceptedMessage string
		limit, acceptedStatus       int
		deps                        Dependencies
	}{
		{
			path: "/login", form: "oidc_request=unavailable&padding=", limit: 64 << 10,
			acceptedStatus: http.StatusBadRequest, acceptedMessage: "authorization request unavailable",
			deps: Dependencies{Authentication: &authentication.Service{}, Sessions: &sessions.Service{}},
		},
		{
			path: "/oidc/consent", form: "padding=", limit: 16 << 10,
			acceptedStatus: http.StatusServiceUnavailable, acceptedMessage: "account unavailable",
			deps: Dependencies{OIDC: &oidcprovider.Service{}},
		},
	} {
		for _, extra := range []int{0, 1} {
			name := "at limit"
			if extra != 0 {
				name = "over limit"
			}
			t.Run(test.path+"/"+name, func(t *testing.T) {
				body := test.form + strings.Repeat("x", test.limit-len(test.form)+extra)
				request := httptest.NewRequest(http.MethodPost, "https://auth.example"+test.path, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				request.Header.Set("Origin", "https://auth.example")
				response := httptest.NewRecorder()
				Handler(test.deps).ServeHTTP(response, request)
				wantStatus, wantMessage := test.acceptedStatus, test.acceptedMessage
				if extra != 0 {
					wantStatus, wantMessage = http.StatusBadRequest, "invalid form"
				}
				if response.Code != wantStatus || !strings.Contains(strings.ToLower(response.Body.String()), wantMessage) {
					t.Fatalf("response = %d %q, want %d containing %q", response.Code, response.Body.String(), wantStatus, wantMessage)
				}
			})
		}
	}
}

func TestBrowserGuardOrderWithUnavailableServices(t *testing.T) {
	for _, test := range []struct {
		path, message string
		status        int
	}{
		{path: "/login", message: "login unavailable\n", status: http.StatusServiceUnavailable},
		{path: "/logout", message: "logout unavailable\n", status: http.StatusServiceUnavailable},
		{path: "/account/delete", message: "cross-origin request rejected\n", status: http.StatusForbidden},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://auth.example"+test.path, strings.NewReader("%"))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", "https://evil.example")
			response := httptest.NewRecorder()
			Handler().ServeHTTP(response, request)
			if response.Code != test.status || response.Body.String() != test.message {
				t.Fatalf("response = %d %q, want %d %q", response.Code, response.Body.String(), test.status, test.message)
			}
		})
	}
}
