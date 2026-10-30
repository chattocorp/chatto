package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/chattocorp/chatto/authling/internal/accounts"
	"github.com/chattocorp/chatto/authling/internal/authorizations"
	"github.com/chattocorp/chatto/authling/internal/sessions"
)

// accountPageData contains only the records needed by the selected account section.
type accountPageData struct {
	accountID, signedInAs, section                                    string
	profile                                                           accounts.Profile
	browserSessions                                                   []sessions.BrowserSession
	authorizationGrants                                               []authorizations.Grant
	passwordChanged, emailChanged, profileUpdated, notificationFailed bool
	sessionRevoked, otherSessionsRevoked, sessionMissing              bool
	appRevoked, appMissing, administrator                             bool
}

// mountAccount registers the account overview and grant/session management.
func mountAccount(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
	accountHandler := func(w http.ResponseWriter, r *http.Request) {
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
		section := strings.TrimPrefix(r.URL.Path, "/account/")
		if r.URL.Path == "/account" {
			section = "profile"
		}
		var profile accounts.Profile
		if section == "profile" {
			profile, err = deps.Accounts.Profile(r.Context(), account.ID)
			if err != nil {
				http.Error(w, "profile unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		var browserSessions []sessions.BrowserSession
		if section == "sessions" {
			browserSessions, err = deps.Sessions.List(r.Context(), account.ID, token)
			if errors.Is(err, sessions.ErrNotFound) {
				clearSessionCookie(w, deps.SecureCookies)
				redirect(w, r, "/login")
				return
			} else if err != nil {
				http.Error(w, "browser sessions unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		var authorizationGrants []authorizations.Grant
		if section == "apps" && deps.Authorizations != nil {
			authorizationGrants, err = deps.Authorizations.List(r.Context(), account.ID)
			if err != nil {
				http.Error(w, "authorized apps unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		emailChanged := r.URL.Query().Get("email_changed") == "1"
		adminAccess, err := deps.Accounts.Administrator(r.Context(), account.ID, deps.Administration.AdminEmails)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		passwordChanged := r.URL.Query().Get("password_changed") == "1"
		render(w, r, http.StatusOK, accountPage(accountPageData{
			accountID: account.ID, signedInAs: email, section: section, profile: profile,
			browserSessions: browserSessions, authorizationGrants: authorizationGrants,
			passwordChanged: passwordChanged, emailChanged: emailChanged,
			profileUpdated:       r.URL.Query().Get("profile_updated") == "1",
			notificationFailed:   emailChanged && r.URL.Query().Get("email_notice_failed") == "1",
			sessionRevoked:       r.URL.Query().Get("session_revoked") == "1",
			otherSessionsRevoked: r.URL.Query().Get("other_sessions_revoked") == "1",
			sessionMissing:       r.URL.Query().Get("session_missing") == "1",
			appRevoked:           r.URL.Query().Get("app_revoked") == "1",
			appMissing:           r.URL.Query().Get("app_missing") == "1",
			administrator:        adminAccess.Allowed(),
		}))
	}
	for _, path := range []string{"/account", "/account/security", "/account/apps", "/account/sessions"} {
		mux.HandleFunc("GET "+path, accountHandler)
	}
	mux.HandleFunc("POST /account/authorizations/revoke", func(w http.ResponseWriter, r *http.Request) {
		if deps.Authorizations == nil {
			http.Error(w, "authorized app management unavailable", http.StatusServiceUnavailable)
			return
		}
		if !guardFormRequest(w, r, publicOrigin, 16<<10) {
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
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		err = deps.Authorizations.Revoke(r.Context(), account.ID, r.FormValue("grant_id"))
		switch {
		case errors.Is(err, authorizations.ErrNotFound):
			redirect(w, r, "/account/apps?app_missing=1")
		case err != nil:
			http.Error(w, "authorized app management unavailable", http.StatusServiceUnavailable)
		default:
			redirect(w, r, "/account/apps?app_revoked=1")
		}
	})
	mux.HandleFunc("POST /account/sessions/revoke", func(w http.ResponseWriter, r *http.Request) {
		if deps.Sessions == nil {
			http.Error(w, "browser session management unavailable", http.StatusServiceUnavailable)
			return
		}
		if !guardFormRequest(w, r, publicOrigin, 16<<10) {
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
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		err = deps.Sessions.RevokeSession(r.Context(), account.ID, r.FormValue("session_id"), token)
		switch {
		case errors.Is(err, sessions.ErrNotFound):
			redirect(w, r, "/account/sessions?session_missing=1")
		case errors.Is(err, sessions.ErrCurrentSession):
			http.Error(w, "use sign out to end this browser session", http.StatusUnprocessableEntity)
		case err != nil:
			http.Error(w, "browser session management unavailable", http.StatusServiceUnavailable)
		default:
			redirect(w, r, "/account/sessions?session_revoked=1")
		}
	})
	mux.HandleFunc("POST /account/sessions/revoke-others", func(w http.ResponseWriter, r *http.Request) {
		if deps.Sessions == nil {
			http.Error(w, "browser session management unavailable", http.StatusServiceUnavailable)
			return
		}
		if !guardFormRequest(w, r, publicOrigin, 16<<10) {
			return
		}
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
		redirect(w, r, "/account/sessions?other_sessions_revoked=1")
	})
}
