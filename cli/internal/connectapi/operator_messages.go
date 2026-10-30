package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	operatorv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/operator/v1"
)

type operatorMessageService struct {
	api *API
}

func operatorHistoricalMessageInput(req *connect.Request[operatorv1.ImportMessageRequest]) (input core.HistoricalMessageInput, err error) {
	createdAt := req.Msg.GetCreatedAt()
	if createdAt == nil || createdAt.CheckValid() != nil {
		return input, invalidArgument("created_at must be a valid timestamp")
	}
	var editedAt *time.Time
	deletedAt, err := operatorHistoricalTime(req.Msg.GetDeletedAt())
	if err != nil {
		return input, err
	}
	if value := req.Msg.GetEditedAt(); value != nil {
		if value.CheckValid() != nil {
			return input, invalidArgument("edited_at must be a valid timestamp")
		}
		parsed := value.AsTime()
		editedAt = &parsed
	}
	var preview *evtv1.LinkPreview
	if req.Msg.GetPreviewUrl() != "" || req.Msg.GetPreviewTitle() != "" || req.Msg.GetPreviewDescription() != "" || req.Msg.GetPreviewType() != "" {
		if req.Msg.GetPreviewUrl() == "" {
			return input, invalidArgument("preview_url is required when preview fields are set")
		}
		preview = &evtv1.LinkPreview{
			Url: req.Msg.GetPreviewUrl(), Title: req.Msg.GetPreviewTitle(),
			Description: req.Msg.GetPreviewDescription(), EmbedType: req.Msg.GetPreviewType(),
		}
	}
	return core.HistoricalMessageInput{
		RoomID: req.Msg.GetRoomId(), AuthorID: req.Msg.GetAuthorId(),
		CreatedAt: createdAt.AsTime(), EditedAt: editedAt, InReplyTo: req.Msg.GetInReplyTo(),
		AttachmentAssetIDs: req.Msg.GetAttachmentAssetIds(), Body: req.Msg.GetBody(), LinkPreview: preview,
		ThreadRootEventID: req.Msg.GetThreadRootEventId(), CreateThread: req.Msg.GetCreateThread(),
		IdempotencyKey: req.Msg.GetIdempotencyKey(),
		Deleted:        req.Msg.GetDeleted(),
		OperationKey:   req.Msg.GetOperationKey(),
		DeletedAt:      deletedAt, SourceActorID: req.Msg.GetSourceActorId(),
	}, nil
}

func (s *operatorMessageService) ImportMessage(ctx context.Context, req *connect.Request[operatorv1.ImportMessageRequest]) (*connect.Response[operatorv1.ImportMessageResponse], error) {
	input, err := operatorHistoricalMessageInput(req)
	if err != nil {
		return nil, err
	}
	event, err := s.api.core.ImportHistoricalMessage(ctx, input)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.ImportMessageResponse{MessageId: event.GetId()}), nil
}

func (s *operatorMessageService) ImportMessages(ctx context.Context, req *connect.Request[operatorv1.ImportMessagesRequest]) (*connect.Response[operatorv1.ImportMessagesResponse], error) {
	operations := make([]core.HistoricalMessageOperation, 0, len(req.Msg.Operations))
	for _, op := range req.Msg.Operations {
		if op == nil || op.Message == nil {
			return nil, invalidArgument("message operation is required")
		}
		input, err := operatorHistoricalMessageInput(connect.NewRequest(op.Message))
		if err != nil {
			return nil, err
		}
		operation := core.HistoricalMessageOperation{Message: input, ThreadRootOperation: op.ThreadRootOperationKey, ReplyToOperation: op.InReplyToOperationKey}
		for _, r := range op.Reactions {
			if r == nil || r.Change == nil {
				return nil, invalidArgument("reaction operation is required")
			}
			at, err := operatorHistoricalTime(r.OccurredAt)
			if err != nil {
				return nil, err
			}
			operation.Reactions = append(operation.Reactions, core.HistoricalMessageChange{OperationKey: r.OperationKey, ActorID: r.ActorId, OccurredAt: at, UserID: r.Change.UserId, Emoji: r.Change.Emoji, Present: r.Change.Present})
		}
		operations = append(operations, operation)
	}
	posts, err := s.api.core.ImportHistoricalMessages(ctx, operations)
	if err != nil {
		return nil, err
	}
	response := &operatorv1.ImportMessagesResponse{}
	for _, post := range posts {
		response.Messages = append(response.Messages, &operatorv1.ImportMessageResponse{MessageId: post.Id})
	}
	return connect.NewResponse(response), nil
}

func (s *operatorMessageService) ImportReaction(ctx context.Context, req *connect.Request[operatorv1.ImportReactionRequest]) (*connect.Response[operatorv1.ImportReactionResponse], error) {
	added, err := s.api.core.ImportHistoricalReaction(ctx, core.HistoricalReactionInput{
		RoomID: req.Msg.GetRoomId(), MessageEventID: req.Msg.GetMessageEventId(), UserID: req.Msg.GetUserId(), Emoji: req.Msg.GetEmoji(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.ImportReactionResponse{Added: added}), nil
}
