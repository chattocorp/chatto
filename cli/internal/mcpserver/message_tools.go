package mcpserver

import (
	"context"
	"slices"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type messageTargetInput struct {
	RoomID    string `json:"room_id" jsonschema:"Room ID returned with the message."`
	MessageID string `json:"message_id" jsonschema:"Message ID, including a thread reply."`
}

type updateMessageInput struct {
	RoomID    string `json:"room_id" jsonschema:"Room ID returned with the message."`
	MessageID string `json:"message_id" jsonschema:"Message whose body to replace."`
	Body      string `json:"body" jsonschema:"Complete replacement Markdown body, maximum 10000 bytes. Other message fields remain unchanged."`
}

type deleteMessageOutput struct {
	RoomID    string `json:"roomId"`
	MessageID string `json:"messageId"`
}

type messageReaction struct {
	Emoji   string `json:"emoji"`
	Count   int    `json:"count"`
	Reacted bool   `json:"reacted" jsonschema:"Whether the authenticated account has this reaction."`
}

type getMessageOutput struct {
	ServerName string            `json:"serverName"`
	Message    messageResult     `json:"message"`
	Reactions  []messageReaction `json:"reactions"`
}

func validateMessageTarget(ctx context.Context, c *core.ChattoCore, actor, room, message string) error {
	for name, value := range map[string]string{"room_id": room, "message_id": message} {
		if err := validateResourceID(name, value, true); err != nil {
			return err
		}
	}
	return requireVisibleToolRoom(ctx, c, actor, room)
}

// getMessageHandler authorizes the target before hydrating its body or reactions.
func getMessageHandler(c *core.ChattoCore) mcp.ToolHandlerFor[messageTargetInput, getMessageOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in messageTargetInput) (*mcp.CallToolResult, getMessageOutput, error) {
		ctx, actor, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, getMessageOutput{}, err
		}
		if err = validateMessageTarget(ctx, c, actor, in.RoomID, in.MessageID); err != nil {
			return nil, getMessageOutput{}, err
		}
		result, err := c.RoomTimelineReads().GetMessage(ctx, actor, in.RoomID, in.MessageID)
		if err != nil {
			return nil, getMessageOutput{}, toolOperationError(ctx, c, actor, in.RoomID, nil, err, "get_message")
		}
		message, err := mcpMessageResult(core.WithDEKRequestCache(ctx), c, result.Event)
		if err != nil {
			return nil, getMessageOutput{}, toolOperationError(ctx, c, actor, in.RoomID, nil, err, "get_message")
		}
		reactions, err := c.GetReactions(ctx, in.MessageID)
		if err != nil {
			return nil, getMessageOutput{}, toolOperationError(ctx, c, actor, in.RoomID, nil, err, "get_message")
		}
		output := getMessageOutput{ServerName: c.ConfigModel().GetEffectiveServerName(), Message: message, Reactions: []messageReaction{}}
		for _, r := range reactions {
			output.Reactions = append(output.Reactions, messageReaction{Emoji: r.Emoji, Count: len(r.UserIDs), Reacted: slices.Contains(r.UserIDs, actor)})
		}
		return nil, output, nil
	}
}

func updateMessageHandler(c *core.ChattoCore) mcp.ToolHandlerFor[updateMessageInput, postMessageOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in updateMessageInput) (*mcp.CallToolResult, postMessageOutput, error) {
		ctx, actor, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, postMessageOutput{}, err
		}
		if err = validateMessageTarget(ctx, c, actor, in.RoomID, in.MessageID); err != nil {
			return nil, postMessageOutput{}, err
		}
		event, _, err := c.Messages().UpdateMessage(ctx, core.MessageUpdateInput{ActorID: actor, RoomID: in.RoomID, EventID: in.MessageID, Body: &in.Body})
		if err != nil {
			return nil, postMessageOutput{}, toolOperationError(ctx, c, actor, in.RoomID, nil, err, "update_message")
		}
		message, err := mcpMessageResult(core.WithDEKRequestCache(ctx), c, event)
		if err != nil {
			f := postedMessageResultError(err)
			f.Message = "The message was updated, but Chatto could not return its details. Read the message before taking another action."
			return nil, postMessageOutput{}, f
		}
		return nil, postMessageOutput{ServerName: c.ConfigModel().GetEffectiveServerName(), Message: message}, nil
	}
}

func deleteMessageHandler(c *core.ChattoCore) mcp.ToolHandlerFor[messageTargetInput, deleteMessageOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in messageTargetInput) (*mcp.CallToolResult, deleteMessageOutput, error) {
		ctx, actor, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, deleteMessageOutput{}, err
		}
		if err = validateMessageTarget(ctx, c, actor, in.RoomID, in.MessageID); err != nil {
			return nil, deleteMessageOutput{}, err
		}
		if err = c.Messages().DeleteMessage(ctx, core.MessageDeleteInput{ActorID: actor, RoomID: in.RoomID, EventID: in.MessageID}); err != nil {
			return nil, deleteMessageOutput{}, toolOperationError(ctx, c, actor, in.RoomID, nil, err, "delete_message")
		}
		return nil, deleteMessageOutput{RoomID: in.RoomID, MessageID: in.MessageID}, nil
	}
}

type sendDirectMessageInput struct {
	UserID         string `json:"user_id" jsonschema:"Recipient user ID. The server finds or creates the direct conversation."`
	Body           string `json:"body" jsonschema:"Markdown message body, maximum 10000 bytes. Supports verified @username mentions and svg rendered or mermaid rendered code fences."`
	IdempotencyKey string `json:"idempotency_key,omitempty" jsonschema:"Optional caller-generated UUID. Retry the exact recipient, body and key within 30 minutes of the first attempt to avoid duplicate messages. Without a key, check uncertain results before sending again."`
}

// sendDirectMessageHandler resolves the canonical DM before using the same send
// operation as post_message. Room creation and message posting are separate
// operations: a failed send can leave an empty DM, and errors disclose that fact.
func sendDirectMessageHandler(c *core.ChattoCore) mcp.ToolHandlerFor[sendDirectMessageInput, postMessageOutput] {
	return func(ctx context.Context, request *mcp.CallToolRequest, in sendDirectMessageInput) (*mcp.CallToolResult, postMessageOutput, error) {
		ctx, actor, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, postMessageOutput{}, err
		}
		if err = validateResourceID("user_id", in.UserID, true); err != nil {
			return nil, postMessageOutput{}, err
		}
		if !core.HasVisibleContent(in.Body) || len(in.Body) > core.MaxMessageBodyLength {
			return nil, postMessageOutput{}, invalidToolArgument("body must contain visible text and be at most 10000 bytes")
		}
		if in.IdempotencyKey != "" {
			key, e := uuid.Parse(in.IdempotencyKey)
			if e != nil || len(in.IdempotencyKey) != 36 || key == uuid.Nil || key.Variant() != uuid.RFC4122 {
				return nil, postMessageOutput{}, invalidToolArgument("idempotency_key must be a non-zero UUID in hyphenated form")
			}
		}
		room, created, err := c.RoomCommands().StartDM(ctx, core.RoomStartDMInput{ActorID: actor, ParticipantIDs: []string{in.UserID}})
		if err != nil {
			return nil, postMessageOutput{}, toolOperationError(ctx, c, actor, "", nil, err, "send_direct_message")
		}
		result, output, err := postMessageHandler(c)(ctx, request, postMessageInput{RoomID: room.Id, Body: in.Body, IdempotencyKey: in.IdempotencyKey})
		if err != nil && created {
			f := classifyToolError(err, "send_direct_message")
			f.ConversationCreated = true
			f.RoomID = room.Id
			f.Message += " The direct conversation was created, but message delivery was not confirmed."
			return nil, postMessageOutput{}, f
		}
		return result, output, err
	}
}
