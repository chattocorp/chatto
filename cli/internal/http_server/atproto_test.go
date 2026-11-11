package http_server

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/identity"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const testATProtoDID = "did:plc:testatprotouser00000000"

// fakeATProtoAuthServer serves the PDS and authorization-server endpoints that
// an AT Protocol sign-in uses. It requires one DPoP nonce retry, like real
// authorization servers.
type fakeATProtoAuthServer struct {
	server *httptest.Server
	// parScope and tokenCode record what the client sent.
	parScope     string
	parLoginHint string
	tokenCode    string
	subject      string
}

func newFakeATProtoAuthServer(t *testing.T) *fakeATProtoAuthServer {
	t.Helper()
	fake := &fakeATProtoAuthServer{subject: testATProtoDID}
	mux := http.NewServeMux()
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
		fake.parScope = r.PostForm.Get("scope")
		fake.parLoginHint = r.PostForm.Get("login_hint")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"request_uri": "urn:request:1", "expires_in": 60})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		require.NotEmpty(t, r.Header.Get("DPoP"))
		require.NoError(t, r.ParseForm())
		fake.tokenCode = r.PostForm.Get("code")
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
		c.JSON(http.StatusOK, map[string]string{"subject": identity.subject, "username": identity.providerUsername, "hint": identity.loginHint})
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
	provider, err := newATProtoProvider("https://chat.example/auth/providers/bsky/callback")
	require.NoError(t, err)
	useFakeATProtoNetwork(t, provider, fake)
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto, ManageUsernames: true}, atproto: provider}
	server, client := newATProtoFlowRouter(t, runtime)

	status, start := getJSON(t, client, server.URL+"/start?login_hint=alice.test")
	require.Equal(t, http.StatusOK, status)
	authURL, err := url.Parse(start["url"])
	require.NoError(t, err)
	require.Equal(t, "pds.test", authURL.Host)
	require.Equal(t, "https://chat.example/auth/providers/bsky/client-metadata.json", authURL.Query().Get("client_id"))
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
}

func TestATProtoSignInRejectsSubjectMismatch(t *testing.T) {
	fake := newFakeATProtoAuthServer(t)
	fake.subject = "did:plc:someoneelse0000000000000"
	provider, err := newATProtoProvider("https://chat.example/auth/providers/bsky/callback")
	require.NoError(t, err)
	useFakeATProtoNetwork(t, provider, fake)
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto}, atproto: provider}
	server, client := newATProtoFlowRouter(t, runtime)

	_, start := getJSON(t, client, server.URL+"/start?login_hint=alice.test")
	status, _ := getJSON(t, client, server.URL+"/callback?"+url.Values{"state": {start["state"]}, "iss": {"https://pds.test"}, "code": {"code-1"}}.Encode())
	require.Equal(t, http.StatusBadRequest, status, "a token for a different DID must not sign in")
}

func TestATProtoStartRejectsAuthServerURLs(t *testing.T) {
	provider, err := newATProtoProvider("https://chat.example/auth/providers/bsky/callback")
	require.NoError(t, err)
	runtime := &authProviderRuntime{config: config.AuthProviderConfig{ID: "bsky", Type: config.AuthProviderTypeATProto}, atproto: provider}
	server, client := newATProtoFlowRouter(t, runtime)
	for _, hint := range []string{"https://evil.example", "http://127.0.0.1"} {
		status, _ := getJSON(t, client, server.URL+"/start?"+url.Values{"login_hint": {hint}}.Encode())
		require.Equal(t, http.StatusBadRequest, status, hint)
	}
}

func TestATProtoClientMetadata(t *testing.T) {
	provider, err := newATProtoProvider("https://chat.example/auth/providers/bsky/callback")
	require.NoError(t, err)
	require.False(t, provider.loopback)
	metadata := provider.clientMetadata("Chatto", "https://chat.example")
	require.NoError(t, metadata.Validate("https://chat.example/auth/providers/bsky/client-metadata.json"))
	require.Equal(t, "atproto", metadata.Scope)
	require.Equal(t, []string{"https://chat.example/auth/providers/bsky/callback"}, metadata.RedirectURIs)
	require.True(t, metadata.DPoPBoundAccessTokens)
	require.Equal(t, "none", metadata.TokenEndpointAuthMethod)
}

func TestATProtoLoopbackProviderForwardsCallbacks(t *testing.T) {
	provider, err := newATProtoProvider("http://localhost:4510/auth/providers/bsky/callback")
	require.NoError(t, err)
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

func TestFrontendATProtoClientMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		webserverURL string
		wantStatus   int
	}{{"https://chat.example", http.StatusOK}, {"http://localhost:4510", http.StatusNotFound}} {
		server := &HTTPServer{config: config.ChattoConfig{Webserver: config.WebserverConfig{URL: tc.webserverURL}}, router: gin.New()}
		server.setupFrontendATProtoRoutes()
		host := strings.TrimPrefix(strings.TrimPrefix(tc.webserverURL, "https://"), "http://")
		request := httptest.NewRequest(http.MethodGet, frontendATProtoClientMetadataPath, nil)
		request.Host = host
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		require.Equal(t, tc.wantStatus, response.Code, tc.webserverURL)
		if tc.wantStatus != http.StatusOK {
			continue
		}
		var metadata oauth.ClientMetadata
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &metadata))
		clientID := "https://chat.example" + frontendATProtoClientMetadataPath
		require.NoError(t, metadata.Validate(clientID))
		require.Equal(t, clientID, metadata.ClientID)
		require.Equal(t, []string{"https://chat.example/atproto/callback"}, metadata.RedirectURIs)
		// The frontend may change only account link records.
		require.Equal(t, "atproto repo:run.chatto.accountLink", metadata.Scope)
	}
}
