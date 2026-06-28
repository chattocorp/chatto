package connectapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type messageService struct {
	api *API
}

func (s *messageService) PostMessage(ctx context.Context, req *connect.Request[apiv1.PostMessageRequest]) (*connect.Response[apiv1.PostMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	attachmentAssetIDs, videoProcessingAssetIDs, challenge, err := s.uploadPostAttachments(ctx, caller.UserID, req.Msg)
	if err != nil {
		return nil, connectError(err)
	}
	if challenge != nil {
		return connect.NewResponse(&apiv1.PostMessageResponse{
			Result: &apiv1.PostMessageResponse_MentionConfirmation{
				MentionConfirmation: &apiv1.MentionConfirmationChallenge{
					RecipientCount: int32(challenge.RecipientCount),
					Token:          challenge.Token,
				},
			},
		}), nil
	}

	result, err := s.api.core.Messages().PostMessage(ctx, core.MessagePostInput{
		ActorID:                  caller.UserID,
		RoomID:                   req.Msg.RoomId,
		Body:                     req.Msg.Body,
		AttachmentAssetIDs:       attachmentAssetIDs,
		VideoProcessingAssetIDs:  videoProcessingAssetIDs,
		ThreadRootEventID:        req.Msg.ThreadRootEventId,
		InReplyTo:                req.Msg.InReplyTo,
		AlsoSendToChannel:        req.Msg.AlsoSendToChannel,
		MentionConfirmationToken: req.Msg.MentionConfirmationToken,
		LinkPreview:              apiMessageLinkPreviewToCore(req.Msg.LinkPreview),
	})
	if err != nil {
		return nil, connectError(err)
	}
	if result == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("message post returned no result"))
	}
	if challenge := result.MentionConfirmation; challenge != nil {
		return connect.NewResponse(&apiv1.PostMessageResponse{
			Result: &apiv1.PostMessageResponse_MentionConfirmation{
				MentionConfirmation: &apiv1.MentionConfirmationChallenge{
					RecipientCount: int32(challenge.RecipientCount),
					Token:          challenge.Token,
				},
			},
		}), nil
	}
	if result.Event == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("message post returned no event"))
	}

	roomID := result.Event.GetMessagePosted().GetRoomId()
	kind := core.KindChannel
	if room, err := s.api.core.FindRoomByID(ctx, roomID); err == nil && room != nil {
		kind = core.KindOfRoom(room)
	}
	apiEvent, includes, err := s.hydratePostedEvent(ctx, caller.UserID, kind, result.Event)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.PostMessageResponse{
		Result:   &apiv1.PostMessageResponse_Event{Event: apiEvent},
		Includes: includes,
	}), nil
}

func (s *messageService) uploadPostAttachments(ctx context.Context, actorID string, req *apiv1.PostMessageRequest) ([]string, []string, *core.MentionConfirmationChallenge, error) {
	attachmentAssetIDs := append([]string(nil), req.GetAttachmentAssetIds()...)
	if len(req.GetAttachments()) == 0 {
		return attachmentAssetIDs, nil, nil, nil
	}

	preflight, err := s.api.core.Messages().PreflightPost(ctx, core.MessagePostInput{
		ActorID:                  actorID,
		RoomID:                   req.GetRoomId(),
		Body:                     req.GetBody(),
		AttachmentAssetIDs:       attachmentAssetIDs,
		HasPendingAttachments:    true,
		ThreadRootEventID:        req.GetThreadRootEventId(),
		AlsoSendToChannel:        req.GetAlsoSendToChannel(),
		MentionConfirmationToken: req.GetMentionConfirmationToken(),
		LinkPreview:              apiMessageLinkPreviewToCore(req.GetLinkPreview()),
	})
	if err != nil {
		return nil, nil, nil, err
	}
	if preflight.MentionConfirmation != nil {
		return nil, nil, preflight.MentionConfirmation, nil
	}

	if !s.api.config.Video.Enabled {
		for _, upload := range req.GetAttachments() {
			if strings.HasPrefix(upload.GetContentType(), "video/") {
				return nil, nil, nil, invalidArgument("video uploads are disabled on this server")
			}
		}
	}

	videoProcessingAssetIDs := make([]string, 0, len(req.GetAttachments()))
	for i, upload := range req.GetAttachments() {
		filename := strings.TrimSpace(upload.GetFilename())
		if filename == "" {
			filename = "attachment"
		}
		contentType := strings.TrimSpace(upload.GetContentType())
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		content := upload.GetContent()
		animatedGIF := s.api.config.Video.Enabled && contentType == "image/gif" && assets.IsAnimatedGIF(content)

		attachment, err := s.api.core.UploadAttachment(ctx, actorID, req.GetRoomId(), filename, contentType, bytes.NewReader(content))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to upload attachment %d (%s): %w", i+1, filename, err)
		}
		attachmentAssetIDs = append(attachmentAssetIDs, attachment.GetId())
		if s.api.config.Video.Enabled && core.AttachmentNeedsVideoProcessing(attachment, animatedGIF) {
			videoProcessingAssetIDs = append(videoProcessingAssetIDs, attachment.GetId())
		}
	}

	return attachmentAssetIDs, videoProcessingAssetIDs, nil, nil
}

func (s *messageService) UpdateMessage(ctx context.Context, req *connect.Request[apiv1.UpdateMessageRequest]) (*connect.Response[apiv1.UpdateMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.Messages().UpdateMessage(ctx, core.MessageUpdateInput{
		ActorID:           caller.UserID,
		RoomID:            req.Msg.RoomId,
		EventID:           req.Msg.EventId,
		Body:              req.Msg.Body,
		AlsoSendToChannel: req.Msg.AlsoSendToChannel,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateMessageResponse{Updated: true}), nil
}

func (s *messageService) DeleteMessage(ctx context.Context, req *connect.Request[apiv1.DeleteMessageRequest]) (*connect.Response[apiv1.DeleteMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.Messages().DeleteMessage(ctx, core.MessageDeleteInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
		EventID: req.Msg.EventId,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteMessageResponse{Deleted: true}), nil
}

func (s *messageService) DeleteAttachment(ctx context.Context, req *connect.Request[apiv1.DeleteAttachmentRequest]) (*connect.Response[apiv1.DeleteAttachmentResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.Messages().DeleteAttachment(ctx, core.MessageAttachmentDeleteInput{
		ActorID:      caller.UserID,
		RoomID:       req.Msg.RoomId,
		EventID:      req.Msg.EventId,
		AttachmentID: req.Msg.AttachmentId,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteAttachmentResponse{Deleted: true}), nil
}

func (s *messageService) DeleteLinkPreview(ctx context.Context, req *connect.Request[apiv1.DeleteLinkPreviewRequest]) (*connect.Response[apiv1.DeleteLinkPreviewResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.Messages().DeleteLinkPreview(ctx, core.MessageLinkPreviewDeleteInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
		EventID: req.Msg.EventId,
		URL:     req.Msg.Url,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteLinkPreviewResponse{Deleted: true}), nil
}

func (s *messageService) SendTypingIndicator(ctx context.Context, req *connect.Request[apiv1.SendTypingIndicatorRequest]) (*connect.Response[apiv1.SendTypingIndicatorResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	var threadRootEventID *string
	if req.Msg.ThreadRootEventId != "" {
		threadRootEventID = &req.Msg.ThreadRootEventId
	}
	if err := s.api.core.Messages().SendTypingIndicator(ctx, core.TypingIndicatorInput{
		ActorID:           caller.UserID,
		RoomID:            req.Msg.RoomId,
		ThreadRootEventID: threadRootEventID,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SendTypingIndicatorResponse{Sent: true}), nil
}

func (s *messageService) hydratePostedEvent(ctx context.Context, viewerID string, kind core.RoomKind, event *corev1.Event) (*apiv1.RoomTimelineEvent, *apiv1.RoomTimelineIncludes, error) {
	reactionsByMessageID, err := s.api.core.GetReactionsBatch(ctx, []string{event.Id})
	if err != nil {
		return nil, nil, err
	}
	h := &timelineHydrator{
		api:                  s.api,
		ctx:                  ctx,
		viewerID:             viewerID,
		kind:                 kind,
		reactionsByMessageID: reactionsByMessageID,
		userIDs:              make(map[string]struct{}),
	}
	apiEvent, err := h.event(&core.RoomEvent{Event: event})
	if err != nil {
		return nil, nil, err
	}
	users, err := h.users()
	if err != nil {
		return nil, nil, err
	}
	return apiEvent, &apiv1.RoomTimelineIncludes{Users: users}, nil
}

func apiMessageLinkPreviewToCore(input *apiv1.LinkPreview) *corev1.LinkPreview {
	if input == nil {
		return nil
	}
	preview := &corev1.LinkPreview{
		Url:         input.GetUrl(),
		Title:       input.GetTitle(),
		Description: input.GetDescription(),
		SiteName:    input.GetSiteName(),
		EmbedType:   input.GetEmbedType(),
	}
	if imageAssetID := input.GetImageAssetId(); imageAssetID != "" {
		preview.ImageAssetId = &imageAssetID
	}
	if embedID := input.GetEmbedId(); embedID != "" {
		preview.EmbedId = &embedID
	}
	return preview
}
