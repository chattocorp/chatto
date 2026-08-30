package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
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
	secondVisible, err := chattoCore.CreateRoom(ctx, core.SystemActorID, core.KindChannel, group.GetId(), "also-visible-to-mcp", "")
	if err != nil {
		t.Fatalf("CreateRoom second visible: %v", err)
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
	const resource = "https://alias.example/mcp"
	credentials, err := chattoCore.CreateOAuthBearerSessionForClientGrant(ctx, viewer.GetId(), "https://agent.example/client.json", resource, config.MCPOAuthScopes(), generation)
	if err != nil {
		t.Fatalf("CreateOAuthBearerSessionForClientGrant: %v", err)
	}
	handler, err := NewHandler(chattoCore, config.ChattoConfig{
		Webserver: config.WebserverConfig{
			URL:            "https://chat.example",
			AllowedOrigins: []string{"https://alias.example"},
		},
		MCP: config.MCPConfig{Enabled: true},
	}, "test")
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	if err := chattoCore.ConfigModel().SetServerConfig(ctx, core.SystemActorID, &configv1.ServerConfig{ServerName: "Engineering Chat"}); err != nil {
		t.Fatalf("SetServerConfig: %v", err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	httpClient := server.Client()
	httpClient.Transport = canonicalHostTransport{base: httpClient.Transport, host: "alias.example"}

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
	initialize := session.InitializeResult()
	if initialize == nil || initialize.ServerInfo == nil || initialize.ServerInfo.Name != "Chatto" || initialize.ServerInfo.Title != "Engineering Chat" {
		t.Fatalf("server info = %#v, want stable Chatto name and Engineering Chat title", initialize)
	}
	if initialize.Instructions != serverInstructions {
		t.Fatalf("instructions = %q, want %q", initialize.Instructions, serverInstructions)
	}
	if !strings.Contains(initialize.Instructions, "server title advertised by this MCP connection") {
		t.Fatalf("instructions = %q, want advertised-title routing guidance", initialize.Instructions)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	wantTools := []string{"get_server_info", "get_current_user", "list_rooms", "list_room_messages", "post_message", "join_room", "leave_room"}
	if len(tools.Tools) != len(wantTools) {
		t.Fatalf("tools = %#v, want %v", tools.Tools, wantTools)
	}
	for _, name := range wantTools {
		if !mcpToolsContain(tools.Tools, name) {
			t.Fatalf("tools = %#v, missing %q", tools.Tools, name)
		}
	}
	wantDescriptionParts := map[string][]string{
		"get_server_info":    {"one Chatto server connected through this MCP endpoint", "match a server that the user names"},
		"get_current_user":   {"this MCP connection uses", "connected server"},
		"list_rooms":         {"source of truth for room lists and room counts", "totalCount", "nextAfterRoomId"},
		"list_room_messages": {"connected Chatto server", "nextBeforeEventId"},
		"post_message":       {"connected Chatto server", "not idempotent"},
		"join_room":          {"connected Chatto server"},
		"leave_room":         {"connected Chatto server"},
	}
	for name, parts := range wantDescriptionParts {
		tool := mcpToolNamed(tools.Tools, name)
		if tool == nil {
			t.Fatalf("tools = %#v, missing %q", tools.Tools, name)
		}
		for _, part := range parts {
			if !strings.Contains(tool.Description, part) {
				t.Fatalf("%s description = %q, want it to contain %q", name, tool.Description, part)
			}
		}
	}
	serverInfoResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_server_info"})
	if err != nil {
		t.Fatalf("CallTool get_server_info: %v", err)
	}
	serverInfoRaw, err := json.Marshal(serverInfoResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal get_server_info structured content: %v", err)
	}
	var serverInfo getServerInfoOutput
	if err := json.Unmarshal(serverInfoRaw, &serverInfo); err != nil {
		t.Fatalf("decode get_server_info output: %v", err)
	}
	if serverInfo.ServerName != "Engineering Chat" || serverInfo.ServerURL != "https://chat.example" || serverInfo.MCPURL != resource || serverInfo.SoftwareVersion != "test" {
		t.Fatalf("get_server_info = %#v", serverInfo)
	}

	canonicalRequest := newRawMCPRequest(credentials.AccessToken, "tools/list", "", `{
		"jsonrpc":"2.0",
		"id":99,
		"method":"tools/list",
		"params":{"_meta":{
			"io.modelcontextprotocol/protocolVersion":"2026-07-28",
			"io.modelcontextprotocol/clientInfo":{"name":"resource-isolation-test","version":"1.0"},
			"io.modelcontextprotocol/clientCapabilities":{}
		}}
	}`)
	canonicalResponse := httptest.NewRecorder()
	handler.ServeHTTP(canonicalResponse, canonicalRequest)
	if canonicalResponse.Code != http.StatusUnauthorized {
		t.Fatalf("alias token on canonical resource status = %d, want %d", canonicalResponse.Code, http.StatusUnauthorized)
	}
	currentUserResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_current_user"})
	if err != nil {
		t.Fatalf("CallTool get_current_user: %v", err)
	}
	var currentUser getCurrentUserOutput
	decodeStructuredContent(t, currentUserResult.StructuredContent, &currentUser)
	if currentUser.ID != viewer.GetId() || currentUser.DisplayName != viewer.GetDisplayName() || currentUser.AccountType != "human" {
		t.Fatalf("get_current_user = %#v", currentUser)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_rooms", Arguments: map[string]any{"limit": 1}})
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
	if output.ServerName != "Engineering Chat" {
		t.Fatalf("server name = %q, want Engineering Chat", output.ServerName)
	}
	if output.TotalCount != 2 {
		t.Fatalf("total count = %d, want 2 visible rooms", output.TotalCount)
	}
	if len(output.Rooms) != 1 || output.NextAfterRoomID == "" {
		t.Fatalf("first room page = %#v, want one room and a continuation", output)
	}
	secondPageResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_rooms", Arguments: map[string]any{
		"limit": 1, "after_room_id": output.NextAfterRoomID,
	}})
	if err != nil {
		t.Fatalf("CallTool list_rooms second page: %v", err)
	}
	var secondPage listRoomsOutput
	decodeStructuredContent(t, secondPageResult.StructuredContent, &secondPage)
	if secondPage.TotalCount != 2 || len(secondPage.Rooms) != 1 || secondPage.NextAfterRoomID != "" {
		t.Fatalf("second room page = %#v, want final page with total count 2", secondPage)
	}
	allRooms := append(slices.Clone(output.Rooms), secondPage.Rooms...)
	for _, visibleRoomID := range []string{visible.GetId(), secondVisible.GetId()} {
		if !roomResultsContain(allRooms, visibleRoomID) {
			t.Fatalf("visible room %q missing from %#v", visibleRoomID, allRooms)
		}
	}
	if roomResultsContain(allRooms, hidden.GetId()) {
		t.Fatalf("hidden room %q present in %#v", hidden.GetId(), allRooms)
	}

	joinResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "join_room", Arguments: map[string]any{"room_id": visible.GetId()}})
	if err != nil || joinResult.IsError {
		t.Fatalf("CallTool join_room: result=%#v err=%v", joinResult, err)
	}
	var joined joinRoomOutput
	decodeStructuredContent(t, joinResult.StructuredContent, &joined)
	if joined.Room.ID != visible.GetId() || !joined.Room.IsMember {
		t.Fatalf("join_room = %#v", joined)
	}

	postResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "post_message", Arguments: map[string]any{
		"room_id": visible.GetId(), "body": "Hello from MCP",
	}})
	if err != nil || postResult.IsError {
		t.Fatalf("CallTool post_message: result=%#v err=%v", postResult, err)
	}
	var posted postMessageOutput
	decodeStructuredContent(t, postResult.StructuredContent, &posted)
	if posted.Message.RoomID != visible.GetId() || posted.Message.AuthorID != viewer.GetId() || posted.Message.Body != "Hello from MCP" || posted.Message.ID == "" {
		t.Fatalf("post_message = %#v", posted)
	}

	messagesResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_room_messages", Arguments: map[string]any{
		"room_id": visible.GetId(), "limit": 10,
	}})
	if err != nil || messagesResult.IsError {
		t.Fatalf("CallTool list_room_messages: result=%#v err=%v", messagesResult, err)
	}
	var messages listRoomMessagesOutput
	decodeStructuredContent(t, messagesResult.StructuredContent, &messages)
	if len(messages.Messages) != 1 || messages.Messages[0].ID != posted.Message.ID || messages.Messages[0].Body != "Hello from MCP" {
		t.Fatalf("list_room_messages = %#v", messages)
	}

	leaveResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "leave_room", Arguments: map[string]any{"room_id": visible.GetId()}})
	if err != nil || leaveResult.IsError {
		t.Fatalf("CallTool leave_room: result=%#v err=%v", leaveResult, err)
	}
	var left leaveRoomOutput
	decodeStructuredContent(t, leaveResult.StructuredContent, &left)
	if left.RoomID != visible.GetId() || !left.Left {
		t.Fatalf("leave_room = %#v", left)
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
		Webserver: config.WebserverConfig{
			URL:            "https://chat.example",
			AllowedOrigins: []string{"https://alias.example"},
		},
		MCP: config.MCPConfig{Enabled: true},
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
	wantTools := []string{"get_server_info", "get_current_user", "list_rooms", "list_room_messages", "post_message", "join_room", "leave_room"}
	if len(tools.Result.Tools) != len(wantTools) {
		t.Fatalf("tools = %#v, want %v", tools.Result.Tools, wantTools)
	}
	for _, name := range wantTools {
		if !rawMCPToolsContain(tools.Result.Tools, name) {
			t.Fatalf("tools = %#v, missing %q", tools.Result.Tools, name)
		}
	}

	aliasList := newRawMCPRequest(bot.APIKey, "tools/list", "", `{
		"jsonrpc":"2.0",
		"id":20,
		"method":"tools/list",
		"params":{"_meta":{
			"io.modelcontextprotocol/protocolVersion":"2026-07-28",
			"io.modelcontextprotocol/clientInfo":{"name":"alias-bot-test","version":"1.0"},
			"io.modelcontextprotocol/clientCapabilities":{}
		}}
	}`)
	aliasList.Host = "alias.example"
	aliasListResponse := httptest.NewRecorder()
	handler.ServeHTTP(aliasListResponse, aliasList)
	if aliasListResponse.Code != http.StatusOK {
		t.Fatalf("bot API key on alias status = %d, want %d: %s", aliasListResponse.Code, http.StatusOK, aliasListResponse.Body.String())
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

	denied := performRawMCPRequest(t, handler, bot.APIKey, "tools/call", "join_room", `{
		"jsonrpc":"2.0",
		"id":4,
		"method":"tools/call",
		"params":{
			"_meta":{
				"io.modelcontextprotocol/protocolVersion":"2026-07-28",
				"io.modelcontextprotocol/clientInfo":{"name":"raw-http-test","version":"1.0"},
				"io.modelcontextprotocol/clientCapabilities":{}
			},
			"name":"join_room",
			"arguments":{"room_id":"`+room.GetId()+`"}
		}
	}`)
	var deniedResult struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	decodeMCPResponse(t, denied, &deniedResult)
	if !deniedResult.Result.IsError || len(deniedResult.Result.Content) == 0 ||
		!strings.Contains(deniedResult.Result.Content[0].Text, "permission_denied") ||
		!strings.Contains(deniedResult.Result.Content[0].Text, string(core.PermRoomJoin)) {
		t.Fatalf("join_room denial = %#v, want visible missing RBAC permission", deniedResult.Result)
	}
}

func TestMCPHandlerServesConfiguredAliasAndRejectsWrongHostAndCrossOriginBrowserPOST(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	chattoCore, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "test-core-secret", Assets: config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}
	handler, err := NewHandler(chattoCore, config.ChattoConfig{
		Webserver: config.WebserverConfig{
			URL:            "https://chat.example",
			AllowedOrigins: []string{"https://alias.example", "*"},
		},
		MCP: config.MCPConfig{Enabled: true},
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

	aliasMetadata := httptest.NewRequest(http.MethodGet, "https://alias.example/.well-known/oauth-protected-resource/mcp", nil)
	aliasMetadataResponse := httptest.NewRecorder()
	handler.ServeHTTP(aliasMetadataResponse, aliasMetadata)
	if aliasMetadataResponse.Code != http.StatusOK {
		t.Fatalf("alias metadata status = %d, want %d: %s", aliasMetadataResponse.Code, http.StatusOK, aliasMetadataResponse.Body.String())
	}
	var metadata struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	if err := json.Unmarshal(aliasMetadataResponse.Body.Bytes(), &metadata); err != nil {
		t.Fatalf("decode alias metadata: %v", err)
	}
	if metadata.Resource != "https://alias.example/mcp" || !slices.Equal(metadata.AuthorizationServers, []string{"https://chat.example"}) {
		t.Fatalf("alias metadata = %#v", metadata)
	}

	aliasMCP := httptest.NewRequest(http.MethodPost, "https://alias.example/mcp", nil)
	aliasMCPResponse := httptest.NewRecorder()
	handler.ServeHTTP(aliasMCPResponse, aliasMCP)
	if aliasMCPResponse.Code != http.StatusUnauthorized || !strings.Contains(aliasMCPResponse.Header().Get("WWW-Authenticate"), "https://alias.example/.well-known/oauth-protected-resource/mcp") {
		t.Fatalf("alias MCP status/challenge = %d/%q, want alias-specific 401 challenge", aliasMCPResponse.Code, aliasMCPResponse.Header().Get("WWW-Authenticate"))
	}

	crossOrigin := httptest.NewRequest(http.MethodPost, "https://chat.example/mcp", nil)
	crossOrigin.Header.Set("Origin", "https://evil.example")
	crossOriginResponse := httptest.NewRecorder()
	handler.ServeHTTP(crossOriginResponse, crossOrigin)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want %d", crossOriginResponse.Code, http.StatusForbidden)
	}
}

func TestMCPHandlerRejectsHumanBearerWithoutCompleteMCPGrant(t *testing.T) {
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
	generation, err := chattoCore.CurrentAuthGeneration(ctx, viewer.GetId())
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	legacyGrant, err := chattoCore.CreateOAuthBearerSessionForClientGrant(ctx, viewer.GetId(), "https://agent.example/client.json", "https://chat.example/mcp", []string{config.MCPRoomsReadScope}, generation)
	if err != nil {
		t.Fatalf("CreateOAuthBearerSessionForClientGrant: %v", err)
	}
	if _, err := verifier(ctx, legacyGrant.AccessToken, nil); err == nil {
		t.Fatal("resource-bound bearer without the complete current MCP grant was accepted")
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
	if token.UserID != bot.User.GetId() || !slices.Equal(token.Scopes, config.MCPOAuthScopes()) {
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

func mcpToolsContain(tools []*mcp.Tool, name string) bool {
	return mcpToolNamed(tools, name) != nil
}

func mcpToolNamed(tools []*mcp.Tool, name string) *mcp.Tool {
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

func decodeStructuredContent(t *testing.T, content any, output any) {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, output); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
}

func rawMCPToolsContain(tools []struct {
	Name string `json:"name"`
}, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
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
