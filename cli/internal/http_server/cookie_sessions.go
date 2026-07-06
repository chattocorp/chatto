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
	sessionKeyUserID              = "user_id"
	sessionKeyCookieSessionID     = "cookie_session_id"
)

func (s *HTTPServer) createCookieSession(c *gin.Context, userID, source string) error {
	sessionID, _, err := s.core.CreateCookieSession(c.Request.Context(), userID, source)
	if err != nil {
		return err
	}
	return saveCookieSession(c, userID, sessionID)
}

func (s *HTTPServer) createCookieSessionForGeneration(c *gin.Context, userID, source string, authGeneration uint64) error {
	sessionID, _, err := s.core.CreateCookieSessionForGeneration(c.Request.Context(), userID, source, authGeneration)
	if err != nil {
		return err
	}
	return saveCookieSession(c, userID, sessionID)
}

func (s *HTTPServer) createConnectBrowserSession(c *gin.Context, userID, source string) (connectapi.BrowserSession, error) {
	if err := s.createCookieSession(c, userID, source); err != nil {
		return connectapi.BrowserSession{}, err
	}
	session := sessions.Default(c)
	cookieCredential, _ := cookieCredentialFromSession(session)
	browserSession := connectapi.BrowserSession{
		Revoke: func(ctx context.Context) error {
			_ = s.core.RevokeCookieSession(ctx, userID, cookieCredential.sessionID)
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

func saveCookieSession(c *gin.Context, userID, sessionID string) error {
	session := sessions.Default(c)
	session.Set(sessionKeyRuntimeCredentialID, sessionID)
	session.Delete(sessionKeyUserID)
	session.Delete(sessionKeyCookieSessionID)
	return session.Save()
}

func clearCookieSessionAuth(session sessions.Session) {
	if session == nil {
		return
	}
	session.Delete(sessionKeyRuntimeCredentialID)
	session.Delete(sessionKeyUserID)
	session.Delete(sessionKeyCookieSessionID)
	_ = session.Save()
}

type cookieCredential struct {
	userID    string
	sessionID string
	legacy    bool
}

type presentedRuntimeCredential struct {
	user         *corev1.User
	auth         authctx.RuntimeCredential
	cookieRecord *corev1.CookieSession
}

func cookieCredentialFromSession(session sessions.Session) (cookieCredential, bool) {
	if session == nil {
		return cookieCredential{}, false
	}
	if sessionID, _ := session.Get(sessionKeyRuntimeCredentialID).(string); sessionID != "" {
		return cookieCredential{sessionID: sessionID}, true
	}
	userID, _ := session.Get(sessionKeyUserID).(string)
	sessionID, _ := session.Get(sessionKeyCookieSessionID).(string)
	return cookieCredential{userID: userID, sessionID: sessionID, legacy: true}, userID != "" && sessionID != ""
}

func cookieSessionIDs(session sessions.Session) (string, string, bool) {
	credential, ok := cookieCredentialFromSession(session)
	if !ok {
		return "", "", false
	}
	return credential.userID, credential.sessionID, true
}

func (s *HTTPServer) validateCookieSession(c *gin.Context) (string, string, *corev1.CookieSession, bool) {
	credential, ok := s.cookiePresentedCredential(c)
	if !ok {
		return "", "", nil, false
	}
	return credential.auth.UserID, credential.auth.Handle, credential.cookieRecord, true
}

func (s *HTTPServer) cookiePresentedCredential(c *gin.Context) (presentedRuntimeCredential, bool) {
	if _, ok := c.Get(sessions.DefaultKey); !ok {
		return presentedRuntimeCredential{}, false
	}
	session := sessions.Default(c)
	credential, ok := cookieCredentialFromSession(session)
	if !ok {
		if session.Get(sessionKeyRuntimeCredentialID) != nil ||
			session.Get(sessionKeyUserID) != nil ||
			session.Get(sessionKeyCookieSessionID) != nil {
			clearCookieSessionAuth(session)
		}
		return presentedRuntimeCredential{}, false
	}

	var record *corev1.CookieSession
	var err error
	if credential.userID != "" {
		record, err = s.core.ValidateCookieSession(c.Request.Context(), credential.userID, credential.sessionID)
	} else {
		record, err = s.core.ValidateCookieCredential(c.Request.Context(), credential.sessionID)
	}
	if err != nil {
		if errors.Is(err, core.ErrCookieSessionNotFound) {
			clearCookieSessionAuth(session)
		} else {
			log.Warn("Failed to validate cookie session", "error", err)
		}
		return presentedRuntimeCredential{}, false
	}
	userID := record.GetUserId()
	if userID == "" {
		clearCookieSessionAuth(session)
		return presentedRuntimeCredential{}, false
	}

	user, err := s.core.GetUser(c.Request.Context(), userID)
	if err != nil {
		log.Warn("Failed to load user from cookie runtime credential", "userId", userID, "error", err)
		return presentedRuntimeCredential{}, false
	}

	return presentedRuntimeCredential{
		user: user,
		auth: authctx.RuntimeCredential{
			Kind:   authctx.RuntimeCredentialKindCookieSession,
			UserID: userID,
			Handle: credential.sessionID,
		},
		cookieRecord: record,
	}, true
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
	session.Delete(sessionKeyUserID)
	session.Delete(sessionKeyCookieSessionID)
	if err := session.Save(); err != nil {
		log.Warn("Failed to save rotated cookie session", "userId", userID, "error", err)
		_ = s.core.RevokeCookieSession(c.Request.Context(), userID, newSessionID)
		return
	}

	if err := s.core.RevokeCookieSession(c.Request.Context(), userID, oldSessionID); err != nil {
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
