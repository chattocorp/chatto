// Package web serves Authling's server-rendered user interface and embedded
// browser assets.
package web

import (
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/authentication"
	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/emailchange"
	"hmans.de/authling/internal/oidcprovider"
	"hmans.de/authling/internal/passwordreset"
	"hmans.de/authling/internal/registration"
	"hmans.de/authling/internal/sessions"
)

//go:embed assets
var embeddedAssets embed.FS

const (
	developmentSessionCookieName = "authling_session"
	secureSessionCookieName      = "__Host-authling_session"
)

var errAmbiguousSessionCookie = errors.New("ambiguous session cookie")

// Dependencies are the Authling-owned services used by the server-rendered
// browser surface.
type Dependencies struct {
	Accounts       *accounts.Service
	Authentication *authentication.Service
	Registration   *registration.Service
	PasswordReset  *passwordreset.Service
	EmailChange    *emailchange.Service
	Sessions       *sessions.Service
	Authorizations *authorizations.Service
	OIDC           *oidcprovider.Service
	SecureCookies  bool
	PublicURL      string
	// TrustProxyHeaders treats X-Forwarded-Host and X-Forwarded-Proto as the
	// browser-facing origin. Enable it only behind a proxy that overwrites them.
	TrustProxyHeaders bool
}

// Handler returns Authling's public HTTP handler. Its pages are rendered on
// the server and remain usable without client-side JavaScript.
func Handler(dependencies ...Dependencies) http.Handler {
	var deps Dependencies
	if len(dependencies) > 0 {
		deps = dependencies[0]
	}
	var publicOrigin *url.URL
	if deps.PublicURL != "" {
		var err error
		publicOrigin, err = url.Parse(deps.PublicURL)
		if err != nil {
			panic("parse configured public URL: " + err.Error())
		}
	}
	mux := http.NewServeMux()
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic("open embedded web assets: " + err.Error())
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, homePage())
	})
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("id")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		render(w, r, http.StatusOK, loginPage("", requestID))
	})
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		if deps.Authentication == nil || deps.Sessions == nil {
			http.Error(w, "login unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, loginPage("Invalid form submission.", ""))
			return
		}
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		account, err := deps.Authentication.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
		if errors.Is(err, accounts.ErrInvalidCredentials) {
			render(w, r, http.StatusUnprocessableEntity, loginPage("The email address or password is incorrect.", requestID))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later.", requestID))
			return
		}
		if err := establishSession(w, r, deps, account.ID); err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later.", requestID))
			return
		}
		if requestID != "" {
			redirect(w, r, "/oidc/consent?id="+url.QueryEscape(requestID))
			return
		}
		redirect(w, r, "/account")
	})
	mux.HandleFunc("GET /password-reset", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("id")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		render(w, r, http.StatusOK, passwordResetPage("", requestID))
	})
	mux.HandleFunc("POST /password-reset", func(w http.ResponseWriter, r *http.Request) {
		if deps.PasswordReset == nil {
			http.Error(w, "password reset unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, passwordResetPage("Invalid form submission.", ""))
			return
		}
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		flow, err := deps.PasswordReset.Start(r.Context(), r.FormValue("email"))
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, passwordResetPage(publicPasswordResetStartError(err), requestID))
			return
		}
		render(w, r, http.StatusOK, passwordResetCodePage(flow, "", requestID))
	})
	mux.HandleFunc("POST /password-reset/verify", func(w http.ResponseWriter, r *http.Request) {
		if deps.PasswordReset == nil {
			http.Error(w, "password reset unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		if err := deps.PasswordReset.Verify(r.Context(), flow, r.FormValue("code")); err != nil {
			render(w, r, http.StatusUnprocessableEntity, passwordResetCodePage(flow, passwordreset.ErrInvalidCode.Error(), requestID))
			return
		}
		render(w, r, http.StatusOK, newPasswordPage(flow, "", deps.PasswordReset.PasswordMinimumLength(), requestID))
	})
	mux.HandleFunc("POST /password-reset/complete", func(w http.ResponseWriter, r *http.Request) {
		if deps.PasswordReset == nil {
			http.Error(w, "password reset unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		account, err := deps.PasswordReset.Complete(r.Context(), flow, r.FormValue("password"))
		if errors.Is(err, accounts.ErrInvalidPassword) {
			render(w, r, http.StatusUnprocessableEntity, newPasswordPage(flow, err.Error(), deps.PasswordReset.PasswordMinimumLength(), requestID))
			return
		}
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, passwordResetPage(passwordreset.ErrInvalidFlow.Error(), requestID))
			return
		}
		if deps.Sessions == nil {
			render(w, r, http.StatusOK, passwordResetCompletePage())
			return
		}
		if err := establishSession(w, r, deps, account.ID); err != nil {
			render(w, r, http.StatusServiceUnavailable, passwordResetCompletePage())
			return
		}
		if requestID != "" {
			redirect(w, r, "/oidc/consent?id="+url.QueryEscape(requestID))
			return
		}
		redirect(w, r, "/account")
	})
	mux.HandleFunc("GET /oidc/consent", func(w http.ResponseWriter, r *http.Request) {
		if deps.OIDC == nil {
			http.Error(w, "OIDC unavailable", http.StatusServiceUnavailable)
			return
		}
		requestID := r.URL.Query().Get("id")
		consent, err := deps.OIDC.Consent(r.Context(), requestID)
		if err != nil {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			redirect(w, r, "/login?id="+url.QueryEscape(requestID))
			return
		} else if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if target, authorized, err := deps.OIDC.TryAuthorize(r.Context(), requestID, account.ID); err != nil {
			http.Error(w, "authorization request unavailable", http.StatusServiceUnavailable)
			return
		} else if authorized {
			redirect(w, r, target)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(consent.RedirectOrigin))
		render(w, r, http.StatusOK, consentPage(consent, email))
	})
	mux.HandleFunc("POST /oidc/consent", func(w http.ResponseWriter, r *http.Request) {
		if deps.OIDC == nil {
			http.Error(w, "OIDC unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			redirect(w, r, "/login?id="+url.QueryEscape(r.FormValue("id")))
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		var target string
		if r.FormValue("decision") == "allow" {
			target, err = deps.OIDC.Authorize(r.Context(), r.FormValue("id"), account.ID)
		} else if r.FormValue("decision") == "deny" {
			target, err = deps.OIDC.Deny(r.Context(), r.FormValue("id"))
		} else {
			http.Error(w, "invalid decision", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		redirect(w, r, target)
	})
	mux.HandleFunc("GET /account", func(w http.ResponseWriter, r *http.Request) {
		account, token, err := authenticatedAccountAndToken(r, deps, true)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		browserSessions, err := deps.Sessions.List(r.Context(), account.ID, token)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		} else if err != nil {
			http.Error(w, "browser sessions unavailable", http.StatusServiceUnavailable)
			return
		}
		var authorizationGrants []authorizations.Grant
		if deps.Authorizations != nil {
			authorizationGrants, err = deps.Authorizations.List(r.Context(), account.ID)
			if err != nil {
				http.Error(w, "authorized apps unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		emailChanged := r.URL.Query().Get("email_changed") == "1"
		passwordChanged := r.URL.Query().Get("password_changed") == "1"
		render(w, r, http.StatusOK, accountPage(
			account.ID,
			email,
			browserSessions,
			authorizationGrants,
			passwordChanged,
			emailChanged,
			r.URL.Query().Get("profile_updated") == "1",
			emailChanged && r.URL.Query().Get("email_notice_failed") == "1",
			r.URL.Query().Get("session_revoked") == "1",
			r.URL.Query().Get("other_sessions_revoked") == "1",
			r.URL.Query().Get("session_missing") == "1",
			r.URL.Query().Get("app_revoked") == "1",
			r.URL.Query().Get("app_missing") == "1",
		))
	})
	mux.HandleFunc("POST /account/authorizations/revoke", func(w http.ResponseWriter, r *http.Request) {
		if deps.Authorizations == nil {
			http.Error(w, "authorized app management unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		err = deps.Authorizations.Revoke(r.Context(), account.ID, r.FormValue("grant_id"))
		switch {
		case errors.Is(err, authorizations.ErrNotFound):
			redirect(w, r, "/account?app_missing=1")
		case err != nil:
			http.Error(w, "authorized app management unavailable", http.StatusServiceUnavailable)
		default:
			redirect(w, r, "/account?app_revoked=1")
		}
	})
	mux.HandleFunc("POST /account/sessions/revoke", func(w http.ResponseWriter, r *http.Request) {
		if deps.Sessions == nil {
			http.Error(w, "browser session management unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		account, token, err := authenticatedAccountAndToken(r, deps, true)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		err = deps.Sessions.RevokeSession(r.Context(), account.ID, r.FormValue("session_id"), token)
		switch {
		case errors.Is(err, sessions.ErrNotFound):
			redirect(w, r, "/account?session_missing=1")
		case errors.Is(err, sessions.ErrCurrentSession):
			http.Error(w, "use sign out to end this browser session", http.StatusUnprocessableEntity)
		case err != nil:
			http.Error(w, "browser session management unavailable", http.StatusServiceUnavailable)
		default:
			redirect(w, r, "/account?session_revoked=1")
		}
	})
	mux.HandleFunc("POST /account/sessions/revoke-others", func(w http.ResponseWriter, r *http.Request) {
		if deps.Sessions == nil {
			http.Error(w, "browser session management unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		account, token, err := authenticatedAccountAndToken(r, deps, true)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if _, err := deps.Sessions.RevokeOtherSessions(r.Context(), account.ID, token); err != nil {
			http.Error(w, "browser session management unavailable", http.StatusServiceUnavailable)
			return
		}
		redirect(w, r, "/account?other_sessions_revoked=1")
	})
	mux.HandleFunc("GET /account/profile", func(w http.ResponseWriter, r *http.Request) {
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		profile, err := deps.Accounts.Profile(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "profile unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		render(w, r, http.StatusOK, profilePage(profile, "", email))
	})
	mux.HandleFunc("POST /account/profile", func(w http.ResponseWriter, r *http.Request) {
		if deps.Accounts == nil {
			http.Error(w, "profile unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, profilePage(accounts.Profile{}, "Invalid form submission.", email))
			return
		}
		input := accounts.Profile{PreferredUsername: r.FormValue("preferred_username"), FullName: r.FormValue("full_name")}
		_, err = deps.Accounts.UpdateProfile(r.Context(), account.ID, input.PreferredUsername, input.FullName)
		if errors.Is(err, accounts.ErrInvalidProfile) {
			render(w, r, http.StatusUnprocessableEntity, profilePage(input, "Enter a preferred username between 2 and 64 characters and a full name no longer than 128 characters.", email))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, profilePage(input, "We couldn't update your profile. Please try again later.", email))
			return
		}
		redirect(w, r, "/account?profile_updated=1")
	})
	mux.HandleFunc("GET /account/password", func(w http.ResponseWriter, r *http.Request) {
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		} else if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if deps.Accounts == nil {
			http.Error(w, "password change unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		render(w, r, http.StatusOK, passwordChangePage("", deps.Accounts.PasswordMinimumLength(), email))
	})
	mux.HandleFunc("POST /account/password", func(w http.ResponseWriter, r *http.Request) {
		if deps.Accounts == nil || deps.Authentication == nil || deps.Sessions == nil {
			http.Error(w, "password change unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, passwordChangePage("Invalid form submission.", deps.Accounts.PasswordMinimumLength(), email))
			return
		}
		newPassword := r.FormValue("new_password")
		if newPassword != r.FormValue("new_password_confirmation") {
			render(w, r, http.StatusUnprocessableEntity, passwordChangePage("New passwords do not match.", deps.Accounts.PasswordMinimumLength(), email))
			return
		}
		changed, err := deps.Authentication.ChangePassword(r.Context(), account.ID, r.FormValue("current_password"), newPassword)
		switch {
		case errors.Is(err, accounts.ErrInvalidCredentials):
			render(w, r, http.StatusUnprocessableEntity, passwordChangePage("The current password is incorrect.", deps.Accounts.PasswordMinimumLength(), email))
			return
		case errors.Is(err, accounts.ErrInvalidPassword), errors.Is(err, accounts.ErrPasswordUnchanged):
			render(w, r, http.StatusUnprocessableEntity, passwordChangePage(err.Error(), deps.Accounts.PasswordMinimumLength(), email))
			return
		case errors.Is(err, accounts.ErrCredentialChanged):
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		case err != nil:
			render(w, r, http.StatusServiceUnavailable, passwordChangePage("We couldn't change your password. Please try again later.", deps.Accounts.PasswordMinimumLength(), email))
			return
		}
		if err := establishSessionAtAuthenticationVersion(w, r, deps, changed.ID, changed.AuthenticationVersion); err != nil {
			clearSessionCookie(w, deps.SecureCookies)
			http.Error(w, "password changed, but a new session could not be established", http.StatusServiceUnavailable)
			return
		}
		redirect(w, r, "/account?password_changed=1")
	})
	mux.HandleFunc("GET /account/email", func(w http.ResponseWriter, r *http.Request) {
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		} else if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		render(w, r, http.StatusOK, emailChangePage("", email))
	})
	mux.HandleFunc("POST /account/email", func(w http.ResponseWriter, r *http.Request) {
		if deps.EmailChange == nil {
			http.Error(w, "email change unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, emailChangePage("Invalid form submission.", email))
			return
		}
		flow, err := deps.EmailChange.Start(r.Context(), account.ID, r.FormValue("password"), r.FormValue("email"))
		switch {
		case errors.Is(err, emailchange.ErrInvalidEmail), errors.Is(err, accounts.ErrEmailUnchanged):
			render(w, r, http.StatusUnprocessableEntity, emailChangePage(err.Error(), email))
			return
		case errors.Is(err, accounts.ErrInvalidCredentials):
			render(w, r, http.StatusUnprocessableEntity, emailChangePage("The current password is incorrect.", email))
			return
		case err != nil:
			render(w, r, http.StatusServiceUnavailable, emailChangePage("We couldn't send an email change code. Please try again later.", email))
			return
		}
		render(w, r, http.StatusOK, emailChangeCodePage(flow, "", email))
	})
	mux.HandleFunc("POST /account/email/verify", func(w http.ResponseWriter, r *http.Request) {
		if deps.EmailChange == nil {
			http.Error(w, "email change unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		flow := r.FormValue("flow")
		if err := deps.EmailChange.Verify(r.Context(), account.ID, flow, r.FormValue("code")); err != nil {
			render(w, r, http.StatusUnprocessableEntity, emailChangeCodePage(flow, emailchange.ErrInvalidCode.Error(), email))
			return
		}
		render(w, r, http.StatusOK, emailChangeConfirmPage(flow, "", email))
	})
	mux.HandleFunc("POST /account/email/complete", func(w http.ResponseWriter, r *http.Request) {
		if deps.EmailChange == nil {
			http.Error(w, "email change unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		flow := r.FormValue("flow")
		completion, err := deps.EmailChange.Complete(r.Context(), account.ID, flow)
		if errors.Is(err, emailchange.ErrInvalidFlow) {
			render(w, r, http.StatusUnprocessableEntity, emailChangePage("We couldn't change that email address. Start again.", email))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, emailChangeConfirmPage(flow, "We couldn't change your email address. Please try again.", email))
			return
		}
		if err := establishSessionAtAuthenticationVersion(w, r, deps, completion.Account.ID, completion.AuthenticationVersion); err != nil {
			clearSessionCookie(w, deps.SecureCookies)
			http.Error(w, "email changed, but a new session could not be established", http.StatusServiceUnavailable)
			return
		}
		target := "/account?email_changed=1"
		if completion.OldAddressNotificationFailed {
			target += "&email_notice_failed=1"
		}
		redirect(w, r, target)
	})
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		if deps.Sessions == nil {
			http.Error(w, "logout unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		cookie, err := sessionCookie(r, deps.SecureCookies)
		if err == nil {
			if err := deps.Sessions.Revoke(r.Context(), cookie.Value); err != nil {
				http.Error(w, "logout unavailable", http.StatusServiceUnavailable)
				return
			}
		} else if !errors.Is(err, http.ErrNoCookie) {
			http.Error(w, "invalid session cookie", http.StatusBadRequest)
			return
		}
		clearSessionCookie(w, deps.SecureCookies)
		redirect(w, r, "/login")
	})
	mux.HandleFunc("GET /signup", func(w http.ResponseWriter, r *http.Request) { render(w, r, http.StatusOK, signupPage("")) })
	mux.HandleFunc("POST /signup", func(w http.ResponseWriter, r *http.Request) {
		if deps.Registration == nil {
			http.Error(w, "signup unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, signupPage("Invalid form submission."))
			return
		}
		flow, err := deps.Registration.Start(r.Context(), r.FormValue("email"))
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(publicStartError(err)))
			return
		}
		render(w, r, http.StatusOK, codePage(flow, ""))
	})
	mux.HandleFunc("POST /signup/verify", func(w http.ResponseWriter, r *http.Request) {
		if deps.Registration == nil {
			http.Error(w, "signup unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		if err := deps.Registration.Verify(r.Context(), flow, r.FormValue("code")); err != nil {
			render(w, r, http.StatusUnprocessableEntity, codePage(flow, registration.ErrInvalidCode.Error()))
			return
		}
		render(w, r, http.StatusOK, passwordPage(flow, "", deps.Registration.PasswordMinimumLength()))
	})
	mux.HandleFunc("POST /signup/complete", func(w http.ResponseWriter, r *http.Request) {
		if deps.Registration == nil {
			http.Error(w, "signup unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		password := r.FormValue("password")
		if password != r.FormValue("password_confirmation") {
			render(w, r, http.StatusUnprocessableEntity, passwordPage(flow, "Passwords do not match.", deps.Registration.PasswordMinimumLength()))
			return
		}
		account, err := deps.Registration.Complete(r.Context(), flow, password)
		if errors.Is(err, accounts.ErrInvalidPassword) {
			render(w, r, http.StatusUnprocessableEntity, passwordPage(flow, err.Error(), deps.Registration.PasswordMinimumLength()))
			return
		}
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(registration.ErrInvalidFlow.Error()))
			return
		}
		if deps.Sessions == nil {
			render(w, r, http.StatusCreated, accountCreatedPage(account.ID))
			return
		}
		if err := establishSession(w, r, deps, account.ID); err != nil {
			render(w, r, http.StatusServiceUnavailable, accountCreatedPage(account.ID))
			return
		}
		redirect(w, r, "/account")
	})
	if deps.OIDC != nil {
		mux.Handle("/", deps.OIDC)
	}
	handler := requireCanonicalHost(mux, publicOrigin)
	if deps.TrustProxyHeaders {
		handler = useTrustedProxyOrigin(handler)
	}
	return securityHeaders(handler)
}

func useTrustedProxyOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedHosts := r.Header.Values("X-Forwarded-Host")
		forwardedProtos := r.Header.Values("X-Forwarded-Proto")
		forwardedHost := strings.Join(forwardedHosts, ",")
		forwardedProto := strings.Join(forwardedProtos, ",")
		if forwardedHost == "" && forwardedProto == "" {
			next.ServeHTTP(w, r)
			return
		}
		forwardedOrigin, err := url.Parse(forwardedProto + "://" + forwardedHost)
		if len(forwardedHosts) != 1 || len(forwardedProtos) != 1 ||
			err != nil || (forwardedProto != "http" && forwardedProto != "https") ||
			forwardedOrigin.Host == "" || forwardedOrigin.User != nil ||
			forwardedOrigin.Path != "" || forwardedOrigin.RawQuery != "" || forwardedOrigin.Fragment != "" {
			http.Error(w, "invalid trusted proxy origin", http.StatusBadRequest)
			return
		}
		r.Host = forwardedOrigin.Host
		if forwardedProto == "https" {
			r.TLS = &tls.ConnectionState{}
		} else {
			r.TLS = nil
		}
		next.ServeHTTP(w, r)
	})
}

func validConsentRequest(r *http.Request, service *oidcprovider.Service, id string) bool {
	_, err := service.Consent(r.Context(), id)
	return err == nil
}

func render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		return
	}
}

func redirect(w http.ResponseWriter, r *http.Request, target string) {
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func establishSession(w http.ResponseWriter, r *http.Request, deps Dependencies, accountID string) error {
	return establishSessionForAuthenticationVersion(w, r, deps, accountID, nil)
}

func establishSessionAtAuthenticationVersion(w http.ResponseWriter, r *http.Request, deps Dependencies, accountID string, authenticationVersion uint64) error {
	return establishSessionForAuthenticationVersion(w, r, deps, accountID, &authenticationVersion)
}

func establishSessionForAuthenticationVersion(w http.ResponseWriter, r *http.Request, deps Dependencies, accountID string, authenticationVersion *uint64) error {
	var token string
	var err error
	if authenticationVersion == nil {
		token, _, err = deps.Sessions.Create(r.Context(), accountID)
	} else {
		token, _, err = deps.Sessions.CreateAtAuthenticationVersion(r.Context(), accountID, *authenticationVersion)
	}
	if err != nil {
		return err
	}
	if previous, cookieErr := sessionCookie(r, deps.SecureCookies); cookieErr == nil {
		if err := deps.Sessions.Revoke(r.Context(), previous.Value); err != nil {
			_ = deps.Sessions.Revoke(r.Context(), token)
			return err
		}
	} else if !errors.Is(cookieErr, http.ErrNoCookie) {
		_ = deps.Sessions.Revoke(r.Context(), token)
		return cookieErr
	}
	setSessionCookie(w, token, deps.SecureCookies)
	return nil
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(secure),
		Value:    token,
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func authenticatedAccount(r *http.Request, deps Dependencies) (accounts.Account, error) {
	return authenticatedAccountMode(r, deps, true)
}

func authenticatedAccountMode(r *http.Request, deps Dependencies, active bool) (accounts.Account, error) {
	account, _, err := authenticatedAccountAndToken(r, deps, active)
	return account, err
}

func authenticatedAccountAndToken(r *http.Request, deps Dependencies, active bool) (accounts.Account, string, error) {
	if deps.Accounts == nil || deps.Sessions == nil {
		return accounts.Account{}, "", fmt.Errorf("session services unavailable")
	}
	cookie, err := sessionCookie(r, deps.SecureCookies)
	if errors.Is(err, http.ErrNoCookie) {
		return accounts.Account{}, "", sessions.ErrNotFound
	}
	if err != nil {
		return accounts.Account{}, "", err
	}
	var state sessions.Session
	if active {
		state, err = deps.Sessions.Validate(r.Context(), cookie.Value)
	} else {
		state, err = deps.Sessions.Inspect(r.Context(), cookie.Value)
	}
	if err != nil {
		return accounts.Account{}, "", err
	}
	account, ok := deps.Accounts.Get(state.AccountID)
	if !ok {
		_ = deps.Sessions.Revoke(r.Context(), cookie.Value)
		return accounts.Account{}, "", sessions.ErrNotFound
	}
	return account, cookie.Value, nil
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(secure),
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func sessionCookieName(secure bool) string {
	if secure {
		return secureSessionCookieName
	}
	return developmentSessionCookieName
}

func sessionCookie(r *http.Request, secure bool) (*http.Cookie, error) {
	name := sessionCookieName(secure)
	var found *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name != name {
			continue
		}
		if found != nil {
			return nil, errAmbiguousSessionCookie
		}
		found = cookie
	}
	if found == nil {
		return nil, http.ErrNoCookie
	}
	return found, nil
}

func publicStartError(err error) string {
	if errors.Is(err, registration.ErrInvalidEmail) {
		return registration.ErrInvalidEmail.Error()
	}
	return "We couldn't send a verification code. Please try again later."
}

func publicPasswordResetStartError(err error) string {
	if errors.Is(err, passwordreset.ErrInvalidEmail) {
		return passwordreset.ErrInvalidEmail.Error()
	}
	return "We couldn't send a password reset code. Please try again later."
}

func sameOrigin(r *http.Request, expected *url.URL) bool {
	origin := r.Header.Get("Origin")
	fetchSite := r.Header.Get("Sec-Fetch-Site")
	if fetchSite != "" && fetchSite != "same-origin" {
		return false
	}
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if expected != nil {
		return sameOriginTuple(parsed, expected)
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	requestOrigin, err := url.Parse(expectedScheme + "://" + r.Host)
	return err == nil && sameOriginTuple(parsed, requestOrigin)
}

func requireCanonicalHost(next http.Handler, expected *url.URL) http.Handler {
	if expected == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHost, err := url.Parse(expected.Scheme + "://" + r.Host)
		if err != nil || requestHost.User != nil || requestHost.Path != "" ||
			requestHost.RawQuery != "" || requestHost.Fragment != "" ||
			!sameHostPort(requestHost, expected) {
			http.Error(w, "request host does not match Authling's public URL", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOriginTuple(left, right *url.URL) bool {
	return left.Scheme == right.Scheme && sameHostPort(left, right)
}

func sameHostPort(left, right *url.URL) bool {
	if !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	switch value.Scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(""))
		// Preserve only the origin so ordinary HTML form POSTs send a usable
		// Origin header without leaking paths to referrers.
		w.Header().Set("Referrer-Policy", "origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func contentSecurityPolicy(additionalFormOrigin string) string {
	formAction := "'self'"
	if additionalFormOrigin != "" {
		formAction += " " + additionalFormOrigin
	}
	return "default-src 'none'; connect-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; base-uri 'none'; form-action " + formAction + "; frame-ancestors 'none'"
}
