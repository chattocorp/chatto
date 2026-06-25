package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type messageService struct {
	api *API
}

func (s *messageService) PostMessage(ctx context.Context, req *connect.Request[apiv1.PostMessageRequest]) (*connect.Response[apiv1.PostMessageResponse], error) {
	return handleAuthedUnary(ctx, func(ctx context.Context, user authenticatedUser) (*apiv1.PostMessageResponse, error) {
		result, err := s.api.core.Messages().PostMessage(ctx, core.MessagePostInput{
			ActorID:                  user.Id,
			RoomID:                   req.Msg.RoomId,
			Body:                     req.Msg.Body,
			AttachmentAssetIDs:       append([]string(nil), req.Msg.AttachmentAssetIds...),
			ThreadRootEventID:        req.Msg.ThreadRootEventId,
			InReplyTo:                req.Msg.InReplyTo,
			AlsoSendToChannel:        req.Msg.AlsoSendToChannel,
			MentionConfirmationToken: req.Msg.MentionConfirmationToken,
			LinkPreview:              apiMessageLinkPreviewToCore(req.Msg.LinkPreview),
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("message post returned no result"))
		}
		if challenge := result.MentionConfirmation; challenge != nil {
			return &apiv1.PostMessageResponse{
				Result: &apiv1.PostMessageResponse_MentionConfirmation{
					MentionConfirmation: &apiv1.MentionConfirmationChallenge{
						RecipientCount: int32(challenge.RecipientCount),
						Token:          challenge.Token,
					},
				},
			}, nil
		}
		if result.Event == nil {
			return nil, connect.NewError(connect.CodeInternal, errors.New("message post returned no event"))
		}

		roomID := result.Event.GetMessagePosted().GetRoomId()
		kind := core.KindChannel
		if room, err := s.api.core.FindRoomByID(ctx, roomID); err == nil && room != nil {
			kind = core.KindOfRoom(room)
		}
		apiEvent, includes, err := s.hydratePostedEvent(ctx, user.Id, kind, result.Event)
		if err != nil {
			return nil, err
		}
		return &apiv1.PostMessageResponse{
			Result:   &apiv1.PostMessageResponse_Event{Event: apiEvent},
			Includes: includes,
		}, nil
	})
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

func apiMessageLinkPreviewToCore(input *apiv1.MessageLinkPreviewInput) *corev1.LinkPreview {
	if input == nil {
		return nil
	}
	preview := &corev1.LinkPreview{
		Url:         input.Url,
		Title:       input.Title,
		Description: input.Description,
		SiteName:    input.SiteName,
		EmbedType:   input.EmbedType,
	}
	if input.ImageAssetId != "" {
		preview.ImageAssetId = &input.ImageAssetId
	}
	if input.EmbedId != "" {
		preview.EmbedId = &input.EmbedId
	}
	return preview
}
