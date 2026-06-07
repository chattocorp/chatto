package http_server

import (
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

func (s *HTTPServer) setupOAuthRoutes() {
	oauth := s.router.Group("/oauth")
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

		// If user is already authenticated and has pending OAuth params from
		// a previous /oauth/authorize visit (e.g., after login on the login page),
		// generate the code immediately without re-validating query params.
		if userID, sessionID, cookieSession, ok := s.validateCookieSession(c); ok {
			s.rotateCookieSessionIfNeeded(c, userID, sessionID, cookieSession)
			if hasPendingOAuthAuthorize(session) {
				s.completeOAuthAuthorize(c, userID, cookieSession.GetAuthGeneration())
				return
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

		if !isValidRedirectURI(redirectURI) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "Invalid redirect_uri: must be HTTPS or localhost",
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
		if userID, sessionID, cookieSession, ok := s.validateCookieSession(c); ok {
			s.rotateCookieSessionIfNeeded(c, userID, sessionID, cookieSession)
			s.completeOAuthAuthorize(c, userID, cookieSession.GetAuthGeneration())
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
}

type oauthTokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectURI  string `json:"redirect_uri"`
}

// completeOAuthAuthorize generates an authorization code and redirects to the
// client's redirect_uri. Called after the user has authenticated, either
// directly (already had a session) or after login/OAuth callback.
func (s *HTTPServer) completeOAuthAuthorize(c *gin.Context, userID string, authGeneration uint64) {
	session := sessions.Default(c)

	redirectURI, _ := session.Get(sessionKeyOAuthRedirectURI).(string)
	codeChallenge, _ := session.Get(sessionKeyOAuthCodeChallenge).(string)
	codeChallengeMethod, _ := session.Get(sessionKeyOAuthCodeMethod).(string)
	state, _ := session.Get(sessionKeyOAuthState).(string)

	// Clear the OAuth session data
	session.Delete(sessionKeyOAuthRedirectURI)
	session.Delete(sessionKeyOAuthCodeChallenge)
	session.Delete(sessionKeyOAuthCodeMethod)
	session.Delete(sessionKeyOAuthState)
	session.Save()

	if redirectURI == "" || codeChallenge == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "No pending authorization request",
		})
		return
	}

	ctx := c.Request.Context()
	code, err := s.core.CreateAuthCodeForGeneration(ctx, userID, redirectURI, codeChallenge, codeChallengeMethod, authGeneration)
	if err != nil {
		log.Error("Failed to create authorization code", "error", err, "userId", userID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": "Failed to generate authorization code",
		})
		return
	}

	// Build redirect URL with code and state
	u, err := url.Parse(redirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
		return
	}

	q := u.Query()
	q.Set("code", code)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()

	c.Redirect(http.StatusTemporaryRedirect, u.String())
}

// hasPendingOAuthAuthorize checks if the session has a pending OAuth authorize flow.
func hasPendingOAuthAuthorize(session sessions.Session) bool {
	redirectURI, _ := session.Get(sessionKeyOAuthRedirectURI).(string)
	return redirectURI != ""
}

// isValidRedirectURI validates a redirect URI for the OAuth authorize flow.
// Accepts:
//   - HTTPS URLs (any origin)
//   - http://localhost:* and http://127.0.0.1:* (for desktop apps and development)
func isValidRedirectURI(uri string) bool {
	u, err := url.Parse(uri)
	if err != nil {
		return false
	}

	// Must have a scheme and host
	if u.Scheme == "" || u.Host == "" {
		return false
	}

	// HTTPS is always allowed
	if u.Scheme == "https" {
		return true
	}

	// HTTP is only allowed for localhost (desktop apps, development)
	if u.Scheme == "http" {
		host := strings.Split(u.Host, ":")[0] // strip port
		return host == "localhost" || host == "127.0.0.1"
	}

	return false
}

// setOAuthTokenCORS sets CORS headers for the token endpoint.
// Wildcard origin — this endpoint is called cross-origin by any Chatto client.
func setOAuthTokenCORS(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "POST, OPTIONS")
	c.Header("Access-Control-Allow-Headers", "Content-Type")
}
