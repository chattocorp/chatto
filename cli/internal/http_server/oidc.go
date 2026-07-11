package http_server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/discord"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"
	"github.com/markbates/goth/providers/google"
	"golang.org/x/oauth2"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
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

func (o *oidcProvider) init(issuerURL, clientID, clientSecret, redirectURL string, scopes []string) error {
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
		Scopes:       append([]string(nil), scopes...),
	}
	o.verifier = provider.Verifier(&oidc.Config{ClientID: clientID})
	o.ready = true

	log.Info("OIDC provider initialized", "issuer", issuerURL)
	return nil
}

func (s *HTTPServer) setupOIDCRoutes() {
	providers := s.config.Auth.Providers
	if len(providers) == 0 {
		return
	}

	configured := make(map[string]*authProviderRuntime, len(providers))
	var legacyOIDCConfig *config.AuthProviderConfig
	for _, providerConfig := range providers {
		runtime, err := newAuthProviderRuntime(providerConfig, s.providerCallbackURL(providerConfig.ID))
		if err != nil {
			s.logger.Error("Skipping invalid auth provider", "provider_id", providerConfig.ID, "provider_type", providerConfig.Type, "error", err)
			continue
		}
		configured[providerConfig.ID] = runtime
		if providerConfig.Type == config.AuthProviderTypeOpenIDConnect {
			providerConfig := providerConfig
			if legacyOIDCConfig == nil || providerConfig.ID == "oidc" {
				legacyOIDCConfig = &providerConfig
			}
		}
	}
	if len(configured) == 0 {
		return
	}

	var legacyOIDCRuntime *authProviderRuntime
	if legacyOIDCConfig != nil {
		runtime, err := newAuthProviderRuntime(*legacyOIDCConfig, s.legacyOIDCCallbackURL())
		if err != nil {
			s.logger.Error("Skipping legacy OIDC auth route", "provider_id", legacyOIDCConfig.ID, "error", err)
		} else {
			legacyOIDCRuntime = runtime
		}
	}

	auth := s.router.Group("/auth")
	auth.Use(limitLegacyRequestBody())
	auth.Use(func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)
		c.Next()
	})

	auth.GET("providers/:providerID", func(c *gin.Context) {
		providerRuntime := configured[c.Param("providerID")]
		if providerRuntime == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_not_found")
			return
		}

		s.handleProviderStart(c, providerRuntime)
	})

	auth.GET("providers/:providerID/callback", func(c *gin.Context) {
		providerRuntime := configured[c.Param("providerID")]
		if providerRuntime == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_not_found")
			return
		}

		s.handleProviderCallback(c, providerRuntime)
	})

	if legacyOIDCRuntime != nil {
		auth.GET("oidc", func(c *gin.Context) {
			s.handleProviderStart(c, legacyOIDCRuntime)
		})
		auth.GET("oidc/callback", func(c *gin.Context) {
			s.handleProviderCallback(c, legacyOIDCRuntime)
		})
	}
}

func (s *HTTPServer) handleProviderStart(c *gin.Context, providerRuntime *authProviderRuntime) {
	session := sessions.Default(c)
	intent := c.Query("intent")
	linkStartRedirect := ""
	if intent == "link" {
		start, err := s.core.ConsumePendingExternalIdentityLinkStart(c.Request.Context(), c.Query("link_start"))
		if err != nil || start.ProviderID != providerRuntime.config.ID {
			if err != nil {
				log.Warn("Provider link start token failed", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "error", err)
			} else {
				log.Warn("Provider link start token provider mismatch", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type)
			}
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
			return
		}
		session.Set(providerSessionKey(providerRuntime.config.ID, "intent"), "link")
		session.Set(providerSessionKey(providerRuntime.config.ID, "link_user_id"), start.BoundUserID)
		if isValidInternalRedirect(start.RedirectPath) {
			linkStartRedirect = start.RedirectPath
		}
	} else {
		session.Set(providerSessionKey(providerRuntime.config.ID, "intent"), "login")
		session.Delete(providerSessionKey(providerRuntime.config.ID, "link_user_id"))
	}

	// Store redirect URL if provided
	if linkStartRedirect != "" {
		session.Set("oauth_redirect", linkStartRedirect)
	} else if redirect := c.Query("redirect"); redirect != "" {
		if isValidInternalRedirect(redirect) {
			session.Set("oauth_redirect", redirect)
		}
	}

	state, err := randomString(32)
	if err != nil {
		log.Error("Failed to generate provider auth state", "provider_id", providerRuntime.config.ID, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}

	session.Set(providerSessionKey(providerRuntime.config.ID, "state"), state)

	var authURL string
	if providerRuntime.config.Type == config.AuthProviderTypeOpenIDConnect {
		if !providerRuntime.ensureOIDC(c) {
			return
		}
		codeVerifier, err := randomString(64)
		if err != nil {
			log.Error("Failed to generate PKCE code verifier", "provider_id", providerRuntime.config.ID, "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
			return
		}
		session.Set(providerSessionKey(providerRuntime.config.ID, "code_verifier"), codeVerifier)
		codeChallenge := s256Challenge(codeVerifier)
		authURL = providerRuntime.oidc.oauth2Config.AuthCodeURL(state,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
	} else {
		gothSession, err := providerRuntime.goth.BeginAuth(state)
		if err != nil {
			log.Error("Failed to begin provider auth", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
			return
		}
		session.Set(providerSessionKey(providerRuntime.config.ID, "session"), gothSession.Marshal())
		authURL, err = gothSession.GetAuthURL()
		if err != nil {
			log.Error("Failed to build provider auth URL", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
			return
		}
	}

	if err := session.Save(); err != nil {
		log.Error("Failed to save provider auth session", "provider_id", providerRuntime.config.ID, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

func (s *HTTPServer) handleProviderCallback(c *gin.Context, providerRuntime *authProviderRuntime) {
	session := sessions.Default(c)
	ctx := c.Request.Context()

	// Verify state
	expectedState, _ := session.Get(providerSessionKey(providerRuntime.config.ID, "state")).(string)
	if expectedState == "" || c.Query("state") != expectedState {
		log.Warn("Provider callback state mismatch", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type)
		session.Delete(providerSessionKey(providerRuntime.config.ID, "state"))
		session.Delete(providerSessionKey(providerRuntime.config.ID, "code_verifier"))
		session.Delete(providerSessionKey(providerRuntime.config.ID, "session"))
		_ = session.Save()
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}

	session.Delete(providerSessionKey(providerRuntime.config.ID, "state"))
	intent, _ := session.Get(providerSessionKey(providerRuntime.config.ID, "intent")).(string)
	linkUserID, _ := session.Get(providerSessionKey(providerRuntime.config.ID, "link_user_id")).(string)
	session.Delete(providerSessionKey(providerRuntime.config.ID, "intent"))
	session.Delete(providerSessionKey(providerRuntime.config.ID, "link_user_id"))

	// Check for error from provider
	if errCode := c.Query("error"); errCode != "" {
		log.Warn("Provider returned auth error", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "error", errCode)
		session.Delete(providerSessionKey(providerRuntime.config.ID, "code_verifier"))
		session.Delete(providerSessionKey(providerRuntime.config.ID, "session"))
		_ = session.Save()
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_denied")
		return
	}

	if providerRuntime.config.Type == config.AuthProviderTypeOpenIDConnect {
		if !providerRuntime.ensureOIDC(c) {
			return
		}
	}

	identity, err := providerRuntime.resolveIdentity(c, session)
	if err != nil {
		log.Error("Provider callback failed", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "error", err)
		session.Delete(providerSessionKey(providerRuntime.config.ID, "code_verifier"))
		session.Delete(providerSessionKey(providerRuntime.config.ID, "session"))
		_ = session.Save()
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
	session.Delete(providerSessionKey(providerRuntime.config.ID, "code_verifier"))
	session.Delete(providerSessionKey(providerRuntime.config.ID, "session"))
	_ = session.Save()

	user, err := s.core.GetUserByExternalIdentity(ctx, identity.issuer, identity.subject)
	if err != nil {
		log.Error("Failed to lookup user by external identity", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
	if user == nil {
		log.Info("Provider login has no linked account", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type)
		s.redirectPendingExternalIdentity(c, session, providerRuntime.config, identity, intent, linkUserID)
		return
	}

	if intent == "link" {
		if linkUserID == "" || linkUserID != user.Id {
			c.Redirect(http.StatusTemporaryRedirect, providerReturnPathWithError(session, "/", "external_identity_conflict"))
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, providerReturnPath(session, "/"))
		return
	}

	log.Info("Provider login matched by external identity", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "userId", user.Id)

	if identity.avatarURL != "" {
		existingAvatar, _ := s.core.GetUserAvatar(ctx, user.Id)
		if existingAvatar == nil {
			if err := s.core.ImportUserAvatarFromURL(ctx, user.Id, identity.avatarURL); err != nil {
				log.Warn("Failed to import provider avatar", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "userId", user.Id, "error", err)
			}
		}
	}

	if err := s.completeProviderLogin(c, session, user.Id, providerRuntime.config); err != nil {
		log.Error("Failed to complete provider login", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "userId", user.Id, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
}

type authProviderRuntime struct {
	config      config.AuthProviderConfig
	callbackURL string
	oidc        *oidcProvider
	goth        goth.Provider
}

type resolvedProviderIdentity struct {
	issuer          string
	subject         string
	verifiedEmail   string
	avatarURL       string
	loginHint       string
	displayNameHint string
}

func newAuthProviderRuntime(providerConfig config.AuthProviderConfig, callbackURL string) (*authProviderRuntime, error) {
	runtime := &authProviderRuntime{config: providerConfig, callbackURL: callbackURL}
	scopes := providerScopes(providerConfig)
	switch providerConfig.Type {
	case config.AuthProviderTypeOpenIDConnect:
		runtime.oidc = &oidcProvider{}
	case config.AuthProviderTypeGitHub:
		runtime.goth = github.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
	case config.AuthProviderTypeGitLab:
		runtime.goth = gitlab.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
	case config.AuthProviderTypeGoogle:
		runtime.goth = google.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
	case config.AuthProviderTypeDiscord:
		runtime.goth = discord.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
	default:
		return nil, fmt.Errorf("unsupported auth provider type %q", providerConfig.Type)
	}
	if runtime.goth != nil {
		runtime.goth.SetName(providerConfig.ID)
	}
	return runtime, nil
}

func providerScopes(providerConfig config.AuthProviderConfig) []string {
	if len(providerConfig.Scopes) > 0 {
		scopes := append([]string(nil), providerConfig.Scopes...)
		if providerConfig.Type == config.AuthProviderTypeOpenIDConnect && !hasScope(scopes, oidc.ScopeOpenID) {
			scopes = append([]string{oidc.ScopeOpenID}, scopes...)
		}
		return scopes
	}
	if providerConfig.Type == config.AuthProviderTypeOpenIDConnect {
		scopes := []string{oidc.ScopeOpenID, "profile"}
		if providerConfig.RequestEmailOrDefault() {
			scopes = append(scopes, "email")
		}
		return scopes
	}
	if !providerConfig.RequestEmailOrDefault() {
		if providerConfig.Type == config.AuthProviderTypeGoogle {
			return []string{"openid", "profile"}
		}
		return nil
	}
	switch providerConfig.Type {
	case config.AuthProviderTypeGitHub:
		return []string{"read:user", "user:email"}
	case config.AuthProviderTypeGoogle:
		return []string{"openid", "profile", "email"}
	case config.AuthProviderTypeDiscord:
		return []string{discord.ScopeIdentify, discord.ScopeEmail}
	case config.AuthProviderTypeGitLab:
		return []string{"read_user"}
	default:
		return nil
	}
}

func hasScope(scopes []string, target string) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func (s *HTTPServer) providerCallbackURL(providerID string) string {
	return strings.TrimRight(s.config.Webserver.URL, "/") + "/auth/providers/" + url.PathEscape(providerID) + "/callback"
}

func (s *HTTPServer) legacyOIDCCallbackURL() string {
	return strings.TrimRight(s.config.Webserver.URL, "/") + "/auth/oidc/callback"
}

func providerSessionKey(providerID, name string) string {
	return "provider_" + providerID + "_" + name
}

func (r *authProviderRuntime) ensureOIDC(c *gin.Context) bool {
	if r.oidc == nil {
		return true
	}
	if err := r.oidc.init(r.config.IssuerURL, r.config.ClientID, r.config.ClientSecret, r.callbackURL, providerScopes(r.config)); err != nil {
		log.Error("OIDC provider not available", "provider_id", r.config.ID, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return false
	}
	return true
}

func (r *authProviderRuntime) resolveIdentity(c *gin.Context, session sessions.Session) (resolvedProviderIdentity, error) {
	if r.config.Type == config.AuthProviderTypeOpenIDConnect {
		return r.resolveOIDCIdentity(c, session)
	}
	return r.resolveGothIdentity(c, session)
}

func (r *authProviderRuntime) resolveOIDCIdentity(c *gin.Context, session sessions.Session) (resolvedProviderIdentity, error) {
	ctx := c.Request.Context()
	codeVerifier, _ := session.Get(providerSessionKey(r.config.ID, "code_verifier")).(string)
	if codeVerifier == "" {
		return resolvedProviderIdentity{}, fmt.Errorf("missing code verifier")
	}

	// Exchange authorization code for tokens
	token, err := r.oidc.oauth2Config.Exchange(ctx, c.Query("code"),
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("token exchange failed: %w", err)
	}

	// Extract and verify the ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return resolvedProviderIdentity{}, fmt.Errorf("token response missing id_token")
	}

	idToken, err := r.oidc.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("id token verification failed: %w", err)
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
		return resolvedProviderIdentity{}, fmt.Errorf("parse id token claims: %w", err)
	}

	log.Info("OIDC token verified", "provider_id", r.config.ID, "issuer", idToken.Issuer)

	// Some providers (e.g. Zitadel) don't include email in the ID token.
	// Fall back to the userinfo endpoint.
	if claims.Email == "" {
		log.Info("OIDC ID token missing email, falling back to userinfo", "provider_id", r.config.ID)
		userInfo, err := r.oidc.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		if err != nil {
			log.Warn("OIDC userinfo fallback failed", "provider_id", r.config.ID, "error", err)
		} else if err := userInfo.Claims(&claims); err != nil {
			log.Warn("OIDC userinfo claims ignored", "provider_id", r.config.ID, "error", err)
		}
	}

	verifiedEmail := ""
	if claims.Email != "" && claims.EmailVerified {
		verifiedEmail = strings.ToLower(strings.TrimSpace(claims.Email))
	}
	return resolvedProviderIdentity{
		issuer:          idToken.Issuer,
		subject:         idToken.Subject,
		verifiedEmail:   verifiedEmail,
		avatarURL:       claims.Picture,
		loginHint:       loginHintFromParts(claims.PreferredUser, verifiedEmail, claims.Name),
		displayNameHint: displayNameHintFromParts(claims.Name, claims.PreferredUser, verifiedEmail),
	}, nil
}

func (r *authProviderRuntime) resolveGothIdentity(c *gin.Context, session sessions.Session) (resolvedProviderIdentity, error) {
	storedSession, _ := session.Get(providerSessionKey(r.config.ID, "session")).(string)
	if storedSession == "" {
		return resolvedProviderIdentity{}, fmt.Errorf("missing provider session")
	}
	gothSession, err := r.goth.UnmarshalSession(storedSession)
	if err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("unmarshal provider session: %w", err)
	}
	if _, err := gothSession.Authorize(r.goth, c.Request.URL.Query()); err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("authorize provider session: %w", err)
	}
	gothUser, err := r.goth.FetchUser(gothSession)
	if err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("fetch provider user: %w", err)
	}
	if gothUser.UserID == "" {
		return resolvedProviderIdentity{}, fmt.Errorf("provider returned empty user id")
	}
	return resolvedProviderIdentity{
		issuer:          r.config.ID,
		subject:         gothUser.UserID,
		verifiedEmail:   r.verifiedEmailFromGothUser(c.Request.Context(), gothUser),
		avatarURL:       gothUser.AvatarURL,
		loginHint:       loginHintFromParts(gothUser.NickName, gothUser.Email, gothUser.Name),
		displayNameHint: displayNameHintFromParts(gothUser.Name, gothUser.NickName, gothUser.Email),
	}, nil
}

func (r *authProviderRuntime) verifiedEmailFromGothUser(ctx context.Context, gothUser goth.User) string {
	switch r.config.Type {
	case config.AuthProviderTypeDiscord:
		if rawBool(gothUser.RawData, "verified") {
			return normalizeProviderEmail(gothUser.Email)
		}
	case config.AuthProviderTypeGoogle:
		if rawBool(gothUser.RawData, "verified_email") || rawBool(gothUser.RawData, "email_verified") {
			return normalizeProviderEmail(gothUser.Email)
		}
	case config.AuthProviderTypeGitHub:
		email, err := fetchGitHubVerifiedPrimaryEmail(ctx, gothUser.AccessToken)
		if err == nil {
			return email
		}
	}
	return ""
}

func rawBool(raw map[string]interface{}, key string) bool {
	if raw == nil {
		return false
	}
	value, ok := raw[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func normalizeProviderEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ""
	}
	return email
}

func fetchGitHubVerifiedPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", fmt.Errorf("missing github access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, github.EmailURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github email endpoint returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, email := range emails {
		if email.Primary && email.Verified {
			normalized := normalizeProviderEmail(email.Email)
			if normalized != "" {
				return normalized, nil
			}
		}
	}
	return "", fmt.Errorf("github account has no verified primary email")
}

func (s *HTTPServer) redirectPendingExternalIdentity(c *gin.Context, session sessions.Session, providerConfig config.AuthProviderConfig, identity resolvedProviderIdentity, intent, linkUserID string) {
	ctx := c.Request.Context()
	flow := core.PendingExternalIdentityFlow{
		ProviderID:      providerConfig.ID,
		ProviderType:    providerConfig.Type,
		ProviderLabel:   providerConfig.LabelOrDefault(),
		Issuer:          identity.issuer,
		Subject:         identity.subject,
		VerifiedEmail:   identity.verifiedEmail,
		AvatarURL:       identity.avatarURL,
		LoginHint:       identity.loginHint,
		DisplayNameHint: identity.displayNameHint,
		RedirectPath:    providerReturnPath(session, "/"),
	}

	var (
		token string
		err   error
	)
	if intent == "link" {
		if linkUserID == "" {
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=authentication_required")
			return
		}
		token, err = s.core.CreatePendingExternalIdentityLinkFlow(ctx, flow, linkUserID)
	} else {
		if !providerConfig.AutoProvisionOrDefault() {
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=external_identity_unlinked")
			return
		}
		token, err = s.core.CreatePendingExternalIdentityCreateFlow(ctx, flow)
	}
	if err != nil {
		log.Error("Failed to create pending external identity flow", "provider_id", providerConfig.ID, "provider_type", providerConfig.Type, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, "/sso/confirm?token="+url.QueryEscape(token))
}

func providerReturnPath(session sessions.Session, fallback string) string {
	if redirect := session.Get("oauth_redirect"); redirect != nil {
		if r, ok := redirect.(string); ok && r != "" && isValidInternalRedirect(r) {
			session.Delete("oauth_redirect")
			_ = session.Save()
			return r
		}
	}
	return fallback
}

func providerReturnPathWithError(session sessions.Session, fallback, errorCode string) string {
	returnPath := providerReturnPath(session, fallback)
	u, err := url.Parse(returnPath)
	if err != nil {
		return fallback
	}
	q := u.Query()
	q.Set("error", errorCode)
	u.RawQuery = q.Encode()
	return u.String()
}

func loginHintFromParts(parts ...string) string {
	for _, part := range parts {
		hint := loginHint(part)
		if hint != "" {
			return hint
		}
	}
	return ""
}

func loginHint(value string) string {
	value = strings.TrimSpace(value)
	if emailName, _, ok := strings.Cut(value, "@"); ok {
		value = emailName
	}
	value = strings.ToLower(value)
	var b strings.Builder
	lastDot := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_' || r == '-':
			b.WriteRune(r)
			lastDot = false
		case r == '.':
			if !lastDot {
				b.WriteRune(r)
				lastDot = true
			}
		case r == ' ':
			if !lastDot {
				b.WriteRune('-')
				lastDot = false
			}
		}
		if b.Len() >= 32 {
			break
		}
	}
	hint := strings.Trim(b.String(), ".-_")
	if len(hint) < 2 {
		return ""
	}
	return hint
}

func displayNameHintFromParts(parts ...string) string {
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if emailName, _, ok := strings.Cut(part, "@"); ok {
			part = emailName
		}
		if part != "" {
			return part
		}
	}
	return ""
}

func (s *HTTPServer) completeProviderLogin(c *gin.Context, session sessions.Session, userID string, providerConfig config.AuthProviderConfig) error {
	ctx := c.Request.Context()
	source := providerConfig.Type + "_login"
	if err := s.createCookieSession(c, userID, source); err != nil {
		return fmt.Errorf("save cookie session: %w", err)
	}
	if err := s.ensureCSRFToken(c); err != nil {
		session = sessions.Default(c)
		cookieCredential, _ := cookieCredentialFromSession(session)
		_ = s.core.RevokeCookieSession(ctx, userID, cookieCredential.sessionID)
		session.Clear()
		_ = session.Save()
		clearCSRFCookie(c)
		return fmt.Errorf("create csrf token: %w", err)
	}
	if err := s.core.RecordLoginSucceeded(ctx, userID, providerConfig.Type+":"+providerConfig.ID); err != nil {
		session = sessions.Default(c)
		cookieCredential, _ := cookieCredentialFromSession(session)
		_ = s.core.RevokeCookieSession(ctx, userID, cookieCredential.sessionID)
		session.Clear()
		_ = session.Save()
		clearCSRFCookie(c)
		return fmt.Errorf("append login audit event: %w", err)
	}

	if hasPendingOAuthAuthorize(session) {
		authGeneration, err := s.core.CurrentAuthGeneration(ctx, userID)
		if err != nil {
			return fmt.Errorf("read auth generation for OAuth authorize: %w", err)
		}
		s.continueOAuthAuthorize(c, userID, authGeneration)
		return nil
	}

	redirectURL := "/"
	if redirect := session.Get("oauth_redirect"); redirect != nil {
		if r, ok := redirect.(string); ok && r != "" && isValidInternalRedirect(r) {
			redirectURL = r
		}
		session.Delete("oauth_redirect")
		_ = session.Save()
	}

	if bearerToken, err := s.core.CreateAuthTokenWithSource(ctx, userID, source); err == nil {
		separator := "?"
		if strings.Contains(redirectURL, "?") {
			separator = "&"
		}
		redirectURL = redirectURL + separator + "token=" + bearerToken
	} else {
		log.Warn("Failed to create auth token on provider login", "provider_id", providerConfig.ID, "userId", userID, "error", err)
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
	return nil
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
