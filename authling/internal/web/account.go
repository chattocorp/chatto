package web

import (
	"errors"
	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/sessions"
	"net/http"
	"net/url"
)

// mountAccount registers the account overview and grant/session management.
func mountAccount(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
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
}
