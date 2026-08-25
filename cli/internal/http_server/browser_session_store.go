package http_server

import (
	"context"
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
	browserSessionCookieName = "chatto_auth"
	browserSessionValueKey   = "runtime_credential"
)

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

func (s *HTTPServer) browserCookieContext(ctx context.Context) context.Context {
	s.ensureBrowserSessionManagers()
	loaded, err := s.browserSessions.Load(ctx, "")
	if err != nil {
		return ctx
	}
	return loaded
}

func (s *HTTPServer) loadBrowserSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		s.ensureBrowserSessionManagers()
		state := &browserSessionRequestState{}
		baseCtx := context.WithValue(c.Request.Context(), browserSessionRequestStateKey{}, state)

		var token string
		if cookie, err := c.Request.Cookie(browserSessionCookieName); err == nil {
			token = cookie.Value
		}
		ctx, err := s.browserSessions.Load(baseCtx, token)
		if err != nil {
			ctx = baseCtx
			ctx, _ = s.browserSessions.Load(ctx, "")
			ctx = context.WithValue(ctx, authenticationValidationErrorKey{}, err)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
