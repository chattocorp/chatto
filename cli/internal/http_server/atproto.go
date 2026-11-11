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

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

// atprotoIdentityScope is the only scope that Chatto's server requests. It
// proves the account DID and grants no access to the user's repository. A
// server with write access could write a false account link record
// (ADR-123).
const atprotoIdentityScope = "atproto"

// atprotoDefaultAuthServer is used when a sign-in starts without a handle.
// The user then enters the handle on this authorization server.
const atprotoDefaultAuthServer = "https://bsky.social"

// atprotoClientMetadataPath is the path of a provider's client metadata
// document below its provider route. Its URL is the provider's client ID.
const atprotoClientMetadataPath = "/client-metadata.json"

// atprotoProvider signs users in with AT Protocol OAuth. It keeps no tokens:
// after the callback verifies the account DID, Chatto discards the session.
type atprotoProvider struct {
	// app holds the shared client configuration and resolvers. Requests use
	// a copy with a store that is bound to the browser's flow session.
	app *oauth.ClientApp
	// loopback is true for http:// deployments, such as development servers.
	// AT Protocol accepts plain HTTP only for its special localhost client,
	// which returns to 127.0.0.1. The callback route forwards such callbacks
	// to the configured origin, where the flow cookie is.
	loopback bool
}

// newATProtoProvider configures the client for a provider whose callback is
// callbackURL. The client ID is the URL of the metadata document that
// setupATProtoRoutes publishes, except for loopback deployments.
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
		clientID := strings.TrimSuffix(callbackURL, "/callback") + atprotoClientMetadataPath
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

// authURL starts a sign-in for identifier, a handle or DID. An empty
// identifier starts at the default authorization server. It returns the URL
// of the authorization page and the state that the callback must return.
func (p *atprotoProvider) authURL(ctx context.Context, session sessions.Session, providerID, identifier string) (string, string, error) {
	if identifier == "" {
		identifier = atprotoDefaultAuthServer
	} else if strings.HasPrefix(identifier, "https://") || strings.HasPrefix(identifier, "http://") {
		// Only Chatto chooses authorization server URLs.
		return "", "", errors.New("invalid atproto identifier")
	}
	store := &atprotoFlowStore{session: session, key: providerSessionKey(providerID, "atproto_request")}
	app := *p.app
	app.Store = store
	redirect, err := app.StartAuthFlow(ctx, strings.TrimPrefix(identifier, "@"))
	if err != nil {
		return "", "", err
	}
	if store.state == "" {
		return "", "", errors.New("atproto request was not stored")
	}
	return redirect, store.state, nil
}

// callback completes a sign-in and returns the verified account DID and its
// handle. The handle is empty when it does not resolve to the DID.
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

func (p *atprotoProvider) callback(ctx context.Context, session sessions.Session, providerID string, params url.Values) (atprotoAccount, error) {
	app := *p.app
	app.Store = &atprotoFlowStore{session: session, key: providerSessionKey(providerID, "atproto_request")}
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

// clientMetadata returns the client metadata document. Loopback clients have
// no document.
func (p *atprotoProvider) clientMetadata(clientName, clientURI string) oauth.ClientMetadata {
	metadata := p.app.Config.ClientMetadata()
	metadata.ClientName = &clientName
	metadata.ClientURI = &clientURI
	return metadata
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
