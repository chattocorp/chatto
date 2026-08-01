// Package web serves Authling's server-rendered user interface and embedded
// browser assets.
package web

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/authentication"
	"hmans.de/authling/internal/registration"
	"hmans.de/authling/internal/sessions"
)

//go:embed assets
var embeddedAssets embed.FS

const (
	developmentSessionCookieName = "authling_session"
	secureSessionCookieName      = "__Host-authling_session"
)

var errAmbiguousSessionCookie = errors.New("ambiguous session cookie")

// Dependencies are the Authling-owned services used by the server-rendered
// browser surface.
type Dependencies struct {
	Accounts       *accounts.Service
	Authentication *authentication.Service
	Registration   *registration.Service
	Sessions       *sessions.Service
	SecureCookies  bool
	PublicURL      string
}

// Handler returns Authling's public HTTP handler. Its pages are rendered on
// the server and remain usable without client-side JavaScript.
func Handler(dependencies ...Dependencies) http.Handler {
	var deps Dependencies
	if len(dependencies) > 0 {
		deps = dependencies[0]
	}
	var publicOrigin *url.URL
	if deps.PublicURL != "" {
		var err error
		publicOrigin, err = url.Parse(deps.PublicURL)
		if err != nil {
			panic("parse configured public URL: " + err.Error())
		}
	}
	mux := http.NewServeMux()
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic("open embedded web assets: " + err.Error())
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, homePage())
	})
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, loginPage(""))
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
			render(w, r, http.StatusBadRequest, loginPage("Invalid form submission."))
			return
		}
		account, err := deps.Authentication.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
		if errors.Is(err, accounts.ErrInvalidCredentials) {
			render(w, r, http.StatusUnprocessableEntity, loginPage("The email address or password is incorrect."))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later."))
			return
		}
		if err := establishSession(w, r, deps, account.ID); err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later."))
			return
		}
		redirect(w, r, "/account")
	})
	mux.HandleFunc("GET /account", func(w http.ResponseWriter, r *http.Request) {
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
		render(w, r, http.StatusOK, accountPage(account.ID))
	})
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
	mux.HandleFunc("GET /signup", func(w http.ResponseWriter, r *http.Request) { render(w, r, http.StatusOK, signupPage("")) })
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
			render(w, r, http.StatusBadRequest, signupPage("Invalid form submission."))
			return
		}
		flow, err := deps.Registration.Start(r.Context(), r.FormValue("email"))
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(publicStartError(err)))
			return
		}
		render(w, r, http.StatusOK, codePage(flow, ""))
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
		flow := r.FormValue("flow")
		if err := deps.Registration.Verify(r.Context(), flow, r.FormValue("code")); err != nil {
			render(w, r, http.StatusUnprocessableEntity, codePage(flow, registration.ErrInvalidCode.Error()))
			return
		}
		render(w, r, http.StatusOK, passwordPage(flow, "", deps.Registration.PasswordMinimumLength()))
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
		flow := r.FormValue("flow")
		account, err := deps.Registration.Complete(r.Context(), flow, r.FormValue("password"))
		if errors.Is(err, accounts.ErrInvalidPassword) {
			render(w, r, http.StatusUnprocessableEntity, passwordPage(flow, err.Error(), deps.Registration.PasswordMinimumLength()))
			return
		}
		if err != nil {
			render(w, r, http.StatusUnprocessableEntity, signupPage(registration.ErrInvalidFlow.Error()))
			return
		}
		if deps.Sessions == nil {
			render(w, r, http.StatusCreated, accountCreatedPage(account.ID))
			return
		}
		if err := establishSession(w, r, deps, account.ID); err != nil {
			render(w, r, http.StatusServiceUnavailable, accountCreatedPage(account.ID))
			return
		}
		redirect(w, r, "/account")
	})
	return securityHeaders(requireCanonicalHost(mux, publicOrigin))
}

func render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(r.Context(), w); err != nil {
		return
	}
}

func redirect(w http.ResponseWriter, r *http.Request, target string) {
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func establishSession(w http.ResponseWriter, r *http.Request, deps Dependencies, accountID string) error {
	token, _, err := deps.Sessions.Create(r.Context(), accountID)
	if err != nil {
		return err
	}
	if previous, cookieErr := sessionCookie(r, deps.SecureCookies); cookieErr == nil {
		if err := deps.Sessions.Revoke(r.Context(), previous.Value); err != nil {
			_ = deps.Sessions.Revoke(r.Context(), token)
			return err
		}
	} else if !errors.Is(cookieErr, http.ErrNoCookie) {
		_ = deps.Sessions.Revoke(r.Context(), token)
		return cookieErr
	}
	setSessionCookie(w, token, deps.SecureCookies)
	return nil
}

func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(secure),
		Value:    token,
		Path:     "/",
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func authenticatedAccount(r *http.Request, deps Dependencies) (accounts.Account, error) {
	if deps.Accounts == nil || deps.Sessions == nil {
		return accounts.Account{}, fmt.Errorf("session services unavailable")
	}
	cookie, err := sessionCookie(r, deps.SecureCookies)
	if errors.Is(err, http.ErrNoCookie) {
		return accounts.Account{}, sessions.ErrNotFound
	}
	if err != nil {
		return accounts.Account{}, err
	}
	state, err := deps.Sessions.Validate(r.Context(), cookie.Value)
	if err != nil {
		return accounts.Account{}, err
	}
	account, ok := deps.Accounts.Get(state.AccountID)
	if !ok {
		_ = deps.Sessions.Revoke(r.Context(), cookie.Value)
		return accounts.Account{}, sessions.ErrNotFound
	}
	return account, nil
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName(secure),
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func sessionCookieName(secure bool) string {
	if secure {
		return secureSessionCookieName
	}
	return developmentSessionCookieName
}

func sessionCookie(r *http.Request, secure bool) (*http.Cookie, error) {
	name := sessionCookieName(secure)
	var found *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name != name {
			continue
		}
		if found != nil {
			return nil, errAmbiguousSessionCookie
		}
		found = cookie
	}
	if found == nil {
		return nil, http.ErrNoCookie
	}
	return found, nil
}

func publicStartError(err error) string {
	if errors.Is(err, registration.ErrInvalidEmail) {
		return registration.ErrInvalidEmail.Error()
	}
	return "We couldn't send a verification code. Please try again later."
}

func sameOrigin(r *http.Request, expected *url.URL) bool {
	origin := r.Header.Get("Origin")
	fetchSite := r.Header.Get("Sec-Fetch-Site")
	if fetchSite != "" && fetchSite != "same-origin" {
		return false
	}
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if expected != nil {
		return sameOriginTuple(parsed, expected)
	}
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	requestOrigin, err := url.Parse(expectedScheme + "://" + r.Host)
	return err == nil && sameOriginTuple(parsed, requestOrigin)
}

func requireCanonicalHost(next http.Handler, expected *url.URL) http.Handler {
	if expected == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHost, err := url.Parse(expected.Scheme + "://" + r.Host)
		if err != nil || requestHost.User != nil || requestHost.Path != "" ||
			requestHost.RawQuery != "" || requestHost.Fragment != "" ||
			!sameHostPort(requestHost, expected) {
			http.Error(w, "request host does not match Authling's public URL", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOriginTuple(left, right *url.URL) bool {
	return left.Scheme == right.Scheme && sameHostPort(left, right)
}

func sameHostPort(left, right *url.URL) bool {
	if !strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	switch value.Scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; font-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		// Preserve only the origin so ordinary HTML form POSTs send a usable
		// Origin header without leaking paths to referrers.
		w.Header().Set("Referrer-Policy", "origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
