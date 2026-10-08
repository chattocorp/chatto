package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/testutil"
)

// Check the parsed-call guard without the SDK's HTTP header validation. A
// future tool must not inherit the identity exemption if its policy is absent.
func TestMCPParsedScopeGuard(t *testing.T) {
	var ctx context.Context
	protected := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{Scopes: []string{config.MCPRoomsReadScope}}, nil
	}, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	}))
	request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer scope-test")
	protected.ServeHTTP(httptest.NewRecorder(), request)
	if ctx == nil {
		t.Fatal("test credential was not authenticated")
	}
	for _, test := range []struct {
		name    string
		allowed bool
	}{
		{"get_server_info", true}, {"get_current_user", true}, {"list_rooms", true},
		{"post_message", false}, {"new_unmapped_tool", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := scopedTools(func(context.Context, string, mcp.Request) (mcp.Result, error) {
				called = true
				return &mcp.CallToolResult{}, nil
			})
			result, err := handler(ctx, "tools/call", &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: test.name}})
			if called != test.allowed {
				t.Fatalf("parsed tool %q executed = %t, want %t", test.name, called, test.allowed)
			}
			if !test.allowed && err == nil && !result.(*mcp.CallToolResult).IsError {
				t.Fatal("denied call had no protocol or tool error")
			}
			if test.name == "post_message" {
				var output toolErrorOutput
				decodeStructuredContent(t, result.(*mcp.CallToolResult).StructuredContent, &output)
				if output.Error.Code != "insufficient_scope" || output.Error.RequiredScope != config.MCPMessagesWriteScope || output.Error.NextAction != "authorize_scope" || output.Error.Outcome != "not_applied" || output.Error.Retry != "after_change" {
					t.Fatalf("parsed scope failure = %#v", output)
				}
			}
		})
	}
	handler := scopedTools(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "get_current_user"}, {Name: "new_unmapped_tool"}}}, nil
	})
	result, err := handler(ctx, "tools/list", &mcp.ListToolsRequest{})
	if err != nil || len(result.(*mcp.ListToolsResult).Tools) != 1 {
		t.Fatal("unmapped tool appeared in discovery")
	}
}

// TestMCPGrantSubsets exercises every supported combination through the wire
// protocol. Missing scope must fail before input or domain authorization.
func TestMCPGrantSubsets(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	c, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "scope-test-secret", Assets: config.AssetsConfig{SigningSecret: "scope-test-assets"},
	})
	if err != nil {
		t.Fatal(err)
	}
	startTestCore(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	user, err := c.CreateUser(ctx, core.SystemActorID, "scope-test-user", "Scope Test User", "password")
	if err != nil {
		t.Fatal(err)
	}
	generation, err := c.CurrentAuthGeneration(ctx, user.GetId())
	if err != nil {
		t.Fatal(err)
	}
	// Keep expected policy independent of the implementation's scope lookup.
	group, err := c.CreateRoomGroup(ctx, core.SystemActorID, "Scope Rooms", "")
	if err != nil {
		t.Fatal(err)
	}
	room, err := c.CreateRoom(ctx, core.SystemActorID, core.KindChannel, group.GetId(), "scope-room", "")
	if err != nil {
		t.Fatal(err)
	}
	tools := []struct{ name, scope string }{
		{"get_current_user", ""}, {"get_server_info", ""},
		{"join_room", config.MCPRoomsWriteScope},
		{"list_room_messages", config.MCPMessagesReadScope}, {"list_rooms", config.MCPRoomsReadScope},
		{"post_message", config.MCPMessagesWriteScope},
		{"leave_room", config.MCPRoomsWriteScope},
	}
	allScopes := config.MCPOAuthScopes()
	for mask := 1; mask < 1<<len(allScopes); mask++ {
		t.Run(fmt.Sprint(mask), func(t *testing.T) {
			if _, err := c.JoinRoom(ctx, user.GetId(), core.KindChannel, user.GetId(), room.GetId()); err != nil {
				t.Fatal(err)
			}
			var scopes []string
			for index, scope := range allScopes {
				if mask&(1<<index) != 0 {
					scopes = append(scopes, scope)
				}
			}
			grant, err := c.CreateOAuthBearerSessionForClientGrant(ctx, user.GetId(), "https://client.example/metadata.json", "https://chat.example/mcp", scopes, generation)
			if err != nil {
				t.Fatal(err)
			}
			handler, err := NewHandler(c, config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://chat.example"}, MCP: config.MCPConfig{Enabled: true}}, "scope-test")
			if err != nil {
				t.Fatal(err)
			}
			request := func(method, name string, args map[string]any) *httptest.ResponseRecorder {
				t.Helper()
				params := map[string]any{"_meta": map[string]any{
					"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{},
				}}
				if name != "" {
					params["name"], params["arguments"] = name, args
				}
				body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
				if err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, newRawMCPRequest(grant.AccessToken, method, name, string(body)))
				return response
			}
			if response := request("server/discover", "", nil); response.Code != http.StatusOK {
				t.Fatalf("discovery status = %d", response.Code)
			}
			var listed struct {
				Result mcp.ListToolsResult `json:"result"`
			}
			response := request("tools/list", "", nil)
			decodeMCPResponse(t, response, &listed)
			var expected, actual []string
			for _, tool := range tools {
				if tool.scope == "" || slices.Contains(scopes, tool.scope) {
					expected = append(expected, tool.name)
				}
			}
			for _, tool := range listed.Result.Tools {
				actual = append(actual, tool.Name)
			}
			slices.Sort(expected)
			if response.Code != http.StatusOK || !slices.Equal(actual, expected) || listed.Result.CacheScope != "private" || listed.Result.TTLMs != 0 {
				t.Fatalf("filtered catalog = %v, expected %v with private zero-TTL caching", actual, expected)
			}
			for _, tool := range tools {
				args := map[string]any{}
				if tool.scope != "" && tool.name != "list_rooms" {
					args["room_id"] = room.GetId()
				}
				if tool.name == "post_message" {
					args["body"] = "Scope test"
				}
				response := request("tools/call", tool.name, args)
				if tool.scope != "" && !slices.Contains(scopes, tool.scope) {
					challenge := response.Header().Get("WWW-Authenticate")
					if response.Code != http.StatusForbidden || !strings.Contains(challenge, `error="insufficient_scope"`) || !strings.Contains(challenge, fmt.Sprintf("scope=%q", tool.scope)) || !strings.Contains(challenge, `resource_metadata="https://chat.example/.well-known/oauth-protected-resource/mcp"`) {
						t.Fatalf("%s missing-scope status/challenge = %d/%q", tool.name, response.Code, challenge)
					}
					var output toolErrorOutput
					decodeMCPResponse(t, response, &output)
					if output.Error.Code != "insufficient_scope" || output.Error.RequiredScope != tool.scope || output.Error.NextAction != "authorize_scope" || output.Error.Outcome != "not_applied" || output.Error.Retry != "after_change" || response.Header().Get("Cache-Control") != "no-store" {
						t.Fatalf("HTTP scope failure = %#v", output)
					}
				} else if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "insufficient_scope") {
					t.Fatalf("%s with granted scope failed: status %d", tool.name, response.Code)
				} else {
					var result struct {
						Result mcp.CallToolResult `json:"result"`
					}
					decodeMCPResponse(t, response, &result)
					if result.Result.IsError {
						t.Fatalf("%s with granted scope failed domain authorization: %v", tool.name, result.Result.Content)
					}
				}
			}

			if !slices.Contains(scopes, config.MCPMessagesWriteScope) {
				body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"post_message","arguments":{"room_id":"missing-room","body":"scope bypass"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
				for _, headers := range [][2]string{{"tools/call", "get_current_user"}, {"server/discover", ""}, {"", ""}} {
					req := newRawMCPRequest(grant.AccessToken, headers[0], headers[1], body)
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, req)
					if response.Code != http.StatusBadRequest {
						t.Fatalf("spoofed/missing headers status = %d, want 400", response.Code)
					}
				}
			}
		})
	}
	t.Run("scope does not grant RBAC permission", func(t *testing.T) {
		grant, err := c.CreateOAuthBearerSessionForClientGrant(ctx, user.GetId(), "https://client.example/metadata.json", "https://chat.example/mcp", []string{config.MCPRoomsWriteScope}, generation)
		if err != nil {
			t.Fatal(err)
		}
		if err := c.DenyUserRoomPermission(ctx, core.SystemActorID, room.GetId(), user.GetId(), core.PermRoomJoin); err != nil {
			t.Fatal(err)
		}
		handler, err := NewHandler(c, config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://chat.example"}, MCP: config.MCPConfig{Enabled: true}}, "scope-test")
		if err != nil {
			t.Fatal(err)
		}
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"join_room","arguments":{"room_id":"` + room.GetId() + `"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`
		response := performRawMCPRequest(t, handler, grant.AccessToken, "tools/call", "join_room", body)
		var result struct {
			Result mcp.CallToolResult `json:"result"`
		}
		decodeMCPResponse(t, response, &result)
		if !result.Result.IsError || !strings.Contains(response.Body.String(), "permission_denied") || !strings.Contains(response.Body.String(), string(core.PermRoomJoin)) || response.Header().Get("WWW-Authenticate") != "" {
			t.Fatal("scope-present RBAC denial did not remain a distinct tool error")
		}
	})
}
