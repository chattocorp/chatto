package web

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"hmans.de/authling/internal/accounts"
)

// mountLogin registers local login and its OIDC return handling.
func mountLogin(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("id")
		returnPath := loginReturnPath(r.URL.Query().Get(loginReturnParameter))
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		render(w, r, http.StatusOK, loginPage("", requestID, returnPath))
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
			render(w, r, http.StatusBadRequest, loginPage("Invalid form submission.", "", ""))
			return
		}
		requestID := r.FormValue("oidc_request")
		returnPath := loginReturnPath(r.FormValue(loginReturnParameter))
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(w, r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		authenticatedAt := time.Now().UTC()
		account, err := deps.Authentication.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
		if errors.Is(err, accounts.ErrInvalidCredentials) {
			render(w, r, http.StatusUnprocessableEntity, loginPage("The email address or password is incorrect.", requestID, returnPath))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later.", requestID, returnPath))
			return
		}
		if err := establishSessionAtAuthenticationVersion(w, r, deps, account.ID, account.AuthenticationVersion, authenticatedAt); err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later.", requestID, returnPath))
			return
		}
		if requestID != "" {
			redirect(w, r, "/oidc/consent?id="+url.QueryEscape(requestID))
			return
		}
		if returnPath != "" {
			redirect(w, r, returnPath)
			return
		}
		redirect(w, r, "/account")
	})
}

// mountLogout registers browser-session logout.
func mountLogout(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
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
}
