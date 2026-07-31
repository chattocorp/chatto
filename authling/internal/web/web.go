// Package web serves Authling's server-rendered user interface and embedded
// browser assets.
package web

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"net/url"

	"github.com/a-h/templ"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/registration"
)

//go:embed assets
var embeddedAssets embed.FS

// Handler returns Authling's public HTTP handler. Its pages are rendered on
// the server and remain usable without client-side JavaScript.
func Handler(registrations ...*registration.Service) http.Handler {
	var signup *registration.Service
	if len(registrations) > 0 {
		signup = registrations[0]
	}
	mux := http.NewServeMux()
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic("open embedded web assets: " + err.Error())
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := homePage().Render(r.Context(), w); err != nil {
			http.Error(w, "render page", http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /signup", func(w http.ResponseWriter, r *http.Request) { render(w, r, http.StatusOK, signupPage("")) })
	mux.HandleFunc("POST /signup", func(w http.ResponseWriter, r *http.Request) {
		if signup == nil {
			http.Error(w, "signup unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			render(w, r, http.StatusBadRequest, signupPage("Invalid form submission."))
			return
		}
		flow, err := signup.Start(r.Context(), r.FormValue("email"))
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(publicStartError(err)))
			return
		}
		render(w, r, http.StatusOK, codePage(flow, ""))
	})
	mux.HandleFunc("POST /signup/verify", func(w http.ResponseWriter, r *http.Request) {
		if signup == nil {
			http.Error(w, "signup unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		if err := signup.Verify(r.Context(), flow, r.FormValue("code")); err != nil {
			render(w, r, http.StatusUnprocessableEntity, codePage(flow, registration.ErrInvalidCode.Error()))
			return
		}
		render(w, r, http.StatusOK, passwordPage(flow, ""))
	})
	mux.HandleFunc("POST /signup/complete", func(w http.ResponseWriter, r *http.Request) {
		if signup == nil {
			http.Error(w, "signup unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		flow := r.FormValue("flow")
		account, err := signup.Complete(r.Context(), flow, r.FormValue("password"))
		if errors.Is(err, accounts.ErrInvalidPassword) {
			render(w, r, http.StatusUnprocessableEntity, passwordPage(flow, err.Error()))
			return
		}
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(registration.ErrInvalidFlow.Error()))
			return
		}
		render(w, r, http.StatusCreated, accountCreatedPage(account.ID))
	})
	return securityHeaders(mux)
}

func render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		return
	}
}

func publicStartError(err error) string {
	if errors.Is(err, registration.ErrInvalidEmail) {
		return registration.ErrInvalidEmail.Error()
	}
	return "We couldn't send a verification code. Please try again later."
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	fetchSite := r.Header.Get("Sec-Fetch-Site")
	// Fetch Metadata describes the relationship before a trusted local proxy
	// potentially rewrites Host. Prefer that browser-controlled signal when it
	// is present, and fail closed for every relationship except same-origin.
	if fetchSite != "" {
		return fetchSite == "same-origin"
	}
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host != r.Host || parsed.User != nil {
		return false
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	return parsed.Scheme == expectedScheme
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
