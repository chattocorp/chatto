package http_server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
)

// Session keys for the OAuth authorize flow.
const (
	sessionKeyOAuthRedirectURI   = "oauth_redirect_uri"
	sessionKeyOAuthCodeChallenge = "oauth_code_challenge"
	sessionKeyOAuthCodeMethod    = "oauth_code_method"
	sessionKeyOAuthState         = "oauth_state"
)

var errNoPendingOAuthAuthorize = errors.New("no pending OAuth authorization request")

func (s *HTTPServer) setupOAuthRoutes() {
	oauth := s.router.Group("/oauth")
	oauth.Use(limitLegacyRequestBody())
	oauth.Use(func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)
		c.Next()
	})

	// GET /oauth/authorize — OAuth 2.0 Authorization endpoint.
	// Validates parameters, stores them in the session, then redirects to the
	// login page. After the user authenticates (via any method), the login flow
	// detects the stored authorize params and issues an authorization code
	// instead of the normal post-login redirect.
	oauth.GET("authorize", func(c *gin.Context) {
		session := sessions.Default(c)

		// If user is already authenticated and returns to /oauth/authorize
		// without fresh query params (e.g., after a login flow restored only the
		// pending session), continue the stored request. Any request carrying a
		// query string is treated as a fresh authorize attempt and overwrites the
		// pending session after validation below.
		if c.Request.URL.RawQuery == "" {
			credential, ok, err := s.oauthCookieCredential(c)
			if err != nil {
				writeAuthenticationUnavailable(c)
				return
			}
			if ok {
				if hasPendingOAuthAuthorize(session) {
					s.continueOAuthAuthorize(c, credential.auth.UserID, credential.cookieRecord.GetAuthGeneration())
					return
				}
			}
		}

		// Validate query parameters for a fresh authorization request
		responseType := c.Query("response_type")
		redirectURI := c.Query("redirect_uri")
		codeChallenge := c.Query("code_challenge")
		codeChallengeMethod := c.Query("code_challenge_method")
		state := c.Query("state")

		if responseType != "code" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "unsupported_response_type",
				"error_description": "Only response_type=code is supported",
			})
			return
		}

		if redirectURI == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "redirect_uri is required",
			})
			return
		}

		if codeChallenge == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "code_challenge is required (PKCE)",
			})
			return
		}

		if codeChallengeMethod != "S256" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "code_challenge_method must be S256",
			})
			return
		}

		if !s.isAllowedOAuthRedirectURI(redirectURI) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "Invalid redirect_uri: must use an allowed HTTPS origin or localhost",
			})
			return
		}

		// Store authorize params in session so they survive the login flow
		session.Set(sessionKeyOAuthRedirectURI, redirectURI)
		session.Set(sessionKeyOAuthCodeChallenge, codeChallenge)
		session.Set(sessionKeyOAuthCodeMethod, codeChallengeMethod)
		session.Set(sessionKeyOAuthState, state)
		session.Save()

		// If user is already authenticated, generate code immediately
		credential, ok, err := s.oauthCookieCredential(c)
		if err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		if ok {
			s.continueOAuthAuthorize(c, credential.auth.UserID, credential.cookieRecord.GetAuthGeneration())
			return
		}

		// Redirect to the regular login page. After the user authenticates,
		// the redirect parameter sends them back to /oauth/authorize which
		// re-validates the query params (or falls back to session data).
		// Include the original query string so params survive even if the
		// session cookie is lost between requests (e.g., concurrent Set-Cookie
		// responses from invalidateAll() overwriting each other).
		redirectTarget := "/oauth/authorize"
		if c.Request.URL.RawQuery != "" {
			redirectTarget += "?" + c.Request.URL.RawQuery
		}
		c.Redirect(http.StatusTemporaryRedirect, "/login?redirect="+url.QueryEscape(redirectTarget))
	})

	// POST /oauth/token — OAuth 2.0 Token endpoint.
	// Exchanges an authorization code + PKCE verifier for a bearer token.
	// This endpoint has wildcard CORS since it's called cross-origin by clients.
	oauth.OPTIONS("token", func(c *gin.Context) {
		setOAuthTokenCORS(c)
		c.Status(http.StatusNoContent)
	})

	oauth.POST("token", func(c *gin.Context) {
		setOAuthTokenCORS(c)

		// Accept both JSON and form-encoded (per OAuth 2.0 spec, form-encoded is standard)
		var req oauthTokenRequest
		if c.ContentType() == "application/x-www-form-urlencoded" {
			req.GrantType = c.PostForm("grant_type")
			req.Code = c.PostForm("code")
			req.CodeVerifier = c.PostForm("code_verifier")
			req.RedirectURI = c.PostForm("redirect_uri")
		} else {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":             "invalid_request",
					"error_description": "Invalid request body",
				})
				return
			}
		}

		if req.GrantType != "authorization_code" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "unsupported_grant_type",
				"error_description": "Only grant_type=authorization_code is supported",
			})
			return
		}

		if req.Code == "" || req.CodeVerifier == "" || req.RedirectURI == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "code, code_verifier, and redirect_uri are required",
			})
			return
		}

		ctx := c.Request.Context()

		token, userID, err := s.core.ExchangeAuthCode(ctx, req.Code, req.CodeVerifier, req.RedirectURI)
		if err != nil {
			status := http.StatusBadRequest
			oauthErr := "invalid_grant"
			desc := err.Error()

			switch err {
			case core.ErrAuthCodeNotFound:
				desc = "Authorization code is invalid or has expired"
			case core.ErrAuthCodeInvalidVerifier:
				desc = "PKCE code_verifier does not match code_challenge"
			case core.ErrAuthCodeRedirectMismatch:
				desc = "redirect_uri does not match the authorization request"
			default:
				status = http.StatusInternalServerError
				oauthErr = "server_error"
				log.Error("OAuth token exchange failed", "error", err)
			}

			c.JSON(status, gin.H{
				"error":             oauthErr,
				"error_description": desc,
			})
			return
		}

		// Fetch user info to include in the response
		response := gin.H{
			"access_token": token,
			"token_type":   "Bearer",
		}

		if user, err := s.core.GetUser(ctx, userID); err == nil {
			response["user"] = gin.H{
				"id":          user.Id,
				"login":       user.Login,
				"displayName": user.DisplayName,
			}
		}

		c.JSON(http.StatusOK, response)
	})

	oauth.GET("consent/request", func(c *gin.Context) {
		_, ok, err := s.oauthCookieCredential(c)
		if err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		params, err := readPendingOAuthAuthorize(sessions.Default(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No pending authorization request"})
			return
		}
		redirectOrigin, ok := s.allowedOAuthRedirectOrigin(params.RedirectURI)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"redirectUri":    params.RedirectURI,
			"redirectOrigin": redirectOrigin,
		})
	})

	oauth.POST("consent/approve", func(c *gin.Context) {
		credential, ok, err := s.oauthCookieCredential(c)
		if err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		params, err := readPendingOAuthAuthorize(sessions.Default(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No pending authorization request"})
			return
		}
		redirectOrigin, ok := s.allowedOAuthRedirectOrigin(params.RedirectURI)
		if !ok {
			clearPendingOAuthAuthorize(sessions.Default(c))
			_ = sessions.Default(c).Save()
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri"})
			return
		}
		if err := s.core.GrantOAuthConsent(c.Request.Context(), credential.auth.UserID, redirectOrigin); err != nil {
			log.Error("Failed to record OAuth consent grant", "error", err, "userId", credential.auth.UserID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record consent"})
			return
		}

		redirectURL, ok := s.completeOAuthAuthorizeURL(c, credential.auth.UserID, credential.cookieRecord.GetAuthGeneration())
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"redirectUrl": redirectURL})
	})

	oauth.POST("consent/deny", func(c *gin.Context) {
		credential, ok, err := s.oauthCookieCredential(c)
		if err != nil {
			writeAuthenticationUnavailable(c)
			return
		}
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			return
		}

		session := sessions.Default(c)
		params, err := readPendingOAuthAuthorize(session)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No pending authorization request"})
			return
		}
		redirectOrigin, ok := s.allowedOAuthRedirectOrigin(params.RedirectURI)
		clearPendingOAuthAuthorize(session)
		if saveErr := session.Save(); saveErr != nil {
			log.Warn("Failed to clear denied OAuth authorize session", "error", saveErr)
		}
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri"})
			return
		}
		if err := s.core.RecordOAuthConsentDenied(c.Request.Context(), credential.auth.UserID, redirectOrigin); err != nil {
			log.Error("Failed to record OAuth consent denial", "error", err, "userId", credential.auth.UserID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record consent denial"})
			return
		}

		redirectURL, err := oauthErrorRedirectURL(params.RedirectURI, params.State, "access_denied", "The user denied the authorization request")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"redirectUrl": redirectURL})
	})
}

func (s *HTTPServer) oauthCookieCredential(c *gin.Context) (presentedRuntimeCredential, bool, error) {
	credential, ok, err := s.cookiePresentedCredential(c)
	if err != nil {
		return presentedRuntimeCredential{}, false, err
	}
	if !ok {
		return presentedRuntimeCredential{}, false, nil
	}
	s.rotateCookieSessionIfNeeded(c, credential.auth.UserID, credential.auth.Handle, credential.cookieRecord)
	return credential, true, nil
}

type oauthTokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectURI  string `json:"redirect_uri"`
}

type pendingOAuthAuthorize struct {
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	State               string
}

func readPendingOAuthAuthorize(session sessions.Session) (pendingOAuthAuthorize, error) {
	redirectURI, _ := session.Get(sessionKeyOAuthRedirectURI).(string)
	codeChallenge, _ := session.Get(sessionKeyOAuthCodeChallenge).(string)
	codeChallengeMethod, _ := session.Get(sessionKeyOAuthCodeMethod).(string)
	state, _ := session.Get(sessionKeyOAuthState).(string)
	if redirectURI == "" || codeChallenge == "" {
		return pendingOAuthAuthorize{}, errNoPendingOAuthAuthorize
	}
	return pendingOAuthAuthorize{
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		State:               state,
	}, nil
}

func clearPendingOAuthAuthorize(session sessions.Session) {
	session.Delete(sessionKeyOAuthRedirectURI)
	session.Delete(sessionKeyOAuthCodeChallenge)
	session.Delete(sessionKeyOAuthCodeMethod)
	session.Delete(sessionKeyOAuthState)
}

func (s *HTTPServer) continueOAuthAuthorize(c *gin.Context, userID string, authGeneration uint64) {
	params, err := readPendingOAuthAuthorize(sessions.Default(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "No pending authorization request",
		})
		return
	}
	redirectOrigin, ok := s.allowedOAuthRedirectOrigin(params.RedirectURI)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
		return
	}
	consented, err := s.core.HasOAuthConsent(c.Request.Context(), userID, redirectOrigin)
	if err != nil {
		log.Error("Failed to check OAuth consent", "error", err, "userId", userID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": "Failed to check OAuth consent",
		})
		return
	}
	if !consented {
		c.Redirect(http.StatusTemporaryRedirect, "/oauth/consent")
		return
	}
	s.completeOAuthAuthorize(c, userID, authGeneration)
}

// completeOAuthAuthorize generates an authorization code and redirects to the
// client's redirect_uri. Called after the user has authenticated, either
// directly (already had a session) or after login/OAuth callback.
func (s *HTTPServer) completeOAuthAuthorize(c *gin.Context, userID string, authGeneration uint64) {
	redirectURL, ok := s.completeOAuthAuthorizeURL(c, userID, authGeneration)
	if !ok {
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

func (s *HTTPServer) completeOAuthAuthorizeURL(c *gin.Context, userID string, authGeneration uint64) (string, bool) {
	session := sessions.Default(c)

	params, err := readPendingOAuthAuthorize(session)

	// Clear the OAuth session data
	clearPendingOAuthAuthorize(session)
	session.Save()

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "No pending authorization request",
		})
		return "", false
	}

	ctx := c.Request.Context()
	code, err := s.core.CreateAuthCodeForGeneration(ctx, userID, params.RedirectURI, params.CodeChallenge, params.CodeChallengeMethod, authGeneration)
	if err != nil {
		log.Error("Failed to create authorization code", "error", err, "userId", userID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": "Failed to generate authorization code",
		})
		return "", false
	}

	// Build redirect URL with code and state
	u, err := url.Parse(params.RedirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
		return "", false
	}

	q := u.Query()
	q.Set("code", code)
	if params.State != "" {
		q.Set("state", params.State)
	}
	u.RawQuery = q.Encode()

	return u.String(), true
}

// hasPendingOAuthAuthorize checks if the session has a pending OAuth authorize flow.
func hasPendingOAuthAuthorize(session sessions.Session) bool {
	redirectURI, _ := session.Get(sessionKeyOAuthRedirectURI).(string)
	return redirectURI != ""
}

func (s *HTTPServer) allowedOAuthRedirectOrigin(uri string) (string, bool) {
	if !s.isAllowedOAuthRedirectURI(uri) {
		return "", false
	}
	u, err := url.Parse(uri)
	if err != nil {
		return "", false
	}
	return canonicalOrigin(u), true
}

// isAllowedOAuthRedirectURI validates a redirect URI for the OAuth authorize
// flow. In addition to requiring HTTPS (except loopback development URLs), it
// only accepts origins this server explicitly trusts: its own public
// webserver.url origin, exact webserver.allowed_origins entries,
// webserver.oauth_redirect_origins entries, and localhost.
func (s *HTTPServer) isAllowedOAuthRedirectURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}

	// Must have a scheme and host
	if u.Scheme == "" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}

	if isLoopbackOAuthRedirectHost(u.Hostname()) {
		return u.Scheme == "http" || u.Scheme == "https"
	}

	if u.Scheme != "https" {
		return false
	}

	redirectOrigin := canonicalOrigin(u)
	for _, allowed := range s.allowedOAuthRedirectOrigins() {
		if allowed == "*" {
			return true
		}
		if redirectOrigin == allowed {
			return true
		}
	}

	return false
}

func (s *HTTPServer) allowedOAuthRedirectOrigins() []string {
	origins := make([]string, 0, len(s.config.Webserver.AllowedOrigins)+len(s.config.Webserver.OAuthRedirectOrigins)+1)
	if origin, ok := parseConfiguredOAuthOrigin(s.config.Webserver.URL); ok {
		origins = append(origins, origin)
	}
	for _, raw := range s.config.Webserver.AllowedOrigins {
		if strings.TrimSpace(raw) == "*" {
			continue
		}
		if origin, ok := parseConfiguredOAuthOrigin(raw); ok {
			origins = append(origins, origin)
		}
	}
	for _, raw := range s.config.Webserver.OAuthRedirectOrigins {
		if strings.TrimSpace(raw) == "*" {
			origins = append(origins, "*")
			continue
		}
		if origin, ok := parseConfiguredOAuthOrigin(raw); ok {
			origins = append(origins, origin)
		}
	}
	return origins
}

func parseConfiguredOAuthOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil {
		return "", false
	}
	if isLoopbackOAuthRedirectHost(u.Hostname()) {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", false
		}
	} else if u.Scheme != "https" {
		return "", false
	}
	return canonicalOrigin(u), true
}

func canonicalOrigin(u *url.URL) string {
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

func isLoopbackOAuthRedirectHost(host string) bool {
	switch strings.ToLower(host) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func oauthErrorRedirectURL(redirectURI, state, code, description string) (string, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("error", code)
	if description != "" {
		q.Set("error_description", description)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// setOAuthTokenCORS sets CORS headers for the token endpoint.
// Wildcard origin — this endpoint is called cross-origin by any Chatto client.
func setOAuthTokenCORS(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
}
