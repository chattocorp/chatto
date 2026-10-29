package mcpserver

import (
	"context"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessageSearch runs the authorized public search operation with the verified
// runtime credential in the context, including validation and sealed cursors.
type MessageSearch interface {
	SearchMessages(context.Context, *apiv1.SearchMessagesRequest) (*apiv1.SearchMessagesResponse, error)
}

type handlerOptions struct{ search MessageSearch }

// Option supplies an optional MCP dependency.
type Option func(*handlerOptions)

// WithMessageSearch supplies the shared public search operation.
func WithMessageSearch(search MessageSearch) Option {
	return func(options *handlerOptions) { options.search = search }
}

type searchMessagesInput struct {
	Query    string `json:"query" jsonschema:"Required search expression, maximum 1024 bytes. Supports words, quoted phrases, AND, in:, from:, before:, after:, and has:attachment."`
	RoomID   string `json:"room_id,omitempty" jsonschema:"Optional room ID to restrict the search."`
	AuthorID string `json:"author_id,omitempty" jsonschema:"Optional author ID to restrict the search."`
	Order    string `json:"order,omitempty" jsonschema:"relevance (default) or newest."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Minimum 1, maximum 100. Default 50."`
	Cursor   string `json:"cursor,omitempty" jsonschema:"Use nextCursor from the preceding page with all other arguments unchanged."`
}

type searchMessageResult struct {
	Message        messageResult `json:"message"`
	RelevanceScore float64       `json:"relevanceScore"`
}

type searchMessagesOutput struct {
	ServerName string                `json:"serverName"`
	Results    []searchMessageResult `json:"results"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

func searchMessagesHandler(c *core.ChattoCore, search MessageSearch) mcp.ToolHandlerFor[searchMessagesInput, searchMessagesOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input searchMessagesInput) (*mcp.CallToolResult, searchMessagesOutput, error) {
		ctx, _, err := authenticatedRequest(ctx)
		if err != nil {
			return nil, searchMessagesOutput{}, err
		}
		limit := input.Limit
		if limit == 0 {
			limit = 50
		}
		if limit < 1 || limit > 100 {
			return nil, searchMessagesOutput{}, invalidToolArgument("limit must be between 1 and 100")
		}
		order := apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_RELEVANCE
		switch input.Order {
		case "", "relevance":
		case "newest":
			order = apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_NEWEST
		default:
			return nil, searchMessagesOutput{}, invalidToolArgument("order must be relevance or newest")
		}
		request := &apiv1.SearchMessagesRequest{Query: input.Query, PageSize: uint32(limit), Order: order, Cursor: input.Cursor}
		if input.RoomID != "" {
			request.RoomId = new(input.RoomID)
		}
		if input.AuthorID != "" {
			request.AuthorId = new(input.AuthorID)
		}
		if search == nil {
			return nil, searchMessagesOutput{}, failure("temporary_failure", "Message search is unavailable. Try again later.", "retry", "after_delay")
		}
		response, err := search.SearchMessages(ctx, request)
		if err != nil {
			// Never forward query text or provider diagnostics to the host.
			switch connect.CodeOf(err) {
			case connect.CodeInvalidArgument:
				err = invalidToolArgument("The search query, filters, or cursor are invalid.")
			case connect.CodeFailedPrecondition:
				err = failure("policy_denied", "Message search is disabled.", "contact_administrator", "after_change")
			case connect.CodeDeadlineExceeded, connect.CodeCanceled:
				err = context.DeadlineExceeded
			}
			return nil, searchMessagesOutput{}, err
		}
		output := searchMessagesOutput{
			ServerName: c.ConfigModel().GetEffectiveServerName(),
			Results:    make([]searchMessageResult, 0, len(response.GetResults())),
			NextCursor: response.GetNextCursor(),
		}
		for _, hit := range response.GetResults() {
			message := hit.GetMessage()
			author, err := c.GetUserReference(ctx, message.GetActorId())
			if err != nil {
				return nil, searchMessagesOutput{}, err
			}
			result := messageResult{
				ID: message.GetId(), RoomID: message.GetRoomId(),
				AuthorID: message.GetActorId(), AuthorDisplayName: author.GetDisplayName(),
				Body: message.GetBody(), CreatedAt: formatTimestamp(message.GetCreatedAt().AsTime()),
				ThreadRootMessageID: message.GetThreadRootEventId(),
				InReplyToMessageID:  message.GetInReplyTo(),
			}
			if message.GetUpdatedAt() != nil {
				result.UpdatedAt = formatTimestamp(message.GetUpdatedAt().AsTime())
			}
			output.Results = append(output.Results, searchMessageResult{Message: result, RelevanceScore: hit.GetRelevanceScore()})
		}
		return nil, output, nil
	}
}
