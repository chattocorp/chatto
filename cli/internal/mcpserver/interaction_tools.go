package mcpserver

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type reactionInput struct {
	RoomID    string `json:"room_id" jsonschema:"Room containing the target message."`
	MessageID string `json:"message_id" jsonschema:"Message to react to, including thread replies."`
	Emoji     string `json:"emoji" jsonschema:"Emoji name such as thumbsup or heart, without colons."`
}
type reactionOutput struct {
	RoomID    string `json:"roomId"`
	MessageID string `json:"messageId"`
	Changed   bool   `json:"changed"`
}

// reactionHandler delegates both target authorization and idempotent reaction
// intent to the canonical operation model. It never retries a mutation itself.
func reactionHandler(c *core.ChattoCore, add bool) mcp.ToolHandlerFor[reactionInput, reactionOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in reactionInput) (*mcp.CallToolResult, reactionOutput, error) {
		ctx, actor, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, reactionOutput{}, err
		}
		for name, value := range map[string]string{"room_id": in.RoomID, "message_id": in.MessageID, "emoji": in.Emoji} {
			if err := validateResourceID(name, value, true); err != nil {
				return nil, reactionOutput{}, err
			}
		}
		if err := requireVisibleToolRoom(ctx, c, actor, in.RoomID); err != nil {
			return nil, reactionOutput{}, err
		}
		input := core.ReactionMutationInput{ActorID: actor, RoomID: in.RoomID, MessageEventID: in.MessageID, Emoji: in.Emoji}
		name := "add_reaction"
		var changed bool
		if add {
			changed, err = c.ReactionModel().AddReaction(ctx, input)
		} else {
			name = "remove_reaction"
			changed, err = c.ReactionModel().RemoveReaction(ctx, input)
		}
		if err != nil {
			return nil, reactionOutput{}, toolOperationError(ctx, c, actor, in.RoomID, []core.Permission{core.PermMessageReact}, err, name)
		}
		return nil, reactionOutput{RoomID: in.RoomID, MessageID: in.MessageID, Changed: changed}, nil
	}
}

type getUserInput struct {
	UserID   string `json:"user_id,omitempty" jsonschema:"User ID. Supply exactly one of user_id or username."`
	Username string `json:"username,omitempty" jsonschema:"Exact username without @. Supply exactly one of user_id or username."`
}
type getUserOutput struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Bio         string `json:"bio"`
	IsBot       bool   `json:"isBot"`
}

// getUserHandler returns only public profile fields, using the same authenticated
// lookup as UserService.GetUser. It does not expose credentials or admin records.
func getUserHandler(c *core.ChattoCore) mcp.ToolHandlerFor[getUserInput, getUserOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in getUserInput) (*mcp.CallToolResult, getUserOutput, error) {
		ctx, actor, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, getUserOutput{}, err
		}
		if (in.UserID == "") == (in.Username == "") {
			return nil, getUserOutput{}, invalidToolArgument("supply exactly one of user_id or username")
		}
		for name, value := range map[string]string{"user_id": in.UserID, "username": in.Username} {
			if err := validateResourceID(name, value, false); err != nil {
				return nil, getUserOutput{}, err
			}
		}
		var user *readmodel.User
		if in.UserID != "" {
			user, err = c.GetUser(ctx, in.UserID)
		} else {
			user, err = c.GetUserByLogin(ctx, in.Username)
		}
		if err != nil {
			return nil, getUserOutput{}, toolOperationError(ctx, c, actor, "", nil, err, "get_user")
		}
		return nil, getUserOutput{ID: user.Id, Username: user.Login, DisplayName: user.DisplayName, Bio: user.Bio, IsBot: user.IsBot}, nil
	}
}
