package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
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
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.ImportMessageResponse{MessageId: event.GetId()}), nil
}
