package http_server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// atprotoIdentityScope is the only scope that Chatto's server requests. It
// proves the account DID and grants no access to the user's repository
// (FDR-058).
const atprotoIdentityScope = "atproto"

// atprotoHandlePagePath is the frontend page that asks for the handle when a
// sign-in starts without one. It continues the same start URL with
// `login_hint`.
const atprotoHandlePagePath = "/sso/handle"

// atprotoMaxIdentifierLength bounds a handle or DID hint. The AT Protocol
// specification limits DIDs to 2 KB.
const atprotoMaxIdentifierLength = 2048

// Bluesky shows the server hostname for this conventional HTTPS client ID.
const atprotoClientMetadataPath = "/oauth-client-metadata.json"

// Older replicas use one client ID per provider.
func legacyATProtoClientID(callbackURL string) string {
	return strings.TrimSuffix(callbackURL, "/callback") + "/client-metadata.json"
}

// atprotoProvider signs users in with AT Protocol OAuth. It keeps no tokens:
// after the callback verifies the account DID, Chatto discards the session.
type atprotoProvider struct {
	// app holds the shared client configuration and resolvers. Requests use
	// a copy with a store that is bound to the browser's flow session.
	app *oauth.ClientApp
	// signingKey opens the origin-scoped key through core for HTTPS clients.
	signingKey func(context.Context, string) (*atcrypto.PrivateKeyP256, error)
	// loopback is true for http:// deployments, such as development servers.
	// AT Protocol accepts plain HTTP only for its special localhost client,
	// which returns to 127.0.0.1. The callback route forwards such callbacks
	// to the configured origin, where the flow cookie is.
	loopback bool

	// Only the metadata route uses this cache. The public document contains
	// the verification key. Signing operations always load the private key.
	metadataMu      sync.Mutex
	metadata        oauth.ClientMetadata
	metadataExpires time.Time
}

// newATProtoProvider configures the client for a provider whose callback is
// callbackURL. The client ID is the URL of the metadata document that
// setupOIDCRoutes publishes, except for loopback deployments.
func newATProtoProvider(callbackURL string) (*atprotoProvider, error) {
	callback, err := url.Parse(callbackURL)
	if err != nil || callback.Host == "" {
		return nil, fmt.Errorf("invalid atproto callback URL")
	}
	scopes := []string{atprotoIdentityScope}
	var cfg oauth.ClientConfig
	loopback := callback.Scheme == "http"
	if loopback {
		relay := *callback
		relay.Host = net.JoinHostPort("127.0.0.1", callback.Port())
		cfg = oauth.NewLocalhostConfig(relay.String(), scopes)
	} else {
		clientID := canonicalOrigin(callback) + atprotoClientMetadataPath
		cfg = oauth.NewPublicConfig(clientID, callbackURL, scopes)
	}
	cfg.UserAgent = "Chatto"
	app := oauth.NewClientApp(&cfg, nil)
	// Keep indigo's public-only transports and bound each request like other
	// provider requests.
	app.Client.Timeout = providerHTTPTimeout
	app.Resolver.Client.Timeout = providerHTTPTimeout
	return &atprotoProvider{app: app, loopback: loopback}, nil
}

// clientConfig returns a request-local configuration. Key-store failures stop
// HTTPS sign-in and metadata publication. A later request can retry.
func (p *atprotoProvider) clientConfig(ctx context.Context) (*oauth.ClientConfig, error) {
	cfg := *p.app.Config
	if p.loopback {
		return &cfg, nil
	}
	if p.signingKey == nil {
		return nil, errors.New("AT Protocol client signing key unavailable")
	}
	key, err := p.signingKey(ctx, strings.TrimSuffix(cfg.ClientID, atprotoClientMetadataPath))
	if err != nil {
		return nil, fmt.Errorf("load AT Protocol client signing key: %w", err)
	}
	public, err := key.PublicKey()
	if err != nil {
		return nil, err
	}
	if err := cfg.SetClientSecret(key, public.Multibase()); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// authURL starts a sign-in for identifier, a handle or DID. It returns the URL
// of the authorization page and the state that the callback must return.
func (p *atprotoProvider) authURL(ctx context.Context, session sessions.Session, providerID, identifier string) (string, string, error) {
	identifier = strings.TrimPrefix(identifier, "@")
	if _, err := syntax.ParseAtIdentifier(identifier); err != nil {
		// This also rejects URLs: only Chatto chooses authorization servers.
		return "", "", fmt.Errorf("%w: %w", errATProtoIdentityNotFound, err)
	}
	store := &atprotoFlowStore{session: session, key: providerSessionKey(providerID, "atproto_request")}
	app := *p.app
	cfg, err := p.clientConfig(ctx)
	if err != nil {
		return "", "", err
	}
	app.Config = cfg
	app.Store = store
	redirect, err := app.StartAuthFlow(ctx, identifier)
	if err != nil {
		if isATProtoIdentityNotFound(err) {
			return "", "", fmt.Errorf("%w: %w", errATProtoIdentityNotFound, err)
		}
		return "", "", err
	}
	if store.state == "" {
		return "", "", errors.New("atproto request was not stored")
	}
	if !p.loopback {
		// Older flows contain only the random state. Bind new flows to the
		// public signing key ID so every replica finishes with the same key.
		session.Set(providerSessionKey(providerID, "atproto_server_client_state"), store.state+":"+*cfg.KeyID)
	}
	return redirect, store.state, nil
}

// errATProtoIdentityNotFound marks an identifier that does not name an AT
// Protocol identity. Other start errors are failures of the identity's
// systems.
var errATProtoIdentityNotFound = errors.New("atproto identity not found")

func isATProtoIdentityNotFound(err error) bool {
	for _, target := range []error{
		identity.ErrHandleNotFound,
		identity.ErrHandleMismatch,
		identity.ErrHandleNotDeclared,
		identity.ErrHandleReservedTLD,
		identity.ErrDIDNotFound,
		identity.ErrInvalidHandle,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// atprotoAccount is the verified result of an AT Protocol sign-in.
type atprotoAccount struct {
	DID syntax.DID
	// Handle is empty when it does not resolve back to the DID.
	Handle string
	// DisplayName and AvatarURL come from the public Bluesky profile record,
	// when the repository has one. They are hints for a new account only.
	DisplayName string
	AvatarURL   string
}

// atprotoProfileRecordLimit bounds the profile record that sign-in reads.
const atprotoProfileRecordLimit = 64 << 10

// callback completes a sign-in and returns the verified account. The token
// subject is the DID; the handle is kept only when it resolves back to it.
func (p *atprotoProvider) callback(ctx context.Context, session sessions.Session, providerID string, params url.Values) (atprotoAccount, error) {
	app := *p.app
	key := providerSessionKey(providerID, "atproto_request")
	state := params.Get("state")
	marker, _ := session.Get(providerSessionKey(providerID, "atproto_server_client_state")).(string)
	boundState, keyID, signed := strings.Cut(marker, ":")
	if !p.loopback && signed && boundState == state {
		cfg, err := p.clientConfig(ctx)
		if err != nil {
			return atprotoAccount{}, err
		}
		if *cfg.KeyID != keyID {
			return atprotoAccount{}, errors.New("AT Protocol signing key changed during sign-in")
		}
		app.Config = cfg
	} else if !p.loopback && boundState != state {
		// Finish an in-flight request from before the server-wide client ID.
		cfg := *app.Config
		cfg.ClientID = legacyATProtoClientID(cfg.CallbackURL)
		app.Config = &cfg
	}
	app.Store = &atprotoFlowStore{session: session, key: key}
	data, err := app.ProcessCallback(ctx, params)
	if err != nil {
		return atprotoAccount{}, err
	}
	account := atprotoAccount{DID: data.AccountDID}
	if ident, err := app.Dir.LookupDID(ctx, data.AccountDID); err == nil {
		if !ident.Handle.IsInvalidHandle() {
			account.Handle = ident.Handle.String()
		}
		account.DisplayName, account.AvatarURL = p.profile(ctx, ident.PDSEndpoint(), data.AccountDID)
	}
	return account, nil
}

// profile reads the public app.bsky.actor.profile record of did from its PDS.
// The record is optional: AT Protocol accounts outside Bluesky can lack it,
// and any failure returns empty values.
func (p *atprotoProvider) profile(ctx context.Context, pds string, did syntax.DID) (displayName, avatarURL string) {
	if !strings.HasPrefix(pds, "https://") {
		return "", ""
	}
	base := strings.TrimRight(pds, "/")
	query := url.Values{"repo": {did.String()}, "collection": {"app.bsky.actor.profile"}, "rkey": {"self"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/xrpc/com.atproto.repo.getRecord?"+query.Encode(), nil)
	if err != nil {
		return "", ""
	}
	resp, err := p.app.Client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	var record struct {
		Value struct {
			DisplayName string `json:"displayName"`
			Avatar      *struct {
				Ref struct {
					Link string `json:"$link"`
				} `json:"ref"`
			} `json:"avatar"`
		} `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, atprotoProfileRecordLimit)).Decode(&record); err != nil {
		return "", ""
	}
	displayName = strings.TrimSpace(record.Value.DisplayName)
	if avatar := record.Value.Avatar; avatar != nil && avatar.Ref.Link != "" {
		blob := url.Values{"did": {did.String()}, "cid": {avatar.Ref.Link}}
		avatarURL = base + "/xrpc/com.atproto.sync.getBlob?" + blob.Encode()
	}
	return displayName, avatarURL
}

const atprotoMetadataTTL = 5 * time.Minute

// publicMetadata returns a copy of the public HTTPS client metadata and its
// remaining lifetime. The caller sets the current server name and callbacks.
func (p *atprotoProvider) publicMetadata(ctx context.Context) (oauth.ClientMetadata, time.Duration, error) {
	p.metadataMu.Lock()
	defer p.metadataMu.Unlock()
	if ttl := time.Until(p.metadataExpires); ttl > 0 {
		return p.metadata, ttl, nil
	}
	cfg, err := p.clientConfig(ctx)
	if err != nil {
		return oauth.ClientMetadata{}, 0, err
	}
	jwks := cfg.PublicJWKS()
	metadata := cfg.ClientMetadata()
	metadata.JWKS = &jwks
	p.metadata = metadata
	p.metadataExpires = time.Now().Add(atprotoMetadataTTL)
	return metadata, atprotoMetadataTTL, nil
}

// writeATProtoMetadata adds the current server identity to the public document.
func (s *HTTPServer) writeATProtoMetadata(c *gin.Context, metadata oauth.ClientMetadata, ttl time.Duration) {
	name := s.authServerName()
	origin := configuredWebserverOrigin(s.config.Webserver.URL)
	metadata.ClientName = &name
	metadata.ClientURI = &origin
	c.Header("Cache-Control", fmt.Sprintf("public, max-age=%d", ttl/time.Second))
	c.JSON(http.StatusOK, metadata)
}

// atprotoFlowStore keeps the request data of one sign-in in the encrypted
// browser flow session, like the PKCE verifier of other providers. Sessions
// are not stored, because Chatto discards the tokens.
type atprotoFlowStore struct {
	session sessions.Session
	key     string
	// state is the state of the request saved during this HTTP request.
	state string
}

func (s *atprotoFlowStore) GetSession(context.Context, syntax.DID, string) (*oauth.ClientSessionData, error) {
	return nil, errors.New("atproto sessions are not stored")
}

func (s *atprotoFlowStore) SaveSession(context.Context, oauth.ClientSessionData) error { return nil }

func (s *atprotoFlowStore) DeleteSession(context.Context, syntax.DID, string) error { return nil }

func (s *atprotoFlowStore) GetAuthRequestInfo(_ context.Context, state string) (*oauth.AuthRequestData, error) {
	raw, _ := s.session.Get(s.key).(string)
	if raw == "" {
		return nil, errors.New("missing atproto request")
	}
	var info oauth.AuthRequestData
	if err := json.Unmarshal([]byte(raw), &info); err != nil {
		return nil, fmt.Errorf("decode atproto request: %w", err)
	}
	if info.State != state {
		return nil, errors.New("atproto request state mismatch")
	}
	return &info, nil
}

func (s *atprotoFlowStore) SaveAuthRequestInfo(_ context.Context, info oauth.AuthRequestData) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	s.session.Set(s.key, string(raw))
	s.state = info.State
	return nil
}

func (s *atprotoFlowStore) DeleteAuthRequestInfo(context.Context, string) error {
	s.session.Delete(s.key)
	return nil
}

// forwardATProtoLoopbackCallback sends a callback that arrived on 127.0.0.1 to
// the configured origin, where the browser has its flow cookie. It reports
// whether it handled the request. The target is fixed, so this is not an open
// redirect.
func (s *HTTPServer) forwardATProtoLoopbackCallback(c *gin.Context, runtime *authProviderRuntime) bool {
	if runtime.atproto == nil || !runtime.atproto.loopback {
		return false
	}
	if _, ok := s.configuredOriginForHost(c.Request.Host); ok {
		return false
	}
	c.Redirect(http.StatusTemporaryRedirect, runtime.callbackURL+"?"+c.Request.URL.RawQuery)
	return true
}

// resolveATProtoIdentity completes an AT Protocol sign-in. The DID is the
// stable subject; the handle is only a hint for new accounts and usernames.
func (r *authProviderRuntime) resolveATProtoIdentity(c *gin.Context, session sessions.Session) (resolvedProviderIdentity, error) {
	account, err := r.atproto.callback(c.Request.Context(), session, r.config.ID, c.Request.URL.Query())
	if err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("complete atproto sign-in: %w", err)
	}
	identity := resolvedProviderIdentity{
		issuer:          r.config.ID,
		subject:         account.DID.String(),
		avatarURL:       account.AvatarURL,
		loginHint:       loginHintFromParts(account.Handle),
		displayNameHint: displayNameHintFromParts(account.DisplayName, account.Handle),
	}
	if r.config.ManageUsernames {
		identity.providerUsername = account.Handle
	}
	return identity, nil
}

// atprotoHandlePageURL returns the handle page for the start request start.
// The page continues start with the handle. errorCode and handle show a
// failed attempt again.
func atprotoHandlePageURL(start *url.URL, errorCode, handle string) string {
	next := *start
	q := next.Query()
	q.Del("login_hint")
	next.RawQuery = q.Encode()
	page := url.Values{"next": {next.RequestURI()}}
	if errorCode != "" {
		page.Set("error", errorCode)
	}
	if handle != "" {
		page.Set("handle", handle)
	}
	return atprotoHandlePagePath + "?" + page.Encode()
}
