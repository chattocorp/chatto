package http_server

import (
	"net/http"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	sessionKeyUserID          = "user_id"
	sessionKeyCookieSessionID = "cookie_session_id"
)

func (s *HTTPServer) createCookieSession(c *gin.Context, userID, source string) error {
	sessionID, _, err := s.core.CreateCookieSession(c.Request.Context(), userID, source)
	if err != nil {
		return err
	}

	session := sessions.Default(c)
	session.Set(sessionKeyUserID, userID)
	session.Set(sessionKeyCookieSessionID, sessionID)
	return session.Save()
}

func clearCookieSessionAuth(session sessions.Session) {
	if session == nil {
		return
	}
	session.Delete(sessionKeyUserID)
	session.Delete(sessionKeyCookieSessionID)
	_ = session.Save()
}

func cookieSessionIDs(session sessions.Session) (string, string, bool) {
	if session == nil {
		return "", "", false
	}
	userID, _ := session.Get(sessionKeyUserID).(string)
	sessionID, _ := session.Get(sessionKeyCookieSessionID).(string)
	return userID, sessionID, userID != "" && sessionID != ""
}

func (s *HTTPServer) validateCookieSession(c *gin.Context) (string, string, *corev1.CookieSession, bool) {
	session := sessions.Default(c)
	userID, sessionID, ok := cookieSessionIDs(session)
	if !ok {
		if userID != "" || sessionID != "" {
			clearCookieSessionAuth(session)
		}
		return "", "", nil, false
	}

	record, err := s.core.ValidateCookieSession(c.Request.Context(), userID, sessionID)
	if err != nil {
		if err != core.ErrCookieSessionNotFound {
			log.Warn("Failed to validate cookie session", "userId", userID, "error", err)
		}
		clearCookieSessionAuth(session)
		return "", "", nil, false
	}

	return userID, sessionID, record, true
}

func (s *HTTPServer) rotateCookieSessionIfNeeded(c *gin.Context, userID, oldSessionID string, record *corev1.CookieSession) {
	if record == nil || record.GetExpiresAt() == nil {
		return
	}
	if !shouldRotateCookieSession(record, s.config.Auth.TokenTTLOrDefault()) {
		return
	}

	newSessionID, _, err := s.core.CreateCookieSession(c.Request.Context(), userID, "session_rotation")
	if err != nil {
		log.Warn("Failed to rotate cookie session", "userId", userID, "error", err)
		return
	}

	session := sessions.Default(c)
	session.Set(sessionKeyUserID, userID)
	session.Set(sessionKeyCookieSessionID, newSessionID)
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
