package http_server

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
)

// Pre-compiled regexes for login validation
var (
	validLoginRegex   = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	invalidCharsRegex = regexp.MustCompile(`[^a-z0-9._-]`)
)

func (s *HTTPServer) setupAuthRoutes() {
	auth := s.router.Group("/auth")

	auth.POST("logout", func(c *gin.Context) {
		ctx := c.Request.Context()

		// Read user ID before clearing session (needed for session terminated event)
		session := sessions.Default(c)
		userID, _ := session.Get("user_id").(string)

		// If authenticated via bearer token, revoke it
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			if token, ok := strings.CutPrefix(authHeader, "Bearer "); ok && strings.TrimSpace(token) != "" {
				if err := s.core.RevokeAuthToken(ctx, strings.TrimSpace(token)); err != nil {
					log.Warn("Failed to revoke bearer token on logout", "error", err)
				}
			}
		}

		// Clear the session cookie
		session.Clear()
		session.Save()

		// Publish session terminated event so other tabs/devices disconnect
		if userID != "" {
			if err := s.core.PublishSessionTerminated(ctx, userID, "logout"); err != nil {
				log.Warn("Failed to publish session terminated event", "error", err)
			}
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Revoke a specific bearer token
	auth.POST("revoke-token", func(c *gin.Context) {
		var req struct {
			Token string `json:"token" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Token is required"})
			return
		}

		ctx := c.Request.Context()
		if err := s.core.RevokeAuthToken(ctx, req.Token); err != nil {
			log.Error("Failed to revoke token", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Password login endpoint
	// Accepts login name (username) via "login" or "identifier" field
	auth.POST("login", func(c *gin.Context) {
		var loginRequest struct {
			Login      string `json:"login"`
			Identifier string `json:"identifier"` // Alternative field name used by frontend
			Password   string `json:"password" binding:"required"`
		}

		// Parse request body
		if err := c.ShouldBindJSON(&loginRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Password is required"})
			return
		}

		// Accept either "login" or "identifier" field
		login := loginRequest.Login
		if login == "" {
			login = loginRequest.Identifier
		}

		if login == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Login is required"})
			return
		}

		// Validate identifier length to prevent abuse
		// Email addresses can be up to 254 characters (RFC 5321), usernames up to 32
		maxLength := 32
		if strings.Contains(login, "@") {
			maxLength = 254
		}
		if len(login) > maxLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid credentials"})
			return
		}

		// Verify credentials by login name
		ctx := c.Request.Context()
		user, err := s.core.VerifyPassword(ctx, login, loginRequest.Password)
		if err != nil {
			log.Error("Login failed", "login", login, "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		// Create session
		session := sessions.Default(c)
		session.Set("user_id", user.Id)
		err = session.Save()
		if err != nil {
			log.Error("Failed to save session", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}

		log.Info("User logged in successfully", "userId", user.Id, "login", user.Login)

		response := gin.H{
			"success": true,
			"user":    gin.H{"id": user.Id, "login": user.Login},
		}

		// Issue a bearer token (cross-origin clients use this instead of the session cookie)
		if token, err := s.core.CreateAuthToken(ctx, user.Id); err == nil {
			response["token"] = token
		} else {
			log.Warn("Failed to create auth token on login", "userId", user.Id, "error", err)
		}

		c.JSON(http.StatusOK, response)
	})

	// Email-first registration endpoint (step 1)
	// Accepts email only, creates a registration token, and sends a verification email.
	// The user completes account creation via POST /auth/register/complete after clicking
	// the email link.
	auth.POST("register", func(c *gin.Context) {
		// Check if registration is enabled
		if !s.config.Auth.DirectRegistrationOrDefault() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Registration is disabled"})
			return
		}

		var req struct {
			Email string `json:"email" binding:"required,email"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address is required"})
			return
		}
		// Normalize at the HTTP boundary so downstream core code can treat email as canonical.
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))

		// Require mailer — can't do email-first registration without email delivery
		if s.mailer == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Email delivery is not configured"})
			return
		}

		ctx := c.Request.Context()

		// Check if email is already claimed — but always return 200 to prevent enumeration
		emailClaimed, err := s.core.IsEmailClaimed(ctx, req.Email)
		if err != nil {
			log.Error("Failed to check email availability", "email", req.Email, "error", err)
		}
		if emailClaimed {
			// Don't reveal that the email is taken — just return success
			log.Info("Registration attempt for already-claimed email", "email", req.Email)
			c.JSON(http.StatusOK, gin.H{
				"message": "If this email is available, you will receive a registration link.",
			})
			return
		}

		// Create registration token
		token, err := s.core.CreateRegistrationToken(ctx, req.Email)
		if err != nil {
			log.Error("Failed to create registration token", "email", req.Email, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}

		// Send registration email
		completeURL := fmt.Sprintf("%s/register/complete?token=%s", s.config.Webserver.URL, token)
		err = s.mailer.Send(email.Message{
			To:      req.Email,
			Subject: "Complete your Chatto registration",
			Body:    fmt.Sprintf("Welcome to Chatto!\n\nClick the link below to complete your registration:\n\n%s\n\nThis link will expire in 24 hours.\n\nIf you didn't request this, you can ignore this email.", completeURL),
		})
		if err != nil {
			log.Error("Failed to send registration email", "email", req.Email, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
			return
		}

		log.Info("Sent registration email", "email", req.Email)
		c.JSON(http.StatusOK, gin.H{
			"message": "If this email is available, you will receive a registration link.",
		})
	})

	// Registration completion endpoint (step 2)
	// Validates the registration token, creates the user account, verifies the email,
	// and creates a session.
	auth.POST("register/complete", func(c *gin.Context) {
		// Check if registration is enabled
		if !s.config.Auth.DirectRegistrationOrDefault() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Registration is disabled"})
			return
		}

		var req struct {
			Token                string `json:"token" binding:"required"`
			Login                string `json:"login" binding:"required"`
			Password             string `json:"password" binding:"required,min=8"`
			PasswordConfirmation string `json:"passwordConfirmation" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Token, login, password, and password confirmation are required"})
			return
		}

		ctx := c.Request.Context()

		// Validate token (not consumed on validation failure — user can retry)
		tokenData, err := s.core.GetRegistrationToken(ctx, req.Token)
		if err != nil {
			if errors.Is(err, core.ErrRegistrationTokenNotFound) || errors.Is(err, core.ErrRegistrationTokenExpired) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired registration link"})
				return
			}
			log.Error("Failed to validate registration token", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}

		// Validate login format
		if !isValidLogin(req.Login) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Login must be 2-32 characters, using only letters, numbers, dots, dashes, or underscores (no consecutive periods)"})
			return
		}

		// Validate passwords match
		if req.Password != req.PasswordConfirmation {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords do not match"})
			return
		}

		// Check if login is blocked
		isBlocked, err := s.core.ConfigManager().IsUsernameBlocked(ctx, req.Login)
		if err != nil {
			log.Error("Failed to check blocked usernames", "login", req.Login, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}
		if isBlocked {
			c.JSON(http.StatusBadRequest, gin.H{"error": "This username is not available"})
			return
		}

		// Check if email was claimed while token was outstanding
		emailClaimed, err := s.core.IsEmailClaimed(ctx, tokenData.Email)
		if err != nil {
			log.Error("Failed to check email availability", "email", tokenData.Email, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}
		if emailClaimed {
			c.JSON(http.StatusConflict, gin.H{"error": "This email address is already in use"})
			return
		}

		// Create user with verified email atomically (use login as display name initially)
		user, err := s.core.CreateVerifiedUser(ctx, "system", req.Login, req.Login, req.Password, tokenData.Email)
		if err != nil {
			if errors.Is(err, core.ErrLoginAlreadyTaken) {
				c.JSON(http.StatusConflict, gin.H{"error": "Username is already taken"})
				return
			}
			if errors.Is(err, core.ErrUsernameBlocked) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "This username is not available"})
				return
			}
			if errors.Is(err, core.ErrEmailAlreadyVerified) {
				c.JSON(http.StatusConflict, gin.H{"error": "This email address is already in use"})
				return
			}
			if errors.Is(err, core.ErrLimitExceeded) {
				c.JSON(http.StatusForbidden, gin.H{"error": "This instance is not accepting new users"})
				return
			}
			log.Error("Registration failed", "login", req.Login, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}

		// Delete registration token (consumed)
		if err := s.core.DeleteRegistrationToken(ctx, req.Token); err != nil {
			log.Error("Failed to delete registration token", "error", err)
			// Don't fail — user was created successfully
		}

		// Create session
		session := sessions.Default(c)
		session.Set("user_id", user.Id)
		if err := session.Save(); err != nil {
			log.Error("Failed to save session", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}

		log.Info("User registered and logged in", "userId", user.Id, "login", user.Login)

		response := gin.H{
			"success": true,
			"user":    gin.H{"id": user.Id, "login": user.Login},
		}

		if token, err := s.core.CreateAuthToken(ctx, user.Id); err == nil {
			response["token"] = token
		} else {
			log.Warn("Failed to create auth token on register", "userId", user.Id, "error", err)
		}

		c.JSON(http.StatusOK, response)
	})

	// Email verification endpoint
	auth.GET("verify-email", func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.Redirect(http.StatusTemporaryRedirect, "/?error=missing_token")
			return
		}

		ctx := c.Request.Context()

		userID, err := s.core.VerifyEmail(ctx, token)
		if err != nil {
			if errors.Is(err, core.ErrTokenNotFound) || errors.Is(err, core.ErrTokenExpired) {
				log.Warn("Email verification failed: invalid or expired token")
				c.Redirect(http.StatusTemporaryRedirect, "/?error=invalid_token")
				return
			}
			if errors.Is(err, core.ErrEmailAlreadyVerified) {
				log.Warn("Email verification failed: email already verified by another user")
				c.Redirect(http.StatusTemporaryRedirect, "/?error=email_taken")
				return
			}
			log.Error("Email verification failed", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/?error=verification_failed")
			return
		}

		log.Info("Email verified successfully", "userId", userID)
		c.Redirect(http.StatusTemporaryRedirect, "/?email_verified=true")
	})

	// Forgot password endpoint - request a password reset email
	// Always returns 200 to prevent email enumeration
	auth.POST("forgot-password", func(c *gin.Context) {
		var req struct {
			Email string `json:"email" binding:"required,email"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email format"})
			return
		}

		ctx := c.Request.Context()
		normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))

		// Create token (returns empty string if email not found - no error)
		token, err := s.core.CreatePasswordResetToken(ctx, normalizedEmail)
		if err != nil {
			// Log error but don't expose to user
			log.Error("Failed to create password reset token", "error", err)
		}

		// Only send email if token was created (email exists and is verified)
		if token != "" && s.mailer != nil {
			resetURL := fmt.Sprintf("%s/reset-password?token=%s", s.config.Webserver.URL, token)
			err = s.mailer.Send(email.Message{
				To:      normalizedEmail,
				Subject: "Reset your Chatto password",
				Body:    fmt.Sprintf("Hi,\n\nWe received a request to reset your password for your Chatto account.\n\nClick the link below to set a new password:\n\n%s\n\nThis link will expire in 1 hour.\n\nIf you didn't request this, you can safely ignore this email.\n\n- The Chatto Team", resetURL),
			})
			if err != nil {
				log.Error("Failed to send password reset email", "email", normalizedEmail, "error", err)
			} else {
				log.Info("Sent password reset email", "email", normalizedEmail)
			}
		}

		// Always return success to prevent email enumeration
		c.JSON(http.StatusOK, gin.H{
			"message": "If that email is registered, you will receive a password reset link.",
		})
	})

	// Reset password endpoint - set a new password using a reset token
	auth.POST("reset-password", func(c *gin.Context) {
		var req struct {
			Token    string `json:"token" binding:"required"`
			Password string `json:"password" binding:"required,min=8"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Token and password (min 8 characters) are required"})
			return
		}

		ctx := c.Request.Context()

		// Hash the new password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			log.Error("Failed to hash password", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process password"})
			return
		}

		// Reset password (validates token, updates password, deletes token)
		err = s.core.ResetPassword(ctx, req.Token, string(hashedPassword))
		if err != nil {
			if errors.Is(err, core.ErrPasswordResetTokenNotFound) || errors.Is(err, core.ErrPasswordResetTokenExpired) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired reset link"})
				return
			}
			log.Error("Failed to reset password", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reset password"})
			return
		}

		log.Info("Password reset successfully")
		c.JSON(http.StatusOK, gin.H{"message": "Password has been reset. You can now log in."})
	})

	// Register test endpoints if built with -tags test_endpoints
	registerTestEndpoints(auth, s)
}


// isValidLogin validates that a login name meets the requirements:
// 2-32 characters, alphanumeric with dots, dashes, or underscores.
// Consecutive periods (..) are not allowed.
func isValidLogin(login string) bool {
	if len(login) < 2 || len(login) > 32 {
		return false
	}
	if strings.Contains(login, "..") {
		return false
	}
	return validLoginRegex.MatchString(login)
}

// deriveLoginFromEmail extracts a login name from an email address.
// Takes the part before @, converts to lowercase, and removes invalid characters.
// Valid characters: alphanumeric, underscore, dash, dot (2-32 chars).
func deriveLoginFromEmail(email string) string {
	// Extract part before @
	parts := strings.Split(email, "@")
	base := strings.ToLower(parts[0])

	// Remove invalid characters (keep only alphanumeric, underscore, dash, dot)
	base = invalidCharsRegex.ReplaceAllString(base, "")

	// Ensure minimum length
	if len(base) < 2 {
		base = "user"
	}

	// Truncate to max length
	if len(base) > 32 {
		base = base[:32]
	}

	return base
}

// isValidInternalRedirect checks if a redirect URL is safe (internal-only).
// Returns true for relative paths like "/chat" or "/settings/profile".
// Rejects absolute URLs, protocol-relative URLs (//evil.com), and other attack vectors.
func isValidInternalRedirect(redirect string) bool {
	// Must start with a single forward slash (relative path)
	if !strings.HasPrefix(redirect, "/") {
		return false
	}
	// Reject protocol-relative URLs (//evil.com) which browsers treat as absolute
	if strings.HasPrefix(redirect, "//") {
		return false
	}
	// Reject backslash variants that some browsers normalize to forward slashes
	if strings.Contains(redirect, "\\") {
		return false
	}
	return true
}
