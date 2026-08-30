package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/testutil"
)

type staticOAuthHandler struct{ token string }

type canonicalHostTransport struct {
	base http.RoundTripper
	host string
}

func (t canonicalHostTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Host = t.host
	return t.base.RoundTrip(clone)
}

func (h staticOAuthHandler) TokenSource(context.Context) (oauth2.TokenSource, error) {
	return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: h.token}), nil
}

func (staticOAuthHandler) Authorize(context.Context, *http.Request, *http.Response) error { return nil }

func TestMCPHandlerListsOnlyVisibleRoomsWithScopedToken(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	chattoCore, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startTestCore(t, chattoCore)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	viewer, err := chattoCore.CreateUser(ctx, core.SystemActorID, "mcp-viewer", "MCP Viewer", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, core.SystemActorID, "MCP Rooms", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	visible, err := chattoCore.CreateRoom(ctx, core.SystemActorID, core.KindChannel, group.GetId(), "visible-to-mcp", "Visible description")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	hidden, err := chattoCore.CreateRoom(ctx, core.SystemActorID, core.KindChannel, group.GetId(), "hidden-from-mcp", "")
	if err != nil {
		t.Fatalf("CreateRoom hidden: %v", err)
	}
	if err := chattoCore.DenyRoomPermission(ctx, core.SystemActorID, hidden.GetId(), core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	generation, err := chattoCore.CurrentAuthGeneration(ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	const resource = "https://chat.example/mcp"
	credentials, err := chattoCore.CreateOAuthBearerSessionForClientGrant(ctx, viewer.GetId(), "https://agent.example/client.json", resource, []string{config.MCPRoomsReadScope}, generation)
	if err != nil {
		t.Fatalf("CreateOAuthBearerSessionForClientGrant: %v", err)
	}
	handler, err := NewHandler(chattoCore, config.ChattoConfig{
		Webserver: config.WebserverConfig{URL: "https://chat.example"},
		MCP:       config.MCPConfig{Enabled: true},
	}, "test")
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	httpClient := server.Client()
	httpClient.Transport = canonicalHostTransport{base: httpClient.Transport, host: "chat.example"}

	client := mcp.NewClient(&mcp.Implementation{Name: "chatto-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.URL + "/mcp", DisableStandaloneSSE: true,
		HTTPClient:   httpClient,
		OAuthHandler: staticOAuthHandler{token: credentials.AccessToken},
	}, nil)
	if err != nil {
		t.Fatalf("Connect MCP client: %v", err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "list_rooms" {
		t.Fatalf("tools = %#v, want list_rooms", tools.Tools)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_rooms", Arguments: map[string]any{"limit": 100}})
	if err != nil {
		t.Fatalf("CallTool list_rooms: %v", err)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output listRoomsOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode list_rooms output: %v", err)
	}
	if !roomResultsContain(output.Rooms, visible.GetId()) {
		t.Fatalf("visible room %q missing from %#v", visible.GetId(), output.Rooms)
	}
	if roomResultsContain(output.Rooms, hidden.GetId()) {
		t.Fatalf("hidden room %q present in %#v", hidden.GetId(), output.Rooms)
	}
}

func TestMCPHandlerServesProtocol20260728OverRawHTTP(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	chattoCore, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startTestCore(t, chattoCore)
	ctx := context.Background()
	owner, err := chattoCore.CreateUser(ctx, core.SystemActorID, "raw-mcp-owner", "Raw MCP Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := chattoCore.CreateBot(ctx, owner.GetId(), "raw_mcp_bot", "Raw MCP Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if err := chattoCore.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), core.PermissionTargetScope{Kind: core.MatrixScopeServer}, core.PermRoomList, core.PermissionStateAllow); err != nil {
		t.Fatalf("grant bot room.list: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, core.SystemActorID, "Raw MCP Rooms", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, core.SystemActorID, core.KindChannel, group.GetId(), "raw-mcp-visible", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	handler, err := NewHandler(chattoCore, config.ChattoConfig{
		Webserver: config.WebserverConfig{URL: "https://chat.example"},
		MCP:       config.MCPConfig{Enabled: true},
	}, "test")
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	unauthenticated := newRawMCPRequest("", "server/discover", "", `{
		"jsonrpc":"2.0",
		"id":1,
		"method":"server/discover",
		"params":{"_meta":{
			"io.modelcontextprotocol/protocolVersion":"2026-07-28",
			"io.modelcontextprotocol/clientInfo":{"name":"raw-http-test","version":"1.0"},
			"io.modelcontextprotocol/clientCapabilities":{}
		}}
	}`)
	unauthenticatedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized || unauthenticatedResponse.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("unauthenticated status/challenge = %d/%q, want 401 with WWW-Authenticate", unauthenticatedResponse.Code, unauthenticatedResponse.Header().Get("WWW-Authenticate"))
	}

	legacyShape := newRawMCPRequest(bot.APIKey, "tools/list", "", `{
		"jsonrpc":"2.0",
		"id":2,
		"method":"tools/list",
		"params":{}
	}`)
	legacyShapeResponse := httptest.NewRecorder()
	handler.ServeHTTP(legacyShapeResponse, legacyShape)
	if legacyShapeResponse.Code != http.StatusBadRequest {
		t.Fatalf("2026 request without per-request metadata status = %d, want 400: %s", legacyShapeResponse.Code, legacyShapeResponse.Body.String())
	}

	discover := performRawMCPRequest(t, handler, bot.APIKey, "server/discover", "", `{
		"jsonrpc":"2.0",
		"id":1,
		"method":"server/discover",
		"params":{"_meta":{
			"io.modelcontextprotocol/protocolVersion":"2026-07-28",
			"io.modelcontextprotocol/clientInfo":{"name":"raw-http-test","version":"1.0"},
			"io.modelcontextprotocol/clientCapabilities":{}
		}}
	}`)
	var discovery struct {
		Result struct {
			SupportedVersions []string `json:"supportedVersions"`
		} `json:"result"`
	}
	decodeMCPResponse(t, discover, &discovery)
	if !slices.Contains(discovery.Result.SupportedVersions, "2026-07-28") {
		t.Fatalf("supportedVersions = %v, want 2026-07-28", discovery.Result.SupportedVersions)
	}

	list := performRawMCPRequest(t, handler, bot.APIKey, "tools/list", "", `{
		"jsonrpc":"2.0",
		"id":2,
		"method":"tools/list",
		"params":{"_meta":{
			"io.modelcontextprotocol/protocolVersion":"2026-07-28",
			"io.modelcontextprotocol/clientInfo":{"name":"raw-http-test","version":"1.0"},
			"io.modelcontextprotocol/clientCapabilities":{}
		}}
	}`)
	var tools struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	decodeMCPResponse(t, list, &tools)
	if len(tools.Result.Tools) != 1 || tools.Result.Tools[0].Name != "list_rooms" {
		t.Fatalf("tools = %#v, want list_rooms", tools.Result.Tools)
	}

	call := performRawMCPRequest(t, handler, bot.APIKey, "tools/call", "list_rooms", `{
		"jsonrpc":"2.0",
		"id":3,
		"method":"tools/call",
		"params":{
			"_meta":{
				"io.modelcontextprotocol/protocolVersion":"2026-07-28",
				"io.modelcontextprotocol/clientInfo":{"name":"raw-http-test","version":"1.0"},
				"io.modelcontextprotocol/clientCapabilities":{}
			},
			"name":"list_rooms",
			"arguments":{"limit":10}
		}
	}`)
	var result struct {
		Result struct {
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	decodeMCPResponse(t, call, &result)
	var output listRoomsOutput
	if err := json.Unmarshal(result.Result.StructuredContent, &output); err != nil {
		t.Fatalf("decode list_rooms structured content: %v", err)
	}
	if !roomResultsContain(output.Rooms, room.GetId()) {
		t.Fatalf("visible room %q missing from %#v", room.GetId(), output.Rooms)
	}
}

func TestMCPHandlerRejectsWrongHostAndCrossOriginBrowserPOST(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	chattoCore, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "test-core-secret", Assets: config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	handler, err := NewHandler(chattoCore, config.ChattoConfig{
		Webserver: config.WebserverConfig{URL: "https://chat.example"},
		MCP:       config.MCPConfig{Enabled: true},
	}, "test")
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	wrongHost := httptest.NewRequest(http.MethodGet, "https://wrong.example/.well-known/oauth-protected-resource/mcp", nil)
	wrongHostResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongHostResponse, wrongHost)
	if wrongHostResponse.Code != http.StatusMisdirectedRequest {
		t.Fatalf("wrong Host status = %d, want %d", wrongHostResponse.Code, http.StatusMisdirectedRequest)
	}

	crossOrigin := httptest.NewRequest(http.MethodPost, "https://chat.example/mcp", nil)
	crossOrigin.Header.Set("Origin", "https://evil.example")
	crossOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want %d", crossOriginResponse.Code, http.StatusForbidden)
	}
}

func TestMCPHandlerRejectsUnscopedHumanBearer(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	chattoCore, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "test-core-secret", Assets: config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startTestCore(t, chattoCore)
	ctx := context.Background()
	viewer, err := chattoCore.CreateUser(ctx, core.SystemActorID, "unscoped-viewer", "Unscoped Viewer", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	credentials, err := chattoCore.CreateBearerSessionWithSource(ctx, viewer.GetId(), "test")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	verifier := tokenVerifier(chattoCore, "https://chat.example/mcp")
	if _, err := verifier(ctx, credentials.AccessToken, nil); err == nil {
		t.Fatal("unscoped first-party bearer was accepted")
	}
}

func TestMCPTokenVerifierAcceptsCurrentBotAPIKey(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	chattoCore, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "test-core-secret", Assets: config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	startTestCore(t, chattoCore)
	ctx := context.Background()
	owner, err := chattoCore.CreateUser(ctx, core.SystemActorID, "mcp-bot-owner", "MCP Bot Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := chattoCore.CreateBot(ctx, owner.GetId(), "mcp_bot", "MCP Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	token, err := tokenVerifier(chattoCore, "https://chat.example/mcp")(ctx, bot.APIKey, nil)
	if err != nil {
		t.Fatalf("verify bot API key: %v", err)
	}
	if token.UserID != bot.User.GetId() || !slices.Contains(token.Scopes, config.MCPRoomsReadScope) {
		t.Fatalf("bot token info = %#v", token)
	}
}

func startTestCore(t *testing.T, chattoCore *core.ChattoCore) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- chattoCore.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("ChattoCore did not stop")
		}
	})
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := chattoCore.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

func roomResultsContain(rooms []roomResult, roomID string) bool {
	for _, room := range rooms {
		if room.ID == roomID {
			return true
		}
	}
	return false
}

func performRawMCPRequest(t *testing.T, handler http.Handler, token, method, name, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := newRawMCPRequest(token, method, name, body)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200: %s", method, response.Code, response.Body.String())
	}
	return response
}

func newRawMCPRequest(token, method, name, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "https://chat.example/mcp", bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("MCP-Protocol-Version", "2026-07-28")
	req.Header.Set("MCP-Method", method)
	if name != "" {
		req.Header.Set("MCP-Name", name)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	return req
}

func decodeMCPResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode MCP response: %v: %s", err, response.Body.String())
	}
}
