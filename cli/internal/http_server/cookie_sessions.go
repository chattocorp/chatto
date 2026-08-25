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
	markAuthenticationCookieResponsePrivate(c)
	session := sessions.Default(c)
	session.Set(sessionKeyRuntimeCredentialID, sessionID)
	session.Delete(retiredSessionKeyUserID)
	session.Delete(retiredSessionKeyCredentialID)
	return session.Save()
}

func (s *HTTPServer) saveValidatedCookieSession(c *gin.Context, sessionID string, expiresAt, now time.Time) error {
	if sessionID == "" || !now.Before(expiresAt) {
		return core.ErrCookieSessionNotFound
	}
	markAuthenticationCookieResponsePrivate(c)
	session := sessions.Default(c)
	session.Options(cookieSessionOptions(
		expiresAt.Sub(now),
		strings.HasPrefix(s.config.Webserver.URL, "https"),
	))
	session.Set(sessionKeyRuntimeCredentialID, sessionID)
	session.Delete(retiredSessionKeyUserID)
	session.Delete(retiredSessionKeyCredentialID)
	return session.Save()
}

func markAuthenticationCookieResponsePrivate(c *gin.Context) {
	if c == nil || strings.Contains(strings.ToLower(c.Writer.Header().Get("Cache-Control")), "no-store") {
		return
	}
	c.Header("Cache-Control", "private, no-store")
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
			Kind:      authctx.RuntimeCredentialKindCookieSession,
			UserID:    userID,
			Handle:    sessionID,
			ExpiresAt: record.GetExpiresAt().AsTime(),
		},
		cookieRecord: record,
	}, true, nil
}

func (s *HTTPServer) renewCookieSessionIfNeeded(c *gin.Context, sessionID string, record *corev1.CookieSession) *corev1.CookieSession {
	if record == nil || record.GetExpiresAt() == nil {
		return record
	}
	now := time.Now()
	if s.cookieSessionRenewalNow != nil {
		now = s.cookieSessionRenewalNow()
	}
	if shouldRenewCookieSession(record, s.config.Auth.TokenTTLOrDefault(), now) {
		renewed, _, err := s.core.RenewCookieSession(c.Request.Context(), sessionID, now)
		if err != nil {
			log.Warn("Failed to renew cookie session", "error", err)
			if errors.Is(err, core.ErrCookieSessionNotFound) {
				return record
			}
		} else {
			record = renewed
		}
	}

	if record.GetExpiresAt() == nil {
		return record
	}
	if err := s.saveValidatedCookieSession(c, sessionID, record.GetExpiresAt().AsTime(), now); err != nil {
		log.Warn("Failed to refresh validated browser cookie", "error", err)
	}
	return record
}

func shouldRenewCookieSession(record *corev1.CookieSession, ttl time.Duration, now time.Time) bool {
	if record == nil || record.GetExpiresAt() == nil || ttl <= 0 {
		return false
	}
	return record.GetExpiresAt().AsTime().Sub(now) <= ttl/4
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
