package web

import (
	"errors"
	"net/http"
	"net/url"

	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/sessions"
)

// mountDeletion requires an active session, same-origin submission, explicit
// confirmation, and a fresh throttled password proof bound to that account.
func mountDeletion(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
	mux.HandleFunc("GET /account/delete", func(w http.ResponseWriter, r *http.Request) {
		_, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		render(w, r, http.StatusOK, deleteAccountPage(""))
	})
	mux.HandleFunc("POST /account/delete", func(w http.ResponseWriter, r *http.Request) {
		if !requireSameOrigin(w, r, publicOrigin) {
			return
		}
		if deps.Authentication == nil {
			http.Error(w, "account deletion unavailable", http.StatusServiceUnavailable)
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
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil || r.PostForm.Get("confirm") != "delete" {
			render(w, r, http.StatusUnprocessableEntity, deleteAccountPage("Confirm that you want to delete this account."))
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		proof, err := deps.Authentication.Login(r.Context(), email, r.PostForm.Get("password"))
		if errors.Is(err, accounts.ErrInvalidCredentials) {
			render(w, r, http.StatusUnprocessableEntity, deleteAccountPage("The password is incorrect, or too many attempts were made. Please try again later."))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, deleteAccountPage("We couldn't verify your password. Please try again later."))
			return
		}
		if proof.ID != account.ID || proof.AuthenticationVersion != account.AuthenticationVersion {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login")
			return
		}
		if err := deps.Accounts.RequestErasure(r.Context(), account.ID, proof.AuthenticationVersion); err != nil {
			render(w, r, http.StatusServiceUnavailable, deleteAccountPage("We couldn't confirm deletion. If you can still sign in, please try again."))
			return
		}
		// The durable request already denies every session, even if this cleanup fails.
		_ = deps.Sessions.Revoke(r.Context(), token)
		clearSessionCookie(w, deps.SecureCookies)
		render(w, r, http.StatusOK, accountDeletedPage())
	})
}
