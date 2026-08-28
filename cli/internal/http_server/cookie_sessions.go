package http_server

import (
	"context"
	"errors"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

const (
	// Retired signed-session keys are deleted when new browser authentication is
	// issued. The 0.5 migration route reads runtime_credential_id once; ordinary
	// authentication never accepts these values.
	sessionKeyRuntimeCredentialID = "runtime_credential_id"
	retiredSessionKeyUserID       = "user_id"
	retiredSessionKeyCredentialID = "cookie_session_id"
	issuedBrowserSessionKey       = "issued_browser_session"
)

func (s *HTTPServer) createCookieSession(c *gin.Context, userID, source string) error {
	data, err := s.core.NewCookieSessionData(c.Request.Context(), userID, source)
	if err != nil {
		return err
	}
	_, err = s.issueBrowserSession(c, data)
	return err
}

func (s *HTTPServer) createCookieSessionForGeneration(c *gin.Context, userID, source string, authGeneration uint64) error {
	data, err := s.core.NewCookieSessionDataForGeneration(c.Request.Context(), userID, source, authGeneration)
	if err != nil {
		return err
	}
	_, err = s.issueBrowserSession(c, data)
	return err
}

// issueBrowserSession rotates the opaque handle before authentication changes
// its authority. SCS deletes the loaded handle with the JetStream revision
// observed by this request, which prevents session fixation and fences a
// concurrent renewal or logout.
func (s *HTTPServer) issueBrowserSession(c *gin.Context, data core.AuthTokenData) (string, error) {
	s.ensureBrowserSessionManagers()
	current, ok, err := s.cookiePresentedCredential(c)
	if err != nil {
		return "", err
	}
	currentToken := ""
	if ok {
		currentToken = current.auth.Handle
	}
	cookieName, err := newBrowserSessionCookieName()
	if err != nil {
		return "", err
	}
	sessionCtx, err := s.browserCookieContext(c, currentToken)
	if err != nil {
		return "", err
	}
	if err := s.browserSessions.RenewToken(sessionCtx); err != nil {
		return "", err
	}
	s.browserSessions.Put(sessionCtx, browserSessionValueKey, data)
	s.browserSessions.SetDeadline(sessionCtx, data.ExpiresAt)
	token, expiry, err := s.browserSessions.Commit(sessionCtx)
	if err != nil {
		return "", err
	}
	for _, session := range current.presentedSessions {
		if session.handle == currentToken {
			continue
		}
		if err := s.core.RevokeCookieSession(c.Request.Context(), session.handle); err != nil {
			_ = s.core.RevokeCookieSession(c.Request.Context(), token)
			return "", err
		}
	}

	markAuthenticationCookieResponsePrivate(c)
	s.clearBrowserSessionCookie(c)
	c.Set(issuedBrowserSessionKey, issuedBrowserSession{name: cookieName, token: token})
	s.writeBrowserSessionCookie(c, cookieName, token, expiry)
	clearLegacyCookieAuthentication(sessions.Default(c))
	return token, nil
}

func (s *HTTPServer) createConnectBrowserSession(c *gin.Context, userID, source string) (connectapi.BrowserSession, error) {
	if err := s.createCookieSession(c, userID, source); err != nil {
		return connectapi.BrowserSession{}, err
	}
	sessionID, _ := s.browserSessionID(c)
	browserSession := connectapi.BrowserSession{
		Revoke: func(ctx context.Context) error {
			if err := s.core.RevokeCookieSession(ctx, sessionID); err != nil {
				return err
			}
			s.clearBrowserSessionCookie(c)
			clearCSRFCookie(c)
			return nil
		},
	}
	if err := s.ensureCSRFToken(c); err != nil {
		_ = browserSession.Revoke(c.Request.Context())
		return connectapi.BrowserSession{}, err
	}
	return browserSession, nil
}

func markAuthenticationCookieResponsePrivate(c *gin.Context) {
	if c == nil || strings.Contains(strings.ToLower(c.Writer.Header().Get("Cache-Control")), "no-store") {
		return
	}
	c.Header("Cache-Control", "private, no-store")
}

func clearLegacyCookieAuthentication(session sessions.Session) {
	_ = removeLegacyCookieAuthentication(session)
}

func removeLegacyCookieAuthentication(session sessions.Session) error {
	if session == nil {
		return nil
	}
	hadAuthentication := session.Get(sessionKeyRuntimeCredentialID) != nil ||
		session.Get(retiredSessionKeyUserID) != nil ||
		session.Get(retiredSessionKeyCredentialID) != nil
	if !hadAuthentication {
		return nil
	}
	session.Delete(sessionKeyRuntimeCredentialID)
	session.Delete(retiredSessionKeyUserID)
	session.Delete(retiredSessionKeyCredentialID)
	return session.Save()
}

func legacyCookieSessionID(session sessions.Session) (string, bool) {
	if session == nil {
		return "", false
	}
	sessionID, _ := session.Get(sessionKeyRuntimeCredentialID).(string)
	return sessionID, sessionID != ""
}

func (s *HTTPServer) clearBrowserSessionCookie(c *gin.Context) {
	s.ensureBrowserSessionManagers()
	markAuthenticationCookieResponsePrivate(c)
	names := make(map[string]struct{})
	if c.Request != nil {
		for _, cookie := range c.Request.Cookies() {
			if isBrowserSessionCookieName(cookie.Name) {
				if len(names) >= browserSessionCleanupLimit {
					break
				}
				names[cookie.Name] = struct{}{}
			}
		}
	}
	if issued, ok := c.Get(issuedBrowserSessionKey); ok {
		if session, ok := issued.(issuedBrowserSession); ok && session.name != "" {
			names[session.name] = struct{}{}
		}
	}
	for name := range names {
		s.writeBrowserSessionCookie(c, name, "", time.Time{})
	}
}

type issuedBrowserSession struct {
	name  string
	token string
}

func (s *HTTPServer) writeBrowserSessionCookie(c *gin.Context, name, token string, expiry time.Time) {
	s.ensureBrowserSessionManagers()
	cookie := &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     s.browserSessions.Cookie.Path,
		Domain:   s.browserSessions.Cookie.Domain,
		HttpOnly: s.browserSessions.Cookie.HttpOnly,
		Secure:   s.browserSessions.Cookie.Secure,
		SameSite: s.browserSessions.Cookie.SameSite,
	}
	if expiry.IsZero() {
		cookie.Expires = time.Unix(1, 0)
		cookie.MaxAge = -1
	} else if s.browserSessions.Cookie.Persist {
		cookie.Expires = time.Unix(expiry.Unix()+1, 0)
		cookie.MaxAge = int(time.Until(expiry).Seconds() + 1)
	}
	c.Writer.Header().Add("Set-Cookie", cookie.String())
	c.Writer.Header().Add("Cache-Control", `no-cache="Set-Cookie"`)
}

type presentedRuntimeCredential struct {
	user              *evtv1.User
	auth              authctx.RuntimeCredential
	cookieRecord      *runtimestatev1.CookieSession
	presentedSessions []presentedCookieSession
}

type presentedCookieSession struct {
	handle string
	userID string
}

func (s *HTTPServer) browserSessionID(c *gin.Context) (string, bool) {
	if issued, ok := c.Get(issuedBrowserSessionKey); ok {
		if session, ok := issued.(issuedBrowserSession); ok && session.token != "" {
			return session.token, true
		}
	}
	cookies, err := browserSessionCookies(c.Request)
	if err == nil && len(cookies) == 1 {
		return cookies[0].token, true
	}
	return "", false
}

func (s *HTTPServer) validateCookieSession(c *gin.Context) (string, string, *runtimestatev1.CookieSession, bool) {
	credential, ok, _ := s.cookiePresentedCredential(c)
	if !ok {
		return "", "", nil, false
	}
	return credential.auth.UserID, credential.auth.Handle, credential.cookieRecord, true
}

func (s *HTTPServer) cookiePresentedCredential(c *gin.Context) (presentedRuntimeCredential, bool, error) {
	if err := authenticationValidationError(c.Request.Context()); err != nil {
		return presentedRuntimeCredential{}, false, err
	}

	var cookies []browserSessionCookie
	if issued, ok := c.Get(issuedBrowserSessionKey); ok {
		if session, ok := issued.(issuedBrowserSession); ok && session.name != "" && session.token != "" {
			// The session issued by this response is authoritative for follow-up
			// work such as CSRF binding. Do not let a request cookie created on a
			// clock-ahead replica outrank it.
			cookies = []browserSessionCookie{{name: session.name, token: session.token}}
		}
	}
	if len(cookies) == 0 {
		var err error
		cookies, err = browserSessionCookies(c.Request)
		if err != nil {
			return presentedRuntimeCredential{}, false, err
		}
	}
	if len(cookies) == 0 {
		return presentedRuntimeCredential{}, false, nil
	}

	var selected browserSessionCookie
	var record *runtimestatev1.CookieSession
	presentedSessions := make([]presentedCookieSession, 0, len(cookies))
	seenTokens := make(map[string]struct{}, len(cookies))
	for _, cookie := range cookies {
		if _, ok := seenTokens[cookie.token]; ok {
			continue
		}
		seenTokens[cookie.token] = struct{}{}
		candidate, err := s.core.ValidateCookieCredential(c.Request.Context(), cookie.token)
		if err != nil {
			if errors.Is(err, core.ErrCookieSessionNotFound) {
				continue
			}
			log.Warn("Failed to validate cookie session", "error", err)
			return presentedRuntimeCredential{}, false, err
		}
		presentedSessions = append(presentedSessions, presentedCookieSession{
			handle: cookie.token,
			userID: candidate.GetUserId(),
		})
		if record == nil || cookieSessionIsNewer(candidate, cookie, record, selected) {
			selected = cookie
			record = candidate
		}
	}
	if record == nil {
		return presentedRuntimeCredential{}, false, nil
	}
	userID := record.GetUserId()
	if userID == "" {
		return presentedRuntimeCredential{}, false, nil
	}

	user, err := s.core.GetUser(c.Request.Context(), userID)
	if err != nil {
		log.Warn("Failed to load user from cookie runtime credential", "userId", userID, "error", err)
		return presentedRuntimeCredential{}, false, nil
	}

	return presentedRuntimeCredential{
		user: user,
		auth: authctx.RuntimeCredential{
			Kind:      authctx.RuntimeCredentialKindCookieSession,
			UserID:    userID,
			Handle:    selected.token,
			ExpiresAt: record.GetExpiresAt().AsTime(),
		},
		cookieRecord:      record,
		presentedSessions: presentedSessions,
	}, true, nil
}

func cookieSessionIsNewer(candidate *runtimestatev1.CookieSession, candidateCookie browserSessionCookie, current *runtimestatev1.CookieSession, currentCookie browserSessionCookie) bool {
	candidateCreatedAt := candidate.GetCreatedAt().AsTime()
	currentCreatedAt := current.GetCreatedAt().AsTime()
	if !candidateCreatedAt.Equal(currentCreatedAt) {
		return candidateCreatedAt.After(currentCreatedAt)
	}
	return candidateCookie.name > currentCookie.name
}

func cookieSessionOptions(cfgTTL time.Duration, secure bool) sessions.Options {
	return sessions.Options{
		MaxAge:   int(cfgTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	}
}
