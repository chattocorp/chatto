package web

import (
	"errors"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/emailchange"
	"hmans.de/authling/internal/sessions"
	"net/http"
	"net/url"
)

// mountEmailChange registers signed-in email verification and replacement.
func mountEmailChange(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
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
		session, err := authenticatedSession(r, deps)
		if err != nil || session.AccountID != account.ID {
			http.Error(w, "session unavailable", http.StatusServiceUnavailable)
			return
		}
		completion, err := deps.EmailChange.Complete(r.Context(), account.ID, flow)
		if errors.Is(err, emailchange.ErrInvalidFlow) {
			render(w, r, http.StatusUnprocessableEntity, emailChangePage("We couldn't change that email address. Start again.", email))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, emailChangeConfirmPage(flow, "We couldn't change your email address. Please try again.", email))
			return
		}
		if err := establishSessionAtAuthenticationVersion(w, r, deps, completion.Account.ID, completion.AuthenticationVersion, session.AuthenticatedAt); err != nil {
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
}
