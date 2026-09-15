package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/oidcprovider"
	"hmans.de/authling/internal/sessions"
)

// mountConsent registers browser approval and denial of OIDC requests.
func mountConsent(mux *http.ServeMux, deps Dependencies, publicOrigin *url.URL) {
	mux.HandleFunc("GET /oidc/consent", func(w http.ResponseWriter, r *http.Request) {
		if deps.OIDC == nil {
			http.Error(w, "OIDC unavailable", http.StatusServiceUnavailable)
			return
		}
		requestID := r.URL.Query().Get("id")
		consent, err := deps.OIDC.Consent(r.Context(), requestID)
		if err != nil {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			if consent.Silent {
				rejectSilentRequest(w, r, deps.OIDC, requestID, true)
				return
			}
			redirect(w, r, "/login?id="+url.QueryEscape(requestID))
			return
		} else if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		session, err := authenticatedSession(r, deps)
		if err != nil || session.AccountID != account.ID {
			http.Error(w, "session unavailable", http.StatusServiceUnavailable)
			return
		}
		if target, authorized, err := deps.OIDC.TryAuthorize(r.Context(), requestID, account.ID, session.AuthenticatedAt); errors.Is(err, oidcprovider.ErrLoginRequired) {
			if consent.Silent {
				rejectSilentRequest(w, r, deps.OIDC, requestID, true)
				return
			}
			redirect(w, r, "/login?id="+url.QueryEscape(requestID))
			return
		} else if err != nil {
			http.Error(w, "authorization request unavailable", http.StatusServiceUnavailable)
			return
		} else if authorized {
			redirect(w, r, target)
			return
		}
		if consent.Silent {
			rejectSilentRequest(w, r, deps.OIDC, requestID, false)
			return
		}
		email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		profile, err := deps.Accounts.Profile(r.Context(), account.ID)
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(consent.RedirectOrigin))
		render(w, r, http.StatusOK, consentPage(consent, email, account.ID, profile))
	})
	mux.HandleFunc("POST /oidc/consent", func(w http.ResponseWriter, r *http.Request) {
		if deps.OIDC == nil {
			http.Error(w, "OIDC unavailable", http.StatusServiceUnavailable)
			return
		}
		if !guardFormRequest(w, r, publicOrigin, 16<<10) {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			redirect(w, r, "/login?id="+url.QueryEscape(r.FormValue("id")))
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		var target string
		if r.FormValue("decision") == "allow" {
			if r.FormValue("consent_version") != strconv.FormatUint(uint64(authorizations.ConsentVersion), 10) {
				http.Error(w, "consent disclosure changed; reload the consent page before authorizing", http.StatusBadRequest)
				return
			}
			session, sessionErr := authenticatedSession(r, deps)
			if sessionErr != nil || session.AccountID != account.ID {
				http.Error(w, "session unavailable", http.StatusServiceUnavailable)
				return
			}
			target, err = deps.OIDC.Authorize(r.Context(), r.FormValue("id"), account.ID, session.AuthenticatedAt)
		} else if r.FormValue("decision") == "deny" {
			target, err = deps.OIDC.Deny(r.Context(), r.FormValue("id"))
		} else {
			http.Error(w, "invalid decision", http.StatusBadRequest)
			return
		}
		if errors.Is(err, oidcprovider.ErrLoginRequired) {
			redirect(w, r, "/login?id="+url.QueryEscape(r.FormValue("id")))
			return
		}
		if err != nil {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		redirect(w, r, target)
	})
}
