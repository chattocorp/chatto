package http_server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
)

const (
	// browserSessionCookieName is the retired single-slot cookie. Accepting it
	// lets an in-flight response from an earlier revision converge on the new
	// slot format without making it authoritative over a newer login.
	browserSessionCookieName       = "chatto_auth"
	browserSessionCookieNamePrefix = browserSessionCookieName + "_"
	browserSessionCookieSlotBytes  = 16
	browserSessionCookieLimit      = 4
	browserSessionCleanupLimit     = 16
	browserSessionValueKey         = "runtime_credential"
)

type browserSessionCookie struct {
	name  string
	token string
}

func isBrowserSessionCookieName(name string) bool {
	if name == browserSessionCookieName {
		return true
	}
	suffix, ok := strings.CutPrefix(name, browserSessionCookieNamePrefix)
	if !ok || len(suffix) != base64.RawURLEncoding.EncodedLen(browserSessionCookieSlotBytes) {
		return false
	}
	for _, char := range suffix {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func browserSessionCookies(request *http.Request) ([]browserSessionCookie, error) {
	if request == nil {
		return nil, nil
	}
	cookies := make([]browserSessionCookie, 0, 1)
	seenTokens := make(map[string]struct{})
	for _, cookie := range request.Cookies() {
		if !isBrowserSessionCookieName(cookie.Name) || cookie.Value == "" {
			continue
		}
		if _, ok := seenTokens[cookie.Value]; ok {
			continue
		}
		if len(cookies) >= browserSessionCookieLimit {
			return nil, errors.New("too many distinct browser session handles")
		}
		seenTokens[cookie.Value] = struct{}{}
		cookies = append(cookies, browserSessionCookie{name: cookie.Name, token: cookie.Value})
	}
	return cookies, nil
}

func newBrowserSessionCookieName() (string, error) {
	buf := make([]byte, browserSessionCookieSlotBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return browserSessionCookieNamePrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

type browserSessionRequestStateKey struct{}

type browserSessionRequestState struct {
	mu        sync.Mutex
	revisions map[string]uint64
}

func browserSessionState(ctx context.Context) *browserSessionRequestState {
	state, _ := ctx.Value(browserSessionRequestStateKey{}).(*browserSessionRequestState)
	return state
}

func (s *browserSessionRequestState) remember(token string, revision uint64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revisions == nil {
		s.revisions = make(map[string]uint64)
	}
	s.revisions[token] = revision
}

func (s *browserSessionRequestState) revision(token string) (uint64, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	revision, ok := s.revisions[token]
	return revision, ok
}

// jetStreamBrowserSessionStore adapts SCS to Chatto's revisioned runtime-state
// service. SCS's store contract is last-write-wins, so Find records the exact
// revision in the request context and Commit uses it for a JetStream CAS update.
// A token that was loaded and then deleted is never recreated by Commit.
type jetStreamBrowserSessionStore struct {
	core *core.ChattoCore
	now  func() time.Time
}

func newJetStreamBrowserSessionStore(chattoCore *core.ChattoCore) *jetStreamBrowserSessionStore {
	return &jetStreamBrowserSessionStore{core: chattoCore, now: time.Now}
}

func (s *jetStreamBrowserSessionStore) Delete(token string) error {
	return s.DeleteCtx(context.Background(), token)
}

func (s *jetStreamBrowserSessionStore) Find(token string) ([]byte, bool, error) {
	return s.FindCtx(context.Background(), token)
}

func (s *jetStreamBrowserSessionStore) Commit(token string, value []byte, _ time.Time) error {
	return s.CommitCtx(context.Background(), token, value, time.Time{})
}

func (s *jetStreamBrowserSessionStore) DeleteCtx(ctx context.Context, token string) error {
	if revision, ok := browserSessionState(ctx).revision(token); ok {
		return s.core.DeleteCookieSessionValue(ctx, token, revision)
	}
	return s.core.RevokeCookieSession(ctx, token)
}

func (s *jetStreamBrowserSessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	entry, err := s.core.LoadCookieSessionValue(ctx, token, s.now())
	if err != nil {
		if errors.Is(err, core.ErrCookieSessionNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	browserSessionState(ctx).remember(token, entry.Revision)
	return entry.Value, true, nil
}

func (s *jetStreamBrowserSessionStore) CommitCtx(ctx context.Context, token string, value []byte, _ time.Time) error {
	if revision, ok := browserSessionState(ctx).revision(token); ok {
		return s.core.UpdateCookieSessionValue(ctx, token, value, revision, s.now())
	}
	return s.core.CreateCookieSessionValue(ctx, token, value, s.now())
}

// browserSessionCodec stores the fixed cookie-credential record directly.
// This keeps the existing typed JSON storage contract readable by the core
// revocation and audit paths while SCS owns the request-local session lifecycle.
type browserSessionCodec struct{}

func (browserSessionCodec) Encode(deadline time.Time, values map[string]any) ([]byte, error) {
	value, ok := values[browserSessionValueKey]
	if !ok {
		return nil, errors.New("browser session credential is missing")
	}
	var tokenData core.AuthTokenData
	switch value := value.(type) {
	case core.AuthTokenData:
		tokenData = value
	case *core.AuthTokenData:
		if value == nil {
			return nil, errors.New("browser session credential is nil")
		}
		tokenData = *value
	default:
		return nil, fmt.Errorf("browser session credential has type %T", value)
	}
	tokenData.ExpiresAt = deadline.UTC()
	return json.Marshal(tokenData)
}

func (browserSessionCodec) Decode(value []byte) (time.Time, map[string]any, error) {
	var tokenData core.AuthTokenData
	if err := json.Unmarshal(value, &tokenData); err != nil {
		return time.Time{}, nil, err
	}
	if tokenData.ExpiresAt.IsZero() {
		return time.Time{}, nil, errors.New("browser session expiry is missing")
	}
	return tokenData.ExpiresAt, map[string]any{browserSessionValueKey: tokenData}, nil
}

func newBrowserSessionManager(store scs.Store, ttl time.Duration, secure bool) *scs.SessionManager {
	manager := scs.New()
	manager.Store = store
	manager.Codec = browserSessionCodec{}
	manager.Lifetime = ttl
	manager.IdleTimeout = 0
	manager.Cookie.Name = browserSessionCookieName
	manager.Cookie.Path = "/"
	manager.Cookie.HttpOnly = true
	manager.Cookie.Secure = secure
	manager.Cookie.SameSite = http.SameSiteLaxMode
	manager.Cookie.Persist = true
	return manager
}

func (s *HTTPServer) ensureBrowserSessionManagers() {
	if s.browserSessions != nil {
		return
	}
	store := newJetStreamBrowserSessionStore(s.core)
	secure := strings.HasPrefix(s.config.Webserver.URL, "https")
	s.browserSessions = newBrowserSessionManager(store, s.config.Auth.TokenTTLOrDefault(), secure)
}

// browserCookieContext loads SCS state only for a request that will mutate the
// browser session. Ordinary authenticated and public requests validate the
// opaque handle directly and do not cause a second JetStream read.
func (s *HTTPServer) browserCookieContext(c *gin.Context, token string) (context.Context, error) {
	s.ensureBrowserSessionManagers()
	state := &browserSessionRequestState{}
	baseCtx := context.WithValue(c.Request.Context(), browserSessionRequestStateKey{}, state)
	loaded, err := s.browserSessions.Load(baseCtx, token)
	if err != nil {
		return nil, err
	}
	return loaded, nil
}
