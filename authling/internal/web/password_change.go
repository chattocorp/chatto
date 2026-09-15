package web

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/sessions"
)

// mountPasswordChange registers signed-in password replacement.
func mountPasswordChange(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
	mux.HandleFunc("GET "+accountPasswordPath, func(w http.ResponseWriter, r *http.Request) {
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, passwordChangeLoginURL())
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
	mux.HandleFunc("POST "+accountPasswordPath, func(w http.ResponseWriter, r *http.Request) {
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
		authenticatedAt := time.Now().UTC()
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
		if err := establishSessionAtAuthenticationVersion(w, r, deps, changed.ID, changed.AuthenticationVersion, authenticatedAt); err != nil {
			clearSessionCookie(w, deps.SecureCookies)
			http.Error(w, "password changed, but a new session could not be established", http.StatusServiceUnavailable)
			return
		}
		redirect(w, r, "/account?password_changed=1")
	})
}
