package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/core"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

const (
	defaultListMessagesLimit = 50
	maxListMessagesLimit     = 100
	maxResourceIDLength      = 256
)

type getCurrentUserInput struct{}

type getCurrentUserOutput struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	AccountType string `json:"accountType"`
}

func getCurrentUserHandler(chattoCore *core.ChattoCore) mcp.ToolHandlerFor[getCurrentUserInput, getCurrentUserOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ getCurrentUserInput) (*mcp.CallToolResult, getCurrentUserOutput, error) {
		ctx, userID, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, getCurrentUserOutput{}, err
		}
		user, err := chattoCore.GetUser(ctx, userID)
		if err != nil {
			return nil, getCurrentUserOutput{}, toolOperationError(ctx, chattoCore, userID, "", nil, err, "get_current_user")
		}
		accountType := "human"
		if user.GetIsBot() {
			accountType = "bot"
		}
		return nil, getCurrentUserOutput{ID: user.GetId(), DisplayName: user.GetDisplayName(), AccountType: accountType}, nil
	}
}

type listRoomMessagesInput struct {
	ThreadRootMessageID string `json:"thread_root_message_id,omitempty" jsonschema:"Read this thread instead of the room timeline. The first page includes its root; older pages contain replies only."`
	RoomID              string `json:"room_id" jsonschema:"Required Chatto room ID."`
	Limit               int    `json:"limit,omitempty" jsonschema:"Minimum 1, maximum 100. Default 50."`
	BeforeEventID       string `json:"before_event_id,omitempty" jsonschema:"Return timeline entries before this event ID."`
}

type messageResult struct {
	ID                  string `json:"id"`
	RoomID              string `json:"roomId"`
	AuthorID            string `json:"authorId"`
	AuthorDisplayName   string `json:"authorDisplayName"`
	Body                string `json:"body,omitempty"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt,omitempty"`
	ThreadRootMessageID string `json:"threadRootMessageId,omitempty"`
	InReplyToMessageID  string `json:"inReplyToMessageId,omitempty"`
	Deleted             bool   `json:"deleted,omitempty"`
}

type listRoomMessagesOutput struct {
	ServerName        string          `json:"serverName"`
	RoomID            string          `json:"roomId"`
	Messages          []messageResult `json:"messages"`
	NextBeforeEventID string          `json:"nextBeforeEventId,omitempty"`
}

func listRoomMessagesHandler(chattoCore *core.ChattoCore) mcp.ToolHandlerFor[listRoomMessagesInput, listRoomMessagesOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input listRoomMessagesInput) (*mcp.CallToolResult, listRoomMessagesOutput, error) {
		ctx, userID, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, listRoomMessagesOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
			return nil, listRoomMessagesOutput{}, err
		}
		if err := validateResourceID("thread_root_message_id", input.ThreadRootMessageID, false); err != nil {
			return nil, listRoomMessagesOutput{}, err
		}
		if err := validateResourceID("before_event_id", input.BeforeEventID, false); err != nil {
			return nil, listRoomMessagesOutput{}, err
		}
		limit := input.Limit
		if limit == 0 {
			limit = defaultListMessagesLimit
		}
		if limit < 1 || limit > maxListMessagesLimit {
			return nil, listRoomMessagesOutput{}, invalidToolArgument(fmt.Sprintf("limit must be between 1 and %d", maxListMessagesLimit))
		}
		if err := requireVisibleToolRoom(ctx, chattoCore, userID, input.RoomID); err != nil {
			return nil, listRoomMessagesOutput{}, err
		}

		var beforeSeq *uint64
		if input.BeforeEventID != "" {
			// Authorize the room read before resolving the opaque cursor. This
			// prevents an event ID from becoming a cross-room existence probe.
			authorized, err := readMessagePage(ctx, chattoCore, userID, input, 1, nil)
			if err != nil {
				return nil, listRoomMessagesOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermMessageRead, core.PermMessageReadInteractions}, err, "list_room_messages")
			}
			cursorEvent, err := chattoCore.GetRoomEventByEventID(ctx, authorized.Kind, input.RoomID, input.BeforeEventID)
			if err != nil {
				return nil, listRoomMessagesOutput{}, err
			}
			if cursorEvent == nil || (input.ThreadRootMessageID != "" && cursorEvent.GetMessagePosted().GetInThread() != input.ThreadRootMessageID) {
				return nil, listRoomMessagesOutput{}, invalidToolArgument("before_event_id is not a valid cursor for this room")
			}
			seq, err := chattoCore.GetEventSequence(ctx, authorized.Kind, input.RoomID, input.BeforeEventID)
			if err != nil {
				return nil, listRoomMessagesOutput{}, err
			}
			beforeSeq = &seq
		}

		page, err := readMessagePage(ctx, chattoCore, userID, input, limit, beforeSeq)
		if err != nil {
			return nil, listRoomMessagesOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermMessageRead, core.PermMessageReadInteractions}, err, "list_room_messages")
		}
		output := listRoomMessagesOutput{
			ServerName: chattoCore.ConfigModel().GetEffectiveServerName(),
			RoomID:     input.RoomID,
			Messages:   make([]messageResult, 0, len(page.Page.Events)),
		}
		ctx = core.WithDEKRequestCache(ctx)
		for _, event := range page.Page.Events {
			if event.GetMessagePosted() == nil {
				continue
			}
			message, err := mcpMessageResult(ctx, chattoCore, event.Event)
			if err != nil {
				return nil, listRoomMessagesOutput{}, err
			}
			output.Messages = append(output.Messages, message)
		}
		if page.Page.HasOlder && len(page.Page.Events) > 0 {
			for _, event := range page.Page.Events {
				if input.ThreadRootMessageID != "" && event.GetId() == input.ThreadRootMessageID {
					continue
				}
				output.NextBeforeEventID = event.GetId()
				break
			}
		}
		return nil, output, nil
	}
}

type postMessageInput struct {
	CreateThread        bool   `json:"create_thread,omitempty" jsonschema:"Start a new thread with this root post, including rooms that require threads. Cannot be combined with thread_root_message_id."`
	ThreadRootMessageID string `json:"thread_root_message_id,omitempty" jsonschema:"Post in this thread, using the root message ID. Omit for a top-level room post."`
	InReplyToMessageID  string `json:"in_reply_to_message_id,omitempty" jsonschema:"Mark this message as a reply to this message ID. Supply thread_root_message_id explicitly for a thread reply. When omitted, Chatto can inherit the target reply’s thread."`

	RoomID         string `json:"room_id" jsonschema:"Required Chatto room ID."`
	Body           string `json:"body" jsonschema:"Required Markdown message body, maximum 10000 bytes. Mention users with verified @username. Fenced code tagged svg rendered or mermaid rendered is rendered by compatible Chatto clients; plain svg or mermaid shows source."`
	IdempotencyKey string `json:"idempotency_key,omitempty" jsonschema:"Optional caller-generated UUID. Reuse the same key and exact arguments to retry safely within 30 minutes of the first attempt. Retries do not extend the window. Generate a new key only for an intended new message."`
}

type postMessageOutput struct {
	ServerName string        `json:"serverName"`
	Message    messageResult `json:"message"`
}

func postMessageHandler(chattoCore *core.ChattoCore) mcp.ToolHandlerFor[postMessageInput, postMessageOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input postMessageInput) (*mcp.CallToolResult, postMessageOutput, error) {
		ctx, userID, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, postMessageOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
			return nil, postMessageOutput{}, err
		}
		for name, value := range map[string]string{"thread_root_message_id": input.ThreadRootMessageID, "in_reply_to_message_id": input.InReplyToMessageID} {
			if err := validateResourceID(name, value, false); err != nil {
				return nil, postMessageOutput{}, err
			}
		}
		if !core.HasVisibleContent(input.Body) {
			return nil, postMessageOutput{}, invalidToolArgument("body must contain visible text")
		}
		if len(input.Body) > core.MaxMessageBodyLength {
			return nil, postMessageOutput{}, invalidToolArgument(fmt.Sprintf("body must not exceed %d bytes", core.MaxMessageBodyLength))
		}
		if err := requireVisibleToolRoom(ctx, chattoCore, userID, input.RoomID); err != nil {
			return nil, postMessageOutput{}, err
		}
		result, err := chattoCore.Messages().PostMessage(ctx, core.MessagePostInput{ActorID: userID, RoomID: input.RoomID, Body: input.Body, CreateThread: input.CreateThread, ThreadRootEventID: input.ThreadRootMessageID, InReplyTo: input.InReplyToMessageID, IdempotencyKey: input.IdempotencyKey})
		if err != nil {
			f := toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermMessagePost}, err, "post_message")
			return nil, postMessageOutput{}, keyedPostFailure(f, input.IdempotencyKey)
		}
		message, err := mcpMessageResult(core.WithDEKRequestCache(ctx), chattoCore, result.Event)
		if err != nil {
			// The canonical command succeeded. Only the exact keyed request
			// can be retried; an unkeyed caller must check the result.
			return nil, postMessageOutput{}, keyedPostFailure(postedMessageResultError(err), input.IdempotencyKey)
		}
		return nil, postMessageOutput{ServerName: chattoCore.ConfigModel().GetEffectiveServerName(), Message: message}, nil
	}
}

type roomMembershipInput struct {
	RoomID string `json:"room_id" jsonschema:"Required Chatto channel room ID."`
}

type joinRoomOutput struct {
	ServerName string     `json:"serverName"`
	Room       roomResult `json:"room"`
}

func joinRoomHandler(chattoCore *core.ChattoCore) mcp.ToolHandlerFor[roomMembershipInput, joinRoomOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input roomMembershipInput) (*mcp.CallToolResult, joinRoomOutput, error) {
		ctx, userID, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, joinRoomOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
			return nil, joinRoomOutput{}, err
		}
		if err := requireVisibleToolRoom(ctx, chattoCore, userID, input.RoomID); err != nil {
			return nil, joinRoomOutput{}, err
		}
		room, err := chattoCore.RoomCommands().JoinRoom(ctx, core.RoomIDInput{ActorID: userID, RoomID: input.RoomID})
		if err != nil {
			return nil, joinRoomOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermRoomJoin}, err, "join_room")
		}
		return nil, joinRoomOutput{
			ServerName: chattoCore.ConfigModel().GetEffectiveServerName(),
			Room:       roomResultFromRoom(room, true),
		}, nil
	}
}

type leaveRoomOutput struct {
	ServerName string `json:"serverName"`
	RoomID     string `json:"roomId"`
	Left       bool   `json:"left"`
}

func leaveRoomHandler(chattoCore *core.ChattoCore) mcp.ToolHandlerFor[roomMembershipInput, leaveRoomOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input roomMembershipInput) (*mcp.CallToolResult, leaveRoomOutput, error) {
		ctx, userID, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, leaveRoomOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
			return nil, leaveRoomOutput{}, err
		}
		if err := requireVisibleToolRoom(ctx, chattoCore, userID, input.RoomID); err != nil {
			return nil, leaveRoomOutput{}, err
		}
		if err := chattoCore.RoomCommands().LeaveRoom(ctx, core.RoomIDInput{ActorID: userID, RoomID: input.RoomID}); err != nil {
			return nil, leaveRoomOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, nil, err, "leave_room")
		}
		return nil, leaveRoomOutput{ServerName: chattoCore.ConfigModel().GetEffectiveServerName(), RoomID: input.RoomID, Left: true}, nil
	}
}

// authenticatedRequest returns the authenticated account ID and a context
// that carries its verified Chatto credential. Without the credential, core
// authorization would treat the call as internal work with entitlement
// semantics, including the effective-owner override.
func authenticatedRequest(ctx context.Context) (context.Context, string, error) {
	token := auth.TokenInfoFromContext(ctx)
	if token == nil || token.UserID == "" {
		return ctx, "", auth.ErrInvalidToken
	}
	credential, ok := token.Extra[runtimeCredentialExtraKey].(authctx.RuntimeCredential)
	if !ok || credential.UserID != token.UserID {
		return ctx, "", auth.ErrInvalidToken
	}
	return authctx.WithCredential(ctx, credential), token.UserID, nil
}

func validateResourceID(name, value string, required bool) error {
	if required && value == "" {
		return invalidToolArgument(fmt.Sprintf("%s is required", name))
	}
	if len(value) > maxResourceIDLength || strings.TrimSpace(value) != value {
		return invalidToolArgument(fmt.Sprintf("%s is invalid", name))
	}
	return nil
}

func mcpMessageResult(ctx context.Context, chattoCore *core.ChattoCore, event *evtv1.Event) (messageResult, error) {
	if event == nil || event.GetMessagePosted() == nil {
		return messageResult{}, fmt.Errorf("message event is unavailable")
	}
	if event.GetCreatedAt() == nil {
		return messageResult{}, fmt.Errorf("message timestamp is unavailable")
	}
	posted := event.GetMessagePosted()
	authorID := core.MessageAuthorID(event)
	result := messageResult{
		ID:                  event.GetId(),
		RoomID:              posted.GetRoomId(),
		AuthorID:            authorID,
		CreatedAt:           formatTimestamp(event.GetCreatedAt().AsTime()),
		ThreadRootMessageID: posted.GetInThread(),
		InReplyToMessageID:  posted.GetInReplyTo(),
	}
	user, err := chattoCore.GetUserReference(ctx, authorID)
	if err != nil {
		return messageResult{}, err
	}
	result.AuthorDisplayName = user.GetDisplayName()
	body, err := chattoCore.GetFullMessageBody(ctx, event.GetId())
	if err != nil {
		return messageResult{}, err
	}
	if body == nil {
		result.Deleted = true
		return result, nil
	}
	result.Body = body.Body
	if body.UpdatedAt != nil {
		result.UpdatedAt = formatTimestamp(*body.UpdatedAt)
	}
	return result, nil
}

func roomResultFromRoom(room *readmodel.Room, isMember bool) roomResult {
	return roomResult{
		ID: room.GetId(), Name: room.GetName(), Description: room.GetDescription(),
		Kind: roomKind(room.GetKind()), Archived: room.GetArchived(), IsMember: isMember,
	}
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

// readMessagePage uses the canonical room/thread read gates before resolving
// or returning any timeline data. Thread roots appear only on the initial page.
func readMessagePage(ctx context.Context, c *core.ChattoCore, actor string, in listRoomMessagesInput, limit int, before *uint64) (*core.RoomTimelineEventsResult, error) {
	if in.ThreadRootMessageID == "" {
		return c.RoomTimelineReads().GetRoomEvents(ctx, core.RoomTimelineEventsInput{ActorID: actor, RoomID: in.RoomID, Limit: limit, BeforeSeq: before})
	}
	thread, err := c.RoomTimelineReads().GetThreadEvents(ctx, core.ThreadTimelineEventsInput{ActorID: actor, RoomID: in.RoomID, ThreadRootEventID: in.ThreadRootMessageID, Limit: limit, BeforeSeq: before})
	if err != nil {
		return nil, err
	}
	page := *thread.Replies
	// Keep chronological order. The caller skips the root when selecting a
	// pagination cursor, so older pages continue before the oldest reply.
	if thread.IncludeRoot {
		page.Events = append([]*core.RoomEvent{thread.Root}, page.Events...)
	}
	return &core.RoomTimelineEventsResult{Kind: thread.Kind, Page: &page}, nil
}
