package http_server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/config"
)

// atprotoHandler bundles the indigo OAuth client and the helpers used by the
// HTTP routes. One instance per HTTPServer.
type atprotoHandler struct {
	s           *HTTPServer
	provider    config.AuthProviderConfig
	app         *oauth.ClientApp
	clientURI   string // public origin used in client metadata
	callbackURL string
	loopback    bool
	scopes      []string
}

func (s *HTTPServer) setupATProtoRoutes() {
	provider, ok := s.config.Auth.ATProtoProvider()
	if !ok {
		return
	}

	h, err := newATProtoHandler(s, provider)
	if err != nil {
		log.Error("Failed to initialize ATProto OAuth client", "error", err)
		return
	}

	auth := s.router.Group("/auth")
	auth.Use(func(c *gin.Context) {
		s.requestContextWithAuditMetadata(c)
		c.Next()
	})
	auth.GET("atproto/client-metadata.json", h.handleClientMetadata)
	auth.GET("atproto/jwks.json", h.handleJWKS)
	auth.GET("atproto", h.handleStartFlow)
	auth.GET("atproto/callback", h.handleCallback)
}

func newATProtoHandler(s *HTTPServer, provider config.AuthProviderConfig) (*atprotoHandler, error) {
	publicURL := strings.TrimRight(s.config.Webserver.URL, "/")
	if publicURL == "" {
		return nil, errors.New("webserver.url must be set to enable AT Protocol sign-in")
	}

	// account:email is requested by default so the shared external-identity
	// account creation path can seed the verified-email list and trigger
	// owners.emails owner auto-promotion. Users can decline this scope on the
	// PDS consent screen and still complete sign-in.
	scopes := provider.Scopes
	if len(scopes) == 0 {
		scopes = []string{"atproto"}
		if provider.RequestEmailOrDefault() {
			scopes = append(scopes, "account:email")
		}
	}

	var cfg oauth.ClientConfig
	var callbackURL string
	loopback := false
	if isLocalhostURL(publicURL) {
		// Loopback dev mode: ATProto OAuth allows a special client_id form
		// (http://localhost?redirect_uri=...&scope=...) so we don't need to
		// host a client-metadata document at a public URL. The spec requires
		// the redirect URI host to be 127.0.0.1 (or [::1]) — not "localhost"
		// — so normalize before constructing the callback URL.
		publicURL = normalizeLoopbackHost(publicURL)
		callbackURL = publicURL + "/auth/atproto/callback"
		cfg = oauth.NewLocalhostConfig(callbackURL, scopes)
		loopback = true
	} else {
		callbackURL = publicURL + "/auth/atproto/callback"
		cfg = oauth.NewPublicConfig(
			publicURL+"/auth/atproto/client-metadata.json",
			callbackURL,
			scopes,
		)
	}
	cfg.UserAgent = "chatto"

	// State (in-flight auth requests + post-callback sessions) lives in the
	// MEMORY_CACHE NATS KV bucket via atprotoOAuthStore. This works in
	// multi-replica deployments without putting ATProto tokens in backups.
	app := oauth.NewClientApp(&cfg, newATProtoOAuthStore(s.core))

	return &atprotoHandler{
		s:           s,
		provider:    provider,
		app:         app,
		clientURI:   publicURL,
		callbackURL: callbackURL,
		loopback:    loopback,
		scopes:      scopes,
	}, nil
}

func isLocalhostURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// normalizeLoopbackHost rewrites a `localhost`-hostname URL to use literal
// 127.0.0.1, because the ATProto OAuth loopback client spec requires the
// redirect URI host to be 127.0.0.1 or [::1] (not "localhost", due to DNS
// resolution ambiguity). If the URL isn't localhost-ish, it's returned as-is.
func normalizeLoopbackHost(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	if parsed.Hostname() != "localhost" {
		return u
	}
	if port := parsed.Port(); port != "" {
		parsed.Host = "127.0.0.1:" + port
	} else {
		parsed.Host = "127.0.0.1"
	}
	return parsed.String()
}

// handleClientMetadata serves the client-metadata.json document at the URL
// used as our client_id. Required for non-localhost deployments per the
// ATProto OAuth spec.
func (h *atprotoHandler) handleClientMetadata(c *gin.Context) {
	meta := h.app.Config.ClientMetadata()
	meta.ClientName = strPtr("Chatto")
	meta.ClientURI = strPtr(h.clientURI)

	if err := meta.Validate(h.app.Config.ClientID); err != nil {
		log.Error("ATProto client metadata validation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, meta)
}

// handleJWKS serves the JSON Web Key Set referenced by client metadata for
// confidential clients. Phase 1 is a public client, so the set is empty —
// the endpoint exists so confidential mode can be enabled later without
// adding routes.
func (h *atprotoHandler) handleJWKS(c *gin.Context) {
	c.JSON(http.StatusOK, h.app.Config.PublicJWKS())
}

// handleStartFlow begins the OAuth flow. Expects ?handle=... (or
// ?identifier=...); resolves the handle to a DID, sends PAR to the user's PDS,
// and redirects to the authorization endpoint.
func (h *atprotoHandler) handleStartFlow(c *gin.Context) {
	ctx := c.Request.Context()
	session := sessions.Default(c)

	identifier := strings.TrimSpace(c.Query("handle"))
	if identifier == "" {
		identifier = strings.TrimSpace(c.Query("identifier"))
	}
	if identifier == "" {
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_no_handle")
		return
	}
	// Strip a leading @ for ergonomic input like "@alice.bsky.social".
	identifier = strings.TrimPrefix(identifier, "@")

	intent := c.Query("intent")
	linkStartRedirect := ""
	if intent == "link" {
		start, err := h.s.core.ConsumePendingExternalIdentityLinkStart(ctx, c.Query("link_start"))
		if err != nil || start.ProviderID != h.provider.ID {
			if err != nil {
				log.Warn("ATProto link start token failed", "error", err)
			} else {
				log.Warn("ATProto link start token provider mismatch", "provider_id", start.ProviderID)
			}
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
			return
		}
		session.Set(providerSessionKey(h.provider.ID, "intent"), "link")
		session.Set(providerSessionKey(h.provider.ID, "link_user_id"), start.BoundUserID)
		if isValidInternalRedirect(start.RedirectPath) {
			linkStartRedirect = start.RedirectPath
		}
	} else {
		session.Set(providerSessionKey(h.provider.ID, "intent"), "login")
		session.Delete(providerSessionKey(h.provider.ID, "link_user_id"))
	}

	if linkStartRedirect != "" {
		session.Set("oauth_redirect", linkStartRedirect)
	} else if redirect := c.Query("redirect"); redirect != "" && isValidInternalRedirect(redirect) {
		session.Set("oauth_redirect", redirect)
	}
	if err := session.Save(); err != nil {
		log.Error("Failed to save ATProto auth session", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=provider_failed")
		return
	}

	scopes, err := h.scopesForStart(ctx, identifier, intent)
	if err != nil {
		log.Warn("ATProto scope selection failed", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_resolve")
		return
	}

	redirectURL, err := h.clientAppForScopes(scopes).StartAuthFlow(ctx, identifier)
	if err != nil {
		log.Warn("ATProto StartAuthFlow failed")
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_resolve")
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

// handleCallback completes the OAuth flow, converts the verified DID into a
// shared external identity flow, and lets the generic SSO code create/link or
// log in the Chatto account.
func (h *atprotoHandler) handleCallback(c *gin.Context) {
	ctx := c.Request.Context()
	session := sessions.Default(c)

	callbackApp := h.callbackClientApp(ctx, c.Query("state"))
	sessData, err := callbackApp.ProcessCallback(ctx, c.Request.URL.Query())
	if err != nil {
		log.Warn("ATProto callback failed", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback&error_description="+url.QueryEscape(err.Error()))
		return
	}

	did := sessData.AccountDID.String()
	providerConfig := h.providerConfig()

	intent, _ := session.Get(providerSessionKey(providerConfig.ID, "intent")).(string)
	linkUserID, _ := session.Get(providerSessionKey(providerConfig.ID, "link_user_id")).(string)
	session.Delete(providerSessionKey(providerConfig.ID, "intent"))
	session.Delete(providerSessionKey(providerConfig.ID, "link_user_id"))
	_ = session.Save()

	// Resolve handle for display. The auth flow doesn't return it, so look it
	// up via the directory. Best-effort: a missing handle isn't fatal.
	handle := ""
	if ident, err := h.app.Dir.LookupDID(ctx, sessData.AccountDID); err == nil {
		handle = ident.Handle.String()
	} else {
		log.Warn("ATProto handle lookup failed", "error", err)
	}

	verifiedEmail := h.fetchVerifiedEmail(ctx, sessData)
	if err := deleteLocalATProtoSession(ctx, callbackApp.Store, sessData); err != nil {
		log.Warn("ATProto local session cleanup failed", "error", err)
	}
	profile := h.fetchProfileHints(ctx, did, sessData.HostURL)

	identity := resolvedProviderIdentity{
		issuer:          providerConfig.ID,
		subject:         did,
		verifiedEmail:   verifiedEmail,
		avatarURL:       profile.avatarURL,
		loginHint:       loginHintFromParts(handle, did),
		displayNameHint: displayNameHintFromParts(profile.displayName, handle, did),
	}

	user, err := h.s.core.GetUserByExternalIdentity(ctx, identity.issuer, identity.subject)
	if err != nil {
		log.Error("ATProto user lookup failed", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback")
		return
	}
	if user == nil {
		log.Info("ATProto login has no linked account")
		h.s.redirectPendingExternalIdentity(c, session, providerConfig, identity, intent, linkUserID)
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

	log.Info("ATProto sign-in successful", "userId", user.Id)

	if identity.avatarURL != "" {
		existingAvatar, _ := h.s.core.GetUserAvatar(ctx, user.Id)
		if existingAvatar == nil {
			if err := h.s.core.ImportUserAvatarFromURL(ctx, user.Id, identity.avatarURL); err != nil {
				log.Warn("Failed to import ATProto avatar", "userId", user.Id, "error", err)
			}
		}
	}

	if err := h.s.completeProviderLogin(c, session, user.Id, providerConfig); err != nil {
		log.Error("Failed to complete ATProto login", "userId", user.Id, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback")
		return
	}
}

func (h *atprotoHandler) providerConfig() config.AuthProviderConfig {
	return h.provider
}

func (h *atprotoHandler) scopesForStart(ctx context.Context, identifier, intent string) ([]string, error) {
	if !slices.Contains(h.scopes, "account:email") {
		return h.scopes, nil
	}
	if intent == "link" {
		return scopesWithout(h.scopes, "account:email"), nil
	}
	if strings.HasPrefix(identifier, "https://") {
		return h.scopes, nil
	}

	atid, err := syntax.ParseAtIdentifier(identifier)
	if err != nil {
		return nil, fmt.Errorf("not a valid account identifier: %w", err)
	}
	ident, err := h.app.Dir.Lookup(ctx, atid)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve account identifier: %w", err)
	}
	did := ident.DID.String()
	user, err := h.s.core.GetUserByExternalIdentity(ctx, h.provider.ID, did)
	if err != nil {
		return nil, fmt.Errorf("lookup ATProto external identity: %w", err)
	}
	if user != nil {
		return scopesWithout(h.scopes, "account:email"), nil
	}
	return h.scopes, nil
}

func (h *atprotoHandler) callbackClientApp(ctx context.Context, state string) *oauth.ClientApp {
	if state == "" {
		return h.app
	}
	info, err := h.app.Store.GetAuthRequestInfo(ctx, state)
	if err != nil || len(info.Scopes) == 0 {
		return h.app
	}
	return h.clientAppForScopes(info.Scopes)
}

func (h *atprotoHandler) clientAppForScopes(scopes []string) *oauth.ClientApp {
	if slices.Equal(scopes, h.app.Config.Scopes) {
		return h.app
	}

	var cfg oauth.ClientConfig
	if h.loopback {
		cfg = oauth.NewLocalhostConfig(h.callbackURL, scopes)
	} else {
		cfg = oauth.NewPublicConfig(h.app.Config.ClientID, h.callbackURL, scopes)
	}
	cfg.UserAgent = h.app.Config.UserAgent
	cfg.PrivateKey = h.app.Config.PrivateKey
	cfg.KeyID = h.app.Config.KeyID
	return oauth.NewClientApp(&cfg, h.app.Store)
}

func scopesWithout(scopes []string, remove string) []string {
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope != remove {
			out = append(out, scope)
		}
	}
	return out
}

func deleteLocalATProtoSession(ctx context.Context, store oauth.ClientAuthStore, sessData *oauth.ClientSessionData) error {
	return store.DeleteSession(ctx, sessData.AccountDID, sessData.SessionID)
}

// fetchVerifiedEmail asks the user's PDS for their account email. If it is
// confirmed, the shared external-identity create flow will attach it as a
// verified email; otherwise account creation proceeds without email.
func (h *atprotoHandler) fetchVerifiedEmail(ctx context.Context, sessData *oauth.ClientSessionData) string {
	if !slices.Contains(sessData.Scopes, "account:email") {
		return ""
	}

	sess, err := h.app.ResumeSession(ctx, sessData.AccountDID, sessData.SessionID)
	if err != nil {
		log.Warn("ATProto email fetch: ResumeSession failed", "error", err)
		return ""
	}

	var resp struct {
		Email          string `json:"email"`
		EmailConfirmed bool   `json:"emailConfirmed"`
	}
	if err := sess.APIClient().Get(ctx, "com.atproto.server.getSession", nil, &resp); err != nil {
		log.Warn("ATProto email fetch: getSession failed", "error", err)
		return ""
	}

	if !resp.EmailConfirmed {
		return ""
	}
	return normalizeProviderEmail(resp.Email)
}

type atprotoProfileHints struct {
	displayName string
	avatarURL   string
}

// fetchProfileHints fetches the user's app.bsky.actor.profile record from
// their PDS for account-creation hints. The record is publicly readable, so
// this does not require retaining ATProto OAuth tokens after identification.
func (h *atprotoHandler) fetchProfileHints(ctx context.Context, did, pdsURL string) atprotoProfileHints {
	if pdsURL == "" {
		return atprotoProfileHints{}
	}

	getRecordURL := fmt.Sprintf("%s/xrpc/com.atproto.repo.getRecord?repo=%s&collection=app.bsky.actor.profile&rkey=self",
		strings.TrimRight(pdsURL, "/"),
		url.QueryEscape(did),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getRecordURL, nil)
	if err != nil {
		log.Warn("ATProto profile fetch: build request failed", "error", err)
		return atprotoProfileHints{}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warn("ATProto profile fetch failed", "error", err)
		return atprotoProfileHints{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 400 here typically just means the record doesn't exist (user never
		// set up a Bluesky profile). That's fine — fall back to handle-only.
		return atprotoProfileHints{}
	}

	var record struct {
		Value struct {
			DisplayName string `json:"displayName"`
			Avatar      *struct {
				Ref struct {
					Link string `json:"$link"`
				} `json:"ref"`
				MimeType string `json:"mimeType"`
			} `json:"avatar"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		log.Warn("ATProto profile decode failed", "error", err)
		return atprotoProfileHints{}
	}

	hints := atprotoProfileHints{displayName: strings.TrimSpace(record.Value.DisplayName)}
	if record.Value.Avatar != nil && record.Value.Avatar.Ref.Link != "" {
		hints.avatarURL = atprotoBlobURL(did, pdsURL, record.Value.Avatar.Ref.Link)
	}
	return hints
}

func atprotoBlobURL(did, pdsURL, cid string) string {
	return fmt.Sprintf("%s/xrpc/com.atproto.sync.getBlob?did=%s&cid=%s",
		strings.TrimRight(pdsURL, "/"),
		url.QueryEscape(did),
		url.QueryEscape(cid),
	)
}

// strPtr is a one-line helper used to populate optional string pointer fields
// in the ATProto client metadata struct.
func strPtr(s string) *string { return &s }
