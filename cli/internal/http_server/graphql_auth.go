package http_server

import (
	"net/http"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	"hmans.de/chatto/internal/graph/dataloader"
)

// injectUserIntoContext extracts the authenticated user from either a bearer token
// or the Gin session cookie, and returns an updated http.Request with the user
// injected into its context.
// Returns the original request if no user is authenticated (allowing unauthenticated requests).
func (s *HTTPServer) injectUserIntoContext(c *gin.Context) *http.Request {
	ctx := c.Request.Context()

	// 1. Check Authorization: Bearer <token> header first (cross-origin clients)
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		if token, ok := strings.CutPrefix(authHeader, "Bearer "); ok && strings.TrimSpace(token) != "" {
			token = strings.TrimSpace(token)
			userID, err := s.core.ValidateAuthToken(ctx, token)
			if err == nil {
				user, err := s.core.GetUser(ctx, userID)
				if err == nil {
					ctx = auth.WithUser(ctx, user)
					return c.Request.WithContext(ctx)
				}
				log.Warn("Bearer token valid but user not found", "userId", userID, "error", err)
			}
			// Invalid/expired token: fall through to session cookie check
		}
	}

	// 2. Fall back to session cookie (embedded SPA clients)
	userID, sessionID, cookieSession, ok := s.validateCookieSession(c)
	if !ok {
		return c.Request
	}

	user, err := s.core.GetUser(ctx, userID)
	if err != nil {
		log.Warn("Failed to load user from session", "userId", userID, "error", err)
		clearCookieSessionAuth(sessions.Default(c))
		return c.Request
	}

	s.rotateCookieSessionIfNeeded(c, userID, sessionID, cookieSession)

	ctx = auth.WithUser(ctx, user)
	return c.Request.WithContext(ctx)
}

// injectDataloadersIntoContext creates fresh dataloaders for this request
// and returns an updated http.Request with the loaders injected into its context.
func injectDataloadersIntoContext(r *http.Request, c *core.ChattoCore) *http.Request {
	ctx := r.Context()
	loaders := dataloader.NewLoaders(c)
	ctx = dataloader.WithLoaders(ctx, loaders)
	return r.WithContext(ctx)
}
