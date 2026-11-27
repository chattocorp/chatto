package http_server

import (
	"errors"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/connectapi"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/email"
	"github.com/chattocorp/chatto/cli/internal/i18n"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// Pre-compiled regexes for login validation
var (
	invalidCharsRegex = regexp.MustCompile(`[^a-z0-9._-]`)
)

func isStaleLoginCredentialError(err error) bool {
	return errors.Is(err, core.ErrCookieSessionNotFound) || errors.Is(err, core.ErrAuthTokenNotFound)
}

func bearerSessionLifetimeSeconds(expiresAt time.Time) int64 {
	remaining := time.Until(expiresAt)
	if remaining <= 0 {
		return 0
	}
	return int64((remaining + time.Second - 1) / time.Second)
}

func addBearerSessionResponse(response gin.H, credentials core.BearerSessionCredentials) {
	response["token"] = credentials.AccessToken
	response["refreshToken"] = credentials.RefreshToken
	response["expiresIn"] = bearerSessionLifetimeSeconds(credentials.AccessTokenExpiresAt)
	response["refreshTokenExpiresIn"] = bearerSessionLifetimeSeconds(credentials.SessionExpiresAt)
}

func requestsCookieOnlyAuthentication(c *gin.Context) bool {
	return strings.EqualFold(
		strings.TrimSpace(c.GetHeader(connectapi.BrowserAuthenticationModeHeader)),
		connectapi.BrowserAuthenticationModeCookie,
	)
}

func requireJSONAuthenticationRequest(c *gin.Context) {
	if !isJSONAuthenticationRequest(c) {
		c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"error": "Content-Type must be application/json"})
		return
	}
	c.Next()
}

func isJSONAuthenticationRequest(c *gin.Context) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

// requireBrowserAuthenticationRequest blocks login CSRF before a browser route
// can replace ambient authentication. Plain HTML forms are rejected and the
// Origin must match the request target or configured public origin. The mode
// header is optional for compatibility with legacy browser requests.
func (s *HTTPServer) requireBrowserAuthenticationRequest(c *gin.Context) {
	if !isJSONAuthenticationRequest(c) {
		c.AbortWithStatusJSON(http.StatusUnsupportedMediaType, gin.H{"error": "Content-Type must be application/json"})
		return
	}
	mode := strings.TrimSpace(c.GetHeader(connectapi.BrowserAuthenticationModeHeader))
	if (mode != "" && !requestsCookieOnlyAuthentication(c)) || !s.requestIsSameOrigin(c.Request) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Same-origin browser authentication required"})
		return
	}
	// Browser-only routes always create cookie authority. Treat an absent header
	// as that mode after the same-origin validation above has made the request
	// safe.
	if mode == "" {
		c.Request.Header.Set(connectapi.BrowserAuthenticationModeHeader, connectapi.BrowserAuthenticationModeCookie)
	}
}

func (s *HTTPServer) authServerName() string { return serverDisplayName(s.core) }

// serverDisplayName returns the effective server name, or "Chatto" when the
// server has none.
func serverDisplayName(c *core.ChattoCore) string {
	if c != nil && c.ConfigModel() != nil {
		if name := c.ConfigModel().GetEffectiveServerName(); strings.TrimSpace(name) != "" {
			return name
		}
	}
	return "Chatto"
}

// emailLocale selects the language of a transactional email from the
// request's Accept-Language header. The bundled frontend sends the language
// that the user selected in the app.
func emailLocale(r *http.Request) string {
	return i18n.Default().Match(r.Header.Get("Accept-Language"))
}

// emailBranding returns the branding that the email queue uses when it renders
// an email. It reads the server name at that time, so a renamed server sends
// queued email with its new name.
func emailBranding(c *core.ChattoCore, serverURL string) func() email.Branding {
	return func() email.Branding {
		return email.Branding{ServerName: serverDisplayName(c), ServerURL: serverURL}
	}
}

func (s *HTTPServer) setupAuthRoutes() {
	auth := s.router.Group("/auth")
	auth.Use(limitLegacyRequestBody())
	s.setupRegistrationAdmissionRoutes(auth)
	auth.Use(func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)
		c.Next()
	})
	auth.GET("browser/csrf", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		_, ok, err := s.cookiePresentedCredential(c)
		if err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}
		if err := s.ensureCSRFToken(c); err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	logout := func(browserOnly bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			ctx := c.Request.Context()
			c.Header("Cache-Control", "no-store")
			c.Header("Pragma", "no-cache")

			loggedOutUserIDs := make(map[string]struct{}, 2)
			revocationFailed := false
			var cookieCredential presentedRuntimeCredential
			if browserOnly {
				var cookieValidationErr error
				cookieCredential, _, cookieValidationErr = s.cookiePresentedCredential(c)
				revocationFailed = cookieValidationErr != nil
				if cookieValidationErr != nil {
					log.Warn("Failed to inspect cookie credential during logout", "error", cookieValidationErr)
				}
			}

			if authHeader := c.GetHeader("Authorization"); authHeader != "" {
				if token, ok := strings.CutPrefix(authHeader, "Bearer "); ok && strings.TrimSpace(token) != "" {
					userID, revoked, err := s.core.RevokePresentedRuntimeCredentialWithReason(ctx, strings.TrimSpace(token), core.AuthTokenPresentationBearer, "logout")
					if err != nil {
						log.Warn("Failed to revoke bearer runtime credential on logout", "error", err)
						revocationFailed = true
					}
					if revoked && userID != "" {
						loggedOutUserIDs[userID] = struct{}{}
					}
				}
			}

			var logoutRequest struct {
				RefreshToken string `json:"refreshToken"`
			}
			if c.Request.Body != nil && c.Request.ContentLength != 0 {
				if err := c.ShouldBindJSON(&logoutRequest); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
					return
				}
			}
			if strings.TrimSpace(logoutRequest.RefreshToken) != "" {
				userID, revoked, err := s.core.RevokeRefreshTokenWithReasonResult(ctx, strings.TrimSpace(logoutRequest.RefreshToken), "logout")
				if err != nil {
					log.Warn("Failed to revoke renewable session on logout", "error", err)
					revocationFailed = true
				}
				if revoked && userID != "" {
					loggedOutUserIDs[userID] = struct{}{}
				}
			}

			if browserOnly {
				for _, session := range cookieCredential.presentedSessions {
					if err := s.core.RevokeCookieSession(ctx, session.handle); err != nil {
						log.Warn("Failed to revoke cookie runtime credential on logout", "error", err)
						revocationFailed = true
					}
					if session.userID != "" {
						loggedOutUserIDs[session.userID] = struct{}{}
					}
				}
			}

			if revocationFailed {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"success": false,
					"error":   "Logout could not be completed",
				})
				return
			}

			if browserOnly {
				// Clear browser state only after authoritative credentials are
				// revoked. The programmatic route never changes ambient cookies.
				session := sessions.Default(c)
				session.Clear()
				if err := session.Save(); err != nil {
					log.Warn("Failed to clear browser session after logout", "error", err)
					c.JSON(http.StatusServiceUnavailable, gin.H{
						"success": false,
						"error":   "Logout could not be completed",
					})
					return
				}
				s.clearBrowserSessionCookie(c)
				clearCSRFCookie(c)
			}

			for userID := range loggedOutUserIDs {
				if err := s.core.PublishSessionTerminated(ctx, userID, "logout"); err != nil {
					log.Warn("Failed to publish session terminated event", "error", err)
				}
				if err := s.core.RecordLogoutSucceeded(ctx, userID); err != nil {
					log.Warn("Failed to append logout audit event", "error", err)
				}
			}

			c.JSON(http.StatusOK, gin.H{"success": true})
		}
	}
	auth.POST("logout", logout(false))
	auth.POST("browser/logout", s.requireBrowserAuthenticationRequest, logout(true))

	// Browser renewal is an explicit, CSRF-protected operation. Ordinary API,
	// frontend, and WebSocket requests only validate the stable session handle,
	// so a slow response cannot unexpectedly replace authentication state.
	auth.POST("browser/session/renew", s.requireBrowserAuthenticationRequest, func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")

		credential, ok, err := s.cookiePresentedCredential(c)
		if err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		now := time.Now()
		if s.cookieSessionRenewalNow != nil {
			now = s.cookieSessionRenewalNow()
		}
		record, renewed, err := s.core.RenewCookieSession(c.Request.Context(), credential.auth.Handle, now)
		if err != nil {
			if errors.Is(err, core.ErrCookieSessionNotFound) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
				return
			}
			writeAuthenticationUnavailable(c)
			return
		}
		if record.GetExpiresAt() == nil {
			writeAuthenticationUnavailable(c)
			return
		}
		for _, session := range credential.presentedSessions {
			if session.handle == credential.auth.Handle {
				continue
			}
			if err := s.core.RevokeCookieSession(c.Request.Context(), session.handle); err != nil {
				writeAuthenticationUnavailable(c)
				return
			}
		}

		cookieName, err := newBrowserSessionCookieName()
		if err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		// A fresh cookie name makes response application commutative. A delayed
		// renewal can re-add only its old (now revoked) handle; it cannot replace
		// a newer login's cookie. Reissuing on no-op retries also repairs a lost
		// successful renewal response.
		markAuthenticationCookieResponsePrivate(c)
		s.clearBrowserSessionCookie(c)
		s.writeBrowserSessionCookie(c, cookieName, credential.auth.Handle, record.GetExpiresAt().AsTime())
		c.JSON(http.StatusOK, gin.H{
			"success":    true,
			"renewed":    renewed,
			"expiresAt":  record.GetExpiresAt().AsTime().UTC().Format(time.RFC3339),
			"renewAfter": record.GetExpiresAt().AsTime().Add(-s.config.Auth.TokenTTLOrDefault() / 4).UTC().Format(time.RFC3339),
		})
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
		if err := s.core.RevokeAuthTokenWithReason(ctx, req.Token, "explicit"); err != nil {
			log.Error("Failed to revoke token", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to revoke token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Revoke portable bearer authority while preserving the browser cookie.
	// The bundled frontend uses this once when it migrates persisted origin
	// credentials to the dedicated HttpOnly session.
	auth.POST("browser/revoke-bearer-session", s.requireBrowserAuthenticationRequest, func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		var req struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
		revocationFailed := false
		if strings.TrimSpace(req.RefreshToken) != "" {
			if err := s.core.RevokeRefreshTokenWithReason(c.Request.Context(), strings.TrimSpace(req.RefreshToken), "browser_cookie_migration"); err != nil {
				revocationFailed = true
			}
		}
		if strings.TrimSpace(req.AccessToken) != "" {
			if err := s.core.RevokeAuthTokenWithReason(c.Request.Context(), strings.TrimSpace(req.AccessToken), "browser_cookie_migration"); err != nil {
				revocationFailed = true
			}
		}
		if revocationFailed {
			writeAuthenticationUnavailable(c)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Password login endpoint
	// Accepts login name (username) via "login" or "identifier" field
	login := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		if !s.config.Auth.DirectLoginOrDefault() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Password login is disabled"})
			return
		}

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

		// Existing usernames remain usable when the assignment limit is lowered.
		if len(login) > core.MaxLoginLength {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid credentials"})
			return
		}

		// Verify credentials by login name
		ctx := c.Request.Context()
		user, authGeneration, err := s.core.VerifyPasswordWithAuthGeneration(ctx, login, loginRequest.Password)
		if err != nil {
			if auditErr := s.core.RecordLoginFailed(ctx, login); auditErr != nil {
				log.Warn("Failed to append failed-login audit event", "error", auditErr)
			}
			log.Error("Login failed", "error", err)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		cookieOnly := requestsCookieOnlyAuthentication(c)
		var bearerCredentials core.BearerSessionCredentials
		var cookieCredentialID string
		if cookieOnly {
			if err := s.createCookieSessionForGeneration(c, user.Id, "password_login", authGeneration); err != nil {
				if isStaleLoginCredentialError(err) {
					if auditErr := s.core.RecordLoginFailed(ctx, login); auditErr != nil {
						log.Warn("Failed to append stale-login audit event", "error", auditErr)
					}
					log.Warn("Login became stale before browser session creation", "userId", user.Id)
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
					return
				}
				log.Error("Failed to save browser session", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
				return
			}
			cookieCredentialID, _ = s.browserSessionID(c)
			if s.passwordLoginSessionCreatedHook != nil {
				s.passwordLoginSessionCreatedHook(c, user.Id, authGeneration)
			}
			// Bearer issuance normally provides the second generation check after
			// password verification. Cookie-only login performs that check directly.
			if _, err := s.core.ValidateCookieCredential(ctx, cookieCredentialID); err != nil {
				_ = s.core.RevokeCookieSession(ctx, cookieCredentialID)
				s.clearBrowserSessionCookie(c)
				if auditErr := s.core.RecordLoginFailed(ctx, login); auditErr != nil {
					log.Warn("Failed to append stale-login audit event", "error", auditErr)
				}
				log.Warn("Cookie-only login became stale before completion")
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
				return
			}
			if err := s.ensureCSRFToken(c); err != nil {
				log.Error("Failed to create CSRF token", "error", err)
				_ = s.core.RevokeCookieSession(ctx, cookieCredentialID)
				s.clearBrowserSessionCookie(c)
				clearCSRFCookie(c)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
				return
			}
		} else {
			// Programmatic direct-auth clients receive only bearer authority. This
			// route never changes ambient browser authentication.
			if s.passwordLoginSessionCreatedHook != nil {
				s.passwordLoginSessionCreatedHook(c, user.Id, authGeneration)
			}
			credentials, err := s.core.CreateBearerSessionWithSourceGeneration(ctx, user.Id, "password_login", authGeneration)
			if err != nil {
				if isStaleLoginCredentialError(err) {
					if auditErr := s.core.RecordLoginFailed(ctx, login); auditErr != nil {
						log.Warn("Failed to append stale-login audit event", "error", auditErr)
					}
					log.Warn("Login became stale before bearer token creation")
					c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
					return
				}
				log.Error("Failed to create auth token on login", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
				return
			}
			bearerCredentials = credentials
		}

		if err := s.core.RecordLoginSucceeded(ctx, user.Id, login); err != nil {
			log.Error("Failed to append login audit event", "userId", user.Id, "error", err)
			if cookieCredentialID != "" {
				_ = s.core.RevokeCookieSession(ctx, cookieCredentialID)
				s.clearBrowserSessionCookie(c)
				clearCSRFCookie(c)
			}
			if bearerCredentials.RefreshToken != "" {
				_ = s.core.RevokeRefreshTokenWithReason(ctx, bearerCredentials.RefreshToken, "login_audit_failed")
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}

		log.Info("User logged in successfully", "userId", user.Id)

		response := gin.H{
			"success": true,
			"user":    gin.H{"id": user.Id, "login": user.Login},
		}

		if bearerCredentials.AccessToken != "" {
			addBearerSessionResponse(response, bearerCredentials)
		}

		c.JSON(http.StatusOK, response)
	}
	auth.POST("login", requireJSONAuthenticationRequest, login)
	auth.POST("browser/login", s.requireBrowserAuthenticationRequest, login)

	// Email-first registration endpoint (step 1)
	// Accepts email only, creates a registration code, and sends it by email.
	// The user exchanges the code via POST /auth/register/verify-code, then
	// completes account creation via POST /auth/register/complete.
	auth.POST("register", s.requireServerSetupComplete, func(c *gin.Context) {
		// Check if registration is enabled
		if !s.config.Auth.DirectRegistrationEnabled() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Registration is disabled"})
			return
		}

		var req struct {
			Email          string `json:"email" binding:"required,email"`
			AdmissionID    string `json:"admissionId"`
			InvitationCode string `json:"invitationCode"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address is required"})
			return
		}
		// Normalize at the HTTP boundary so downstream core code can treat email as canonical.
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))

		ctx := c.Request.Context()
		invitationID := ""
		if err := s.core.WaitForRegistrationPolicy(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Registration is temporarily unavailable"})
			return
		}
		if s.core.InvitationRequired() {
			var err error
			if req.InvitationCode != "" {
				invitationID, err = s.core.ValidateInvitationCode(ctx, req.InvitationCode)
			} else {
				invitationID, err = s.registrationInvitation(c, req.AdmissionID)
			}
			if err != nil {
				if errors.Is(err, core.ErrInvitationInvalid) {
					c.JSON(http.StatusBadRequest, gin.H{"error": "This invitation code is invalid or no longer available"})
				} else {
					c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Registration is temporarily unavailable"})
				}
				return
			}
		}

		// Require mailer — can't do email-first registration without email delivery.
		// In invite-only mode this check deliberately follows admission validation,
		// so an uninvited request cannot probe later registration preconditions.
		if s.mailer == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Email delivery is not configured"})
			return
		}

		// Check if email is already claimed — but always return 200 to prevent enumeration
		emailClaimed, err := s.core.IsEmailClaimed(ctx, req.Email)
		if err != nil {
			log.Error("Failed to check email availability", "error", err)
		}
		if emailClaimed {
			// Don't reveal that the email is taken — just return success
			log.Info("Registration attempt for already-claimed email")
			c.JSON(http.StatusOK, gin.H{
				"message": "If this email is available, you will receive a registration code.",
			})
			return
		}

		// Create registration code
		expiresAt := time.Now().Add(s.config.Auth.EmailOTP.TTLOrDefault())
		code, err := s.core.CreateRegistrationCodeForInvitation(ctx, req.Email, invitationID)
		if err != nil {
			if errors.Is(err, core.ErrRegistrationCodeLimitExceeded) ||
				errors.Is(err, core.ErrRegistrationCodeExhausted) {
				log.Info("Registration code request throttled")
				c.JSON(http.StatusOK, gin.H{
					"message": "If this email is available, you will receive a registration code.",
				})
				return
			}
			log.Error("Failed to create registration code", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}

		// Send registration email
		err = s.mailer.Enqueue(c.Request.Context(), email.Delivery{
			To:        req.Email,
			Content:   email.RegistrationCode{Code: code, TTL: s.config.Auth.EmailOTP.TTLOrDefault()},
			Locale:    emailLocale(c.Request),
			ExpiresAt: expiresAt,
		})
		if err != nil {
			log.Error("Failed to send registration email", "error", err)
			if cancelErr := s.core.CancelRegistrationCode(ctx, req.Email, code); cancelErr != nil {
				log.Warn("Failed to cancel undelivered registration code", "error", cancelErr)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
			return
		}

		log.Info("Queued registration email")
		c.JSON(http.StatusOK, gin.H{
			"message": "If this email is available, you will receive a registration code.",
		})
	})

	// Registration code verification endpoint (step 2)
	// Validates the emailed six-digit code and returns a short-lived completion token.
	auth.POST("register/verify-code", s.requireServerSetupComplete, func(c *gin.Context) {
		if !s.config.Auth.DirectRegistrationEnabled() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Registration is disabled"})
			return
		}

		var req struct {
			Email string `json:"email" binding:"required,email"`
			Code  string `json:"code" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address and verification code are required"})
			return
		}
		req.Email = strings.ToLower(strings.TrimSpace(req.Email))

		token, err := s.core.VerifyRegistrationCode(c.Request.Context(), req.Email, req.Code)
		if err != nil {
			if errors.Is(err, core.ErrRegistrationCodeNotFound) ||
				errors.Is(err, core.ErrRegistrationCodeExpired) ||
				errors.Is(err, core.ErrRegistrationCodeInvalid) ||
				errors.Is(err, core.ErrRegistrationCodeExhausted) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired registration code"})
				return
			}
			log.Error("Failed to verify registration code", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"completionToken": token})
	})

	// Registration completion endpoint (step 2)
	// Validates the registration completion token, creates the user account,
	// verifies the email, and creates a session.
	completeRegistration := func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		// Check if registration is enabled
		if !s.config.Auth.DirectRegistrationEnabled() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Registration is disabled"})
			return
		}

		var req struct {
			Token                string `json:"token" binding:"required"`
			Login                string `json:"login" binding:"required"`
			Password             string `json:"password" binding:"required"`
			PasswordConfirmation string `json:"passwordConfirmation" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Token, login, password, and password confirmation are required"})
			return
		}

		ctx := c.Request.Context()

		if err := core.ValidatePassword(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate token (not consumed on validation failure — user can retry)
		tokenData, err := s.core.GetRegistrationToken(ctx, req.Token)
		if err != nil {
			if errors.Is(err, core.ErrRegistrationTokenNotFound) || errors.Is(err, core.ErrRegistrationTokenExpired) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired registration code"})
				return
			}
			log.Error("Failed to validate registration completion token", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}

		// Validate login format
		if err := validateRegistrationLogin(req.Login, s.core.LoginLengthLimit()); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Validate passwords match
		if req.Password != req.PasswordConfirmation {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Passwords do not match"})
			return
		}

		// Check if login is blocked
		if s.core.ConfigModel().IsUsernameBlocked(req.Login) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "This username is not available"})
			return
		}

		// Check if email was claimed while token was outstanding
		emailClaimed, err := s.core.IsEmailClaimed(ctx, tokenData.Email)
		if err != nil {
			log.Error("Failed to check email availability", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}
		if emailClaimed {
			c.JSON(http.StatusConflict, gin.H{"error": "This email address is already in use"})
			return
		}

		// Create user with verified email atomically (use login as display name initially)
		user, err := s.core.CreateSelfServiceUser(ctx, "system", req.Login, core.LoginDisplayName(req.Login), req.Password, tokenData.Email, tokenData.InvitationID)
		if err != nil {
			if errors.Is(err, core.ErrInvitationInvalid) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "This invitation code is invalid or no longer available"})
				return
			}
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
			if errors.Is(err, core.ErrPasswordTooShort) || errors.Is(err, core.ErrPasswordTooLong) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			log.Error("Registration failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Registration failed"})
			return
		}

		// Server membership is implicit; global rooms appear automatically.
		clearRegistrationAdmission(c, tokenData.InvitationID)

		// Delete registration completion token (consumed)
		if err := s.core.DeleteRegistrationToken(ctx, req.Token); err != nil {
			log.Error("Failed to delete registration completion token", "error", err)
			// Don't fail — user was created successfully
		}

		response := gin.H{
			"success": true,
			"user":    gin.H{"id": user.Id, "login": user.Login},
		}
		if requestsCookieOnlyAuthentication(c) {
			if err := s.createCookieSession(c, user.Id, "registration_complete"); err != nil {
				log.Error("Failed to save browser session", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
				return
			}
			cookieCredentialID, _ := s.browserSessionID(c)
			if err := s.ensureCSRFToken(c); err != nil {
				log.Error("Failed to create CSRF token", "error", err)
				_ = s.core.RevokeCookieSession(ctx, cookieCredentialID)
				s.clearBrowserSessionCookie(c)
				clearCSRFCookie(c)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
				return
			}
		} else {
			credentials, err := s.core.CreateBearerSessionWithSource(ctx, user.Id, "registration")
			if err != nil {
				log.Error("Failed to create auth token on register", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
				return
			}
			addBearerSessionResponse(response, credentials)
		}

		log.Info("User registered and logged in", "userId", user.Id)

		c.JSON(http.StatusOK, response)
	}
	auth.POST("register/complete", s.requireServerSetupComplete, requireJSONAuthenticationRequest, completeRegistration)
	auth.POST("browser/register/complete", s.requireServerSetupComplete, s.requireBrowserAuthenticationRequest, completeRegistration)

	// Authenticated email verification code request.
	auth.POST("verify-email/request-code", func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		if authenticationValidationError(req.Context()) != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service temporarily unavailable"})
			return
		}
		user := authctx.ForContext(req.Context())
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}
		if s.mailer == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Email delivery is not configured"})
			return
		}

		var body struct {
			Email string `json:"email" binding:"required,email"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address is required"})
			return
		}
		body.Email = strings.ToLower(strings.TrimSpace(body.Email))

		expiresAt := time.Now().Add(s.config.Auth.EmailOTP.TTLOrDefault())
		code, err := s.core.CreateEmailVerificationCode(req.Context(), user.Id, body.Email)
		if err != nil {
			if errors.Is(err, core.ErrEmailVerificationCodeLimitExceeded) ||
				errors.Is(err, core.ErrEmailVerificationCodeExhausted) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many verification code requests. Please try again later."})
				return
			}
			log.Error("Failed to create email verification code", "userId", user.Id, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification code"})
			return
		}
		if err := s.mailer.Enqueue(c.Request.Context(), email.Delivery{
			To:        body.Email,
			Content:   email.EmailVerificationCode{Code: code, TTL: s.config.Auth.EmailOTP.TTLOrDefault()},
			Locale:    emailLocale(c.Request),
			ExpiresAt: expiresAt,
		}); err != nil {
			log.Error("Failed to send email verification code", "userId", user.Id, "error", err)
			if cancelErr := s.core.CancelEmailVerificationCode(req.Context(), user.Id, body.Email, code); cancelErr != nil {
				log.Warn("Failed to cancel undelivered email verification code", "userId", user.Id, "error", cancelErr)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send verification code"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Verification code sent."})
	})

	// Authenticated email verification code confirmation.
	auth.POST("verify-email/confirm-code", func(c *gin.Context) {
		req := s.injectUserIntoContext(c)
		if authenticationValidationError(req.Context()) != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Authentication service temporarily unavailable"})
			return
		}
		user := authctx.ForContext(req.Context())
		if user == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		var body struct {
			Email string `json:"email" binding:"required,email"`
			Code  string `json:"code" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "A valid email address and verification code are required"})
			return
		}
		body.Email = strings.ToLower(strings.TrimSpace(body.Email))

		if _, err := s.core.VerifyEmailCode(req.Context(), user.Id, body.Email, body.Code); err != nil {
			if errors.Is(err, core.ErrTokenNotFound) ||
				errors.Is(err, core.ErrTokenExpired) ||
				errors.Is(err, core.ErrEmailVerificationCodeInvalid) ||
				errors.Is(err, core.ErrEmailVerificationCodeExhausted) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired verification code"})
				return
			}
			if errors.Is(err, core.ErrEmailAlreadyVerified) {
				c.JSON(http.StatusConflict, gin.H{"error": "This email address is already in use"})
				return
			}
			log.Error("Email verification failed", "userId", user.Id, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Email verification failed"})
			return
		}

		log.Info("Email verified successfully", "userId", user.Id)
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	// Forgot password endpoint - request a password reset email
	// When enabled, always returns 200 to prevent email enumeration.
	auth.POST("forgot-password", func(c *gin.Context) {
		if !s.config.Auth.DirectLoginOrDefault() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Password changes are disabled because this server uses SSO"})
			return
		}
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
		expiresAt := time.Now().Add(core.PasswordResetTokenTTL)
		token, err := s.core.CreatePasswordResetToken(ctx, normalizedEmail)
		if errors.Is(err, core.ErrPasswordResetRequestThrottled) {
			log.Info("Password reset request throttled")
		} else if err != nil {
			// Log error but don't expose to user
			log.Error("Failed to create password reset token", "error", err)
		}

		// Only send email if token was created (email exists and is verified)
		if token != "" && s.mailer != nil {
			err = s.mailer.Enqueue(c.Request.Context(), email.Delivery{
				To:        normalizedEmail,
				Content:   email.PasswordReset{Token: token, TTL: core.PasswordResetTokenTTL},
				Locale:    emailLocale(c.Request),
				ExpiresAt: expiresAt,
			})
			if err != nil {
				log.Error("Failed to send password reset email", "error", err)
				if cancelErr := s.core.CancelPasswordResetToken(ctx, token); cancelErr != nil {
					log.Error("Failed to cancel undelivered password reset token", "error", cancelErr)
				}
			} else {
				log.Info("Queued password reset email")
			}
		} else if token != "" {
			if cancelErr := s.core.CancelPasswordResetToken(ctx, token); cancelErr != nil {
				log.Error("Failed to cancel unsendable password reset token", "error", cancelErr)
			}
		}

		// Always return success to prevent email enumeration
		c.JSON(http.StatusOK, gin.H{
			"message": "If that email is registered, you will receive a password reset link.",
		})
	})

	// Reset password endpoint - set a new password using a reset token
	auth.POST("reset-password", func(c *gin.Context) {
		if !s.config.Auth.DirectLoginOrDefault() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Password changes are disabled because this server uses SSO"})
			return
		}
		var req struct {
			Token    string `json:"token" binding:"required"`
			Password string `json:"password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Token and password are required"})
			return
		}

		// Use the same UTF-8 byte limits as all other password-setting paths.
		if err := core.ValidatePassword(req.Password); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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

// Registration also excludes consecutive periods.
func validateRegistrationLogin(login string, maxLength int) error {
	if strings.Contains(login, "..") {
		return errors.New("username cannot contain consecutive periods")
	}
	return core.ValidateLogin(login, maxLength)
}

// deriveLoginFromEmail extracts a login name from an email address.
// Takes the part before @, converts to lowercase, and removes invalid characters.
// Valid characters: alphanumeric, underscore, dash, dot.
func deriveLoginFromEmail(email string, maxLength int) string {
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
	if len(base) > maxLength {
		base = base[:maxLength]
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
	for _, char := range redirect {
		if char <= 0x1f || char == 0x7f {
			return false
		}
	}
	return true
}

// requireServerSetupComplete rejects registration before sending mail, consuming
// a registration token, or creating a user. Core repeats this gate inside OCC.
func (s *HTTPServer) requireServerSetupComplete(c *gin.Context) {
	required, err := s.core.SetupRequired(c.Request.Context())
	if err != nil {
		log.Error("Failed to read server setup state", "error", err)
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Server setup state is unavailable"})
		return
	}
	if required {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Complete first-run setup before creating accounts"})
	}
}
