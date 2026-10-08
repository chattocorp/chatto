package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hmans.de/chatto/internal/config"
)

// toolScope is the OAuth ceiling for a catalog tool. Identity tools need no
// additional scope after resource-bound authentication. The second result is
// false for an unmapped tool, so adding a tool cannot silently grant authority.
func toolScope(name string) (string, bool) {
	switch name {
	case "get_server_info", "get_current_user":
		return "", true
	case "list_rooms":
		return config.MCPRoomsReadScope, true
	case "list_room_messages":
		return config.MCPMessagesReadScope, true
	case "post_message":
		return config.MCPMessagesWriteScope, true
	case "join_room", "leave_room":
		return config.MCPRoomsWriteScope, true
	default:
		return "", false
	}
}

func hasToolScope(ctx context.Context, name string) bool {
	token := auth.TokenInfoFromContext(ctx)
	scope, known := toolScope(name)
	return known && token != nil && (scope == "" || slices.Contains(token.Scopes, scope))
}

// requireToolScope returns the OAuth transport challenge for a known tool.
// The SDK verifies that the protocol headers match the JSON request. The
// parsed-call guard below also enforces the ceiling before tool execution.
func requireToolScope(metadataURL string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.Header.Get("Mcp-Name")
		scope, known := toolScope(name)
		if r.Method == http.MethodPost && r.Header.Get("Mcp-Method") == "tools/call" && known && scope != "" && !hasToolScope(r.Context(), name) {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Bearer error=\"insufficient_scope\", scope=%q, resource_metadata=%q", scope, metadataURL))
			http.Error(w, "insufficient_scope: authorize the required MCP scope", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// scopedTools filters discovery by the authenticated grant and checks the
// parsed tool name independently of HTTP header hints. Scope-filtered lists
// cannot be cached publicly or retained through a credential change.
func scopedTools(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
		if call, ok := request.(*mcp.CallToolRequest); ok {
			scope, known := toolScope(call.Params.Name)
			if !known {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: fmt.Sprintf("unknown tool %q", call.Params.Name)}
			}
			if !hasToolScope(ctx, call.Params.Name) {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{
					Text: fmt.Sprintf("insufficient_scope: authorize MCP scope %q", scope),
				}}}, nil
			}
		}
		result, err := next(ctx, method, request)
		if list, ok := result.(*mcp.ListToolsResult); ok && err == nil {
			list.Tools = slices.DeleteFunc(list.Tools, func(tool *mcp.Tool) bool { return !hasToolScope(ctx, tool.Name) })
			list.CacheScope = "private"
			list.TTLMs = 0
		}
		return result, err
	}
}
