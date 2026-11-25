package http_server

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"
)

const testATProtoDID = "did:plc:testatprotouser00000000"
const testATProtoWebDID = "did:web:alice.example.com"

func newTestATProtoProvider(t *testing.T, callback string) *atprotoProvider {
	t.Helper()
	p, err := newATProtoProvider(callback)
	require.NoError(t, err)
	key, err := atcrypto.GeneratePrivateKeyP256()
	require.NoError(t, err)
	p.signingKey = func(context.Context, string) (*atcrypto.PrivateKeyP256, error) { return key, nil }
	return p
}

// fakeATProtoAuthServer serves the PDS and authorization-server endpoints that
// an AT Protocol sign-in uses. It requires one DPoP nonce retry, like real
// authorization servers.
type fakeATProtoAuthServer struct {
	server *httptest.Server
	// parScope and tokenCode record what the client sent.
	parAssertion, tokenAssertion string
	parClientID                  string
	parScope                     string
	parLoginHint                 string
	tokenCode                    string
	subject                      string
}

func newFakeATProtoAuthServer(t *testing.T) *fakeATProtoAuthServer {
	t.Helper()
	fake := &fakeATProtoAuthServer{subject: testATProtoDID}
	mux := http.NewServeMux()
	mux.HandleFunc("GET alice.example.com/.well-known/atproto-did", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testATProtoWebDID))
	})
	mux.HandleFunc("GET alice.example.com/.well-known/did.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":          testATProtoWebDID,
			"alsoKnownAs": []string{"at://alice.example.com"},
			"service": []map[string]string{{
				"id": "#atproto_pds", "type": "AtprotoPersonalDataServer", "serviceEndpoint": "https://pds.test",
			}},
		})
	})
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"authorization_servers": []string{"https://pds.test"}})
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                           "https://pds.test",
			"authorization_endpoint":                           "https://pds.test/authorize",
			"token_endpoint":                                   "https://pds.test/token",
			"pushed_authorization_request_endpoint":            "https://pds.test/par",
			"response_types_supported":                         []string{"code"},
			"grant_types_supported":                            []string{"authorization_code", "refresh_token"},
			"code_challenge_methods_supported":                 []string{"S256"},
			"token_endpoint_auth_methods_supported":            []string{"none", "private_key_jwt"},
			"token_endpoint_auth_signing_alg_values_supported": []string{"ES256"},
			"scopes_supported":                                 []string{"atproto"},
			"authorization_response_iss_parameter_supported":   true,
			"require_pushed_authorization_requests":            true,
			"dpop_signing_alg_values_supported":                []string{"ES256"},
			"client_id_metadata_document_supported":            true,
		})
	})
	mux.HandleFunc("POST /par", func(w http.ResponseWriter, r *http.Request) {
		require.NotEmpty(t, r.Header.Get("DPoP"))
		w.Header().Set("DPoP-Nonce", "nonce-1")
		if !dpopHasNonce(t, r.Header.Get("DPoP")) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "use_dpop_nonce"})
			return
		}
		require.NoError(t, r.ParseForm())
		fake.parClientID = r.PostForm.Get("client_id")
		fake.parAssertion = r.PostForm.Get("client_assertion")
		fake.parScope = r.PostForm.Get("scope")
		fake.parLoginHint = r.PostForm.Get("login_hint")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"request_uri": "urn:request:1", "expires_in": 60})
	})
	mux.HandleFunc("GET /xrpc/com.atproto.repo.getRecord", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("collection") != "app.bsky.actor.profile" || r.URL.Query().Get("rkey") != "self" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"uri": "at://" + fake.subject + "/app.bsky.actor.profile/self",
			"value": map[string]any{
				"$type":       "app.bsky.actor.profile",
				"displayName": "  Alice Example  ",
				"avatar":      map[string]any{"$type": "blob", "ref": map[string]any{"$link": "bafkreiavatar"}, "mimeType": "image/jpeg"},
			},
		})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		require.NotEmpty(t, r.Header.Get("DPoP"))
		require.NoError(t, r.ParseForm())
		require.Equal(t, fake.parClientID, r.PostForm.Get("client_id"))
		fake.tokenCode = r.PostForm.Get("code")
		fake.tokenAssertion = r.PostForm.Get("client_assertion")
		w.Header().Set("DPoP-Nonce", "nonce-1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access", "token_type": "DPoP", "scope": "atproto",
			"refresh_token": "refresh", "expires_in": 300, "sub": fake.subject,
		})
	})
	fake.server = httptest.NewTLSServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// dpopHasNonce reports whether the DPoP proof's payload carries a nonce.
func dpopHasNonce(t *testing.T, proof string) bool {
	t.Helper()
	parts := strings.Split(proof, ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims struct {
		Nonce string `json:"nonce"`
	}
	require.NoError(t, json.Unmarshal(payload, &claims))
	return claims.Nonce != ""
}

// useFakeATProtoNetwork sends every request of provider to fake and makes
// the test DID resolve to it.
func useFakeATProtoNetwork(t *testing.T, provider *atprotoProvider, fake *fakeATProtoAuthServer) {
	t.Helper()
	addr := fake.server.Listener.Addr().String()
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		// The test certificate is valid for example.com, not pds.test.
		TLSClientConfig: &tls.Config{RootCAs: fake.server.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs, ServerName: "example.com"},
	}}
	provider.app.Client = client
	provider.app.Resolver.Client = client
	dir := identity.NewMockDirectory()
	dir.Insert(identity.Identity{
		DID:      syntax.DID(testATProtoDID),
		Handle:   syntax.Handle("alice.test"),
		Services: map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: "https://pds.test"}},
	})
	provider.app.Dir = dir
}

// newATProtoFlowRouter serves start and callback routes around runtime with
// the same encrypted cookie sessions as production.
func newATProtoFlowRouter(t *testing.T, runtime *authProviderRuntime) (*httptest.Server, *http.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("chatto_session", cookie.NewStore([]byte("0123456789abcdef0123456789abcdef"), []byte("0123456789abcdef0123456789abcdef"))))
	var identity resolvedProviderIdentity
	router.GET("/start", func(c *gin.Context) {
		session := sessions.Default(c)
		authURL, state, err := runtime.atproto.authURL(c.Request.Context(), session, runtime.config.ID, c.Query("login_hint"))
		if err != nil {
			c.String(http.StatusBadRequest, "start failed")
			return
		}
		require.NoError(t, session.Save())
		c.JSON(http.StatusOK, map[string]string{"url": authURL, "state": state})
	})
	router.GET("/callback", func(c *gin.Context) {
		var err error
		identity, err = runtime.resolveATProtoIdentity(c, sessions.Default(c))
		if err != nil {
			c.String(http.StatusBadRequest, "callback failed")
			return
		}
		c.JSON(http.StatusOK, map[string]string{"subject": identity.subject, "username": identity.providerUsername, "hint": identity.loginHint, "display": identity.displayNameHint, "avatar": identity.avatarURL})
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return server, &http.Client{Jar: jar}
}

func getJSON(t *testing.T, client *http.Client, target string) (int, map[string]string) {
	t.Helper()
	response, err := client.Get(target)
	require.NoError(t, err)
	defer response.Body.Close()
	body := map[string]string{}
	if response.StatusCode == http.StatusOK {
		require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
	}
	return response.StatusCode, body
}

func TestATProtoSignInVerifiesDIDWithIdentityScopeOnly(t *testing.T) {
	fake := newFakeATProtoAuthServer(t)
	provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	useFakeATProtoNetwork(t, provider, fake)
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto, ManageUsernames: true}, atproto: provider}
	server, client := newATProtoFlowRouter(t, runtime)

	status, start := getJSON(t, client, server.URL+"/start?login_hint=alice.test")
	require.Equal(t, http.StatusOK, status)
	authURL, err := url.Parse(start["url"])
	require.NoError(t, err)
	require.Equal(t, "pds.test", authURL.Host)
	require.Equal(t, "https://chat.example/oauth-client-metadata.json", authURL.Query().Get("client_id"))
	require.Equal(t, "atproto", fake.parScope, "the server must request identity only")
	require.Equal(t, "alice.test", fake.parLoginHint)

	// A callback with another state is rejected without a token request.
	status, _ = getJSON(t, client, server.URL+"/callback?"+url.Values{"state": {"other"}, "iss": {"https://pds.test"}, "code": {"code-1"}}.Encode())
	require.Equal(t, http.StatusBadRequest, status)
	require.Empty(t, fake.tokenCode)

	status, result := getJSON(t, client, server.URL+"/callback?"+url.Values{"state": {start["state"]}, "iss": {"https://pds.test"}, "code": {"code-1"}}.Encode())
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "code-1", fake.tokenCode)
	require.Equal(t, testATProtoDID, result["subject"])
	require.Equal(t, "alice.test", result["username"])
	require.Equal(t, "alice.test", result["hint"])
	// The public Bluesky profile supplies the new account's hints.
	require.Equal(t, "Alice Example", result["display"])
	require.Equal(t, "https://pds.test/xrpc/com.atproto.sync.getBlob?cid=bafkreiavatar&did="+url.QueryEscape(testATProtoDID), result["avatar"])
	metadata, _, err := provider.publicMetadata(t.Context())
	require.NoError(t, err)
	encoded, err := json.Marshal(metadata.JWKS)
	require.NoError(t, err)
	var public jose.JSONWebKeySet
	require.NoError(t, json.Unmarshal(encoded, &public))
	require.Len(t, public.Keys, 1)
	require.True(t, public.Keys[0].IsPublic())
	for _, assertion := range []string{fake.parAssertion, fake.tokenAssertion} {
		token, err := jwt.ParseSigned(assertion, []jose.SignatureAlgorithm{jose.ES256})
		require.NoError(t, err)
		require.Equal(t, public.Keys[0].KeyID, token.Headers[0].KeyID)
		var claims jwt.Claims
		require.NoError(t, token.Claims(public.Keys[0].Key, &claims))
		require.NoError(t, claims.Validate(jwt.Expected{
			Issuer: metadata.ClientID, Subject: metadata.ClientID,
			AnyAudience: jwt.Audience{"https://pds.test"}, Time: time.Now(),
		}))
		require.NotEmpty(t, claims.ID)
	}
	require.NotEqual(t, fake.parAssertion, fake.tokenAssertion)
}

func TestATProtoSignInRequiresTheOriginalSigningKey(t *testing.T) {
	provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	fake := newFakeATProtoAuthServer(t)
	useFakeATProtoNetwork(t, provider, fake)
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky"}, atproto: provider}
	server, client := newATProtoFlowRouter(t, runtime)
	status, start := getJSON(t, client, server.URL+"/start?login_hint=alice.test")
	require.Equal(t, http.StatusOK, status)
	callback := server.URL + "/callback?" + url.Values{
		"state": {start["state"]}, "iss": {"https://pds.test"}, "code": {"code-1"},
	}.Encode()

	original := provider.signingKey
	provider.signingKey = func(context.Context, string) (*atcrypto.PrivateKeyP256, error) {
		return nil, errors.New("wrapping key unavailable")
	}
	status, _ = getJSON(t, client, callback)
	require.Equal(t, http.StatusBadRequest, status)
	status, _ = getJSON(t, client, server.URL+"/start?login_hint=alice.test")
	require.Equal(t, http.StatusBadRequest, status)

	replacement, err := atcrypto.GeneratePrivateKeyP256()
	require.NoError(t, err)
	provider.signingKey = func(context.Context, string) (*atcrypto.PrivateKeyP256, error) { return replacement, nil }
	status, _ = getJSON(t, client, callback)
	require.Equal(t, http.StatusBadRequest, status)
	require.Empty(t, fake.tokenCode)

	provider.signingKey = original
	status, _ = getJSON(t, client, callback)
	require.Equal(t, http.StatusOK, status)
}

// A callback can reach an upgraded replica after an older replica sent PAR.
func TestATProtoCompletesPreviousClientSignIn(t *testing.T) {
	const callback = "https://chat.example/auth/providers/bsky/callback"
	for _, clientID := range []string{"https://chat.example/auth/providers/bsky/client-metadata.json", "https://chat.example/oauth-client-metadata.json"} {
		t.Run(clientID, func(t *testing.T) {
			provider := newTestATProtoProvider(t, callback)
			fake := newFakeATProtoAuthServer(t)
			useFakeATProtoNetwork(t, provider, fake)
			router := gin.New()
			router.Use(sessions.Sessions("flow", cookie.NewStore([]byte("test-only-cookie-key"))))
			router.GET("/", func(c *gin.Context) {
				session := sessions.Default(c)
				previous := *provider.app
				cfg := *previous.Config
				cfg.ClientID = clientID
				previous.Config = &cfg
				store := &atprotoFlowStore{session: session, key: providerSessionKey("bsky", "atproto_request")}
				previous.Store = store
				_, err := previous.StartAuthFlow(c.Request.Context(), "alice.test")
				require.NoError(t, err)
				if clientID == provider.app.Config.ClientID {
					session.Set(providerSessionKey("bsky", "atproto_server_client_state"), store.state)
				}
				account, err := provider.callback(c.Request.Context(), session, "bsky", url.Values{
					"state": {store.state}, "iss": {"https://pds.test"}, "code": {"code-1"},
				})
				require.NoError(t, err)
				require.Equal(t, testATProtoDID, account.DID.String())
				require.Empty(t, fake.tokenAssertion)
			})
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		})
	}
}

func TestATProtoSignInWithWebDID(t *testing.T) {
	for _, identifier := range []string{testATProtoWebDID, "alice.example.com"} {
		t.Run(identifier, func(t *testing.T) {
			fake := newFakeATProtoAuthServer(t)
			fake.subject = testATProtoWebDID
			provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
			useFakeATProtoNetwork(t, provider, fake)
			// Resolve the handle and DID document over the local TLS transport.
			provider.app.Dir = &identity.BaseDirectory{
				HTTPClient:            http.Client{Transport: provider.app.Client.Transport},
				SkipDNSDomainSuffixes: []string{".example.com"},
			}
			runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto, ManageUsernames: true}, atproto: provider}
			server, client := newATProtoFlowRouter(t, runtime)
			status, start := getJSON(t, client, server.URL+"/start?login_hint="+url.QueryEscape(identifier))
			require.Equal(t, http.StatusOK, status)
			status, result := getJSON(t, client, server.URL+"/callback?"+url.Values{"state": {start["state"]}, "iss": {"https://pds.test"}, "code": {"code-1"}}.Encode())
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, testATProtoWebDID, result["subject"])
			require.Equal(t, "alice.example.com", result["username"])
		})
	}
}

func TestATProtoSignInRejectsSubjectMismatch(t *testing.T) {
	fake := newFakeATProtoAuthServer(t)
	fake.subject = "did:plc:someoneelse0000000000000"
	provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	useFakeATProtoNetwork(t, provider, fake)
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto}, atproto: provider}
	server, client := newATProtoFlowRouter(t, runtime)

	_, start := getJSON(t, client, server.URL+"/start?login_hint=alice.test")
	status, _ := getJSON(t, client, server.URL+"/callback?"+url.Values{"state": {start["state"]}, "iss": {"https://pds.test"}, "code": {"code-1"}}.Encode())
	require.Equal(t, http.StatusBadRequest, status, "a token for a different DID must not sign in")
}

func TestATProtoStartRejectsAuthServerURLs(t *testing.T) {
	provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto}, atproto: provider}
	server, client := newATProtoFlowRouter(t, runtime)
	for _, hint := range []string{"https://evil.example", "http://127.0.0.1"} {
		status, _ := getJSON(t, client, server.URL+"/start?"+url.Values{"login_hint": {hint}}.Encode())
		require.Equal(t, http.StatusBadRequest, status, hint)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestATProtoStartClassifiesIdentityErrors(t *testing.T) {
	provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	dir := identity.NewMockDirectory()
	dir.Insert(identity.Identity{
		DID:      syntax.DID(testATProtoDID),
		Handle:   syntax.Handle("alice.test"),
		Services: map[string]identity.ServiceEndpoint{"atproto_pds": {Type: "AtprotoPersonalDataServer", URL: "https://pds.test"}},
	})
	provider.app.Dir = dir
	// The PDS of alice.test does not answer.
	provider.app.Resolver.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unreachable")
	})}

	for _, tc := range []struct {
		identifier string
		notFound   bool
	}{
		{identifier: "nobody.test", notFound: true},
		{identifier: "not a handle", notFound: true},
		{identifier: "https://evil.example", notFound: true},
		{identifier: "@alice.test", notFound: false},
	} {
		_, _, err := provider.authURL(t.Context(), nil, "bsky", tc.identifier)
		require.Error(t, err, tc.identifier)
		require.Equal(t, tc.notFound, errors.Is(err, errATProtoIdentityNotFound), tc.identifier)
	}
}

func TestATProtoClientMetadata(t *testing.T) {
	provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	require.False(t, provider.loopback)
	metadata, _, err := provider.publicMetadata(t.Context())
	require.NoError(t, err)
	require.NoError(t, metadata.Validate("https://chat.example/oauth-client-metadata.json"))
	require.Equal(t, "atproto", metadata.Scope)
	require.Equal(t, []string{"https://chat.example/auth/providers/bsky/callback"}, metadata.RedirectURIs)
	require.True(t, metadata.DPoPBoundAccessTokens)
	require.Equal(t, "private_key_jwt", metadata.TokenEndpointAuthMethod)
	require.Equal(t, "ES256", *metadata.TokenEndpointAuthSigningAlg)
	require.Len(t, metadata.JWKS.Keys, 1)
}

func TestATProtoPublicMetadataCache(t *testing.T) {
	p := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	first, _, err := p.publicMetadata(t.Context())
	require.NoError(t, err)
	p.signingKey = func(context.Context, string) (*atcrypto.PrivateKeyP256, error) {
		return nil, errors.New("key store unavailable")
	}
	cached, ttl, err := p.publicMetadata(t.Context())
	require.NoError(t, err)
	require.Equal(t, first.JWKS, cached.JWKS)
	require.Positive(t, ttl)
	_, err = p.clientConfig(t.Context())
	require.Error(t, err)

	p.metadataExpires = time.Now().Add(-time.Second)
	_, _, err = p.publicMetadata(t.Context())
	require.Error(t, err)
	replacement := newTestATProtoProvider(t, p.app.Config.CallbackURL)
	p.signingKey = replacement.signingKey
	refreshed, _, err := p.publicMetadata(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, *first.JWKS.Keys[0].KeyID, *refreshed.JWKS.Keys[0].KeyID)
}

func TestATProtoPublishedMetadataIdentifiesServer(t *testing.T) {
	const origin = "https://chat.example"
	server, client, chattoCore := setupTestHTTPServerWithHook(t, func(s *HTTPServer) {
		s.config.Webserver.URL = origin
		s.config.Auth.Providers = []config.AuthProviderConfig{
			{ID: "bsky", Type: config.AuthProviderTypeATProto},
			{ID: "github", Type: config.AuthProviderTypeGitHub},
			{ID: "handles", Type: config.AuthProviderTypeATProto},
		}
		s.setupOIDCRoutes()
	})
	setTestServerName(t, t.Context(), chattoCore, "Engineering")
	for path, callbacks := range map[string][]string{
		"/oauth-client-metadata.json":               {origin + "/auth/providers/bsky/callback", origin + "/auth/providers/handles/callback"},
		"/auth/providers/bsky/client-metadata.json": {origin + "/auth/providers/bsky/callback"},
	} {
		resp, err := client.Get(server.URL + path)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var metadata oauth.ClientMetadata
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&metadata))
		resp.Body.Close()
		require.NoError(t, metadata.Validate(origin+path))
		require.Equal(t, "Engineering", *metadata.ClientName)
		require.Equal(t, origin, *metadata.ClientURI)
		require.Equal(t, callbacks, metadata.RedirectURIs)
		if path == atprotoClientMetadataPath {
			require.True(t, metadata.IsConfidential())
		} else {
			require.Equal(t, "none", metadata.TokenEndpointAuthMethod)
			require.Nil(t, metadata.JWKS)
		}
	}
}

func TestATProtoLoopbackProviderForwardsCallbacks(t *testing.T) {
	provider, err := newATProtoProvider("http://localhost:4510/auth/providers/bsky/callback")
	require.NoError(t, err)
	cfg, err := provider.clientConfig(t.Context())
	require.NoError(t, err)
	require.False(t, cfg.IsConfidential())
	require.True(t, provider.loopback)
	require.Equal(t, "http://127.0.0.1:4510/auth/providers/bsky/callback", provider.app.Config.CallbackURL)

	s := &HTTPServer{config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: "http://localhost:4510"}}}
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky"}, callbackURL: "http://localhost:4510/auth/providers/bsky/callback", atproto: provider}
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		host    string
		forward bool
	}{{"127.0.0.1:4510", true}, {"localhost:4510", false}} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/auth/providers/bsky/callback?state=s&code=c", nil)
		require.Equal(t, tc.forward, s.forwardATProtoLoopbackCallback(c, runtime), tc.host)
		if tc.forward {
			require.Equal(t, "http://localhost:4510/auth/providers/bsky/callback?state=s&code=c", recorder.Header().Get("Location"))
		}
	}
}

// A start without a handle, or with an unknown handle, goes to the handle
// page, which continues the same start request (FDR-058).
func TestATProtoStartAsksForTheHandle(t *testing.T) {
	provider := newTestATProtoProvider(t, "https://chat.example/auth/providers/bsky/callback")
	dir := identity.NewMockDirectory()
	provider.app.Dir = dir
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto}, atproto: provider}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("chatto_session", cookie.NewStore([]byte("0123456789abcdef0123456789abcdef"))))
	router.GET("/auth/providers/bsky", func(c *gin.Context) {
		(&HTTPServer{}).handleProviderStart(c, runtime, providerStartIntent(c))
	})

	for _, tc := range []struct {
		query string
		want  url.Values
	}{
		{
			query: "redirect=%2Fservers%2Fcallback&intent=link&link_start=tok",
			want:  url.Values{"next": {"/auth/providers/bsky?intent=link&link_start=tok&redirect=%2Fservers%2Fcallback"}},
		},
		{
			query: "redirect=%2Fchat&login_hint=nobody.test",
			want: url.Values{
				"next":   {"/auth/providers/bsky?redirect=%2Fchat"},
				"error":  {"atproto_identity_not_found"},
				"handle": {"nobody.test"},
			},
		},
	} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/providers/bsky?"+tc.query, nil))
		require.Equal(t, http.StatusSeeOther, recorder.Code, tc.query)
		location, err := url.Parse(recorder.Header().Get("Location"))
		require.NoError(t, err)
		require.Equal(t, atprotoHandlePagePath, location.Path, tc.query)
		require.Equal(t, tc.want, location.Query(), tc.query)
	}
}
