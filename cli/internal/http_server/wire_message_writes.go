package http_server

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWirePostMessage(ctx context.Context, userID, requestID string, body *apiv1.PostMessageRequest) (*apiv1.PostMessageResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	if body.GetThreadRootEventId() != "" {
		canPost, err := c.server.core.CanPostInThread(ctx, userID, kind, body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !canPost {
			return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
		}
	} else {
		canPost, err := c.server.core.CanPostMessage(ctx, userID, kind, body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !canPost {
			return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
		}
	}

	if body.GetAlsoSendToChannel() {
		if body.GetThreadRootEventId() == "" {
			return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: also_send_to_channel requires thread_root_event_id", errWireInvalidArgument))
		}
		canEcho, err := c.server.core.CanEchoMessage(ctx, userID, kind, body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !canEcho {
			return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
		}
		canPost, err := c.server.core.CanPostMessage(ctx, userID, kind, body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !canPost {
			return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
		}
	}

	mentionScope := core.MentionConfirmationScope{
		UserID:            userID,
		RoomID:            body.GetRoomId(),
		Kind:              kind,
		Body:              body.GetBody(),
		ThreadRootEventID: body.GetThreadRootEventId(),
		AlsoSendToChannel: body.GetAlsoSendToChannel(),
	}
	mentionConfirmed := body.GetLargeMentionConfirmed()
	if body.GetBody() != "" && !mentionConfirmed {
		recipientCount, err := c.server.core.MentionNotificationRecipientCountForBody(ctx, kind, body.GetRoomId(), userID, body.GetBody())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if recipientCount > core.LargeMentionNotificationThreshold {
			if err := c.server.core.ValidateMentionConfirmationToken(body.GetMentionConfirmationToken(), mentionScope); err != nil {
				token, err := c.server.core.CreateMentionConfirmationToken(mentionScope, recipientCount)
				if err != nil {
					return nil, c.errorFromRequestErr(requestID, err)
				}
				return nil, mentionConfirmationWireError(requestID, recipientCount, token)
			}
			mentionConfirmed = true
		}
	}

	opts := []core.PostMessageOption{}
	if mentionConfirmed {
		opts = append(opts, core.WithLargeMentionConfirmed())
	}
	if len(body.GetVideoProcessingAssetIds()) > 0 {
		opts = append(opts, core.WithVideoProcessingAssets(body.GetVideoProcessingAssetIds()...))
	}
	event, err := c.server.core.PostMessage(
		ctx,
		kind,
		body.GetRoomId(),
		userID,
		body.GetBody(),
		body.GetAttachmentAssetIds(),
		body.GetThreadRootEventId(),
		body.GetInReplyToEventId(),
		wireLinkPreviewInput(body.GetLinkPreview()),
		body.GetAlsoSendToChannel(),
		opts...,
	)
	if err != nil {
		var confirmErr *core.MentionConfirmationRequiredError
		if errors.As(err, &confirmErr) {
			token, tokenErr := c.server.core.CreateMentionConfirmationToken(mentionScope, confirmErr.RecipientCount)
			if tokenErr != nil {
				return nil, c.errorFromRequestErr(requestID, tokenErr)
			}
			return nil, mentionConfirmationWireError(requestID, confirmErr.RecipientCount, token)
		}
		return nil, c.errorFromRequestErr(requestID, err)
	}
	c.autoMarkRoomReadAfterPost(ctx, userID, kind, body.GetRoomId(), body.GetThreadRootEventId(), event.GetId())
	seq, err := c.server.core.GetEventSequence(ctx, kind, body.GetRoomId(), event.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.PostMessageResponse{Event: cloneEvent(event), Sequence: seq}, nil
}

func (c *wireConn) handleWireAddReaction(ctx context.Context, userID, requestID string, body *apiv1.AddReactionRequest) (*apiv1.AddReactionResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	canReact, err := c.server.core.CanReactToMessage(ctx, userID, kind, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canReact {
		return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
	}

	changed, err := c.server.core.AddReaction(ctx, kind, body.GetRoomId(), body.GetMessageEventId(), body.GetEmoji(), userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.AddReactionResponse{Changed: changed}, nil
}

func wireLinkPreviewInput(input *apiv1.LinkPreviewInput) *corev1.LinkPreview {
	if input == nil || input.GetUrl() == "" {
		return nil
	}
	return &corev1.LinkPreview{
		Url:          input.GetUrl(),
		Title:        input.GetTitle(),
		Description:  input.GetDescription(),
		SiteName:     input.GetSiteName(),
		ImageAssetId: optionalString(input.GetImageAssetId()),
		EmbedType:    input.GetEmbedType(),
		EmbedId:      optionalString(input.GetEmbedId()),
	}
}

func mentionConfirmationWireError(requestID string, recipientCount int, token string) *wirev1.WireError {
	err := wireError(requestID, wirev1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT, "mention confirmation required", false)
	err.MentionConfirmationRequired = &wirev1.MentionConfirmationRequiredError{
		RecipientCount: int32(recipientCount),
		Token:          token,
	}
	return err
}

func (c *wireConn) handleWireRemoveReaction(ctx context.Context, userID, requestID string, body *apiv1.RemoveReactionRequest) (*apiv1.RemoveReactionResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	canReact, err := c.server.core.CanReactToMessage(ctx, userID, kind, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if !canReact {
		return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
	}

	changed, err := c.server.core.RemoveReaction(ctx, kind, body.GetRoomId(), body.GetMessageEventId(), body.GetEmoji(), userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.RemoveReactionResponse{Changed: changed}, nil
}

func (c *wireConn) handleWireDeleteMessage(ctx context.Context, userID, requestID string, body *apiv1.DeleteMessageRequest) (*apiv1.DeleteMessageResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	messageBodyKey, err := c.resolveWireMessageBodyKey(ctx, kind, body.GetRoomId(), body.GetEventId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	authorID, err := c.server.core.GetMessageAuthorID(ctx, kind, messageBodyKey)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if authorID == "" {
		return &apiv1.DeleteMessageResponse{}, nil
	}
	if userID != authorID {
		canManage, err := c.server.core.CanManageOthersMessage(ctx, userID, kind, body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !canManage {
			return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
		}
	}

	if err := c.server.core.DeleteMessage(ctx, userID, kind, body.GetRoomId(), messageBodyKey); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DeleteMessageResponse{}, nil
}

func (c *wireConn) handleWireDeleteAttachment(ctx context.Context, userID, requestID string, body *apiv1.DeleteAttachmentRequest) (*apiv1.DeleteAttachmentResponse, *wirev1.WireError) {
	if body.GetAttachmentId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: attachment_id is required", errWireInvalidArgument))
	}
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	messageBodyKey, err := c.resolveWireMessageBodyKey(ctx, kind, body.GetRoomId(), body.GetEventId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.DeleteAttachmentFromMessage(ctx, userID, kind, body.GetRoomId(), messageBodyKey, body.GetAttachmentId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DeleteAttachmentResponse{}, nil
}

func (c *wireConn) handleWireDeleteLinkPreview(ctx context.Context, userID, requestID string, body *apiv1.DeleteLinkPreviewRequest) (*apiv1.DeleteLinkPreviewResponse, *wirev1.WireError) {
	if body.GetUrl() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: url is required", errWireInvalidArgument))
	}
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	messageBodyKey, err := c.resolveWireMessageBodyKey(ctx, kind, body.GetRoomId(), body.GetEventId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.DeleteLinkPreviewFromMessage(ctx, userID, kind, body.GetRoomId(), messageBodyKey, body.GetUrl()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DeleteLinkPreviewResponse{}, nil
}

func (c *wireConn) handleWireUpdateMessage(ctx context.Context, userID, requestID string, body *apiv1.UpdateMessageRequest) (*apiv1.UpdateMessageResponse, *wirev1.WireError) {
	_, kind, err := c.authorizedRoom(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	messageBodyKey, err := c.resolveWireMessageBodyKey(ctx, kind, body.GetRoomId(), body.GetEventId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	messageBody, err := c.server.core.GetFullMessageBody(ctx, kind, messageBodyKey)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if messageBody == nil {
		return nil, c.errorFromRequestErr(requestID, core.ErrMessageNotFound)
	}

	if messageBody.AuthorId != userID {
		canManage, err := c.server.core.CanManageOthersMessage(ctx, userID, kind, body.GetRoomId())
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if !canManage {
			return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
		}
	}

	var editOptions []core.EditMessageOption
	if body.AlsoSendToChannel != nil {
		if messageBody.AuthorId != userID {
			return nil, c.errorFromRequestErr(requestID, core.ErrNotMessageAuthor)
		}
		if body.GetAlsoSendToChannel() {
			canEcho, err := c.server.core.CanEchoMessage(ctx, userID, kind, body.GetRoomId())
			if err != nil {
				return nil, c.errorFromRequestErr(requestID, err)
			}
			if !canEcho {
				return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
			}
			canPost, err := c.server.core.CanPostMessage(ctx, userID, kind, body.GetRoomId())
			if err != nil {
				return nil, c.errorFromRequestErr(requestID, err)
			}
			if !canPost {
				return nil, wireError(requestID, wirev1.ErrorCode_ERROR_CODE_PERMISSION_DENIED, "permission denied", false)
			}
		}
		editOptions = append(editOptions, core.WithMessageChannelEcho(body.GetAlsoSendToChannel()))
	}

	if err := c.server.core.EditMessage(ctx, userID, kind, body.GetRoomId(), messageBodyKey, body.GetBody(), editOptions...); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateMessageResponse{}, nil
}

func (c *wireConn) resolveWireMessageBodyKey(ctx context.Context, kind core.RoomKind, roomID, eventID string) (string, error) {
	if eventID == "" {
		return "", fmt.Errorf("%w: event_id is required", errWireInvalidArgument)
	}
	event, err := c.server.core.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return "", err
	}
	if event == nil {
		return "", core.ErrMessageNotFound
	}
	if event.GetMessagePosted() == nil {
		return "", fmt.Errorf("%w: event is not a message", errWireInvalidArgument)
	}
	return event.GetId(), nil
}

func (c *wireConn) autoMarkRoomReadAfterPost(ctx context.Context, userID string, kind core.RoomKind, roomID, threadRootEventID, postedEventID string) {
	lastRootID := postedEventID
	if threadRootEventID != "" {
		id, _, exists, err := c.server.core.GetRoomLastEvent(ctx, kind, roomID)
		if err != nil {
			c.server.logger.Warn("Failed to get room last event for wire auto-mark-read", "error", err)
		} else if exists {
			lastRootID = id
		}
	}
	if lastRootID == "" {
		return
	}
	if err := c.server.core.SetLastReadEventID(ctx, kind, userID, roomID, lastRootID); err != nil {
		c.server.logger.Warn("Failed to auto-mark room as read for wire post", "error", err)
		return
	}
	c.server.core.NotifyRoomMarkedAsRead(ctx, userID, kind, roomID)
}
