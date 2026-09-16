// Package web serves Authling's server-rendered user interface and embedded
// browser assets.
package web

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/authentication"
	"hmans.de/authling/internal/authorizations"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/emailchange"
	"hmans.de/authling/internal/oidcprovider"
	"hmans.de/authling/internal/passwordreset"
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
	// Site is display text shared by every browser page.
	Site           config.SiteConfig
	Accounts       *accounts.Service
	Authentication *authentication.Service
	Registration   *registration.Service
	PasswordReset  *passwordreset.Service
	EmailChange    *emailchange.Service
	Sessions       *sessions.Service
	Authorizations *authorizations.Service
	OIDC           *oidcprovider.Service
	SecureCookies  bool
	PublicURL      string
	// TrustProxyHeaders treats X-Forwarded-Host and X-Forwarded-Proto as the
	// browser-facing origin. Enable it only behind a proxy that overwrites them.
	TrustProxyHeaders bool
}

const changePasswordWellKnownPath = "/.well-known/change-password"
const accountPasswordPath = "/account/password"
const loginReturnParameter = "return_to"

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
	mux.Handle("GET /assets/", browserAssets)
	mux.HandleFunc("GET "+changePasswordWellKnownPath, func(w http.ResponseWriter, r *http.Request) {
		redirect(w, r, accountPasswordPath)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, homePage())
	})
	mountLogin(mux, deps, publicOrigin)
	mountPasswordReset(mux, deps, publicOrigin)
	mountConsent(mux, deps, publicOrigin)
	mountDeletion(mux, deps, publicOrigin)
	mountAccount(mux, deps, publicOrigin)
	mountProfile(mux, deps, publicOrigin)
	mountPasswordChange(mux, deps, publicOrigin)
	mountEmailChange(mux, deps, publicOrigin)
	mountLogout(mux, deps, publicOrigin)
	mountRegistration(mux, deps, publicOrigin)
	if deps.OIDC != nil {
		mux.Handle("/", deps.OIDC)
	}
	handler := redirectToCanonicalOrigin(mux, publicOrigin)
	if deps.TrustProxyHeaders {
		handler = useTrustedProxyOrigin(handler)
	}
	site := deps.Site.Resolve(deps.PublicURL)
	return securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), siteContextKey{}, site)))
	}))
}

func useTrustedProxyOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedHosts := r.Header.Values("X-Forwarded-Host")
		forwardedProtos := r.Header.Values("X-Forwarded-Proto")
		forwardedHost := strings.Join(forwardedHosts, ",")
		forwardedProto := strings.Join(forwardedProtos, ",")
		if forwardedHost == "" && forwardedProto == "" {
			next.ServeHTTP(w, r)
			return
		}
		forwardedOrigin, err := url.Parse(forwardedProto + "://" + forwardedHost)
		if len(forwardedHosts) != 1 || len(forwardedProtos) != 1 ||
			err != nil || (forwardedProto != "http" && forwardedProto != "https") ||
			forwardedOrigin.Host == "" || forwardedOrigin.User != nil ||
			forwardedOrigin.Path != "" || forwardedOrigin.RawQuery != "" || forwardedOrigin.Fragment != "" {
			http.Error(w, "invalid trusted proxy origin", http.StatusBadRequest)
			return
		}
		r.Host = forwardedOrigin.Host
		if forwardedProto == "https" {
			r.TLS = &tls.ConnectionState{}
		} else {
			r.TLS = nil
		}
		next.ServeHTTP(w, r)
	})
}

// validConsentRequest also permits the validated client origin in form-action.
// A successful login or recovery POST can reach that origin through automatic
// consent reuse. Browsers apply the initiating form's CSP to that redirect chain.
func validConsentRequest(w http.ResponseWriter, r *http.Request, service *oidcprovider.Service, id string) bool {
	consent, err := service.Consent(r.Context(), id)
	if err != nil {
		return false
	}
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy(consent.RedirectOrigin))
	return true
}

// rejectSilentRequest returns a protocol error without rendering an interactive page.
func rejectSilentRequest(w http.ResponseWriter, r *http.Request, service *oidcprovider.Service, id string, loginRequired bool) {
	target, err := service.RejectSilent(r.Context(), id, loginRequired)
	if err != nil {
		http.Error(w, "authorization request unavailable", http.StatusServiceUnavailable)
		return
	}
	redirect(w, r, target)
}

func loginReturnPath(candidate string) string {
	if candidate == accountPasswordPath {
		return candidate
	}
	return ""
}

func passwordChangeLoginURL() string {
	return "/login?" + url.Values{loginReturnParameter: {accountPasswordPath}}.Encode()
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

// establishSessionAtAuthenticationVersion never upgrades a stale login or
// recovery proof to a credential generation that did not authorize it.
func establishSessionAtAuthenticationVersion(w http.ResponseWriter, r *http.Request, deps Dependencies, accountID string, authenticationVersion uint64, authenticatedAt time.Time) error {
	token, _, err := deps.Sessions.CreateAtAuthenticationVersion(r.Context(), accountID, authenticationVersion, authenticatedAt)
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

// authenticatedSession returns server-owned authentication evidence for OIDC.
func authenticatedSession(r *http.Request, deps Dependencies) (sessions.Session, error) {
	if deps.Sessions == nil {
		return sessions.Session{}, fmt.Errorf("session services unavailable")
	}
	cookie, err := sessionCookie(r, deps.SecureCookies)
	if errors.Is(err, http.ErrNoCookie) {
		return sessions.Session{}, sessions.ErrNotFound
	}
	if err != nil {
		return sessions.Session{}, err
	}
	return deps.Sessions.Validate(r.Context(), cookie.Value)
}

func authenticatedAccount(r *http.Request, deps Dependencies) (accounts.Account, error) {
	return authenticatedAccountMode(r, deps, true)
}

func authenticatedAccountMode(r *http.Request, deps Dependencies, active bool) (accounts.Account, error) {
	account, _, err := authenticatedAccountAndToken(r, deps, active)
	return account, err
}

func authenticatedAccountAndToken(r *http.Request, deps Dependencies, active bool) (accounts.Account, string, error) {
	if deps.Accounts == nil || deps.Sessions == nil {
		return accounts.Account{}, "", fmt.Errorf("session services unavailable")
	}
	cookie, err := sessionCookie(r, deps.SecureCookies)
	if errors.Is(err, http.ErrNoCookie) {
		return accounts.Account{}, "", sessions.ErrNotFound
	}
	if err != nil {
		return accounts.Account{}, "", err
	}
	var state sessions.Session
	if active {
		state, err = deps.Sessions.Validate(r.Context(), cookie.Value)
	} else {
		state, err = deps.Sessions.Inspect(r.Context(), cookie.Value)
	}
	if err != nil {
		return accounts.Account{}, "", err
	}
	account, ok := deps.Accounts.Get(state.AccountID)
	if !ok {
		_ = deps.Sessions.Revoke(r.Context(), cookie.Value)
		return accounts.Account{}, "", sessions.ErrNotFound
	}
	return account, cookie.Value, nil
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

func publicPasswordResetStartError(err error) string {
	if errors.Is(err, passwordreset.ErrInvalidEmail) {
		return passwordreset.ErrInvalidEmail.Error()
	}
	return "We couldn't send a password reset code. Please try again later."
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

// redirectToCanonicalOrigin serves application routes only at the configured
// public origin. Redirect targets use trusted configuration, never a request
// host or an absolute request URI. A 307 preserves methods without caching a
// hostname choice permanently.
func redirectToCanonicalOrigin(next http.Handler, expected *url.URL) http.Handler {
	if expected == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		requestOrigin, err := url.Parse(scheme + "://" + r.Host)
		if err != nil || requestOrigin.Host == "" || requestOrigin.User != nil || requestOrigin.Path != "" ||
			requestOrigin.RawQuery != "" || requestOrigin.Fragment != "" {
			http.Error(w, "invalid request host", http.StatusBadRequest)
			return
		}
		if !sameOriginTuple(requestOrigin, expected) {
			target := *expected
			target.Path, target.RawPath = r.URL.Path, r.URL.RawPath
			target.RawQuery, target.ForceQuery = r.URL.RawQuery, r.URL.ForceQuery
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, target.String(), http.StatusTemporaryRedirect)
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
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(""))
		// Preserve only the origin so ordinary HTML form POSTs send a usable
		// Origin header without leaking paths to referrers.
		w.Header().Set("Referrer-Policy", "origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func contentSecurityPolicy(additionalFormOrigin string) string {
	formAction := "'self'"
	if additionalFormOrigin != "" {
		formAction += " " + additionalFormOrigin
	}
	return "default-src 'none'; connect-src 'self'; style-src 'self'; font-src 'self'; img-src 'self' data:; base-uri 'none'; form-action " + formAction + "; frame-ancestors 'none'"
}

// siteContextKey keeps each handler's display configuration local to its requests.
type siteContextKey struct{}

func siteFromContext(ctx context.Context) config.SiteConfig {
	site, _ := ctx.Value(siteContextKey{}).(config.SiteConfig)
	return site.Resolve("")
}

func sitePageTitle(ctx context.Context, title string) string {
	name := siteFromContext(ctx).Name
	if title == "" {
		return name
	}
	return title + " · " + name
}
