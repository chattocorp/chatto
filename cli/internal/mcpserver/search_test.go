package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/connectapi"
	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	searchsvc "github.com/chattocorp/chatto/cli/internal/search"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// The provider supplies untrusted hits; the real API must authorize and hydrate
// them before the MCP adapter can expose their bodies.
type mcpSearchProvider struct {
	query func(*searchv1.QueryRequest) (*searchv1.QueryResponse, error)
}

func (p *mcpSearchProvider) Query(_ context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	return p.query(request)
}

func (*mcpSearchProvider) GetStatus(context.Context) (*searchv1.GetStatusResponse, error) {
	return &searchv1.GetStatusResponse{State: searchv1.ProviderState_PROVIDER_STATE_READY}, nil
}

func TestMCPSearchMessages(t *testing.T) {
	_, nc := testutil.StartSharedNATS(t)
	c, err := core.NewChattoCore(context.Background(), nc, config.CoreConfig{
		SecretKey: "search-test-secret", Assets: config.AssetsConfig{SigningSecret: "search-test-assets"},
	})
	require.NoError(t, err)
	startTestCore(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	viewer, err := c.CreateUser(ctx, core.SystemActorID, "search-viewer", "Search Viewer", "password")
	require.NoError(t, err)
	other, err := c.CreateUser(ctx, core.SystemActorID, "search-other", "Search Other", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, core.SystemActorID, core.KindChannel, "", "search-room", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, viewer.Id, core.KindChannel, viewer.Id, room.Id)
	require.NoError(t, err)
	hidden, err := c.CreateRoom(ctx, core.SystemActorID, core.KindChannel, "", "hidden-search-room", "")
	require.NoError(t, err)
	dm, _, err := c.FindOrCreateDM(ctx, viewer.Id, []string{other.Id})
	require.NoError(t, err)
	privateDM, _, err := c.FindOrCreateDM(ctx, other.Id, []string{core.SystemActorID})
	require.NoError(t, err)
	post := func(kind core.RoomKind, roomID, body string) *searchv1.QueryHit {
		t.Helper()
		message, err := c.PostMessage(ctx, kind, roomID, viewer.Id, body, nil, "", "", nil, false)
		require.NoError(t, err)
		events, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(roomID).Subject(evtstream.EventMessageBody))
		require.NoError(t, err)
		for _, event := range events {
			if bodyEvent := event.GetMessageBody(); bodyEvent.GetEventId() == message.Id && bodyEvent.GetBody() != nil {
				return &searchv1.QueryHit{RoomId: roomID, MessageId: message.Id, BodyEventId: bodyEvent.GetBody().GetBodyEventId(), RelevanceScore: 2.5}
			}
		}
		t.Fatal("message body event not found")
		return nil
	}
	channelHit := post(core.KindChannel, room.Id, "searchable channel body")
	dmHit := post(core.KindDM, dm.Id, "searchable DM body")
	deletedHit := post(core.KindChannel, room.Id, "deleted body")
	require.NoError(t, c.DeleteMessage(ctx, viewer.Id, core.KindChannel, room.Id, deletedHit.MessageId))
	var queries []*searchv1.QueryRequest
	provider := &mcpSearchProvider{query: func(request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		queries = append(queries, request)
		return &searchv1.QueryResponse{Hits: []*searchv1.QueryHit{
			channelHit, dmHit, deletedHit,
			{RoomId: hidden.Id, MessageId: "hidden-message", BodyEventId: "hidden-body"},
			{RoomId: privateDM.Id, MessageId: "private-dm-message", BodyEventId: "private-dm-body"},
		}, NextCursor: []byte("private-provider-cursor")}, nil
	}}
	cfg := config.ChattoConfig{Webserver: config.WebserverConfig{URL: "https://chat.example"}}
	cfg.Search.Enabled = true
	api := connectapi.New(c, cfg, "test", connectapi.WithMessageSearchProviderClient(provider))
	handler, err := NewHandler(c, cfg, "test", WithMessageSearch(api))
	require.NoError(t, err)
	grant := func(userID string, scopes ...string) string {
		t.Helper()
		generation, err := c.CurrentAuthGeneration(ctx, userID)
		require.NoError(t, err)
		credentials, err := c.CreateOAuthBearerSessionForClientGrant(ctx, userID, "https://agent.example/client.json", "https://chat.example/mcp", scopes, generation)
		require.NoError(t, err)
		return credentials.AccessToken
	}
	token := grant(viewer.Id, config.MCPMessagesReadScope)
	call := func(token string, input searchMessagesInput) *mcp.CallToolResult {
		t.Helper()
		response := performRawMCPRequest(t, handler, token, "tools/call", "search_messages", rawToolCallBody(t, "search_messages", input))
		require.Equal(t, http.StatusOK, response.Code)
		var wire struct {
			Result *mcp.CallToolResult `json:"result"`
		}
		decodeMCPResponse(t, response, &wire)
		require.NotNil(t, wire.Result)
		return wire.Result
	}
	list := func(handler http.Handler, token string) *mcp.ListToolsResult {
		t.Helper()
		response := performRawMCPRequest(t, handler, token, "tools/list", "", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`)
		var wire struct {
			Result *mcp.ListToolsResult `json:"result"`
		}
		decodeMCPResponse(t, response, &wire)
		require.NotNil(t, wire.Result)
		return wire.Result
	}
	catalog := list(handler, token)
	var searchTool *mcp.Tool
	for _, tool := range catalog.Tools {
		if tool.Name == "search_messages" {
			searchTool = tool
		}
	}
	require.NotNil(t, searchTool)
	require.True(t, searchTool.Annotations.ReadOnlyHint)
	require.True(t, searchTool.Annotations.IdempotentHint)

	input := searchMessagesInput{Query: `searchable AND "body" after:2025-01-01`, Limit: 5, Order: "newest"}
	result := call(token, input)
	require.False(t, result.IsError)
	var output searchMessagesOutput
	decodeStructuredContent(t, result.StructuredContent, &output)
	require.Len(t, output.Results, 2)
	require.Equal(t, "searchable channel body", output.Results[0].Message.Body)
	require.Equal(t, "Search Viewer", output.Results[0].Message.AuthorDisplayName)
	require.Equal(t, channelHit.MessageId, output.Results[0].Message.ID)
	require.Equal(t, 2.5, output.Results[0].RelevanceScore)
	require.Equal(t, dm.Id, output.Results[1].Message.RoomID)
	require.ElementsMatch(t, []string{room.Id, dm.Id}, queries[0].RoomIds)
	require.Equal(t, uint32(5), queries[0].PageSize)
	require.Equal(t, searchv1.SearchOrder_SEARCH_ORDER_NEWEST, queries[0].Order)
	require.NotEmpty(t, output.NextCursor)
	require.NotContains(t, output.NextCursor, "private-provider-cursor")
	input.Cursor = output.NextCursor
	require.False(t, call(token, input).IsError)
	require.Equal(t, []byte("private-provider-cursor"), queries[1].Cursor)

	t.Run("filters and empty pages retain continuation", func(t *testing.T) {
		original := provider.query
		defer func() { provider.query = original }()
		provider.query = func(request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
			queries = append(queries, request)
			return &searchv1.QueryResponse{Hits: []*searchv1.QueryHit{deletedHit}, NextCursor: []byte("next-page")}, nil
		}
		filtered := searchMessagesInput{Query: "searchable", RoomID: room.Id, AuthorID: other.Id}
		result := call(token, filtered)
		require.False(t, result.IsError)
		var output searchMessagesOutput
		decodeStructuredContent(t, result.StructuredContent, &output)
		require.Empty(t, output.Results)
		require.NotEmpty(t, output.NextCursor)
		require.Equal(t, []string{room.Id}, queries[len(queries)-1].RoomIds)
		require.Equal(t, []string{other.Id}, queries[len(queries)-1].AuthorIds)
	})
	t.Run("invalid arguments and cursor binding", func(t *testing.T) {
		for _, invalid := range []searchMessagesInput{
			{Query: ""}, {Query: strings.Repeat("x", 1025)}, {Query: "searchable", Limit: -1},
			{Query: "searchable", Limit: 101}, {Query: "searchable", Order: "oldest"},
			{Query: `"unclosed`}, {Query: "different", Cursor: input.Cursor},
			{Query: "searchable", Cursor: strings.Repeat("x", 8193)},
		} {
			f := assertToolFailure(t, call(token, invalid), catalog, "search_messages")
			require.Equal(t, "invalid_argument", f.Code)
		}
		otherToken := grant(other.Id, config.MCPMessagesReadScope)
		f := assertToolFailure(t, call(otherToken, input), catalog, "search_messages")
		require.Equal(t, "invalid_argument", f.Code)
	})
	t.Run("missing scope and disabled search", func(t *testing.T) {
		roomToken := grant(viewer.Id, config.MCPRoomsReadScope)
		for _, tool := range list(handler, roomToken).Tools {
			require.NotEqual(t, "search_messages", tool.Name)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, newRawMCPRequest(roomToken, "tools/call", "search_messages", rawToolCallBody(t, "search_messages", map[string]any{})))
		require.Equal(t, http.StatusForbidden, response.Code)
		require.Contains(t, response.Header().Get("WWW-Authenticate"), config.MCPMessagesReadScope)
		cfg.Search.Enabled = false
		disabled, err := NewHandler(c, cfg, "test", WithMessageSearch(api))
		require.NoError(t, err)
		for _, tool := range list(disabled, token).Tools {
			require.NotEqual(t, "search_messages", tool.Name)
		}
		response = httptest.NewRecorder()
		disabled.ServeHTTP(response, newRawMCPRequest(token, "tools/call", "search_messages", rawToolCallBody(t, "search_messages", input)))
		require.Equal(t, http.StatusBadRequest, response.Code)
		require.Contains(t, response.Body.String(), `"error"`)
	})
	t.Run("provider failure is sanitized", func(t *testing.T) {
		provider.query = func(*searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
			return nil, errors.Join(searchsvc.ErrUnavailable, errors.New("private provider diagnostic"))
		}
		result := call(token, searchMessagesInput{Query: "searchable"})
		f := assertToolFailure(t, result, catalog, "search_messages")
		require.Equal(t, "temporary_failure", f.Code)
		require.Equal(t, "after_delay", f.Retry)
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "private provider diagnostic")
	})
	t.Run("revoked read permission removes room from provider scope", func(t *testing.T) {
		require.NoError(t, c.DenyRoomPermission(ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageRead))
		require.NoError(t, c.DenyRoomPermission(ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermMessageReadInteractions))
		provider.query = func(request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
			require.NotContains(t, request.RoomIds, room.Id)
			return &searchv1.QueryResponse{Hits: []*searchv1.QueryHit{channelHit, dmHit}}, nil
		}
		result := call(token, searchMessagesInput{Query: "searchable"})
		require.False(t, result.IsError)
		var output searchMessagesOutput
		decodeStructuredContent(t, result.StructuredContent, &output)
		require.Len(t, output.Results, 1)
		require.Equal(t, dm.Id, output.Results[0].Message.RoomID)
	})
}
