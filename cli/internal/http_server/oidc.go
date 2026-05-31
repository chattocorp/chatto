package http_server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

// Session keys for the OIDC login flow (PKCE).
const (
	sessionKeyOIDCState        = "oidc_state"
	sessionKeyOIDCCodeVerifier = "oidc_code_verifier"
)

// oidcProvider holds the lazily-initialized OIDC provider, oauth2 config, and
// token verifier. Initialized on first login attempt. Retries on failure.
type oidcProvider struct {
	mu           sync.Mutex
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
	ready        bool
}

func (o *oidcProvider) init(issuerURL, clientID, clientSecret, redirectURL string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.ready {
		return nil
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		log.Error("Failed to initialize OIDC provider", "issuer", issuerURL, "error", err)
		return err
	}

	o.provider = provider
	o.oauth2Config = oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}
	o.verifier = provider.Verifier(&oidc.Config{ClientID: clientID})
	o.ready = true

	log.Info("OIDC provider initialized", "issuer", issuerURL)
	return nil
}

func (s *HTTPServer) setupOIDCRoutes() {
	if !s.config.Auth.OIDC.IsConfigured() {
		return
	}

	oidcConfig := s.config.Auth.OIDC
	op := &oidcProvider{}

	// Helper that initializes the provider on first use and returns an error page on failure.
	ensureProvider := func(c *gin.Context) bool {
		if err := op.init(oidcConfig.IssuerURL, oidcConfig.ClientID, oidcConfig.ClientSecret, s.config.Webserver.URL+"/auth/oidc/callback"); err != nil {
			log.Error("OIDC provider not available", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return false
		}
		return true
	}

	auth := s.router.Group("/auth")
	auth.Use(func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)
		c.Next()
	})

	// GET /auth/oidc — Redirect to OIDC provider with PKCE
	auth.GET("oidc", func(c *gin.Context) {
		if !ensureProvider(c) {
			return
		}

		session := sessions.Default(c)

		// Store redirect URL if provided
		if redirect := c.Query("redirect"); redirect != "" {
			if isValidInternalRedirect(redirect) {
				session.Set("oauth_redirect", redirect)
			}
		}

		state, err := randomString(32)
		if err != nil {
			log.Error("Failed to generate OIDC state", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		codeVerifier, err := randomString(64)
		if err != nil {
			log.Error("Failed to generate PKCE code verifier", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		session.Set(sessionKeyOIDCState, state)
		session.Set(sessionKeyOIDCCodeVerifier, codeVerifier)
		session.Save()

		codeChallenge := s256Challenge(codeVerifier)

		authURL := op.oauth2Config.AuthCodeURL(state,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)

		c.Redirect(http.StatusTemporaryRedirect, authURL)
	})

	// GET /auth/oidc/callback — Handle OIDC provider callback
	auth.GET("oidc/callback", func(c *gin.Context) {
		if !ensureProvider(c) {
			return
		}

		session := sessions.Default(c)
		ctx := c.Request.Context()

		// Verify state
		expectedState, _ := session.Get(sessionKeyOIDCState).(string)
		if expectedState == "" || c.Query("state") != expectedState {
			log.Warn("OIDC callback state mismatch")
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		codeVerifier, _ := session.Get(sessionKeyOIDCCodeVerifier).(string)
		if codeVerifier == "" {
			log.Warn("OIDC callback missing code verifier")
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		// Clear OIDC session state
		session.Delete(sessionKeyOIDCState)
		session.Delete(sessionKeyOIDCCodeVerifier)
		session.Save()

		// Check for error from provider
		if errCode := c.Query("error"); errCode != "" {
			log.Warn("OIDC provider returned error", "error", errCode, "description", c.Query("error_description"))
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_denied")
			return
		}

		log.Info("OIDC callback received, exchanging code")

		// Exchange authorization code for tokens
		token, err := op.oauth2Config.Exchange(ctx, c.Query("code"),
			oauth2.SetAuthURLParam("code_verifier", codeVerifier),
		)
		if err != nil {
			log.Error("OIDC token exchange failed", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		// Extract and verify the ID token
		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok {
			log.Error("OIDC token response missing id_token")
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		idToken, err := op.verifier.Verify(ctx, rawIDToken)
		if err != nil {
			log.Error("OIDC ID token verification failed", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		// Extract claims from the ID token first
		var claims struct {
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
			PreferredUser string `json:"preferred_username"`
			Picture       string `json:"picture"`
		}
		if err := idToken.Claims(&claims); err != nil {
			log.Error("Failed to parse OIDC claims", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		log.Info("OIDC token verified", "sub", idToken.Subject, "issuer", idToken.Issuer)

		// Some providers (e.g. Zitadel) don't include email in the ID token.
		// Fall back to the userinfo endpoint.
		if claims.Email == "" {
			log.Info("OIDC ID token missing email, falling back to userinfo", "sub", idToken.Subject)
			userInfo, err := op.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
			if err != nil {
				log.Error("Failed to fetch OIDC userinfo", "error", err)
				c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
				return
			}
			if err := userInfo.Claims(&claims); err != nil {
				log.Error("Failed to parse OIDC userinfo claims", "error", err)
				c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
				return
			}
		}

		if claims.Email == "" || !claims.EmailVerified {
			log.Warn("OIDC provider returned no verified email", "hasEmail", claims.Email != "", "verified", claims.EmailVerified)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_no_email")
			return
		}
		// Normalize at the HTTP boundary so downstream core code can treat email as canonical.
		claims.Email = strings.ToLower(strings.TrimSpace(claims.Email))

		issuer := idToken.Issuer
		subject := idToken.Subject

		// 1. Try to find user by OIDC subject (stable identity across email changes)
		user, err := s.core.GetUserByOIDCSubject(ctx, issuer, subject)
		if err != nil {
			log.Error("Failed to lookup user by OIDC subject", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		if user != nil {
			log.Info("OIDC login matched by subject", "sub", subject, "userId", user.Id)
		} else {
			// 2. No subject link — try to find existing user by verified email
			user, _ = s.core.GetUserByVerifiedEmail(ctx, claims.Email)

			if user != nil {
				log.Info("OIDC login matched by email, linking subject", "sub", subject, "userId", user.Id)
			} else {
				log.Info("OIDC login creating new user", "sub", subject)
				// 3. No existing user — create a new one
				login := deriveLoginFromEmail(claims.Email)
				displayName := claims.Name
				if displayName == "" {
					displayName = claims.PreferredUser
				}
				if displayName == "" {
					displayName = login
				}

				// Create user with verified email atomically (OIDC provider already verified it)
				user, err = s.core.CreateVerifiedUser(ctx, "system", login, displayName, "", claims.Email)
				if err != nil {
					log.Error("Failed to create user from OIDC", "error", err)
					c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
					return
				}

				// Server membership is implicit; global rooms appear automatically.
			}

			// Link the OIDC subject to this user for future logins
			if err := s.core.LinkOIDCSubject(ctx, issuer, subject, user.Id); err != nil {
				log.Error("Failed to link OIDC subject", "error", err, "userId", user.Id, "subject", subject)
			} else {
				log.Info("Linked OIDC subject to user", "userId", user.Id, "subject", subject)
			}
		}

		// Fetch avatar from OIDC provider if user doesn't have one
		if claims.Picture != "" {
			existingAvatar, _ := s.core.GetUserAvatar(ctx, user.Id)
			if existingAvatar == nil {
				if err := fetchAndUploadAvatarFromURL(ctx, claims.Picture, s, user.Id); err != nil {
					log.Error("Failed to fetch OIDC avatar", "error", err)
				}
			}
		}

		// Create session
		session.Set("user_id", user.Id)
		if err := session.Save(); err != nil {
			log.Error("Failed to save session", "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}
		if err := s.core.RecordLoginSucceeded(ctx, user.Id, claims.Email); err != nil {
			log.Error("Failed to append OIDC login audit event", "userId", user.Id, "error", err)
			session.Clear()
			_ = session.Save()
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=oidc_failed")
			return
		}

		// If there's a pending OAuth authorize flow, complete it
		if hasPendingOAuthAuthorize(session) {
			s.completeOAuthAuthorize(c, user.Id)
			return
		}

		// Get and clear redirect URL
		redirectURL := "/"
		if redirect := session.Get("oauth_redirect"); redirect != nil {
			if r, ok := redirect.(string); ok && r != "" && isValidInternalRedirect(r) {
				redirectURL = r
			}
			session.Delete("oauth_redirect")
			session.Save()
		}

		// Append bearer token for cross-origin clients
		if bearerToken, err := s.core.CreateAuthToken(ctx, user.Id); err == nil {
			separator := "?"
			if strings.Contains(redirectURL, "?") {
				separator = "&"
			}
			redirectURL = redirectURL + separator + "token=" + bearerToken
		} else {
			log.Warn("Failed to create auth token on OIDC login", "userId", user.Id, "error", err)
		}

		c.Redirect(http.StatusTemporaryRedirect, redirectURL)
	})
}

// randomString generates a URL-safe random string of n bytes.
func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// s256Challenge computes the S256 PKCE code challenge from a code verifier.
func s256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// fetchAndUploadAvatarFromURL downloads an avatar from a URL and uploads it.
func fetchAndUploadAvatarFromURL(ctx context.Context, avatarURL string, s *HTTPServer, userID string) error {
	// Validate the URL before making a request
	u, err := url.Parse(avatarURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil // silently skip invalid URLs
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	asset, err := s.core.UploadUserAvatar(ctx, userID, resp.Body)
	if err != nil {
		return err
	}

	return s.core.SetUserAvatar(ctx, userID, asset)
}
