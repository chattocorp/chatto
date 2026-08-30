package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
		userID, err := authenticatedUserID(ctx)
		if err != nil {
			return nil, getCurrentUserOutput{}, err
		}
		user, err := chattoCore.GetUser(ctx, userID)
		if err != nil {
			return nil, getCurrentUserOutput{}, toolOperationError(ctx, chattoCore, userID, "", nil, err)
		}
		accountType := "human"
		if user.GetIsBot() {
			accountType = "bot"
		}
		return nil, getCurrentUserOutput{ID: user.GetId(), DisplayName: user.GetDisplayName(), AccountType: accountType}, nil
	}
}

type listRoomMessagesInput struct {
	RoomID        string `json:"room_id" jsonschema:"Required Chatto room ID."`
	Limit         int    `json:"limit,omitempty" jsonschema:"Minimum 1, maximum 100. Default 50."`
	BeforeEventID string `json:"before_event_id,omitempty" jsonschema:"Return timeline entries before this event ID."`
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
		userID, err := authenticatedUserID(ctx)
		if err != nil {
			return nil, listRoomMessagesOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
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
			return nil, listRoomMessagesOutput{}, fmt.Errorf("limit must be between 1 and %d", maxListMessagesLimit)
		}

		var beforeSeq *uint64
		if input.BeforeEventID != "" {
			// Authorize the room read before resolving the opaque cursor. This
			// prevents an event ID from becoming a cross-room existence probe.
			authorized, err := chattoCore.RoomTimelineReads().GetRoomEvents(ctx, core.RoomTimelineEventsInput{
				ActorID: userID, RoomID: input.RoomID, Limit: 1,
			})
			if err != nil {
				return nil, listRoomMessagesOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermMessageRead, core.PermMessageReadInteractions}, err)
			}
			cursorEvent, err := chattoCore.GetRoomEventByEventID(ctx, authorized.Kind, input.RoomID, input.BeforeEventID)
			if err != nil {
				return nil, listRoomMessagesOutput{}, err
			}
			if cursorEvent == nil {
				return nil, listRoomMessagesOutput{}, fmt.Errorf("before_event_id was not found in this room")
			}
			seq, err := chattoCore.GetEventSequence(ctx, authorized.Kind, input.RoomID, input.BeforeEventID)
			if err != nil {
				return nil, listRoomMessagesOutput{}, err
			}
			beforeSeq = &seq
		}

		page, err := chattoCore.RoomTimelineReads().GetRoomEvents(ctx, core.RoomTimelineEventsInput{
			ActorID: userID, RoomID: input.RoomID, Limit: limit, BeforeSeq: beforeSeq,
		})
		if err != nil {
			return nil, listRoomMessagesOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermMessageRead, core.PermMessageReadInteractions}, err)
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
			output.NextBeforeEventID = page.Page.Events[0].GetId()
		}
		return nil, output, nil
	}
}

type postMessageInput struct {
	RoomID string `json:"room_id" jsonschema:"Required Chatto room ID."`
	Body   string `json:"body" jsonschema:"Required message body, maximum 10000 bytes."`
}

type postMessageOutput struct {
	ServerName string        `json:"serverName"`
	Message    messageResult `json:"message"`
}

func postMessageHandler(chattoCore *core.ChattoCore) mcp.ToolHandlerFor[postMessageInput, postMessageOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input postMessageInput) (*mcp.CallToolResult, postMessageOutput, error) {
		userID, err := authenticatedUserID(ctx)
		if err != nil {
			return nil, postMessageOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
			return nil, postMessageOutput{}, err
		}
		if !core.HasVisibleContent(input.Body) {
			return nil, postMessageOutput{}, fmt.Errorf("body must contain visible text")
		}
		if len(input.Body) > core.MaxMessageBodyLength {
			return nil, postMessageOutput{}, fmt.Errorf("body must not exceed %d bytes", core.MaxMessageBodyLength)
		}
		result, err := chattoCore.Messages().PostMessage(ctx, core.MessagePostInput{ActorID: userID, RoomID: input.RoomID, Body: input.Body})
		if err != nil {
			return nil, postMessageOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermMessagePost}, err)
		}
		message, err := mcpMessageResult(core.WithDEKRequestCache(ctx), chattoCore, result.Event)
		if err != nil {
			return nil, postMessageOutput{}, err
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
		userID, err := authenticatedUserID(ctx)
		if err != nil {
			return nil, joinRoomOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
			return nil, joinRoomOutput{}, err
		}
		room, err := chattoCore.RoomCommands().JoinRoom(ctx, core.RoomIDInput{ActorID: userID, RoomID: input.RoomID})
		if err != nil {
			return nil, joinRoomOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, []core.Permission{core.PermRoomJoin}, err)
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
		userID, err := authenticatedUserID(ctx)
		if err != nil {
			return nil, leaveRoomOutput{}, err
		}
		if err := validateResourceID("room_id", input.RoomID, true); err != nil {
			return nil, leaveRoomOutput{}, err
		}
		if err := chattoCore.RoomCommands().LeaveRoom(ctx, core.RoomIDInput{ActorID: userID, RoomID: input.RoomID}); err != nil {
			return nil, leaveRoomOutput{}, toolOperationError(ctx, chattoCore, userID, input.RoomID, nil, err)
		}
		return nil, leaveRoomOutput{ServerName: chattoCore.ConfigModel().GetEffectiveServerName(), RoomID: input.RoomID, Left: true}, nil
	}
}

func authenticatedUserID(ctx context.Context) (string, error) {
	token := auth.TokenInfoFromContext(ctx)
	if token == nil || token.UserID == "" {
		return "", auth.ErrInvalidToken
	}
	return token.UserID, nil
}

func validateResourceID(name, value string, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if len(value) > maxResourceIDLength || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is invalid", name)
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
	result := messageResult{
		ID:                  event.GetId(),
		RoomID:              posted.GetRoomId(),
		AuthorID:            event.GetActorId(),
		CreatedAt:           formatTimestamp(event.GetCreatedAt().AsTime()),
		ThreadRootMessageID: posted.GetInThread(),
		InReplyToMessageID:  posted.GetInReplyTo(),
	}
	user, err := chattoCore.GetUserReference(ctx, event.GetActorId())
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

func roomResultFromRoom(room *evtv1.Room, isMember bool) roomResult {
	return roomResult{
		ID: room.GetId(), Name: room.GetName(), Description: room.GetDescription(),
		Kind: roomKind(room.GetKind()), Archived: room.GetArchived(), IsMember: isMember,
	}
}

func toolOperationError(ctx context.Context, chattoCore *core.ChattoCore, userID, roomID string, permissions []core.Permission, err error) error {
	switch {
	case errors.Is(err, core.ErrNotRoomMember):
		return fmt.Errorf("not_room_member: this operation requires current room membership")
	case errors.Is(err, core.ErrPermissionDenied):
		missing := missingRoomPermissions(ctx, chattoCore, userID, roomID, permissions)
		if len(missing) > 0 {
			quoted := make([]string, len(missing))
			for i, permission := range missing {
				quoted[i] = fmt.Sprintf("%q", permission)
			}
			return fmt.Errorf("permission_denied: Chatto RBAC requires %s for this room; ask an administrator to grant it", strings.Join(quoted, " or "))
		}
		return fmt.Errorf("permission_denied: Chatto denied this operation because of room policy or another authorization rule")
	case errors.Is(err, core.ErrNotFound):
		return fmt.Errorf("not_found: the requested Chatto resource does not exist or is not visible")
	default:
		return err
	}
}

func missingRoomPermissions(ctx context.Context, chattoCore *core.ChattoCore, userID, roomID string, permissions []core.Permission) []core.Permission {
	if roomID == "" || len(permissions) == 0 {
		return nil
	}
	room, err := chattoCore.FindRoomByID(ctx, roomID)
	if err != nil || room == nil {
		return nil
	}
	missing := make([]core.Permission, 0, len(permissions))
	for _, permission := range permissions {
		explanation, err := chattoCore.PermResolver().ExplainRoomPermission(ctx, userID, core.KindOfRoom(room), roomID, permission)
		if err != nil {
			return nil
		}
		if explanation.State != core.DecisionAllow {
			missing = append(missing, permission)
		}
	}
	if len(missing) != len(permissions) {
		return nil
	}
	return missing
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
