package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/chattocorp/chatto/authling/internal/config"
	"github.com/chattocorp/chatto/authling/internal/sessions"
	"github.com/chattocorp/chatto/authling/internal/sitesettings"
)

const appearancePath = "/admin/appearance"

// mountAppearance supports administrator settings through native and boosted forms.
func mountAppearance(mux *http.ServeMux, deps Dependencies, origin *url.URL) {
	for _, method := range []string{"GET", "POST"} {
		mux.HandleFunc(method+" "+appearancePath, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			if r.Method == http.MethodPost && !guardFormRequest(w, r, origin, 16<<10) {
				return
			}
			account, err := authenticatedAccount(r, deps)
			if errors.Is(err, sessions.ErrNotFound) {
				clearSessionCookie(w, deps.SecureCookies)
				redirect(w, r, "/login?return_to="+url.QueryEscape(appearancePath))
				return
			}
			if err != nil || deps.SiteSettings == nil {
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
			email, err := deps.Accounts.EmailAddress(r.Context(), account.ID)
			if err != nil {
				http.Error(w, "administration unavailable", http.StatusServiceUnavailable)
				return
			}
			if r.Method == http.MethodGet {
				snapshot, err := deps.SiteSettings.Read(r.Context())
				if err != nil {
					http.Error(w, "site settings unavailable", http.StatusServiceUnavailable)
					return
				}
				render(w, r, http.StatusOK, appearancePage(email, snapshot, "", r.URL.Query().Get("saved") == "1"))
				return
			}
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form submission", http.StatusBadRequest)
				return
			}
			revision, err := strconv.ParseUint(r.PostForm.Get("revision"), 10, 64)
			if err != nil {
				http.Error(w, "invalid settings revision", http.StatusBadRequest)
				return
			}
			input := sitesettings.Snapshot{Revision: revision, Site: config.SiteConfig{
				Name: r.PostForm.Get("name"), Description: r.PostForm.Get("description"), Introduction: r.PostForm.Get("introduction"),
			}}
			if err := sitesettings.Validate(input.Site); err != nil {
				render(w, r, http.StatusUnprocessableEntity, appearancePage(email, input, "Enter a site name of up to 120 characters. Description and introduction can each have up to 500 characters. Use single-line plain text.", false))
				return
			}
			switch r.PostForm.Get("action") {
			case "save":
				err := deps.SiteSettings.Save(r.Context(), account.ID, input)
				switch {
				case errors.Is(err, sitesettings.ErrForbidden):
					http.Error(w, "administrator access required", http.StatusForbidden)
				case errors.Is(err, sitesettings.ErrConflict):
					render(w, r, http.StatusConflict, appearancePage(email, input, "Someone changed these settings while you were editing. Your text is still below. Open the saved settings in another tab and compare before starting a new edit.", false))
				case err != nil:
					render(w, r, http.StatusServiceUnavailable, appearancePage(email, input, "We couldn't confirm the save. Your text is still below. Check the saved settings before trying again.", false))
				default:
					redirect(w, r, appearancePath+"?saved=1")
				}
			default:
				http.Error(w, "invalid settings action", http.StatusBadRequest)
			}
		})
	}
}
