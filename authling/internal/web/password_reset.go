package web

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/passwordreset"
)

// mountPasswordReset registers email verification and password recovery.
func mountPasswordReset(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
	mux.HandleFunc("GET /password-reset", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("id")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
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
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
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
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
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
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		authenticatedAt := time.Now().UTC()
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
		if err := establishSessionAtAuthenticationVersion(w, r, deps, account.ID, account.AuthenticationVersion, authenticatedAt); err != nil {
			render(w, r, http.StatusServiceUnavailable, passwordResetCompletePage())
			return
		}
		if requestID != "" {
			redirect(w, r, "/oidc/consent?id="+url.QueryEscape(requestID))
			return
		}
		redirect(w, r, "/account")
	})
}
