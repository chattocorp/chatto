package http_server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
func (p *atprotoProvider) callback(ctx context.Context, session sessions.Session, providerID string, params url.Values) (syntax.DID, string, error) {
	app := *p.app
	app.Store = &atprotoFlowStore{session: session, key: providerSessionKey(providerID, "atproto_request")}
	data, err := app.ProcessCallback(ctx, params)
	if err != nil {
		return "", "", err
	}
	handle := ""
	if ident, err := app.Dir.LookupDID(ctx, data.AccountDID); err == nil && !ident.Handle.IsInvalidHandle() {
		handle = ident.Handle.String()
	}
	return data.AccountDID, handle, nil
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
	did, handle, err := r.atproto.callback(c.Request.Context(), session, r.config.ID, c.Request.URL.Query())
	if err != nil {
		return resolvedProviderIdentity{}, fmt.Errorf("complete atproto sign-in: %w", err)
	}
	identity := resolvedProviderIdentity{
		issuer:          r.config.ID,
		subject:         did.String(),
		loginHint:       loginHintFromParts(handle),
		displayNameHint: displayNameHintFromParts(handle),
	}
	if r.config.ManageUsernames {
		identity.providerUsername = handle
	}
	return identity, nil
}
