package mcpserver

import (
	"context"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPMessageTools exercises direct delivery and message corrections through
// the official SDK and the production authentication and authorization gates.
func TestMCPMessageTools(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	c, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{SecretKey: "message-tools-test", Assets: config.AssetsConfig{SigningSecret: "message-tools-assets"}})
	if err != nil {
		t.Fatal(err)
	}
	startTestCore(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	viewer, err := c.CreateUser(ctx, core.SystemActorID, "message-tools-viewer", "Message Tools Viewer", "password")
	if err != nil {
		t.Fatal(err)
	}
	actor := viewer.Id
	generation, err := c.CurrentAuthGeneration(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := c.CreateOAuthBearerSessionForClientGrant(ctx, actor, "https://client.example/metadata.json", "https://chat.example/mcp", config.MCPOAuthScopes(), generation)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(c, config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://chat.example"}, MCP: config.MCPConfig{Enabled: true}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	client.Transport = canonicalHostTransport{base: client.Transport, host: "chat.example"}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "message-test", Version: "test"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", DisableStandaloneSSE: true, HTTPClient: client, OAuthHandler: staticOAuthHandler{token: grant.AccessToken}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	recipient, err := c.CreateUser(ctx, core.SystemActorID, "mcp-dm-recipient", "MCP DM Recipient", "password")
	if err != nil {
		t.Fatal(err)
	}
	call := func(name string, args any, wantError bool) *mcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || result.IsError != wantError {
			t.Fatalf("%s: err=%v result=%v", name, err, result)
		}
		return result
	}
	args := map[string]any{"user_id": recipient.Id, "body": "Direct hello", "idempotency_key": "450f1a5d-c2cf-42ad-84aa-d88110956711"}
	var sent, retried, second postMessageOutput
	decodeStructuredContent(t, call("send_direct_message", args, false).StructuredContent, &sent)
	decodeStructuredContent(t, call("send_direct_message", args, false).StructuredContent, &retried)
	if sent.Message.ID == "" || sent.Message.RoomID == "" || retried.Message.ID != sent.Message.ID {
		t.Fatal("DM receipt or exact retry failed")
	}
	args["body"] = "Changed intent"
	call("send_direct_message", args, true)
	args["user_id"] = actor
	call("send_direct_message", args, true)
	decodeStructuredContent(t, call("send_direct_message", map[string]any{"user_id": recipient.Id, "body": "Another message"}, false).StructuredContent, &second)
	if second.Message.RoomID != sent.Message.RoomID || second.Message.ID == sent.Message.ID {
		t.Fatal("DM was not reused")
	}
	var reply postMessageOutput
	decodeStructuredContent(t, call("post_message", map[string]any{"room_id": sent.Message.RoomID, "thread_root_message_id": sent.Message.ID, "body": "DM thread"}, false).StructuredContent, &reply)
	target := map[string]any{"room_id": sent.Message.RoomID, "message_id": reply.Message.ID}
	call("add_reaction", map[string]any{"room_id": sent.Message.RoomID, "message_id": reply.Message.ID, "emoji": "heart"}, false)
	var read getMessageOutput
	decodeStructuredContent(t, call("get_message", target, false).StructuredContent, &read)
	if read.Message.Body != "DM thread" || read.Message.ThreadRootMessageID != sent.Message.ID || len(read.Reactions) != 1 || read.Reactions[0].Count != 1 || !read.Reactions[0].Reacted {
		t.Fatal("message read lost body, thread, or reactions")
	}
	var edited postMessageOutput
	decodeStructuredContent(t, call("update_message", map[string]any{"room_id": sent.Message.RoomID, "message_id": reply.Message.ID, "body": "Corrected"}, false).StructuredContent, &edited)
	if edited.Message.Body != "Corrected" || edited.Message.ThreadRootMessageID != sent.Message.ID {
		t.Fatal("edit lost body or thread")
	}
	decodeStructuredContent(t, call("get_message", target, false).StructuredContent, &read)
	if read.Message.Body != "Corrected" {
		t.Fatal("read did not observe edit")
	}
	other, err := c.Messages().PostMessage(ctx, core.MessagePostInput{ActorID: recipient.Id, RoomID: sent.Message.RoomID, Body: "Recipient message"})
	if err != nil {
		t.Fatal(err)
	}
	call("update_message", map[string]any{"room_id": sent.Message.RoomID, "message_id": other.Event.Id, "body": "Unauthorized"}, true)
	call("delete_message", map[string]any{"room_id": sent.Message.RoomID, "message_id": other.Event.Id}, true)
	call("delete_message", target, false)
	call("get_message", target, true)
	privateRoom, _, err := c.RoomCommands().StartDM(ctx, core.RoomStartDMInput{ActorID: recipient.Id, ParticipantIDs: []string{recipient.Id}})
	if err != nil {
		t.Fatal(err)
	}
	privateMessage, err := c.Messages().PostMessage(ctx, core.MessagePostInput{ActorID: recipient.Id, RoomID: privateRoom.Id, Body: "Private note"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"get_message", "update_message", "delete_message"} {
		args := map[string]any{"room_id": privateRoom.Id, "message_id": privateMessage.Event.Id}
		if name == "update_message" {
			args["body"] = "Hidden"
		}
		result := call(name, args, true)
		var failure toolErrorOutput
		decodeStructuredContent(t, result.StructuredContent, &failure)
		if failure.Error.Code != "not_found" {
			t.Fatalf("%s revealed private room: %s", name, failure.Error.Code)
		}
	}
	for _, name := range []string{"get_message", "update_message", "delete_message"} {
		args := map[string]any{"room_id": "missing-room", "message_id": other.Event.Id}
		if name == "update_message" {
			args["body"] = "Hidden"
		}
		call(name, args, true)
	}
}
