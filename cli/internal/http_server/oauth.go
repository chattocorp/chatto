package http_server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/idna"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// The signed browser session carries only an opaque handle. Validated request
// details live in shared RUNTIME_STATE so large CIMD URLs cannot overflow the
// browser cookie and every replica can continue the flow.
const sessionKeyOAuthPendingAuthorize = "oauth_pending_authorize"

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
		providerID := c.Query("provider_id")
		clientID := c.Query("client_id")

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
		if clientID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "client_id is required",
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

		client, err := s.resolveOAuthClient(c.Request.Context(), clientID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_client",
				"error_description": "The OAuth client metadata could not be verified",
			})
			return
		}
		if err := s.core.RequireOAuthClientAllowed(c.Request.Context(), client.ClientID); err != nil {
			if errors.Is(err, core.ErrOAuthClientBlocked) {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":             "invalid_client",
					"error_description": "The OAuth client is blocked by this server",
				})
				return
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":             "server_error",
				"error_description": "Failed to check OAuth client policy",
			})
			return
		}
		if !client.allowsRedirectURI(redirectURI) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "redirect_uri is not registered for this client",
			})
			return
		}

		// Store validated request details behind a small opaque session handle so
		// the flow survives login, restarts, and routing to another replica.
		if err := s.storePendingOAuthAuthorize(c.Request.Context(), session, pendingOAuthAuthorize{
			RedirectURI:         redirectURI,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			State:               state,
			ClientID:            client.ClientID,
			ClientName:          client.ClientName,
			ClientURI:           client.ClientURI,
		}); err != nil {
			log.Error("Failed to store pending OAuth authorization request", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":             "server_error",
				"error_description": "Failed to store authorization request",
			})
			return
		}

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

		// A client can hint at one of this server's configured providers.
		// The server validates the ID and starts only its own provider route. This
		// lets a client with an existing IdP session skip the redundant server
		// login screen without letting the client supply an issuer or endpoint.
		if providerID != "" && s.hasPublicAuthProvider(providerID) {
			providerPath := "/auth/providers/" + url.PathEscape(providerID)
			c.Redirect(http.StatusTemporaryRedirect, providerPath+"?redirect="+url.QueryEscape("/oauth/authorize"))
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
	// Exchanges an authorization code or rotates a renewable bearer session.
	// This endpoint has wildcard CORS since it's called cross-origin by clients.
	oauth.OPTIONS("token", func(c *gin.Context) {
		setOAuthTokenCORS(c)
		c.Status(http.StatusNoContent)
	})

	oauth.POST("token", func(c *gin.Context) {
		setOAuthTokenCORS(c)
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")

		// Accept both JSON and form-encoded (per OAuth 2.0 spec, form-encoded is standard)
		var req oauthTokenRequest
		if c.ContentType() == "application/x-www-form-urlencoded" {
			req.GrantType = c.PostForm("grant_type")
			req.Code = c.PostForm("code")
			req.CodeVerifier = c.PostForm("code_verifier")
			req.RedirectURI = c.PostForm("redirect_uri")
			req.ClientID = c.PostForm("client_id")
			req.RefreshToken = c.PostForm("refresh_token")
			req.RefreshRequestID = c.PostForm("refresh_request_id")
		} else {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":             "invalid_request",
					"error_description": "Invalid request body",
				})
				return
			}
		}

		ctx := c.Request.Context()
		switch req.GrantType {
		case "authorization_code":
			if req.Code == "" || req.CodeVerifier == "" || req.RedirectURI == "" || req.ClientID == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":             "invalid_request",
					"error_description": "code, code_verifier, redirect_uri, and client_id are required",
				})
				return
			}

			credentials, userID, err := s.core.ExchangeAuthCodeForClientSession(ctx, req.Code, req.CodeVerifier, req.RedirectURI, req.ClientID)
			if err != nil {
				writeOAuthCodeExchangeError(c, err)
				return
			}
			response := oauthBearerSessionResponse(credentials)
			if user, err := s.core.GetUser(ctx, userID); err == nil {
				response["user"] = gin.H{
					"id":          user.Id,
					"login":       user.Login,
					"displayName": user.DisplayName,
				}
			}
			c.JSON(http.StatusOK, response)

		case "refresh_token":
			if req.RefreshToken == "" || req.RefreshRequestID == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":             "invalid_request",
					"error_description": "refresh_token and refresh_request_id are required",
				})
				return
			}
			credentials, err := s.core.RefreshBearerSession(ctx, req.RefreshToken, req.RefreshRequestID, req.ClientID)
			if err != nil {
				writeOAuthRefreshError(c, err)
				return
			}
			c.JSON(http.StatusOK, oauthBearerSessionResponse(credentials))

		default:
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "unsupported_grant_type",
				"error_description": "Only authorization_code and refresh_token grants are supported",
			})
		}
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

		params, err := s.readPendingOAuthAuthorize(c.Request.Context(), sessions.Default(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No pending authorization request"})
			return
		}
		redirectOrigin, ok := s.pendingOAuthRedirectOrigin(params)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"redirectUri":    params.RedirectURI,
			"redirectOrigin": redirectOrigin,
			"clientId":       params.ClientID,
			"clientName":     params.ClientName,
			"clientUri":      params.ClientURI,
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

		params, err := s.consumePendingOAuthAuthorize(c.Request.Context(), sessions.Default(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No pending authorization request"})
			return
		}
		redirectOrigin, ok := s.pendingOAuthRedirectOrigin(params)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri"})
			return
		}
		if err := s.core.GrantOAuthClientConsent(c.Request.Context(), credential.auth.UserID, params.ClientID, params.ClientName, params.ClientURI, redirectOrigin); err != nil {
			log.Error("Failed to record OAuth consent grant", "error", err, "userId", credential.auth.UserID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to record consent"})
			return
		}

		redirectURL, ok := s.completeOAuthAuthorizeParamsURL(c, credential.auth.UserID, credential.cookieRecord.GetAuthGeneration(), params)
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
		params, err := s.consumePendingOAuthAuthorize(c.Request.Context(), session)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No pending authorization request"})
			return
		}
		redirectOrigin, ok := s.pendingOAuthRedirectOrigin(params)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid redirect_uri"})
			return
		}
		if err := s.core.RecordOAuthClientConsentDenied(c.Request.Context(), credential.auth.UserID, params.ClientID, params.ClientName, params.ClientURI, redirectOrigin); err != nil {
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

func (s *HTTPServer) hasPublicAuthProvider(providerID string) bool {
	for _, provider := range s.config.Auth.Providers {
		if provider.ID == providerID {
			return true
		}
	}
	return false
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
	GrantType        string `json:"grant_type"`
	Code             string `json:"code"`
	CodeVerifier     string `json:"code_verifier"`
	RedirectURI      string `json:"redirect_uri"`
	ClientID         string `json:"client_id"`
	RefreshToken     string `json:"refresh_token"`
	RefreshRequestID string `json:"refresh_request_id"`
}

func oauthBearerSessionResponse(credentials core.BearerSessionCredentials) gin.H {
	return gin.H{
		"access_token":             credentials.AccessToken,
		"token_type":               "Bearer",
		"expires_in":               bearerSessionLifetimeSeconds(credentials.AccessTokenExpiresAt),
		"refresh_token":            credentials.RefreshToken,
		"refresh_token_expires_in": bearerSessionLifetimeSeconds(credentials.SessionExpiresAt),
	}
}

func writeOAuthCodeExchangeError(c *gin.Context, err error) {
	status := http.StatusBadRequest
	oauthErr := "invalid_grant"
	desc := "Authorization code is invalid or has expired"
	switch {
	case errors.Is(err, core.ErrAuthCodeNotFound):
	case errors.Is(err, core.ErrAuthCodeInvalidVerifier):
		desc = "PKCE code_verifier does not match code_challenge"
	case errors.Is(err, core.ErrAuthCodeRedirectMismatch):
		desc = "redirect_uri does not match the authorization request"
	case errors.Is(err, core.ErrAuthCodeClientMismatch):
		desc = "client_id does not match the authorization request"
	case errors.Is(err, core.ErrOAuthClientBlocked):
		oauthErr = "invalid_client"
		desc = "The OAuth client is blocked by this server"
	default:
		status = http.StatusInternalServerError
		oauthErr = "server_error"
		desc = "Failed to exchange authorization code"
		log.Error("OAuth token exchange failed", "error", err)
	}
	c.JSON(status, gin.H{"error": oauthErr, "error_description": desc})
}

func writeOAuthRefreshError(c *gin.Context, err error) {
	if errors.Is(err, core.ErrRefreshRequestIDInvalid) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "refresh_request_id is invalid",
		})
		return
	}
	if errors.Is(err, core.ErrRefreshTokenNotFound) ||
		errors.Is(err, core.ErrRefreshTokenReused) ||
		errors.Is(err, core.ErrRefreshTokenClientMismatch) ||
		errors.Is(err, core.ErrOAuthClientBlocked) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_grant",
			"error_description": "Refresh token is invalid, expired, or revoked",
		})
		return
	}
	log.Error("OAuth token refresh failed", "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":             "server_error",
		"error_description": "Failed to refresh bearer session",
	})
}

type pendingOAuthAuthorize = core.PendingOAuthAuthorize

func pendingOAuthAuthorizeToken(session sessions.Session) string {
	token, _ := session.Get(sessionKeyOAuthPendingAuthorize).(string)
	return token
}

func (s *HTTPServer) storePendingOAuthAuthorize(ctx context.Context, session sessions.Session, pending pendingOAuthAuthorize) error {
	previousToken := pendingOAuthAuthorizeToken(session)
	token, err := s.core.CreatePendingOAuthAuthorize(ctx, pending)
	if err != nil {
		return err
	}
	session.Set(sessionKeyOAuthPendingAuthorize, token)
	if err := session.Save(); err != nil {
		_ = s.core.DiscardPendingOAuthAuthorize(ctx, token)
		return err
	}
	if previousToken != "" && previousToken != token {
		_ = s.core.DiscardPendingOAuthAuthorize(ctx, previousToken)
	}
	return nil
}

func (s *HTTPServer) readPendingOAuthAuthorize(ctx context.Context, session sessions.Session) (pendingOAuthAuthorize, error) {
	pending, err := s.core.GetPendingOAuthAuthorize(ctx, pendingOAuthAuthorizeToken(session))
	if errors.Is(err, core.ErrPendingOAuthAuthorizeNotFound) {
		return pendingOAuthAuthorize{}, errNoPendingOAuthAuthorize
	}
	return pending, err
}

func (s *HTTPServer) consumePendingOAuthAuthorize(ctx context.Context, session sessions.Session) (pendingOAuthAuthorize, error) {
	token := pendingOAuthAuthorizeToken(session)
	pending, err := s.core.ConsumePendingOAuthAuthorize(ctx, token)
	if errors.Is(err, core.ErrPendingOAuthAuthorizeNotFound) {
		return pendingOAuthAuthorize{}, errNoPendingOAuthAuthorize
	}
	if err != nil {
		return pendingOAuthAuthorize{}, err
	}
	session.Delete(sessionKeyOAuthPendingAuthorize)
	if err := session.Save(); err != nil {
		return pendingOAuthAuthorize{}, err
	}
	return pending, nil
}

func (s *HTTPServer) continueOAuthAuthorize(c *gin.Context, userID string, authGeneration uint64) {
	params, err := s.readPendingOAuthAuthorize(c.Request.Context(), sessions.Default(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "No pending authorization request",
		})
		return
	}
	redirectOrigin, ok := s.pendingOAuthRedirectOrigin(params)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
		return
	}
	consented, err := s.core.HasOAuthClientConsent(c.Request.Context(), userID, params.ClientID, redirectOrigin)
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
	params, err := s.consumePendingOAuthAuthorize(c.Request.Context(), sessions.Default(c))
	if err != nil {
		if errors.Is(err, errNoPendingOAuthAuthorize) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_request",
				"error_description": "No pending authorization request",
			})
		} else {
			log.Error("Failed to consume pending OAuth authorization request", "error", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":             "server_error",
				"error_description": "Failed to continue authorization request",
			})
		}
		return "", false
	}
	return s.completeOAuthAuthorizeParamsURL(c, userID, authGeneration, params)
}

func (s *HTTPServer) completeOAuthAuthorizeParamsURL(c *gin.Context, userID string, authGeneration uint64, params pendingOAuthAuthorize) (string, bool) {
	ctx := c.Request.Context()
	if err := s.core.RequireOAuthClientAllowed(ctx, params.ClientID); err != nil {
		if errors.Is(err, core.ErrOAuthClientBlocked) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_client",
				"error_description": "The OAuth client is blocked by this server",
			})
			return "", false
		}
		log.Error("Failed to check OAuth client policy", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":             "server_error",
			"error_description": "Failed to check OAuth client policy",
		})
		return "", false
	}
	source := corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD
	if params.ClientID == config.ChattoDesktopOrigin {
		source = corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_BUILT_IN
	}
	redirectOrigin, ok := s.pendingOAuthRedirectOrigin(params)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
		return "", false
	}

	// Parse the already-validated redirect before creating any durable or
	// runtime authorization state so a malformed value cannot leave a phantom
	// completed authorization.
	u, err := url.Parse(params.RedirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
		return "", false
	}
	code, err := s.core.CreateOAuthClientAuthorizationCode(ctx, core.OAuthClientAuthorization{
		UserID:         userID,
		ClientID:       params.ClientID,
		ClientName:     params.ClientName,
		ClientOrigin:   params.ClientURI,
		RedirectOrigin: redirectOrigin,
		Source:         source,
	}, params.RedirectURI, params.CodeChallenge, params.CodeChallengeMethod, authGeneration)
	if err != nil {
		if errors.Is(err, core.ErrOAuthClientBlocked) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid_client",
				"error_description": "The OAuth client is blocked by this server",
			})
			return "", false
		}
		log.Error("Failed to complete OAuth authorization", "error", err, "userId", userID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":             "server_error",
			"error_description": "Failed to complete authorization",
		})
		return "", false
	}

	// Build redirect URL with code and state
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
	return pendingOAuthAuthorizeToken(session) != ""
}

func (s *HTTPServer) pendingOAuthRedirectOrigin(params pendingOAuthAuthorize) (string, bool) {
	u, err := url.Parse(params.RedirectURI)
	if err != nil || u.Scheme == "" {
		return "", false
	}
	if u.Host == "" {
		return strings.ToLower(u.Scheme) + ":", true
	}
	return canonicalOrigin(u), true
}

func canonicalOrigin(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	hostname := strings.ToLower(u.Hostname())
	if ascii, err := idna.Lookup.ToASCII(hostname); err == nil {
		hostname = ascii
	}
	if address, err := netip.ParseAddr(hostname); err == nil {
		hostname = address.String()
	}
	port := u.Port()
	if numericPort, err := strconv.ParseUint(port, 10, 16); err == nil {
		port = strconv.FormatUint(numericPort, 10)
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	} else if strings.Contains(hostname, ":") {
		host = "[" + hostname + "]"
	}
	return scheme + "://" + host
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
