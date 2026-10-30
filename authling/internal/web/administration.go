package web

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/chattocorp/chatto/authling/internal/sessions"
)

// mountAdministration exposes only a read-only, authenticated account directory.
func mountAdministration(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			clearSessionCookie(w, deps.SecureCookies)
			redirect(w, r, "/login?return_to=%2Fadmin")
			return
		}
		if err != nil {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		access, err := deps.Accounts.Administrator(r.Context(), account.ID, deps.Administration.AdminEmails)
		if err != nil {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		if !access.Allowed() {
			http.Error(w, "administrator access required", http.StatusForbidden)
			return
		}
		after := r.URL.Query().Get("after")
		if len(after) > 128 {
			http.Error(w, "invalid account cursor", http.StatusBadRequest)
			return
		}
		rows, next, err := deps.Accounts.RegisteredAccounts(r.Context(), after, 50, deps.Administration.AdminEmails)
		if err != nil {
			http.Error(w, "account directory unavailable", http.StatusServiceUnavailable)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		// A long directory read must not retain privileges revoked while it ran.
		access, err = deps.Accounts.Administrator(r.Context(), account.ID, deps.Administration.AdminEmails)
		if err != nil || !access.Allowed() {
			http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
			return
		}
		nextURL := ""
		if next != "" {
			nextURL = "/admin?" + url.Values{"after": {next}}.Encode()
		}
		render(w, r, http.StatusOK, administrationPage(email, rows, access.Configured, after != "", nextURL))
	})
}
