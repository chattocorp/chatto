package http_server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	// Retired signed-session keys are migrated to the dedicated SCS cookie and
	// then deleted. They are never the primary authentication representation.
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
	sessionCtx := s.browserCookieContext(c.Request.Context())
	if err := s.browserSessions.RenewToken(sessionCtx); err != nil {
		return "", err
	}
	s.browserSessions.Put(sessionCtx, browserSessionValueKey, data)
	s.browserSessions.SetDeadline(sessionCtx, data.ExpiresAt)
	token, expiry, err := s.browserSessions.Commit(sessionCtx)
	if err != nil {
		return "", err
	}

	c.Set(issuedBrowserSessionKey, token)
	markAuthenticationCookieResponsePrivate(c)
	s.browserSessions.WriteSessionCookie(sessionCtx, c.Writer, token, expiry)
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
	if session == nil {
		return
	}
	hadAuthentication := session.Get(sessionKeyRuntimeCredentialID) != nil ||
		session.Get(retiredSessionKeyUserID) != nil ||
		session.Get(retiredSessionKeyCredentialID) != nil
	if !hadAuthentication {
		return
	}
	session.Delete(sessionKeyRuntimeCredentialID)
	session.Delete(retiredSessionKeyUserID)
	session.Delete(retiredSessionKeyCredentialID)
	_ = session.Save()
}

func (s *HTTPServer) clearBrowserSessionCookie(c *gin.Context) {
	s.ensureBrowserSessionManagers()
	markAuthenticationCookieResponsePrivate(c)
	s.browserSessions.WriteSessionCookie(c.Request.Context(), c.Writer, "", time.Time{})
}

type presentedRuntimeCredential struct {
	user         *corev1.User
	auth         authctx.RuntimeCredential
	cookieRecord *corev1.CookieSession
}

func (s *HTTPServer) browserSessionID(c *gin.Context) (string, bool) {
	if issued, ok := c.Get(issuedBrowserSessionKey); ok {
		if handle, ok := issued.(string); ok && handle != "" {
			return handle, true
		}
	}
	if cookie, err := c.Request.Cookie(browserSessionCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, true
	}
	if _, exists := c.Get(sessions.DefaultKey); exists {
		session := sessions.Default(c)
		if token, _ := session.Get(sessionKeyRuntimeCredentialID).(string); token != "" {
			return token, true
		}
	}
	return "", false
}

func (s *HTTPServer) validateCookieSession(c *gin.Context) (string, string, *corev1.CookieSession, bool) {
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

	sessionID, ok := s.browserSessionID(c)
	if !ok {
		return presentedRuntimeCredential{}, false, nil
	}

	record, err := s.core.ValidateCookieCredential(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, core.ErrCookieSessionNotFound) {
			return presentedRuntimeCredential{}, false, nil
		}
		log.Warn("Failed to validate cookie session", "error", err)
		return presentedRuntimeCredential{}, false, err
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

	// A release upgrade migrates the old mixed-purpose signed cookie to the
	// dedicated opaque authentication cookie without replacing the server-side
	// authority. Normal requests never re-sign an existing SCS cookie.
	_, issuedInThisResponse := c.Get(issuedBrowserSessionKey)
	if _, err := c.Request.Cookie(browserSessionCookieName); err != nil &&
		!issuedInThisResponse &&
		c.Request.URL.Path != serverDiscoveryConnectPath {
		markAuthenticationCookieResponsePrivate(c)
		cookieCtx := s.browserCookieContext(c.Request.Context())
		s.browserSessions.WriteSessionCookie(cookieCtx, c.Writer, sessionID, record.GetExpiresAt().AsTime())
		clearLegacyCookieAuthentication(sessions.Default(c))
	}

	return presentedRuntimeCredential{
		user: user,
		auth: authctx.RuntimeCredential{
			Kind:      authctx.RuntimeCredentialKindCookieSession,
			UserID:    userID,
			Handle:    sessionID,
			ExpiresAt: record.GetExpiresAt().AsTime(),
		},
		cookieRecord: record,
	}, true, nil
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
