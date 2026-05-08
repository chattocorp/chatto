//go:build test_endpoints

package http_server

import (
	"net/http"
	"net/url"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/email"
)

// createMailer creates a mock mailer when test endpoints are enabled.
// Returns (mockMailer, mailer) where mailer is used for sending emails.
// The smtpConfig parameter is ignored when test endpoints are enabled.
func createMailer(_ config.SMTPConfig) (*email.MockSender, email.Sender) {
	mock := email.NewMockSender(true)
	log.Info("Test endpoints enabled - using mock email sender")
	return mock, mock
}

// registerTestEndpoints registers test-only HTTP endpoints for development and testing.
// These endpoints bypass security controls and should NEVER be available in production.
//
// Available endpoints:
//   - GET /auth/test/last-email - Retrieve the last captured email
//   - DELETE /auth/test/emails - Clear all captured emails
//   - POST /auth/test/verify-email - Directly verify a user's email
//   - POST /auth/test/create-user - Directly create a user without registration flow
//   - POST /auth/test/oauth-callback - Simulate OAuth callback
//   - POST /auth/test/oauth-authorize - Mint an OAuth authorization code without UI interaction
func registerTestEndpoints(auth *gin.RouterGroup, s *HTTPServer) {
	if s.mockMailer == nil {
		return
	}

	log.Warn("TEST EMAIL ENDPOINTS ENABLED - These endpoints bypass email verification and OAuth. " +
		"Ensure this build is not used in production!")

	auth.GET("test/last-email", func(c *gin.Context) {
		msg := s.mockMailer.LastMessage()
		if msg == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "No emails captured"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"to":      msg.To,
			"subject": msg.Subject,
			"body":    msg.Body,
		})
	})

	auth.DELETE("test/emails", func(c *gin.Context) {
		s.mockMailer.Reset()
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Test-only endpoint to directly verify a user's email (bypasses email verification flow)
	auth.POST("test/verify-email", func(c *gin.Context) {
		var req struct {
			UserID string `json:"userId" binding:"required"`
			Email  string `json:"email" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := s.core.AddVerifiedEmailDirect(c.Request.Context(), req.UserID, req.Email); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Test-only endpoint to create a user directly. Bypasses registration,
	// email verification, and session creation. Replaces the production
	// `createUser` GraphQL mutation that was removed for security (see #175):
	// production `createUser` was unauthenticated and let any caller win the
	// race to become instance owner. The test endpoint is gated behind the
	// `test_endpoints` build tag so it is never compiled into release
	// binaries — same trust model as `/auth/test/verify-email` above.
	auth.POST("test/create-user", func(c *gin.Context) {
		var req struct {
			Login       string `json:"login" binding:"required"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		displayName := req.DisplayName
		if displayName == "" {
			displayName = req.Login
		}
		user, err := s.core.CreateUser(c.Request.Context(), "system", req.Login, displayName, req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Auto-join the deployment's server space so the test user is a
		// member by default. No-op on fresh installs.
		s.core.JoinServer(c.Request.Context(), user.Id)
		c.JSON(http.StatusOK, gin.H{
			"id":          user.Id,
			"login":       user.Login,
			"displayName": user.DisplayName,
		})
	})

	// Test-only endpoint to create a registration token (bypasses email delivery).
	// Returns the token so E2E tests can navigate directly to /register/complete?token=...
	auth.POST("test/create-registration-token", func(c *gin.Context) {
		var req struct {
			Email string `json:"email" binding:"required,email"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		token, err := s.core.CreateRegistrationToken(c.Request.Context(), req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": token, "email": req.Email})
	})

	// Test-only endpoint to simulate OAuth callback.
	// This replicates the logic in the real OAuth callback for testing.
	auth.POST("test/oauth-callback", func(c *gin.Context) {
		var req struct {
			Email       string `json:"email" binding:"required,email"`
			DisplayName string `json:"displayName"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		ctx := c.Request.Context()
		var isNewUser bool

		// Try to find existing user by verified email (same as real OAuth callback)
		existingUser, err := s.core.GetUserByVerifiedEmail(ctx, req.Email)
		if err != nil {
			// User does not exist, create a new one
			login := deriveLoginFromEmail(req.Email)
			displayName := req.DisplayName
			if displayName == "" {
				displayName = login
			}
			newUser, err := s.core.CreateUser(ctx, "system", login, displayName, "")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user: " + err.Error()})
				return
			}
			isNewUser = true

			// Auto-join the deployment's server space so the new user is a
			// member by default. No-op on fresh installs.
			s.core.JoinServer(ctx, newUser.Id)

			// Auto-verify OAuth email (same as real OAuth callback)
			if req.Email != "" {
				if err := s.core.AddVerifiedEmailDirect(ctx, newUser.Id, req.Email); err != nil {
					log.Warn("Failed to auto-verify OAuth email", "error", err, "userId", newUser.Id)
					// Don't fail - continue with login
				} else {
					log.Info("Auto-verified OAuth email", "userId", newUser.Id, "email", req.Email)
				}
			}

			// Create session
			session := sessions.Default(c)
			session.Set("user_id", newUser.Id)
			if err := session.Save(); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save session"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"success":   true,
				"isNewUser": isNewUser,
				"user": gin.H{
					"id":    newUser.Id,
					"login": newUser.Login,
				},
			})
			return
		}

		// Create session for existing user
		session := sessions.Default(c)
		session.Set("user_id", existingUser.Id)
		if err := session.Save(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save session"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":   true,
			"isNewUser": false,
			"user": gin.H{
				"id":    existingUser.Id,
				"login": existingUser.Login,
			},
		})
	})

	// Test-only endpoint to mint an OAuth authorization code for a known user
	// without going through the login UI. Used by multi-instance E2E tests that
	// drive the real Add-Server dialog → /oauth/authorize → /instances/callback
	// flow but bypass the human OAuth login form via Playwright route interception.
	auth.POST("test/oauth-authorize", func(c *gin.Context) {
		var req struct {
			UserID              string `json:"userId" binding:"required"`
			RedirectURI         string `json:"redirectUri" binding:"required"`
			CodeChallenge       string `json:"codeChallenge" binding:"required"`
			CodeChallengeMethod string `json:"codeChallengeMethod" binding:"required"`
			State               string `json:"state"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.CodeChallengeMethod != "S256" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "code_challenge_method must be S256"})
			return
		}
		if !isValidRedirectURI(req.RedirectURI) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri: must be HTTPS or localhost"})
			return
		}

		ctx := c.Request.Context()
		if _, err := s.core.GetUser(ctx, req.UserID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found: " + err.Error()})
			return
		}

		code, err := s.core.CreateAuthCode(ctx, req.UserID, req.RedirectURI, req.CodeChallenge, req.CodeChallengeMethod)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create auth code: " + err.Error()})
			return
		}

		u, err := url.Parse(req.RedirectURI)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid redirect_uri"})
			return
		}
		q := u.Query()
		q.Set("code", code)
		if req.State != "" {
			q.Set("state", req.State)
		}
		u.RawQuery = q.Encode()

		c.JSON(http.StatusOK, gin.H{"redirectURL": u.String()})
	})
}

// registerTestWebhookEndpoints registers test-only webhook endpoints that bypass
// LiveKit HMAC validation. These allow E2E tests to simulate call join/leave events
// without a real LiveKit server.
func registerTestWebhookEndpoints(webhooks *gin.RouterGroup, s *HTTPServer) {
	log.Warn("TEST WEBHOOK ENDPOINTS ENABLED - These endpoints bypass webhook HMAC validation.")

	// Simulate a participant joining a call
	webhooks.POST("/test/call-join", func(c *gin.Context) {
		var req struct {
			SpaceID     string `json:"spaceId" binding:"required"`
			RoomID      string `json:"roomId" binding:"required"`
			UserID      string `json:"userId" binding:"required"`
			DisplayName string `json:"displayName"`
			Login       string `json:"login"`
			AvatarURL   string `json:"avatarUrl"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := s.core.HandleCallParticipantJoined(
			c.Request.Context(),
			req.SpaceID, req.RoomID,
			req.UserID, req.DisplayName,
			req.Login, req.AvatarURL,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Simulate a participant leaving a call
	webhooks.POST("/test/call-leave", func(c *gin.Context) {
		var req struct {
			SpaceID string `json:"spaceId" binding:"required"`
			RoomID  string `json:"roomId" binding:"required"`
			UserID  string `json:"userId" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := s.core.HandleCallParticipantLeft(
			c.Request.Context(),
			req.SpaceID, req.RoomID,
			req.UserID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
}
