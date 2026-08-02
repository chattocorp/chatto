// Package web serves Authling's server-rendered user interface and embedded
// browser assets.
package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/coder/websocket"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/authentication"
	"hmans.de/authling/internal/oidcprovider"
	"hmans.de/authling/internal/registration"
	"hmans.de/authling/internal/sessions"
	"hmans.de/authling/internal/tinybasesync"
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
	OIDC           *oidcprovider.Service
	AccountSync    *tinybasesync.Hub
	SecureCookies  bool
	PublicURL      string
	TrustedProxies []netip.Prefix
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
	accountSyncAuthenticationAdmission := newAccountSyncAuthenticationAdmission()
	assets, err := fs.Sub(embeddedAssets, "assets")
	if err != nil {
		panic("open embedded web assets: " + err.Error())
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServerFS(assets)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, homePage())
	})
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.URL.Query().Get("id")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		render(w, r, http.StatusOK, loginPage("", requestID))
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
			render(w, r, http.StatusBadRequest, loginPage("Invalid form submission.", ""))
			return
		}
		requestID := r.FormValue("oidc_request")
		if requestID != "" && (deps.OIDC == nil || !validConsentRequest(r, deps.OIDC, requestID)) {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		account, err := deps.Authentication.Login(r.Context(), r.FormValue("email"), r.FormValue("password"))
		if errors.Is(err, accounts.ErrInvalidCredentials) {
			render(w, r, http.StatusUnprocessableEntity, loginPage("The email address or password is incorrect.", requestID))
			return
		}
		if err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later.", requestID))
			return
		}
		if err := establishSession(w, r, deps, account.ID); err != nil {
			render(w, r, http.StatusServiceUnavailable, loginPage("We couldn't sign you in. Please try again later.", requestID))
			return
		}
		if requestID != "" {
			redirect(w, r, "/oidc/consent?id="+url.QueryEscape(requestID))
			return
		}
		redirect(w, r, "/account")
	})
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
		if _, err := authenticatedAccount(r, deps); errors.Is(err, sessions.ErrNotFound) {
			redirect(w, r, "/login?id="+url.QueryEscape(requestID))
			return
		} else if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(consent.RedirectOrigin))
		render(w, r, http.StatusOK, consentPage(consent))
	})
	mux.HandleFunc("POST /oidc/consent", func(w http.ResponseWriter, r *http.Request) {
		if deps.OIDC == nil {
			http.Error(w, "OIDC unavailable", http.StatusServiceUnavailable)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
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
			target, err = deps.OIDC.Authorize(r.Context(), r.FormValue("id"), account.ID)
		} else if r.FormValue("decision") == "deny" {
			target, err = deps.OIDC.Deny(r.Context(), r.FormValue("id"))
		} else {
			http.Error(w, "invalid decision", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "authorization request unavailable", http.StatusBadRequest)
			return
		}
		redirect(w, r, target)
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
	mux.HandleFunc("GET /data/sync", func(w http.ResponseWriter, r *http.Request) {
		if deps.AccountSync == nil {
			http.Error(w, "account sync unavailable", http.StatusServiceUnavailable)
			return
		}
		if requestsSubprotocol(r, accountSyncTokenSubprotocol) {
			serveTokenAccountSync(w, r, deps, accountSyncAuthenticationAdmission)
			return
		}
		if !sameOrigin(r, publicOrigin) {
			http.Error(w, "cross-origin request rejected", http.StatusForbidden)
			return
		}
		account, err := authenticatedAccount(r, deps)
		if errors.Is(err, sessions.ErrNotFound) {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if err != nil {
			http.Error(w, "account unavailable", http.StatusServiceUnavailable)
			return
		}
		connection, err := deps.AccountSync.Connect(r.Context(), account.ID)
		if errors.Is(err, tinybasesync.ErrConnectionLimit) || errors.Is(err, tinybasesync.ErrCapacityLimit) {
			http.Error(w, "account sync connection limit reached", http.StatusTooManyRequests)
			return
		}
		if err != nil {
			http.Error(w, "account sync unavailable", http.StatusServiceUnavailable)
			return
		}
		defer connection.Close()
		serveAccountSync(w, r, connection, func(ctx context.Context, active bool) bool {
			current, err := authenticatedAccountMode(r.WithContext(ctx), deps, active)
			return err == nil && current.ID == account.ID
		})
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
	if deps.OIDC != nil {
		mux.Handle("/", deps.OIDC)
	}
	return securityHeaders(requireCanonicalHost(mux, publicOrigin))
}

const (
	accountSyncAuthCheckInterval            = 30 * time.Second
	accountSyncAuthenticationTimeout        = 2 * time.Second
	accountSyncTokenSubprotocol             = "authling.account-data.v1"
	maxAccountSyncAuthenticationMessageSize = 8 << 10
	maxPendingAccountSyncAuthentications    = 64
	maxPendingAccountSyncAuthPerSource      = 8
)

type accountSyncAuthentication struct {
	Type        string `json:"type"`
	AccessToken string `json:"access_token"`
}

type accountSyncAuthenticationAdmission struct {
	mu       sync.Mutex
	slots    chan struct{}
	bySource map[string]int
}

func newAccountSyncAuthenticationAdmission() *accountSyncAuthenticationAdmission {
	return &accountSyncAuthenticationAdmission{
		slots: make(chan struct{}, maxPendingAccountSyncAuthentications), bySource: map[string]int{},
	}
}

func (admission *accountSyncAuthenticationAdmission) acquire(source string) bool {
	admission.mu.Lock()
	defer admission.mu.Unlock()
	if admission.bySource[source] >= maxPendingAccountSyncAuthPerSource {
		return false
	}
	select {
	case admission.slots <- struct{}{}:
		admission.bySource[source]++
		return true
	default:
		return false
	}
}

func (admission *accountSyncAuthenticationAdmission) release(source string) {
	admission.mu.Lock()
	if admission.bySource[source] == 1 {
		delete(admission.bySource, source)
	} else {
		admission.bySource[source]--
	}
	admission.mu.Unlock()
	<-admission.slots
}

func monitorAccountSyncAuthorization(ctx context.Context, interval time.Duration, authorized func(context.Context, bool) bool, expire func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !authorized(ctx, false) {
				expire()
				return
			}
		}
	}
}

func serveAccountSync(w http.ResponseWriter, r *http.Request, syncConnection *tinybasesync.Connection, authorized func(context.Context, bool) bool) {
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	runAccountSync(r.Context(), connection, syncConnection, authorized)
}

func serveTokenAccountSync(w http.ResponseWriter, r *http.Request, deps Dependencies, admission *accountSyncAuthenticationAdmission) {
	if deps.OIDC == nil || deps.Accounts == nil {
		http.Error(w, "account sync authentication unavailable", http.StatusServiceUnavailable)
		return
	}
	origin, ok := accountSyncRequestOrigin(r)
	if !ok {
		http.Error(w, "account sync origin rejected", http.StatusForbidden)
		return
	}
	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{accountSyncTokenSubprotocol}, CompressionMode: websocket.CompressionDisabled,
		// The access token is bound to the validated Origin below. The library's
		// host-pattern check cannot express origins discovered at runtime.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	source := accountSyncNetworkSource(r, deps.TrustedProxies)
	if !admission.acquire(source) {
		_ = connection.Close(websocket.StatusPolicyViolation, "account sync authentication limit reached")
		return
	}
	slotHeld := true
	defer func() {
		if slotHeld {
			admission.release(source)
		}
	}()
	connection.SetReadLimit(maxAccountSyncAuthenticationMessageSize)
	authContext, cancel := context.WithTimeout(r.Context(), accountSyncAuthenticationTimeout)
	messageType, data, err := connection.Read(authContext)
	cancel()
	if err != nil || messageType != websocket.MessageText {
		_ = connection.Close(websocket.StatusPolicyViolation, "authentication required")
		return
	}
	authentication, ok := decodeAccountSyncAuthentication(data)
	if !ok {
		_ = connection.Close(websocket.StatusPolicyViolation, "authentication failed")
		return
	}
	grant, err := deps.OIDC.AuthorizeAccountDataToken(r.Context(), authentication.AccessToken, origin)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, "authentication failed")
		return
	}
	if _, exists := deps.Accounts.Get(grant.AccountID); !exists {
		_ = connection.Close(websocket.StatusPolicyViolation, "authentication failed")
		return
	}
	syncConnection, err := deps.AccountSync.Connect(r.Context(), grant.AccountID)
	if errors.Is(err, tinybasesync.ErrConnectionLimit) || errors.Is(err, tinybasesync.ErrCapacityLimit) {
		_ = connection.Close(websocket.StatusPolicyViolation, "account sync connection limit reached")
		return
	}
	if err != nil {
		_ = connection.Close(websocket.StatusInternalError, "sync unavailable")
		return
	}
	defer syncConnection.Close()
	slotHeld = false
	admission.release(source)
	readyContext, readyCancel := context.WithTimeout(r.Context(), accountSyncAuthenticationTimeout)
	err = connection.Write(readyContext, websocket.MessageText, []byte(`{"type":"ready"}`))
	readyCancel()
	if err != nil {
		syncConnection.Close()
		connection.CloseNow()
		return
	}
	runAccountSync(r.Context(), connection, syncConnection, func(ctx context.Context, _ bool) bool {
		current, err := deps.OIDC.AuthorizeAccountDataToken(ctx, authentication.AccessToken, origin)
		if err != nil || current.AccountID != grant.AccountID || current.ClientID != grant.ClientID {
			return false
		}
		_, exists := deps.Accounts.Get(current.AccountID)
		return exists
	})
}

func accountSyncNetworkSource(r *http.Request, trustedProxies []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		host = r.RemoteAddr
	}
	direct, err := netip.ParseAddr(host)
	if err != nil {
		if host != "" {
			return host
		}
		return "unknown"
	}
	direct = direct.Unmap()
	for _, prefix := range trustedProxies {
		if !prefix.Contains(direct) {
			continue
		}
		values := r.Header.Values("X-Forwarded-For")
		if len(values) == 1 && !strings.Contains(values[0], ",") {
			if forwarded, parseErr := netip.ParseAddr(strings.TrimSpace(values[0])); parseErr == nil {
				return forwarded.Unmap().String()
			}
		}
		break
	}
	return direct.String()
}

func runAccountSync(ctx context.Context, connection *websocket.Conn, syncConnection *tinybasesync.Connection, authorized func(context.Context, bool) bool) {
	defer connection.CloseNow()
	connection.SetReadLimit(tinybasesync.MaxWireMessageSize)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	writeErrors := make(chan error, 1)
	go func() {
		for {
			message, err := syncConnection.Next(ctx)
			if err == nil && !authorized(ctx, false) {
				_ = connection.Close(websocket.StatusPolicyViolation, "authentication expired")
				writeErrors <- errors.New("account sync authorization expired")
				return
			}
			if err != nil {
				if ctx.Err() == nil {
					_ = connection.Close(websocket.StatusInternalError, "sync unavailable")
				}
				writeErrors <- err
				return
			}
			encoded, err := tinybasesync.EncodeWireMessage(message)
			if err == nil {
				err = connection.Write(ctx, websocket.MessageText, encoded)
			}
			if err != nil {
				writeErrors <- err
				return
			}
		}
	}()
	go func() {
		monitorAccountSyncAuthorization(ctx, accountSyncAuthCheckInterval, authorized, func() {
			_ = connection.Close(websocket.StatusPolicyViolation, "authentication expired")
			cancel()
		})
	}()
	for {
		messageType, data, err := connection.Read(ctx)
		if err != nil {
			return
		}
		if messageType != websocket.MessageText {
			_ = connection.Close(websocket.StatusUnsupportedData, "text messages required")
			return
		}
		message, err := tinybasesync.DecodeWireMessage(data)
		if err != nil {
			_ = connection.Close(websocket.StatusPolicyViolation, "invalid sync message")
			return
		}
		if !authorized(ctx, true) {
			_ = connection.Close(websocket.StatusPolicyViolation, "authentication expired")
			return
		}
		if err := syncConnection.Handle(ctx, message); errors.Is(err, tinybasesync.ErrRateLimit) {
			_ = connection.Close(websocket.StatusPolicyViolation, "sync rate limit exceeded")
			return
		} else if err != nil {
			_ = connection.Close(websocket.StatusInternalError, "sync unavailable")
			return
		}
		select {
		case <-writeErrors:
			return
		default:
		}
	}
}

func decodeAccountSyncAuthentication(data []byte) (accountSyncAuthentication, bool) {
	var authentication accountSyncAuthentication
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&authentication); err != nil {
		return accountSyncAuthentication{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return accountSyncAuthentication{}, false
	}
	return authentication, authentication.Type == "authenticate" && authentication.AccessToken != ""
}

func requestsSubprotocol(r *http.Request, expected string) bool {
	for _, value := range r.Header.Values("Sec-WebSocket-Protocol") {
		for protocol := range strings.SplitSeq(value, ",") {
			if strings.TrimSpace(protocol) == expected {
				return true
			}
		}
	}
	return false
}

func accountSyncRequestOrigin(r *http.Request) (string, bool) {
	values := r.Header.Values("Origin")
	if len(values) != 1 {
		return "", false
	}
	origin := values[0]
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || parsed.Host == "" || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if strings.EqualFold(parsed.Scheme, "https") {
		return origin, true
	}
	if !strings.EqualFold(parsed.Scheme, "http") {
		return "", false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	address := net.ParseIP(host)
	return origin, host == "localhost" || strings.HasSuffix(host, ".localhost") || address != nil && address.IsLoopback()
}

func validConsentRequest(r *http.Request, service *oidcprovider.Service, id string) bool {
	_, err := service.Consent(r.Context(), id)
	return err == nil
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
	return authenticatedAccountMode(r, deps, true)
}

func authenticatedAccountMode(r *http.Request, deps Dependencies, active bool) (accounts.Account, error) {
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
	var state sessions.Session
	if active {
		state, err = deps.Sessions.Validate(r.Context(), cookie.Value)
	} else {
		state, err = deps.Sessions.Inspect(r.Context(), cookie.Value)
	}
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
