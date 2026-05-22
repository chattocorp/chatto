package http_server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/charmbracelet/log"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"hmans.de/chatto/internal/core"
)

// atprotoSessionKey holds the redirect URL across the auth flow, mirroring
// the OIDC handler's use of the gin session. The OAuth state itself lives
// in the indigo ClientApp's in-memory auth-request store.
const atprotoSessionRedirectKey = "atproto_redirect"

// atprotoHandler bundles the indigo OAuth client and the helpers used by the
// HTTP routes. One instance per HTTPServer.
type atprotoHandler struct {
	s         *HTTPServer
	app       *oauth.ClientApp
	clientURI string // public origin used in client metadata
	scopes    []string
}

func (s *HTTPServer) setupATProtoRoutes() {
	if !s.config.Auth.ATProto.IsConfigured() {
		return
	}

	h, err := newATProtoHandler(s)
	if err != nil {
		log.Error("Failed to initialize ATProto OAuth client", "error", err)
		return
	}

	auth := s.router.Group("/auth")
	auth.GET("atproto/client-metadata.json", h.handleClientMetadata)
	auth.GET("atproto/jwks.json", h.handleJWKS)
	auth.GET("atproto", h.handleStartFlow)
	auth.GET("atproto/callback", h.handleCallback)
}

func newATProtoHandler(s *HTTPServer) (*atprotoHandler, error) {
	publicURL := strings.TrimRight(s.config.Webserver.URL, "/")
	if publicURL == "" {
		return nil, errors.New("webserver.url must be set to enable AT Protocol sign-in")
	}

	// account:email is requested so we can seed the Chatto account's
	// verified-email list (and trigger owners.emails owner auto-promotion)
	// on first sign-in. Users can decline just this scope on the PDS
	// consent screen and still complete sign-in — the email seed becomes
	// a no-op. See maybeSeedEmail.
	scopes := []string{"atproto", "account:email"}

	var cfg oauth.ClientConfig
	if isLocalhostURL(publicURL) {
		// Loopback dev mode: ATProto OAuth allows a special client_id form
		// (http://localhost?redirect_uri=...&scope=...) so we don't need to
		// host a client-metadata document at a public URL. The spec requires
		// the redirect URI host to be 127.0.0.1 (or [::1]) — not "localhost"
		// — so normalize before constructing the callback URL.
		publicURL = normalizeLoopbackHost(publicURL)
		callbackURL := publicURL + "/auth/atproto/callback"
		cfg = oauth.NewLocalhostConfig(callbackURL, scopes)
	} else {
		callbackURL := publicURL + "/auth/atproto/callback"
		cfg = oauth.NewPublicConfig(
			publicURL+"/auth/atproto/client-metadata.json",
			callbackURL,
			scopes,
		)
	}
	cfg.UserAgent = "chatto"

	// State (in-flight auth requests + post-callback sessions) lives in the
	// AUTH_TOKENS NATS KV bucket via atprotoOAuthStore. Survives server
	// restart mid-flow and works in multi-replica deployments.
	app := oauth.NewClientApp(&cfg, newATProtoOAuthStore(s.core))

	return &atprotoHandler{
		s:         s,
		app:       app,
		clientURI: publicURL,
		scopes:    scopes,
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

// handleStartFlow begins the OAuth flow. Expects ?handle=... (or ?identifier=...);
// resolves the handle to a DID, sends PAR to the user's PDS, and redirects to
// the authorization endpoint.
func (h *atprotoHandler) handleStartFlow(c *gin.Context) {
	ctx := c.Request.Context()

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

	if redirect := c.Query("redirect"); redirect != "" && isValidInternalRedirect(redirect) {
		session := sessions.Default(c)
		session.Set(atprotoSessionRedirectKey, redirect)
		_ = session.Save()
	}

	redirectURL, err := h.app.StartAuthFlow(ctx, identifier)
	if err != nil {
		log.Warn("ATProto StartAuthFlow failed", "identifier", identifier, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_resolve&error_description="+url.QueryEscape(err.Error()))
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

// handleCallback completes the OAuth flow, looks up or creates the Chatto
// user, mirrors basic profile data on first sign-in, and issues a session.
func (h *atprotoHandler) handleCallback(c *gin.Context) {
	ctx := c.Request.Context()

	sessData, err := h.app.ProcessCallback(ctx, c.Request.URL.Query())
	if err != nil {
		log.Warn("ATProto callback failed", "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback&error_description="+url.QueryEscape(err.Error()))
		return
	}

	did := sessData.AccountDID.String()

	// Resolve handle for display. The auth flow doesn't return it, so look it
	// up via the directory. Best-effort: a missing handle isn't fatal.
	handle := ""
	if ident, err := h.app.Dir.LookupDID(ctx, sessData.AccountDID); err == nil {
		handle = ident.Handle.String()
	} else {
		log.Warn("ATProto handle lookup failed", "did", did, "error", err)
	}

	// Look up existing user by DID, or create one.
	user, err := h.s.core.GetUserByATProtoDID(ctx, did)
	if err != nil {
		log.Error("ATProto user lookup failed", "did", did, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback")
		return
	}

	isNewUser := user == nil
	if isNewUser {
		login := h.deriveLoginForHandle(ctx, handle, did)
		displayName := handle
		if displayName == "" {
			displayName = did
		}

		user, err = h.s.core.CreateUser(ctx, "system", login, displayName, "")
		if err != nil {
			log.Error("ATProto user creation failed", "did", did, "login", login, "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback")
			return
		}

		if err := h.s.core.LinkATProtoDID(ctx, did, user.Id); err != nil {
			log.Error("ATProto DID link failed", "did", did, "userId", user.Id, "error", err)
			c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback")
			return
		}

		// Best-effort profile mirroring on first sign-in. Failures here must
		// not block sign-in — the user exists; profile is just nice-to-have.
		h.mirrorProfile(ctx, user.Id, did, sessData.HostURL)

		// If the user granted account:email, seed a verified email. This
		// also triggers owners.emails owner auto-promotion via the shared
		// addVerifiedEmail hook. Best-effort: a declined scope or failed
		// fetch leaves the user without an email; they can add one later.
		h.maybeSeedEmail(ctx, user.Id, sessData)
	}

	// Always revoke the OAuth session. We don't keep ATProto credentials
	// past sign-in — we only use them once for identity verification.
	if err := h.app.Logout(ctx, sessData.AccountDID, sessData.SessionID); err != nil {
		log.Warn("ATProto session revocation failed", "did", did, "error", err)
	}

	// Issue Chatto session.
	session := sessions.Default(c)
	session.Set("user_id", user.Id)
	if err := session.Save(); err != nil {
		log.Error("Failed to save session after ATProto sign-in", "userId", user.Id, "error", err)
		c.Redirect(http.StatusTemporaryRedirect, "/login?error=atproto_callback")
		return
	}

	log.Info("ATProto sign-in successful", "userId", user.Id, "did", did, "handle", handle, "new", isNewUser)

	if hasPendingOAuthAuthorize(session) {
		h.s.completeOAuthAuthorize(c, user.Id)
		return
	}

	redirectURL := "/"
	if v := session.Get(atprotoSessionRedirectKey); v != nil {
		if s, ok := v.(string); ok && s != "" && isValidInternalRedirect(s) {
			redirectURL = s
		}
		session.Delete(atprotoSessionRedirectKey)
		_ = session.Save()
	}

	if bearerToken, err := h.s.core.CreateAuthToken(ctx, user.Id); err == nil {
		separator := "?"
		if strings.Contains(redirectURL, "?") {
			separator = "&"
		}
		redirectURL = redirectURL + separator + "token=" + bearerToken
	} else {
		log.Warn("Failed to create bearer token on ATProto sign-in", "userId", user.Id, "error", err)
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

// deriveLoginForHandle picks a login derived from the ATProto handle. The
// handle is already a unique public identifier and a valid Chatto login by
// shape; the only fixups are length and collision suffixing. Falls back to a
// DID-derived login if no handle is available.
func (h *atprotoHandler) deriveLoginForHandle(ctx context.Context, handle, did string) string {
	base := strings.ToLower(strings.TrimSpace(handle))
	if base == "" {
		// did:plc:abc123... → "abc123..." truncated. Ugly but legal and unique.
		base = "atproto-" + strings.TrimPrefix(strings.TrimPrefix(did, "did:plc:"), "did:web:")
		base = invalidCharsRegex.ReplaceAllString(base, "")
	}
	if len(base) < 2 {
		base = "user"
	}
	if len(base) > 32 {
		base = base[:32]
	}

	// On collision, try suffixes -2, -3, ..., -100.
	candidate := base
	for attempt := 2; ; attempt++ {
		if !h.s.loginInUse(ctx, candidate) {
			return candidate
		}
		if attempt > 100 {
			// Extremely unlikely; let CreateUser fail and surface the error.
			return candidate
		}
		suffix := fmt.Sprintf("-%d", attempt)
		maxBase := 32 - len(suffix)
		if maxBase < 2 {
			maxBase = 2
		}
		trimmed := base
		if len(trimmed) > maxBase {
			trimmed = trimmed[:maxBase]
		}
		candidate = trimmed + suffix
	}
}

// loginInUse is a small helper on HTTPServer (defined here for proximity to
// its sole caller) that returns true if a login is already claimed. The
// "not found" sentinel from the KV index is the not-claimed case and must
// not propagate as a generic error — otherwise the collision-suffix loop
// would treat every probe as taken and never settle on the bare handle.
func (s *HTTPServer) loginInUse(ctx context.Context, login string) bool {
	u, err := s.core.GetUserByLogin(ctx, login)
	if errors.Is(err, core.ErrNotFound) {
		return false
	}
	if err != nil {
		// On any other lookup error, assume in-use to be safe; the create
		// call will surface the real error if it really collides.
		return true
	}
	return u != nil
}

// maybeSeedEmail asks the user's PDS for their account email and, if it's
// confirmed, attaches it to the Chatto user as a verified email. Skipped
// silently if the user declined the account:email scope or anything along
// the way fails — email seeding is a nicety, not a correctness requirement.
func (h *atprotoHandler) maybeSeedEmail(ctx context.Context, userID string, sessData *oauth.ClientSessionData) {
	if !slices.Contains(sessData.Scopes, "account:email") {
		return
	}

	sess, err := h.app.ResumeSession(ctx, sessData.AccountDID, sessData.SessionID)
	if err != nil {
		log.Warn("ATProto email seed: ResumeSession failed", "error", err)
		return
	}

	var resp struct {
		Email          string `json:"email"`
		EmailConfirmed bool   `json:"emailConfirmed"`
	}
	if err := sess.APIClient().Get(ctx, "com.atproto.server.getSession", nil, &resp); err != nil {
		log.Warn("ATProto email seed: getSession failed", "error", err)
		return
	}

	email := strings.ToLower(strings.TrimSpace(resp.Email))
	if email == "" || !resp.EmailConfirmed {
		return
	}

	if err := h.s.core.AddVerifiedEmailDirect(ctx, userID, email); err != nil {
		// Most likely "email already claimed by another user" — not actionable
		// from here; leave the ATProto account without an email and let the
		// user resolve it manually if it matters.
		log.Warn("ATProto email seed: AddVerifiedEmailDirect failed", "userId", userID, "error", err)
	}
}

// mirrorProfile fetches the user's app.bsky.actor.profile record from their
// PDS and seeds the Chatto display name and avatar. All failures are logged
// and swallowed — profile mirroring is a UX nicety, not a correctness
// requirement.
//
// We talk directly to the user's PDS (no Bluesky-Inc dependency); the profile
// record is publicly readable so no auth is needed.
func (h *atprotoHandler) mirrorProfile(ctx context.Context, userID, did, pdsURL string) {
	if pdsURL == "" {
		return
	}

	getRecordURL := fmt.Sprintf("%s/xrpc/com.atproto.repo.getRecord?repo=%s&collection=app.bsky.actor.profile&rkey=self",
		strings.TrimRight(pdsURL, "/"),
		url.QueryEscape(did),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getRecordURL, nil)
	if err != nil {
		log.Warn("ATProto profile fetch: build request failed", "error", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warn("ATProto profile fetch failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 400 here typically just means the record doesn't exist (user never
		// set up a Bluesky profile). That's fine — fall back to handle-only.
		return
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
		return
	}

	if dn := strings.TrimSpace(record.Value.DisplayName); dn != "" {
		if _, err := h.s.core.UpdateUserDisplayName(ctx, userID, dn); err != nil {
			log.Warn("ATProto display name update failed", "userId", userID, "error", err)
		}
	}

	if record.Value.Avatar != nil && record.Value.Avatar.Ref.Link != "" {
		h.fetchAndStoreATProtoAvatar(ctx, userID, did, pdsURL, record.Value.Avatar.Ref.Link)
	}
}

func (h *atprotoHandler) fetchAndStoreATProtoAvatar(ctx context.Context, userID, did, pdsURL, cid string) {
	blobURL := fmt.Sprintf("%s/xrpc/com.atproto.sync.getBlob?did=%s&cid=%s",
		strings.TrimRight(pdsURL, "/"),
		url.QueryEscape(did),
		url.QueryEscape(cid),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, blobURL, nil)
	if err != nil {
		log.Warn("ATProto avatar blob request build failed", "error", err)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Warn("ATProto avatar blob fetch failed", "error", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Warn("ATProto avatar blob bad status", "status", resp.StatusCode)
		return
	}

	asset, err := h.s.core.UploadUserAvatar(ctx, userID, io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		log.Warn("ATProto avatar upload failed", "userId", userID, "error", err)
		return
	}
	if err := h.s.core.SetUserAvatar(ctx, userID, asset); err != nil {
		log.Warn("ATProto avatar set failed", "userId", userID, "error", err)
	}
}

// strPtr is a one-line helper used to populate optional string pointer fields
// in the ATProto client metadata struct.
func strPtr(s string) *string { return &s }
