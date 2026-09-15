package web

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/registration"
)

// mountRegistration registers verified-email signup.
func mountRegistration(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
	mux.HandleFunc("GET /signup", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("id")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		render(w, r, http.StatusOK, signupPage("", requestID))
	})
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
			render(w, r, http.StatusBadRequest, signupPage("Invalid form submission.", ""))
			return
		}
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		flow, err := deps.Registration.Start(r.Context(), r.FormValue("email"))
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(publicStartError(err), requestID))
			return
		}
		render(w, r, http.StatusOK, codePage(flow, "", requestID))
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
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		if err := deps.Registration.Verify(r.Context(), flow, r.FormValue("code")); err != nil {
			render(w, r, http.StatusUnprocessableEntity, codePage(flow, registration.ErrInvalidCode.Error(), requestID))
			return
		}
		render(w, r, http.StatusOK, passwordPage(flow, "", deps.Registration.PasswordMinimumLength(), requestID))
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
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		password := r.FormValue("password")
		if password != r.FormValue("password_confirmation") {
			render(w, r, http.StatusUnprocessableEntity, passwordPage(flow, "Passwords do not match.", deps.Registration.PasswordMinimumLength(), requestID))
			return
		}
		authenticatedAt := time.Now().UTC()
		account, err := deps.Registration.Complete(r.Context(), flow, password)
		if errors.Is(err, accounts.ErrInvalidPassword) {
			render(w, r, http.StatusUnprocessableEntity, passwordPage(flow, err.Error(), deps.Registration.PasswordMinimumLength(), requestID))
			return
		}
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(registration.ErrInvalidFlow.Error(), requestID))
			return
		}
		if deps.Sessions == nil {
			render(w, r, http.StatusCreated, accountCreatedPage(account.ID))
			return
		}
		if err := establishSessionAtAuthenticationVersion(w, r, deps, account.ID, account.AuthenticationVersion, authenticatedAt); err != nil {
			render(w, r, http.StatusServiceUnavailable, accountCreatedPage(account.ID))
			return
		}
		if requestID != "" {
			redirect(w, r, "/oidc/consent?id="+url.QueryEscape(requestID))
			return
		}
		redirect(w, r, "/account")
	})
}
