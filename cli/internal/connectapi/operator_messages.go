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

func (s *operatorMessageService) ImportMessage(ctx context.Context, req *connect.Request[operatorv1.ImportMessageRequest]) (*connect.Response[operatorv1.ImportMessageResponse], error) {
	createdAt := req.Msg.GetCreatedAt()
	if createdAt == nil || createdAt.CheckValid() != nil {
		return nil, invalidArgument("created_at must be a valid timestamp")
	}
	var editedAt *time.Time
	deletedAt, err := operatorHistoricalTime(req.Msg.GetDeletedAt())
	if err != nil {
		return nil, err
	}
	if value := req.Msg.GetEditedAt(); value != nil {
		if value.CheckValid() != nil {
			return nil, invalidArgument("edited_at must be a valid timestamp")
		}
		parsed := value.AsTime()
		editedAt = &parsed
	}
	var preview *evtv1.LinkPreview
	if req.Msg.GetPreviewUrl() != "" || req.Msg.GetPreviewTitle() != "" || req.Msg.GetPreviewDescription() != "" || req.Msg.GetPreviewType() != "" {
		if req.Msg.GetPreviewUrl() == "" {
			return nil, invalidArgument("preview_url is required when preview fields are set")
		}
		preview = &evtv1.LinkPreview{
			Url: req.Msg.GetPreviewUrl(), Title: req.Msg.GetPreviewTitle(),
			Description: req.Msg.GetPreviewDescription(), EmbedType: req.Msg.GetPreviewType(),
		}
	}
	event, err := s.api.core.ImportHistoricalMessage(ctx, core.HistoricalMessageInput{
		RoomID: req.Msg.GetRoomId(), AuthorID: req.Msg.GetAuthorId(),
		CreatedAt: createdAt.AsTime(), EditedAt: editedAt, InReplyTo: req.Msg.GetInReplyTo(),
		AttachmentAssetIDs: req.Msg.GetAttachmentAssetIds(), Body: req.Msg.GetBody(), LinkPreview: preview,
		ThreadRootEventID: req.Msg.GetThreadRootEventId(), CreateThread: req.Msg.GetCreateThread(),
		IdempotencyKey: req.Msg.GetIdempotencyKey(),
		Deleted:        req.Msg.GetDeleted(),
		OperationKey:   req.Msg.GetOperationKey(),
		DeletedAt:      deletedAt, SourceActorID: req.Msg.GetSourceActorId(),
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.ImportMessageResponse{MessageId: event.GetId()}), nil
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
