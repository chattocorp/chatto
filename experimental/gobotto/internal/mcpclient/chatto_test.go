package mcpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chatto-chat/chatto/experimental/gobotto/internal/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestChattoDiscoveryAndCrossRoomDelivery(t *testing.T) {
	remote := mcp.NewServer(&mcp.Implementation{Name: "Chatto", Version: "test"}, &mcp.ServerOptions{Instructions: "Use Markdown and mermaid rendered fences."})
	var calls []map[string]string
	var unsupportedCalls int
	remote.AddTool(&mcp.Tool{Name: "post_message", Description: "Post a message", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"thread_root_message_id": map[string]any{"type": "string"}}, "additionalProperties": true}}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]string
		if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
			t.Error(err)
		}
		calls = append(calls, args)
		return &mcp.CallToolResult{StructuredContent: map[string]any{"message": map[string]string{"id": "posted", "roomId": args["room_id"]}}}, nil
	})
	remote.AddTool(&mcp.Tool{Name: "add_reaction", InputSchema: map[string]any{"type": "object", "additionalProperties": true}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{StructuredContent: map[string]any{"messageId": "target", "changed": true}}, nil
	})
	remote.AddTool(&mcp.Tool{Name: "remove_reaction", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		unsupportedCalls++
		return &mcp.CallToolResult{StructuredContent: map[string]any{"messageId": "target"}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remote }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || r.Header.Get("Authorization") != "Bearer synthetic-secret" {
			t.Error("wrong endpoint or credential")
		}
		handler.ServeHTTP(w, r)
	}))
	defer host.Close()
	manager, err := Open(context.Background(), config.MCP{}, ChattoConnection{URL: host.URL, Token: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if len(manager.tools) != 3 {
		t.Fatal("automatic catalog missing tools")
	}
	if err := manager.Deliver(context.Background(), "remove_reaction", map[string]string{}); err == nil || unsupportedCalls != 0 {
		t.Fatal("host delivery dispatched an unsupported operation")
	}
	discovery, _ := manager.discover(context.Background(), json.RawMessage(`{"name":"mcp_chatto__post_message"}`))
	if !strings.Contains(discovery, "mermaid rendered") || strings.Contains(discovery, "synthetic-secret") {
		t.Fatal("server guidance missing or secret exposed")
	}
	result, _ := manager.dispatch(context.Background(), json.RawMessage(`{"name":"mcp_chatto__post_message","arguments":{"room_id":"other-room","thread_root_message_id":"other-thread","in_reply_to_message_id":"other-message","body":"Requested cross-room post"}}`))
	if !strings.Contains(result, `"status":"complete"`) {
		t.Fatal("cross-room tool failed", result)
	}
	if err := manager.Deliver(context.Background(), "post_message", map[string]string{"room_id": "current-room", "body": "Default reply"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Deliver(context.Background(), "add_reaction", map[string]string{"room_id": "other-room", "message_id": "target", "emoji": "heart"}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0]["room_id"] != "other-room" || calls[0]["thread_root_message_id"] != "other-thread" || calls[0]["in_reply_to_message_id"] != "other-message" || calls[1]["room_id"] != "current-room" {
		t.Fatal("destination arguments changed")
	}
	// A same-size catalog replacement must not pass reconnect validation.
	remote.RemoveTools("post_message")
	remote.AddTool(&mcp.Tool{Name: "replacement", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	manager.chatto.mu.Lock()
	_ = manager.chatto.session.Close()
	manager.chatto.session = nil
	manager.chatto.mu.Unlock()
	if err := manager.Deliver(context.Background(), "post_message", map[string]string{"room_id": "other-room", "body": "must not run"}); err == nil {
		t.Fatal("accepted changed catalog")
	}
	if len(calls) != 2 {
		t.Fatal("reconnect replayed a write")
	}

}
