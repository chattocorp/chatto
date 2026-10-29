package http_server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/discord"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"
	"github.com/markbates/goth/providers/google"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
	"golang.org/x/sync/singleflight"
)

// providerHTTPTimeout bounds each outbound request to an external login
// provider, including the response body. Without it, a provider that accepts a
// connection and never answers would hold sign-in requests indefinitely.
const providerHTTPTimeout = 10 * time.Second

// newProviderHTTPClient returns the client for all outbound requests of one
// provider. It has no fixed transport, so it uses http.DefaultTransport.
func newProviderHTTPClient() *http.Client {
	return &http.Client{Timeout: providerHTTPTimeout}
}

// oidcProvider discovers its issuer on the first sign-in and tries again after
// a failed attempt.
type oidcProvider struct {
	// httpClient makes discovery, token, UserInfo, and key-set requests.
	httpClient *http.Client
	// initFlight shares one discovery attempt among concurrent sign-ins.
	initFlight singleflight.Group
	// discovery is nil until discovery succeeds. It does not change after that.
	discovery atomic.Pointer[oidcDiscovery]
}

// oidcDiscovery is the result of one successful discovery. It is not changed.
type oidcDiscovery struct {
	provider     *oidc.Provider
	oauth2Config oauth2.Config
	verifier     *oidc.IDTokenVerifier
	authMethod   string
}

// init discovers the issuer once. Concurrent callers share one attempt, so an
// unavailable issuer delays each sign-in by at most one request timeout.
func (o *oidcProvider) init(issuerURL, clientID, clientSecret, redirectURL string, scopes []string, authMethod string) (*oidcDiscovery, error) {
	if discovery := o.discovery.Load(); discovery != nil {
		return discovery, nil
	}
	discovery, err, _ := o.initFlight.Do("", func() (any, error) {
		// An earlier attempt can finish between the check above and this one.
		if discovery := o.discovery.Load(); discovery != nil {
			return discovery, nil
		}
		return o.discover(issuerURL, clientID, clientSecret, redirectURL, scopes, authMethod)
	})
	if err != nil {
		return nil, err
	}
	return discovery.(*oidcDiscovery), nil
}

func (o *oidcProvider) discover(issuerURL, clientID, clientSecret, redirectURL string, scopes []string, authMethod string) (*oidcDiscovery, error) {
	// The provider and key set keep this context for later requests, so it must
	// not be a browser request context. The client timeout bounds each request.
	ctx := oidc.ClientContext(context.Background(), o.httpClient)
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, err
	}
	var metadata struct {
		Methods json.RawMessage `json:"token_endpoint_auth_methods_supported"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, err
	}
	method, style, err := selectOIDCTokenAuth(authMethod, clientSecret, metadata.Methods)
	if err != nil {
		return nil, err
	}

	oauth2Config := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       append([]string(nil), scopes...),
	}
	// Set a concrete style before exchange. Auto-detection can consume a code
	// with the wrong method and then submit the same code a second time.
	oauth2Config.Endpoint.AuthStyle = style
	// Keep the library's signature verification and rotating-key cache while
	// ignoring unsupported JWK types/curves in mixed provider key sets. The
	// timeout matters: go-oidc shares one key-set request among verifications,
	// so a request that never ends would block every later verification.
	keyClient := &http.Client{Timeout: o.httpClient.Timeout, Transport: oidcKeyTransport{base: http.DefaultTransport}}
	discovery := &oidcDiscovery{
		provider:     provider,
		oauth2Config: oauth2Config,
		verifier:     provider.VerifierContext(oidc.ClientContext(ctx, keyClient), &oidc.Config{ClientID: clientID}),
		authMethod:   method,
	}
	o.discovery.Store(discovery)

	log.Info("OIDC provider initialized", "issuer", issuerURL)
	return discovery, nil
}

// selectOIDCTokenAuth gives explicit configuration precedence over discovery.
// Missing metadata means Basic; an explicit empty list is not a default.
func selectOIDCTokenAuth(override, secret string, advertised json.RawMessage) (string, oauth2.AuthStyle, error) {
	method := override
	if method == "" {
		if secret == "" {
			method = "none"
		} else if len(advertised) == 0 {
			method = "client_secret_basic"
		} else {
			var methods []string
			if err := json.Unmarshal(advertised, &methods); err != nil {
				return "", 0, errors.New("invalid token authentication metadata")
			}
			if slices.Contains(methods, "client_secret_basic") {
				method = "client_secret_basic"
			} else if slices.Contains(methods, "client_secret_post") {
				method = "client_secret_post"
			}
		}
	}
	switch method {
	case "none":
		if secret == "" {
			return method, oauth2.AuthStyleInParams, nil
		}
	case "client_secret_basic", "client_secret_post":
		if secret != "" {
			if method == "client_secret_basic" {
				return method, oauth2.AuthStyleInHeader, nil
			}
			return method, oauth2.AuthStyleInParams, nil
		}
	}
	return "", 0, errors.New("no supported token authentication method for client credentials")
}

// safeOAuthError prevents provider-controlled strings from entering logs.
func safeOAuthError(code string) string {
	switch code {
	case "invalid_request", "invalid_client", "invalid_grant", "unauthorized_client", "unsupported_grant_type", "invalid_scope", "access_denied", "server_error", "temporarily_unavailable", "login_required", "consent_required", "interaction_required":
		return code
	default:
		return "unknown"
	}
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
		invitationID, _ := s.registrationInvitation(c, c.Query("admission"))
		if invitationID != "" {
			session.Set(providerSessionKey(providerRuntime.config.ID, "invitation_id"), invitationID)
		} else {
			session.Delete(providerSessionKey(providerRuntime.config.ID, "invitation_id"))
		}
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
		nonce, err := randomString(32)
		if err != nil {
			log.Error("Failed to generate OIDC nonce", "provider_id", providerRuntime.config.ID, "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
			return
		}
		session.Set(providerSessionKey(providerRuntime.config.ID, "code_verifier"), codeVerifier)
		session.Set(providerSessionKey(providerRuntime.config.ID, "nonce"), nonce)
		codeChallenge := s256Challenge(codeVerifier)
		authURL = providerRuntime.oidc.discovery.Load().oauth2Config.AuthCodeURL(state,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
			oidc.Nonce(nonce),
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
		deleteProviderExchangeState(session, providerRuntime.config.ID)
		session.Delete(providerSessionKey(providerRuntime.config.ID, "invitation_id"))
		_ = session.Save()
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}

	session.Delete(providerSessionKey(providerRuntime.config.ID, "state"))
	intent, _ := session.Get(providerSessionKey(providerRuntime.config.ID, "intent")).(string)
	linkUserID, _ := session.Get(providerSessionKey(providerRuntime.config.ID, "link_user_id")).(string)
	invitationID, _ := session.Get(providerSessionKey(providerRuntime.config.ID, "invitation_id")).(string)
	session.Delete(providerSessionKey(providerRuntime.config.ID, "intent"))
	session.Delete(providerSessionKey(providerRuntime.config.ID, "link_user_id"))
	session.Delete(providerSessionKey(providerRuntime.config.ID, "invitation_id"))

	// Check for error from provider
	if errCode := c.Query("error"); errCode != "" {
		log.Warn("Provider returned auth error", "provider_id", providerRuntime.config.ID, "error", safeOAuthError(errCode))
		deleteProviderExchangeState(session, providerRuntime.config.ID)
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
		log.Error("Provider callback failed", "provider_id", providerRuntime.config.ID)
		deleteProviderExchangeState(session, providerRuntime.config.ID)
		_ = session.Save()
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
	deleteProviderExchangeState(session, providerRuntime.config.ID)
	_ = session.Save()

	// The group restriction applies to every sign-in and link, also for linked
	// accounts. The error does not name the restriction.
	if !providerRuntime.config.AdmitsGroups(identity.groups.Groups) {
		log.Warn("Provider sign-in denied by group restriction", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type)
		if intent == "link" {
			c.Redirect(http.StatusTemporaryRedirect, providerReturnPathWithError(session, "/", "provider_access_denied"))
			return
		}
		// A user who may not sign in through the provider must not keep the
		// roles that it gave. The sign-in stays denied, also when this fails.
		user, err := s.core.GetUserByExternalIdentity(ctx, identity.issuer, identity.subject)
		if err != nil {
			log.Error("Failed to lookup user for provider group role revocation", "provider_id", providerRuntime.config.ID, "error", err)
		} else if user != nil {
			if err := s.core.RevokeProviderGroupRoles(ctx, user.Id, core.ProviderRoleSource{ProviderID: providerRuntime.config.ID, Issuer: identity.issuer}); err != nil {
				log.Error("Provider group role revocation failed after denied sign-in", "provider_id", providerRuntime.config.ID, "userId", user.Id, "error", err)
			}
		}
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_access_denied")
		return
	}

	// The authenticated link-start handoff already records consent and binds the
	// target account. Complete it only after the provider identity is verified;
	// linking must never replace the browser's current login session.
	if intent == "link" {
		if linkUserID == "" {
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=authentication_required")
			return
		}
		if err := s.core.LinkExternalIdentity(ctx, providerRuntime.config.ID, providerRuntime.config.Type, identity.issuer, identity.subject, linkUserID); err != nil {
			errorCode := "provider_failed"
			if errors.Is(err, core.ErrExternalIdentityAlreadyClaimed) {
				errorCode = "external_identity_conflict"
			}
			c.Redirect(http.StatusTemporaryRedirect, providerReturnPathWithError(session, "/", errorCode))
			return
		}
		// The link is committed. A failed sync is repeated at the next sign-in.
		if err := s.core.SyncProviderGroupRoles(ctx, linkUserID, core.ProviderRoleSource{ProviderID: providerRuntime.config.ID, Issuer: identity.issuer, Subject: identity.subject}, providerRuntime.config.GroupRoleMappings(), identity.groups); err != nil {
			log.Warn("Provider group role sync failed after link", "provider_id", providerRuntime.config.ID, "userId", linkUserID, "error", err)
		}
		c.Redirect(http.StatusTemporaryRedirect, providerReturnPath(session, "/"))
		return
	}

	user, authGeneration, err := s.core.GetUserByExternalIdentityForAuthentication(ctx, identity.issuer, identity.subject)
	if err != nil {
		log.Error("Failed to lookup user by external identity", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
	if user == nil {
		log.Info("Provider login has no linked account", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type)
		s.redirectPendingExternalIdentity(c, session, providerRuntime.config, identity, invitationID)
		return
	}

	log.Info("Provider login matched by external identity", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "userId", user.Id)

	// Enrich upgraded hash-only links even when group-role sync is disabled.
	if providerRuntime.config.Type == config.AuthProviderTypeOpenIDConnect {
		if err := s.core.CompleteLegacyOIDCIdentityLink(ctx, user.Id, providerRuntime.config.ID, identity.issuer, identity.subject); err != nil {
			log.Error("Legacy OIDC identity completion failed", "provider_id", providerRuntime.config.ID, "userId", user.Id, "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
			return
		}
	}

	// Sync before the session exists, so a failed revocation cannot leave a
	// signed-in user with a role that the provider removed.
	if err := s.core.SyncProviderGroupRoles(ctx, user.Id, core.ProviderRoleSource{ProviderID: providerRuntime.config.ID, Issuer: identity.issuer, Subject: identity.subject}, providerRuntime.config.GroupRoleMappings(), identity.groups); err != nil {
		log.Error("Provider group role sync failed", "provider_id", providerRuntime.config.ID, "userId", user.Id, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}

	if identity.avatarURL != "" {
		existingAvatar, _ := s.core.GetUserAvatar(ctx, user.Id)
		if existingAvatar == nil {
			if err := s.core.ImportUserAvatarFromURL(ctx, user.Id, identity.avatarURL); err != nil {
				log.Warn("Failed to import provider avatar", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "userId", user.Id, "error", err)
			}
		}
	}

	if err := s.completeProviderLogin(c, session, user.Id, authGeneration, providerRuntime.config); err != nil {
		log.Error("Failed to complete provider login", "provider_id", providerRuntime.config.ID, "provider_type", providerRuntime.config.Type, "userId", user.Id, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
}

type authProviderRuntime struct {
	config      config.AuthProviderConfig
	callbackURL string
	// httpClient makes every outbound request to this provider.
	httpClient *http.Client
	oidc       *oidcProvider
	goth       goth.Provider
	// discordOAuth2 exchanges Discord codes. Goth's Discord session ignores
	// the provider client and would use http.DefaultClient without a timeout.
	discordOAuth2 *oauth2.Config
}

type resolvedProviderIdentity struct {
	issuer          string
	subject         string
	verifiedEmail   string
	avatarURL       string
	loginHint       string
	displayNameHint string
	// groups holds the OIDC groups claim when allowed_groups or group_roles
	// needs it.
	groups core.ProviderGroups
}

func newAuthProviderRuntime(providerConfig config.AuthProviderConfig, callbackURL string) (*authProviderRuntime, error) {
	httpClient := newProviderHTTPClient()
	runtime := &authProviderRuntime{config: providerConfig, callbackURL: callbackURL, httpClient: httpClient}
	scopes := providerScopes(providerConfig)
	switch providerConfig.Type {
	case config.AuthProviderTypeOpenIDConnect:
		runtime.oidc = &oidcProvider{httpClient: httpClient}
	case config.AuthProviderTypeGitHub:
		provider := github.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
		provider.HTTPClient = httpClient
		runtime.goth = provider
	case config.AuthProviderTypeGitLab:
		provider := gitlab.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
		provider.HTTPClient = httpClient
		runtime.goth = provider
	case config.AuthProviderTypeGoogle:
		provider := google.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
		provider.HTTPClient = httpClient
		runtime.goth = provider
	case config.AuthProviderTypeDiscord:
		provider := discord.New(providerConfig.ClientID, providerConfig.ClientSecret, callbackURL, scopes...)
		provider.HTTPClient = httpClient
		runtime.goth = provider
		runtime.discordOAuth2 = &oauth2.Config{
			ClientID:     providerConfig.ClientID,
			ClientSecret: providerConfig.ClientSecret,
			RedirectURL:  callbackURL,
			Endpoint:     endpoints.Discord,
		}
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
		if providerConfig.Type == config.AuthProviderTypeOpenIDConnect && !slices.Contains(scopes, oidc.ScopeOpenID) {
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

func (s *HTTPServer) providerCallbackURL(providerID string) string {
	return configuredWebserverOrigin(s.config.Webserver.URL) + "/auth/providers/" + url.PathEscape(providerID) + "/callback"
}

func (s *HTTPServer) legacyOIDCCallbackURL() string {
	return configuredWebserverOrigin(s.config.Webserver.URL) + "/auth/oidc/callback"
}

func providerSessionKey(providerID, name string) string {
	return "provider_" + providerID + "_" + name
}

// deleteProviderExchangeState removes the single-use values that bind one
// provider authorization to its callback. The caller saves the session.
func deleteProviderExchangeState(session sessions.Session, providerID string) {
	session.Delete(providerSessionKey(providerID, "code_verifier"))
	session.Delete(providerSessionKey(providerID, "nonce"))
	session.Delete(providerSessionKey(providerID, "session"))
}

func (r *authProviderRuntime) ensureOIDC(c *gin.Context) bool {
	if r.oidc == nil {
		return true
	}
	if _, err := r.oidc.init(r.config.IssuerURL, r.config.ClientID, r.config.ClientSecret, r.callbackURL, providerScopes(r.config), r.config.TokenEndpointAuthMethod); err != nil {
		log.Error("OIDC provider not available", "provider_id", r.config.ID)
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
	// The callback calls ensureOIDC first, so discovery has succeeded.
	discovery := r.oidc.discovery.Load()
	ctx := oidc.ClientContext(c.Request.Context(), r.httpClient)
	codeVerifier, _ := session.Get(providerSessionKey(r.config.ID, "code_verifier")).(string)
	if codeVerifier == "" {
		return resolvedProviderIdentity{}, fmt.Errorf("missing code verifier")
	}
	// A sign-in started before the nonce was introduced fails once; the user
	// starts again.
	nonce, _ := session.Get(providerSessionKey(r.config.ID, "nonce")).(string)
	if nonce == "" {
		return resolvedProviderIdentity{}, fmt.Errorf("missing nonce")
	}

	// Exchange authorization code for tokens
	token, err := discovery.oauth2Config.Exchange(ctx, c.Query("code"),
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		status, code := 0, "unknown"
		if responseError, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
			if responseError.Response != nil {
				status = responseError.Response.StatusCode
			}
			code = safeOAuthError(responseError.ErrorCode)
		}
		log.Warn("OIDC token exchange failed", "provider_id", r.config.ID, "auth_method", discovery.authMethod, "http_status", status, "oauth_error", code)
		return resolvedProviderIdentity{}, errors.New("token exchange failed")
	}

	// Extract and verify the ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return resolvedProviderIdentity{}, fmt.Errorf("token response missing id_token")
	}

	idToken, err := discovery.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("id token verification failed: %w", err)
	}
	// go-oidc does not check the nonce. It binds the ID token to this browser's
	// authorization request, so an ID token issued for another request fails.
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nonce)) != 1 {
		return resolvedProviderIdentity{}, errors.New("id token nonce mismatch")
	}

	// Extract claims from the ID token first
	type profileClaims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		PreferredUser string `json:"preferred_username"`
		Picture       string `json:"picture"`
	}
	var claims profileClaims
	if err := idToken.Claims(&claims); err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("parse id token claims: %w", err)
	}

	log.Info("OIDC token verified", "provider_id", r.config.ID, "issuer", idToken.Issuer)

	// Chatto reads groups only for allowed_groups or group_roles. A groups
	// claim in the ID token wins over UserInfo, like the profile claims.
	needGroups := len(r.config.AllowedGroups) > 0 || len(r.config.GroupRoleMappings()) > 0
	var groups core.ProviderGroups
	if needGroups {
		groups.Groups, groups.Known = groupsFromClaims(idToken.Claims, r.config.GroupsClaimOrDefault())
	}

	// UserInfo can supply names even when the ID token already has email.
	if claims.PreferredUser == "" || claims.Name == "" || (claims.Email == "" && slices.Contains(discovery.oauth2Config.Scopes, "email")) || (needGroups && !groups.Known) {
		userInfo, err := discovery.provider.UserInfo(ctx, oauth2.StaticTokenSource(token))
		if err != nil {
			log.Warn("OIDC userinfo fallback failed", "provider_id", r.config.ID)
		} else {
			if userInfo.Subject != idToken.Subject {
				return resolvedProviderIdentity{}, errors.New("userinfo subject mismatch")
			}
			if needGroups && !groups.Known {
				groups.Groups, groups.Known = groupsFromClaims(userInfo.Claims, r.config.GroupsClaimOrDefault())
			}
			var supplemental profileClaims
			if err := userInfo.Claims(&supplemental); err != nil {
				log.Warn("OIDC userinfo claims ignored", "provider_id", r.config.ID)
			} else {
				if claims.PreferredUser == "" {
					claims.PreferredUser = supplemental.PreferredUser
				}
				if claims.Name == "" {
					claims.Name = supplemental.Name
				}
				if claims.Picture == "" {
					claims.Picture = supplemental.Picture
				}
				// Never attach verification from one response to another email.
				if claims.Email == "" {
					claims.Email, claims.EmailVerified = supplemental.Email, supplemental.EmailVerified
				}
			}
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
		groups:          groups,
	}, nil
}

// mappedGroups returns the groups that group_roles names. Pending account
// creation stores only these, so it keeps no other group data of the user.
func mappedGroups(mappings []config.GroupRole, groups []string) []string {
	var mapped []string
	for _, group := range groups {
		if slices.ContainsFunc(mappings, func(mapping config.GroupRole) bool { return mapping.Group == group }) && !slices.Contains(mapped, group) {
			mapped = append(mapped, group)
		}
	}
	return mapped
}

// groupsFromClaims reads a groups claim that holds a string or a list of
// strings. It ignores list items that are not strings. Another shape, or a
// missing claim, returns false.
func groupsFromClaims(decode func(any) error, name string) ([]string, bool) {
	var claims map[string]any
	if err := decode(&claims); err != nil {
		return nil, false
	}
	switch value := claims[name].(type) {
	case string:
		return []string{value}, true
	case []any:
		groups := make([]string, 0, len(value))
		for _, item := range value {
			if group, ok := item.(string); ok {
				groups = append(groups, group)
			}
		}
		return groups, true
	default:
		return nil, false
	}
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
	if err := r.authorizeGothSession(c.Request.Context(), gothSession, c.Request.URL.Query()); err != nil {
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

// authorizeGothSession exchanges the provider code with the bounded client.
func (r *authProviderRuntime) authorizeGothSession(ctx context.Context, session goth.Session, params url.Values) error {
	discordSession, ok := session.(*discord.Session)
	if !ok {
		_, err := session.Authorize(r.goth, params)
		return err
	}
	token, err := r.discordOAuth2.Exchange(context.WithValue(ctx, oauth2.HTTPClient, r.httpClient), params.Get("code"))
	if err != nil {
		return err
	}
	if !token.Valid() {
		return errors.New("invalid token received from provider")
	}
	discordSession.AccessToken, discordSession.RefreshToken, discordSession.ExpiresAt = token.AccessToken, token.RefreshToken, token.Expiry
	return nil
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
		email, err := fetchGitHubVerifiedPrimaryEmail(ctx, r.httpClient, gothUser.AccessToken)
		if err == nil {
			return email
		}
	}
	return ""
}

func rawBool(raw map[string]any, key string) bool {
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

func fetchGitHubVerifiedPrimaryEmail(ctx context.Context, client *http.Client, accessToken string) (string, error) {
	if strings.TrimSpace(accessToken) == "" {
		return "", fmt.Errorf("missing github access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, github.EmailURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
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

func (s *HTTPServer) redirectPendingExternalIdentity(c *gin.Context, session sessions.Session, providerConfig config.AuthProviderConfig, identity resolvedProviderIdentity, invitationID string) {
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
		Groups:          mappedGroups(providerConfig.GroupRoleMappings(), identity.groups.Groups),
		GroupsKnown:     identity.groups.Known,
		RedirectPath:    providerReturnPath(session, "/"),
	}

	if !providerConfig.AutoProvisionOrDefault() {
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=external_identity_unlinked")
		return
	}
	flow.InvitationID = invitationID
	token, err := s.core.CreatePendingExternalIdentityCreateFlow(ctx, flow)
	if err != nil {
		log.Error("Failed to create pending external identity flow", "provider_id", providerConfig.ID, "provider_type", providerConfig.Type, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}
	if s.core.InvitationRequired() && invitationID == "" {
		c.Redirect(http.StatusTemporaryRedirect, "/register?external_identity="+url.QueryEscape(token))
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

func (s *HTTPServer) completeProviderLogin(c *gin.Context, session sessions.Session, userID string, authGeneration uint64, providerConfig config.AuthProviderConfig) error {
	ctx := c.Request.Context()
	source := providerConfig.Type + "_login"
	if err := s.createCookieSessionForGeneration(c, userID, source, authGeneration); err != nil {
		return fmt.Errorf("save cookie session: %w", err)
	}
	if err := s.ensureCSRFToken(c); err != nil {
		session = sessions.Default(c)
		cookieCredentialID, _ := s.browserSessionID(c)
		_ = s.core.RevokeCookieSession(ctx, cookieCredentialID)
		s.clearBrowserSessionCookie(c)
		clearCSRFCookie(c)
		return fmt.Errorf("create csrf token: %w", err)
	}
	if err := s.core.RecordLoginSucceeded(ctx, userID, providerConfig.Type+":"+providerConfig.ID); err != nil {
		session = sessions.Default(c)
		cookieCredentialID, _ := s.browserSessionID(c)
		_ = s.core.RevokeCookieSession(ctx, cookieCredentialID)
		s.clearBrowserSessionCookie(c)
		clearCSRFCookie(c)
		return fmt.Errorf("append login audit event: %w", err)
	}

	if hasPendingOAuthAuthorize(session) {
		// A provider-hinted authorize flow stores this return path for the
		// unlinked-account confirmation path. A matched account continues the
		// pending authorize request directly, so do not leave a stale redirect
		// in the browser session.
		session.Delete("oauth_redirect")
		_ = session.Save()
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
