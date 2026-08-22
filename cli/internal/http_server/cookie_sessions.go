package http_server

import (
	"context"
	"errors"
	"net/http"
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
	sessionKeyRuntimeCredentialID = "runtime_credential_id"
	// Retired signed-session keys are deleted when cookie auth is saved or
	// cleared, but are never accepted as authentication inputs.
	retiredSessionKeyUserID       = "user_id"
	retiredSessionKeyCredentialID = "cookie_session_id"
)

func (s *HTTPServer) createCookieSession(c *gin.Context, userID, source string) error {
	sessionID, _, err := s.core.CreateCookieSession(c.Request.Context(), userID, source)
	if err != nil {
		return err
	}
	return saveCookieSession(c, sessionID)
}

func (s *HTTPServer) createCookieSessionForGeneration(c *gin.Context, userID, source string, authGeneration uint64) error {
	sessionID, _, err := s.core.CreateCookieSessionForGeneration(c.Request.Context(), userID, source, authGeneration)
	if err != nil {
		return err
	}
	return saveCookieSession(c, sessionID)
}

func (s *HTTPServer) createConnectBrowserSession(c *gin.Context, userID, source string) (connectapi.BrowserSession, error) {
	if err := s.createCookieSession(c, userID, source); err != nil {
		return connectapi.BrowserSession{}, err
	}
	session := sessions.Default(c)
	sessionID, _ := cookieCredentialIDFromSession(session)
	browserSession := connectapi.BrowserSession{
		Revoke: func(ctx context.Context) error {
			_ = s.core.RevokeCookieSession(ctx, sessionID)
			clearCookieSessionAuth(session)
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

func saveCookieSession(c *gin.Context, sessionID string) error {
	session := sessions.Default(c)
	session.Set(sessionKeyRuntimeCredentialID, sessionID)
	session.Delete(retiredSessionKeyUserID)
	session.Delete(retiredSessionKeyCredentialID)
	return session.Save()
}

func clearCookieSessionAuth(session sessions.Session) {
	if session == nil {
		return
	}
	session.Delete(sessionKeyRuntimeCredentialID)
	session.Delete(retiredSessionKeyUserID)
	session.Delete(retiredSessionKeyCredentialID)
	_ = session.Save()
}

type presentedRuntimeCredential struct {
	user         *corev1.User
	auth         authctx.RuntimeCredential
	cookieRecord *corev1.CookieSession
}

func cookieCredentialIDFromSession(session sessions.Session) (string, bool) {
	if session == nil {
		return "", false
	}
	if sessionID, _ := session.Get(sessionKeyRuntimeCredentialID).(string); sessionID != "" {
		return sessionID, true
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
	if _, ok := c.Get(sessions.DefaultKey); !ok {
		return presentedRuntimeCredential{}, false, nil
	}
	session := sessions.Default(c)
	sessionID, ok := cookieCredentialIDFromSession(session)
	if !ok {
		if session.Get(sessionKeyRuntimeCredentialID) != nil ||
			session.Get(retiredSessionKeyUserID) != nil ||
			session.Get(retiredSessionKeyCredentialID) != nil {
			clearCookieSessionAuth(session)
		}
		return presentedRuntimeCredential{}, false, nil
	}

	record, err := s.core.ValidateCookieCredential(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, core.ErrCookieSessionNotFound) {
			clearCookieSessionAuth(session)
			return presentedRuntimeCredential{}, false, nil
		}
		log.Warn("Failed to validate cookie session", "error", err)
		return presentedRuntimeCredential{}, false, err
	}
	userID := record.GetUserId()
	if userID == "" {
		clearCookieSessionAuth(session)
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
			Kind:   authctx.RuntimeCredentialKindCookieSession,
			UserID: userID,
			Handle: sessionID,
		},
		cookieRecord: record,
	}, true, nil
}

func (s *HTTPServer) rotateCookieSessionIfNeeded(c *gin.Context, userID, oldSessionID string, record *corev1.CookieSession) {
	if record == nil || record.GetExpiresAt() == nil {
		return
	}
	if !shouldRotateCookieSession(record, s.config.Auth.TokenTTLOrDefault()) {
		return
	}

	newSessionID, _, err := s.core.CreateCookieSessionForGenerationPreservingFreshAuth(c.Request.Context(), userID, "session_rotation", record.GetAuthGeneration(), record)
	if err != nil {
		log.Warn("Failed to rotate cookie session", "userId", userID, "error", err)
		if errors.Is(err, core.ErrCookieSessionNotFound) {
			clearCookieSessionAuth(sessions.Default(c))
		}
		return
	}

	session := sessions.Default(c)
	session.Set(sessionKeyRuntimeCredentialID, newSessionID)
	session.Delete(retiredSessionKeyUserID)
	session.Delete(retiredSessionKeyCredentialID)
	if err := session.Save(); err != nil {
		log.Warn("Failed to save rotated cookie session", "userId", userID, "error", err)
		_ = s.core.RevokeCookieSession(c.Request.Context(), newSessionID)
		return
	}

	if err := s.core.RevokeCookieSession(c.Request.Context(), oldSessionID); err != nil {
		log.Warn("Failed to revoke old rotated cookie session", "userId", userID, "error", err)
	}
}

func shouldRotateCookieSession(record *corev1.CookieSession, ttl time.Duration) bool {
	if record == nil || record.GetExpiresAt() == nil || ttl <= 0 {
		return false
	}
	return time.Until(record.GetExpiresAt().AsTime()) <= ttl/4
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
