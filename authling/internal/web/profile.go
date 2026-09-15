package web

import (
	"errors"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/sessions"
	"net/http"
	"net/url"
)

// mountProfile registers account profile forms.
func mountProfile(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
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
}
