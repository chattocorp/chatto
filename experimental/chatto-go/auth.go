package chatto

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chattocorp/chatto/experimental/chatto-go/internal/httpjson"
)

// HumanSession contains secret credentials. Never log it. Persist atomically
// with private permissions. RefreshRequestID must survive uncertain refreshes.
type HumanSession struct {
	Origin           string
	ClientID         string
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	RefreshRequestID string
}

// Client returns an immutable API client for this session's current access token.
func (s HumanSession) Client() (*Client, error) { return New(s.Origin, s.AccessToken) }

func authOrigin(origin string) (string, error) {
	c, err := New(origin, "validate-origin")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(c.endpoint, "/api/connect/chatto.api.v1."), nil
}

type tokenResponse struct {
	Access         string `json:"access_token"`
	Refresh        string `json:"refresh_token"`
	Expires        int64  `json:"expires_in"`
	RefreshExpires int64  `json:"refresh_token_expires_in"`
}

func (r tokenResponse) session(origin, clientID string) (HumanSession, error) {
	if r.Access == "" || r.Refresh == "" || r.Expires <= 0 || r.RefreshExpires <= 0 || r.Expires > 315360000 || r.RefreshExpires > 315360000 {
		return HumanSession{}, errors.New("invalid authentication response")
	}
	now := time.Now()
	return HumanSession{Origin: origin, ClientID: clientID, AccessToken: r.Access, RefreshToken: r.Refresh, AccessExpiresAt: now.Add(time.Duration(r.Expires) * time.Second), RefreshExpiresAt: now.Add(time.Duration(r.RefreshExpires) * time.Second)}, nil
}

// LoginPassword signs in as a human on servers that enable direct login. The
// SDK does not retain the password or follow HTTP redirects.
func LoginPassword(ctx context.Context, origin, login, password string) (HumanSession, error) {
	origin, err := authOrigin(origin)
	if err != nil {
		return HumanSession{}, err
	}
	var out struct {
		Token          string `json:"token"`
		Refresh        string `json:"refreshToken"`
		Expires        int64  `json:"expiresIn"`
		RefreshExpires int64  `json:"refreshTokenExpiresIn"`
	}
	err = httpjson.Post(ctx, origin+"/auth/login", "", map[string]string{"login": login, "password": password}, &out)
	if err != nil {
		return HumanSession{}, err
	}
	return (tokenResponse{out.Token, out.Refresh, out.Expires, out.RefreshExpires}).session(origin, "")
}

// LoginBrowser performs authorization-code login with S256 PKCE. open receives
// the browser URL; callers may display it or launch a browser without a shell.
// The listener binds only to 127.0.0.1 and closes on completion or cancellation.
// The server must enable auth.loopback_client_enabled. The built-in client is
// displayed as "Unverified local app" and has a fixed 24-hour session window.
func LoginBrowser(ctx context.Context, origin string, open func(string) error) (HumanSession, error) {
	origin, err := authOrigin(origin)
	if err != nil {
		return HumanSession{}, err
	}
	if open == nil {
		return HumanSession{}, errors.New("browser URL handler is required")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return HumanSession{}, errors.New("cannot open login callback")
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	state, verifier := rand.Text(), rand.Text()+rand.Text()
	hash := sha256.Sum256([]byte(verifier))
	redirect := "http://" + listener.Addr().String() + "/servers/callback?mode=popup"
	const clientID = "chatto://loopback"
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect}, "state": {state}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}}
	type callback struct {
		code string
		err  error
	}
	result := make(chan callback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/servers/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != http.MethodGet || r.Host != listener.Addr().String() || subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid login callback", http.StatusBadRequest)
			return
		}
		if issuer := r.URL.Query().Get("iss"); issuer != "" && strings.TrimRight(issuer, "/") != origin {
			http.Error(w, "Unexpected login issuer", http.StatusBadRequest)
			return
		}
		v := callback{code: r.URL.Query().Get("code")}
		if v.code == "" || r.URL.Query().Get("error") != "" {
			v.err = errors.New("browser login was not authorized")
		}
		select {
		case result <- v:
			_, _ = w.Write([]byte("Return to your application. You can close this tab."))
		default:
			http.Error(w, "Login callback already received", http.StatusConflict)
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	defer func() { _ = server.Close(); <-done }()
	if err := open(origin + "/oauth/authorize?" + q.Encode()); err != nil {
		return HumanSession{}, errors.New("cannot open browser login")
	}
	select {
	case <-ctx.Done():
		return HumanSession{}, ctx.Err()
	case v := <-result:
		if v.err != nil {
			return HumanSession{}, v.err
		}
		var out tokenResponse
		err := httpjson.Post(ctx, origin+"/oauth/token", "", map[string]string{"grant_type": "authorization_code", "code": v.code, "code_verifier": verifier, "client_id": clientID, "redirect_uri": redirect}, &out)
		if err != nil {
			return HumanSession{}, err
		}
		return out.session(origin, clientID)
	}
}

// PrepareRefresh sets an idempotency UUID. Persist the session BEFORE Refresh.
// A failed or uncertain refresh must reuse this identifier and refresh token.
func (s *HumanSession) PrepareRefresh() {
	if s.RefreshRequestID != "" {
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	s.RefreshRequestID = uuidString(b)
}

// Refresh rotates the session once. The caller must persist the returned session
// before using it. No retry is automatic; retain the old prepared session on error.
func (s HumanSession) Refresh(ctx context.Context) (HumanSession, error) {
	origin, err := authOrigin(s.Origin)
	if err != nil {
		return HumanSession{}, err
	}
	if s.RefreshRequestID == "" || s.RefreshToken == "" {
		return HumanSession{}, errors.New("prepare and persist refresh before sending")
	}
	var out tokenResponse
	err = httpjson.Post(ctx, origin+"/oauth/token", "", map[string]string{"grant_type": "refresh_token", "refresh_token": s.RefreshToken, "refresh_request_id": s.RefreshRequestID, "client_id": s.ClientID}, &out)
	if err != nil {
		return HumanSession{}, err
	}
	return out.session(origin, s.ClientID)
}

// Logout revokes the human session, including its refresh credential.
func (s HumanSession) Logout(ctx context.Context) error {
	origin, err := authOrigin(s.Origin)
	if err != nil {
		return err
	}
	return httpjson.Post(ctx, origin+"/auth/logout", s.AccessToken, map[string]string{"refreshToken": s.RefreshToken}, &map[string]any{})
}
