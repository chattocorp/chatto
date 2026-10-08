package http_server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/testutil"
)

// TestMCPCredentialsAcrossReplicas uses real HTTP listeners and a shared NATS
// store. A new request must observe revocation even on a different replica.
func TestMCPCredentialsAcrossReplicas(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	first := newMCPTestReplica(t, nc, time.Minute)
	second := newMCPTestReplica(t, nc, time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	user, err := first.core.CreateUser(ctx, core.SystemActorID, "mcp-interop-user", "MCP Interop User", "password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	generation, err := first.core.CurrentAuthGeneration(ctx, user.GetId())
	if err != nil {
		t.Fatalf("get auth generation: %v", err)
	}
	issue := func(t *testing.T, resource string, scopes []string) core.BearerSessionCredentials {
		t.Helper()
		credentials, err := first.core.CreateOAuthBearerSessionForClientGrant(ctx, user.GetId(), testOAuthClientID, resource, scopes, generation)
		if err != nil {
			t.Fatalf("issue MCP grant: %v", err)
		}
		return credentials
	}
	const resource = "https://chatto.example/mcp"
	const alias = "https://alias.example/mcp"
	servers := []*httptest.Server{httptest.NewServer(first.router), httptest.NewServer(second.router)}
	for _, server := range servers {
		t.Cleanup(server.Close)
	}
	assertAccess := func(t *testing.T, token, host string, want int) {
		t.Helper()
		for _, server := range servers {
			response := mcpInteropRequest(t, server, token, host, "server/discover", nil)
			if response.Code != want {
				t.Fatalf("MCP status = %d, want %d", response.Code, want)
			}
			if want == http.StatusUnauthorized && !strings.Contains(response.Header().Get("WWW-Authenticate"), "/.well-known/oauth-protected-resource/mcp") {
				t.Fatal("rejected MCP request has no protected-resource challenge")
			}
		}
	}
	assertRefresh := func(t *testing.T, refresh string, want int) {
		t.Helper()
		for _, server := range servers {
			form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {testOAuthClientID}, "resource": {resource}}
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/oauth/token", strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatalf("build refresh request: %v", err)
			}
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatalf("refresh HTTP request: %v", err)
			}
			var result struct {
				Error string `json:"error"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&result)
			response.Body.Close()
			if decodeErr != nil || response.StatusCode != want || result.Error != "invalid_grant" {
				t.Fatalf("refresh status/error = %d/%q, want %d/invalid_grant (decode: %v)", response.StatusCode, result.Error, want, decodeErr)
			}
		}
	}

	t.Run("wrong resource", func(t *testing.T) {
		credentials := issue(t, alias, config.MCPOAuthScopes())
		assertAccess(t, credentials.AccessToken, "alias.example", http.StatusOK)
		assertAccess(t, credentials.AccessToken, "chatto.example", http.StatusUnauthorized)
	})
	t.Run("missing scope", func(t *testing.T) {
		credentials := issue(t, resource, []string{config.MCPRoomsReadScope})
		assertAccess(t, credentials.AccessToken, "chatto.example", http.StatusUnauthorized)
	})
	t.Run("revoked grant", func(t *testing.T) {
		credentials := issue(t, resource, config.MCPOAuthScopes())
		assertAccess(t, credentials.AccessToken, "chatto.example", http.StatusOK)
		if err := first.core.RevokeRefreshTokenWithReason(ctx, credentials.RefreshToken, "explicit"); err != nil {
			t.Fatalf("revoke grant: %v", err)
		}
		assertAccess(t, credentials.AccessToken, "chatto.example", http.StatusUnauthorized)
		assertRefresh(t, credentials.RefreshToken, http.StatusBadRequest)
	})
	t.Run("invalid refresh credential", func(t *testing.T) {
		assertRefresh(t, "invalid-refresh-credential", http.StatusBadRequest)
	})
	t.Run("blocked client", func(t *testing.T) {
		if err := first.core.GrantUserPermission(ctx, core.SystemActorID, user.GetId(), core.PermServerManage); err != nil {
			t.Fatalf("grant client-management permission: %v", err)
		}
		if err := first.core.RecordOAuthClientAuthorization(ctx, user.GetId(), testOAuthClientID, "Interop Client", "https://client.example", "https://client.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
			t.Fatalf("record client: %v", err)
		}
		credentials := issue(t, resource, config.MCPOAuthScopes())
		assertAccess(t, credentials.AccessToken, "chatto.example", http.StatusOK)
		if _, err := first.core.UpdateOAuthClientPolicy(ctx, user.GetId(), testOAuthClientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED); err != nil {
			t.Fatalf("block client: %v", err)
		}
		assertAccess(t, credentials.AccessToken, "chatto.example", http.StatusUnauthorized)
		assertRefresh(t, credentials.RefreshToken, http.StatusBadRequest)
	})
}

func TestMCPRejectsExpiredAccessCredentialOverHTTP(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	replica := newMCPTestReplica(t, nc, time.Second)
	ctx := context.Background()
	user, err := replica.core.CreateUser(ctx, core.SystemActorID, "mcp-expiry-user", "MCP Expiry User", "password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	generation, err := replica.core.CurrentAuthGeneration(ctx, user.GetId())
	if err != nil {
		t.Fatalf("get auth generation: %v", err)
	}
	credentials, err := replica.core.CreateOAuthBearerSessionForClientGrant(ctx, user.GetId(), testOAuthClientID, "https://chatto.example/mcp", config.MCPOAuthScopes(), generation)
	if err != nil {
		t.Fatalf("issue grant: %v", err)
	}
	server := httptest.NewServer(replica.router)
	defer server.Close()
	if response := mcpInteropRequest(t, server, credentials.AccessToken, "chatto.example", "server/discover", nil); response.Code != http.StatusOK {
		t.Fatalf("current access status = %d, want 200", response.Code)
	}
	time.Sleep(time.Until(credentials.AccessTokenExpiresAt) + time.Millisecond)
	if response := mcpInteropRequest(t, server, credentials.AccessToken, "chatto.example", "server/discover", nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("expired access status = %d, want 401", response.Code)
	}
}

// newMCPTestReplica mounts the production OAuth and MCP routes on a distinct
// core. Only client metadata retrieval is replaced with a fixed test document.
func newMCPTestReplica(t *testing.T, nc *nats.Conn, accessTTL time.Duration) *HTTPServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	chattoCore, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "mcp-interop-secret", AuthAccessTokenTTL: accessTTL,
		Assets: config.AssetsConfig{SigningSecret: "mcp-interop-assets-secret"},
	})
	if err != nil {
		t.Fatalf("create replica: %v", err)
	}
	startCoreServices(t, chattoCore)
	s := &HTTPServer{
		core: chattoCore, nc: nc, router: gin.New(), version: "interop-test",
		config: config.ChattoConfig{
			Webserver: config.WebserverConfig{URL: "https://chatto.example", AllowedOrigins: []string{"https://alias.example"}},
			MCP:       config.MCPConfig{Enabled: true},
		},
	}
	s.oauthClientResolveHook = func(_ context.Context, clientID string) (OAuthClient, bool, error) {
		return OAuthClient{ClientID: testOAuthClientID, ClientName: "Interop Client", RedirectURIs: []string{"https://client.example/callback"}}, clientID == testOAuthClientID, nil
	}
	s.setupOAuthRoutes()
	if err := s.setupMCPRoutes(); err != nil {
		t.Fatalf("mount MCP routes: %v", err)
	}
	return s
}

// mcpInteropRequest sends the 2026-07-28 wire format without an MCP SDK.
func mcpInteropRequest(t *testing.T, server *httptest.Server, token, host, method string, args map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	params := map[string]any{"_meta": map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientInfo":         map[string]string{"name": "interop-test", "version": "1"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
	}}
	maps.Copy(params, args)
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatalf("encode MCP request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/mcp", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build MCP request: %v", err)
	}
	request.Host = host
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", method)
	if name, ok := args["name"].(string); ok {
		request.Header.Set("Mcp-Name", name)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("send MCP request: %v", err)
	}
	defer response.Body.Close()
	recorded := httptest.NewRecorder()
	recorded.HeaderMap = response.Header
	recorded.Code = response.StatusCode
	if _, err := io.Copy(recorded.Body, response.Body); err != nil {
		t.Fatalf("read MCP response: %v", err)
	}
	return recorded
}

func TestMCPRequestLogsExcludeCredentialsAndContent(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	s := newMCPTestReplica(t, nc, time.Minute)
	var output bytes.Buffer
	logger := log.New(&output)
	logger.SetLevel(log.DebugLevel)
	logger.SetFormatter(log.JSONFormatter)
	s.router = gin.New()
	s.router.Use(requestLogger(logger))
	if err := s.setupMCPRoutes(); err != nil {
		t.Fatalf("mount MCP routes: %v", err)
	}
	server := httptest.NewServer(s.router)
	defer server.Close()
	assertPrivateLog := func(canaries ...string) {
		t.Helper()
		for _, canary := range canaries {
			if strings.Contains(output.String(), canary) {
				t.Fatal("MCP request log contains private request or response data")
			}
		}
		var entry map[string]any
		if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
			t.Fatalf("decode request log: %v", err)
		}
		assertLogField(t, entry, "path", "/mcp")
	}
	const secret = "private-access-token-canary"
	response := mcpInteropRequest(t, server, secret, "chatto.example", "tools/call", map[string]any{
		"name": "post_message", "arguments": map[string]string{"room_id": "private-room-canary", "body": "private-message-canary"},
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid credential status = %d, want 401", response.Code)
	}
	assertPrivateLog(secret, "private-room-canary", "private-message-canary")
	output.Reset()

	// Successful tool results can contain account data. Verify that the data
	// reaches the authenticated client without entering debug request logs.
	ctx := context.Background()
	const login, displayName, password = "private-login-canary", "private-display-canary", "private-password-canary"
	user, err := s.core.CreateUser(ctx, core.SystemActorID, login, displayName, password)
	if err != nil {
		t.Fatalf("create private account: %v", err)
	}
	generation, err := s.core.CurrentAuthGeneration(ctx, user.GetId())
	if err != nil {
		t.Fatalf("get auth generation: %v", err)
	}
	credentials, err := s.core.CreateOAuthBearerSessionForClientGrant(ctx, user.GetId(), testOAuthClientID, "https://chatto.example/mcp", config.MCPOAuthScopes(), generation)
	if err != nil {
		t.Fatalf("issue private grant: %v", err)
	}
	response = mcpInteropRequest(t, server, credentials.AccessToken, "chatto.example", "tools/call", map[string]any{"name": "get_current_user"})
	var result struct {
		Result struct {
			StructuredContent struct {
				DisplayName string `json:"displayName"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode identity result: %v", err)
	}
	if response.Code != http.StatusOK || result.Result.StructuredContent.DisplayName != displayName {
		t.Fatal("authenticated identity call did not return the private display name")
	}
	assertPrivateLog(login, displayName, password, credentials.AccessToken, credentials.RefreshToken)
}
