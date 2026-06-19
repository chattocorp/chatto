package http_server

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	wireAttachmentThumbWidth  = 960
	wireAttachmentThumbHeight = 800
	wireLinkPreviewWidth      = 600
	wireLinkPreviewHeight     = 314
)

type attachmentViewOptions struct {
	thumbnailWidth  int
	thumbnailHeight int
	thumbnailFit    string
}

func defaultAttachmentViewOptions() attachmentViewOptions {
	return attachmentViewOptions{
		thumbnailWidth:  wireAttachmentThumbWidth,
		thumbnailHeight: wireAttachmentThumbHeight,
		thumbnailFit:    "contain",
	}
}

func (c *wireConn) roomEventsPage(ctx context.Context, userID string, kind core.RoomKind, result *core.RoomEventsResult) (*apiv1.RoomEventsPage, error) {
	page := &apiv1.RoomEventsPage{}
	if result == nil {
		return page, nil
	}
	page.HasOlder = result.HasOlder
	page.HasNewer = result.HasNewer
	page.StartSequence = result.StartCursorSeq
	page.EndSequence = result.EndCursorSeq
	page.Events = make([]*apiv1.RoomEventView, 0, len(result.Events))
	for _, event := range result.Events {
		view, err := c.roomEventView(ctx, userID, kind, event)
		if err != nil {
			return nil, err
		}
		if view != nil {
			page.Events = append(page.Events, view)
		}
	}
	return page, nil
}

func (c *wireConn) roomEventsAroundPage(ctx context.Context, userID string, kind core.RoomKind, result *core.RoomEventsAroundResult) (*apiv1.RoomEventsPage, error) {
	page := &apiv1.RoomEventsPage{}
	if result == nil {
		return page, nil
	}
	page.HasOlder = result.HasOlder
	page.HasNewer = result.HasNewer
	page.TargetIndex = int32(result.TargetIndex)
	if len(result.Events) > 0 {
		page.StartSequence = result.Events[0].Sequence
		page.EndSequence = result.Events[len(result.Events)-1].Sequence
	}
	page.Events = make([]*apiv1.RoomEventView, 0, len(result.Events))
	for _, event := range result.Events {
		view, err := c.roomEventView(ctx, userID, kind, event)
		if err != nil {
			return nil, err
		}
		if view != nil {
			page.Events = append(page.Events, view)
		}
	}
	return page, nil
}

func (c *wireConn) roomEventView(ctx context.Context, userID string, kind core.RoomKind, event *core.RoomEvent) (*apiv1.RoomEventView, error) {
	if event == nil || event.Event == nil {
		return nil, nil
	}
	payload, err := c.roomEventPayload(ctx, userID, kind, event.Event)
	if err != nil {
		return nil, err
	}
	return &apiv1.RoomEventView{
		Id:        event.Event.GetId(),
		CreatedAt: cloneTimestamp(event.Event.GetCreatedAt()),
		ActorId:   event.Event.GetActorId(),
		Actor:     c.optionalUser(ctx, event.Event.GetActorId()),
		Sequence:  event.Sequence,
		RawEvent:  cloneEvent(event.Event),
		Event:     payload,
	}, nil
}

func (c *wireConn) threadRootEventView(ctx context.Context, userID string, kind core.RoomKind, roomID, threadRootEventID string) (*apiv1.RoomEventView, error) {
	rootEvent, err := c.server.core.GetRoomEventByEventID(ctx, kind, roomID, threadRootEventID)
	if err != nil {
		return nil, err
	}
	if rootEvent == nil {
		return nil, core.ErrMessageNotFound
	}
	seq, err := c.server.core.GetEventSequence(ctx, kind, roomID, rootEvent.GetId())
	if err != nil {
		return nil, err
	}
	return c.roomEventView(ctx, userID, kind, &core.RoomEvent{Event: rootEvent, Sequence: seq})
}

func (c *wireConn) roomEventPayload(ctx context.Context, userID string, kind core.RoomKind, event *corev1.Event) (*apiv1.RoomEventPayload, error) {
	if event == nil {
		return nil, nil
	}

	switch e := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		view, err := c.messagePostedView(ctx, userID, kind, event.GetId(), e.MessagePosted)
		if err != nil {
			return nil, err
		}
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_MessagePosted{MessagePosted: view}}, nil
	case *corev1.Event_MessageEdited:
		view, err := c.messageEditedView(ctx, userID, e.MessageEdited)
		if err != nil {
			return nil, err
		}
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_MessageEdited{MessageEdited: view}}, nil
	case *corev1.Event_MessageRetracted:
		value := e.MessageRetracted
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_MessageRetracted{MessageRetracted: &apiv1.MessageRetractedView{
			RoomId:         value.GetRoomId(),
			MessageEventId: value.GetEventId(),
			Reason:         optionalString(value.GetReason()),
		}}}, nil
	case *corev1.Event_RoomCreated:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_RoomCreated{RoomCreated: roomScopedView(e.RoomCreated.GetRoomId())}}, nil
	case *corev1.Event_RoomUpdated:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_RoomUpdated{RoomUpdated: roomScopedView(e.RoomUpdated.GetRoomId())}}, nil
	case *corev1.Event_RoomDeleted:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_RoomDeleted{RoomDeleted: roomScopedView(e.RoomDeleted.GetRoomId())}}, nil
	case *corev1.Event_RoomArchived:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_RoomArchived{RoomArchived: roomScopedView(e.RoomArchived.GetRoomId())}}, nil
	case *corev1.Event_RoomUnarchived:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_RoomUnarchived{RoomUnarchived: roomScopedView(e.RoomUnarchived.GetRoomId())}}, nil
	case *corev1.Event_UserJoinedRoom:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_UserJoinedRoom{UserJoinedRoom: roomScopedView(e.UserJoinedRoom.GetRoomId())}}, nil
	case *corev1.Event_UserLeftRoom:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_UserLeftRoom{UserLeftRoom: roomScopedView(e.UserLeftRoom.GetRoomId())}}, nil
	case *corev1.Event_ReactionAdded:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_ReactionAdded{ReactionAdded: &apiv1.ReactionEventView{
			RoomId:         e.ReactionAdded.GetRoomId(),
			MessageEventId: e.ReactionAdded.GetMessageEventId(),
			Emoji:          e.ReactionAdded.GetEmoji(),
		}}}, nil
	case *corev1.Event_ReactionRemoved:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_ReactionRemoved{ReactionRemoved: &apiv1.ReactionEventView{
			RoomId:         e.ReactionRemoved.GetRoomId(),
			MessageEventId: e.ReactionRemoved.GetMessageEventId(),
			Emoji:          e.ReactionRemoved.GetEmoji(),
		}}}, nil
	case *corev1.Event_AssetProcessingStarted:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_AssetProcessingStarted{AssetProcessingStarted: c.assetProcessingView(e.AssetProcessingStarted.GetAssetId(), e.AssetProcessingStarted.GetMessageEventId())}}, nil
	case *corev1.Event_AssetProcessingSucceeded:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_AssetProcessingSucceeded{AssetProcessingSucceeded: c.assetProcessingView(e.AssetProcessingSucceeded.GetAssetId(), e.AssetProcessingSucceeded.GetMessageEventId())}}, nil
	case *corev1.Event_AssetProcessingFailed:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_AssetProcessingFailed{AssetProcessingFailed: c.assetProcessingView(e.AssetProcessingFailed.GetAssetId(), e.AssetProcessingFailed.GetMessageEventId())}}, nil
	case *corev1.Event_AssetDeleted:
		roomID, _ := c.server.core.Assets.AssetRoomID(e.AssetDeleted.GetAssetId())
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_AssetDeleted{AssetDeleted: &apiv1.AssetDeletedView{
			RoomId:  roomID,
			AssetId: e.AssetDeleted.GetAssetId(),
		}}}, nil
	case *corev1.Event_ServerMemberDeleted:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_ServerMemberDeleted{ServerMemberDeleted: &apiv1.ServerMemberDeletedView{UserId: e.ServerMemberDeleted.GetUserId()}}}, nil
	case *corev1.Event_VoiceCallStarted:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_CallStarted{CallStarted: callEventView(e.VoiceCallStarted.GetRoomId(), e.VoiceCallStarted.GetCallId())}}, nil
	case *corev1.Event_VoiceCallParticipantJoined:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_CallParticipantJoined{CallParticipantJoined: callEventView(e.VoiceCallParticipantJoined.GetRoomId(), e.VoiceCallParticipantJoined.GetCallId())}}, nil
	case *corev1.Event_VoiceCallParticipantLeft:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_CallParticipantLeft{CallParticipantLeft: callEventView(e.VoiceCallParticipantLeft.GetRoomId(), e.VoiceCallParticipantLeft.GetCallId())}}, nil
	case *corev1.Event_VoiceCallEnded:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_CallEnded{CallEnded: callEventView(e.VoiceCallEnded.GetRoomId(), e.VoiceCallEnded.GetCallId())}}, nil
	case *corev1.Event_ThreadCreated:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_ThreadCreated{ThreadCreated: &apiv1.ThreadCreatedView{
			RoomId:            e.ThreadCreated.GetRoomId(),
			ThreadRootEventId: e.ThreadCreated.GetThreadRootEventId(),
		}}}, nil
	case *corev1.Event_RoomMemberBanned:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_RoomMemberBanned{RoomMemberBanned: &apiv1.RoomModerationEventView{
			RoomId:    e.RoomMemberBanned.GetRoomId(),
			UserId:    e.RoomMemberBanned.GetUserId(),
			Reason:    optionalString(e.RoomMemberBanned.GetReason()),
			ExpiresAt: cloneTimestamp(e.RoomMemberBanned.GetExpiresAt()),
		}}}, nil
	case *corev1.Event_RoomMemberUnbanned:
		return &apiv1.RoomEventPayload{Payload: &apiv1.RoomEventPayload_RoomMemberUnbanned{RoomMemberUnbanned: &apiv1.RoomModerationEventView{
			RoomId: e.RoomMemberUnbanned.GetRoomId(),
			UserId: e.RoomMemberUnbanned.GetUserId(),
			Reason: optionalString(e.RoomMemberUnbanned.GetReason()),
		}}}, nil
	default:
		return nil, nil
	}
}

func (c *wireConn) messagePostedView(ctx context.Context, userID string, kind core.RoomKind, eventID string, payload *corev1.MessagePostedEvent) (*apiv1.MessagePostedView, error) {
	view := &apiv1.MessagePostedView{}
	if payload == nil {
		return view, nil
	}
	view.RoomId = payload.GetRoomId()
	view.InReplyTo = optionalString(payload.GetInReplyTo())
	view.ThreadRootEventId = optionalString(payload.GetInThread())
	view.EchoOfEventId = optionalString(payload.GetEchoOfEventId())
	view.EchoFromThreadRootEventId = optionalString(payload.GetEchoFromThreadRootEventId())
	if payload.GetInThread() != "" && payload.GetEchoOfEventId() == "" {
		if echoID, ok := c.server.core.RoomTimeline.ChannelEchoEventID(eventID); ok {
			view.ChannelEchoEventId = optionalString(echoID)
		}
	}

	body, err := c.server.core.GetFullMessageBodyByEventID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	c.applyMessageBodyView(userID, payload.GetRoomId(), eventID, body, view)

	if payload.GetInThread() == "" {
		c.applyThreadMetadata(ctx, userID, kind, payload.GetRoomId(), eventID, view)
	}

	reactions, err := c.reactionSummaryViews(ctx, userID, eventID)
	if err != nil {
		return nil, err
	}
	view.Reactions = reactions

	return view, nil
}

func (c *wireConn) messageEditedView(ctx context.Context, userID string, payload *corev1.MessageEditedEvent) (*apiv1.MessageEditedView, error) {
	view := &apiv1.MessageEditedView{}
	if payload == nil {
		return view, nil
	}
	view.RoomId = payload.GetRoomId()
	view.MessageEventId = payload.GetEventId()
	body, err := c.server.core.GetFullMessageBodyByEventID(ctx, payload.GetEventId())
	if err != nil {
		return nil, err
	}
	if body == nil {
		return view, nil
	}
	view.Body = optionalStringAllowEmpty(body.Body)
	view.Attachments = c.attachmentViews(userID, payload.GetRoomId(), payload.GetEventId(), body.Attachments)
	view.LinkPreview = c.linkPreviewView(body.LinkPreview)
	if body.UpdatedAt != nil {
		view.UpdatedAt = timestamppb.New(*body.UpdatedAt)
	}
	return view, nil
}

func (c *wireConn) applyMessageBodyView(userID, roomID, eventID string, body *core.DecryptedMessageBody, view *apiv1.MessagePostedView) {
	if body == nil || view == nil {
		return
	}
	view.Body = optionalStringAllowEmpty(body.Body)
	view.Attachments = c.attachmentViews(userID, roomID, eventID, body.Attachments)
	view.LinkPreview = c.linkPreviewView(body.LinkPreview)
	if body.UpdatedAt != nil {
		view.UpdatedAt = timestamppb.New(*body.UpdatedAt)
	}
}

func (c *wireConn) applyThreadMetadata(ctx context.Context, userID string, kind core.RoomKind, roomID, eventID string, view *apiv1.MessagePostedView) {
	metadata, err := c.server.core.GetThreadMetadata(ctx, kind, roomID, eventID)
	if err != nil {
		c.server.logger.Debug("Wire failed to hydrate thread metadata", "error", err, "event_id", eventID)
		return
	}
	if metadata != nil {
		view.ReplyCount = int32(metadata.ReplyCount)
		if metadata.LastReplyAt != nil {
			view.LastReplyAt = timestamppb.New(*metadata.LastReplyAt)
		}
		for i, participantID := range metadata.ParticipantIDs {
			if i >= 5 {
				break
			}
			if user := c.optionalUser(ctx, participantID); user != nil {
				view.ThreadParticipants = append(view.ThreadParticipants, user)
			}
		}
	}
	if userID == "" {
		return
	}
	following, err := c.server.core.IsFollowingThread(ctx, kind, userID, roomID, eventID)
	if err != nil {
		c.server.logger.Debug("Wire failed to hydrate thread follow state", "error", err, "event_id", eventID)
		return
	}
	view.ViewerIsFollowingThread = &following
}

func (c *wireConn) reactionSummaryViews(ctx context.Context, userID, eventID string) ([]*apiv1.ReactionSummaryView, error) {
	summaries, err := c.server.core.GetReactions(ctx, eventID)
	if err != nil {
		return nil, err
	}
	views := make([]*apiv1.ReactionSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		view := &apiv1.ReactionSummaryView{
			Emoji:      summary.Emoji,
			Count:      int32(len(summary.UserIDs)),
			HasReacted: containsString(summary.UserIDs, userID),
		}
		for i, reactionUserID := range summary.UserIDs {
			if i >= 5 {
				break
			}
			if user := c.optionalUser(ctx, reactionUserID); user != nil {
				view.Users = append(view.Users, user)
			}
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *wireConn) attachmentViews(userID, roomID, messageBodyID string, attachments []*corev1.Attachment) []*apiv1.AttachmentView {
	return c.attachmentViewsWithOptions(userID, roomID, messageBodyID, attachments, defaultAttachmentViewOptions())
}

func (c *wireConn) attachmentViewsWithOptions(userID, roomID, messageBodyID string, attachments []*corev1.Attachment, opts attachmentViewOptions) []*apiv1.AttachmentView {
	views := make([]*apiv1.AttachmentView, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		views = append(views, c.attachmentViewWithOptions(userID, roomID, messageBodyID, attachment, opts))
	}
	return views
}

func (c *wireConn) attachmentView(userID, roomID, messageBodyID string, attachment *corev1.Attachment) *apiv1.AttachmentView {
	return c.attachmentViewWithOptions(userID, roomID, messageBodyID, attachment, defaultAttachmentViewOptions())
}

func (c *wireConn) attachmentViewWithOptions(userID, roomID, messageBodyID string, attachment *corev1.Attachment, opts attachmentViewOptions) *apiv1.AttachmentView {
	assetID := attachment.GetId()
	view := &apiv1.AttachmentView{
		Id:                assetID,
		Filename:          attachment.GetFilename(),
		ContentType:       attachment.GetContentType(),
		Width:             attachment.GetWidth(),
		Height:            attachment.GetHeight(),
		AssetUrl:          c.assetURLView(c.server.core.GetStableAttachmentAssetURL(assetID, userID)),
		ThumbnailAssetUrl: c.assetURLView(c.server.core.GetStableTransformedAttachmentAssetURL(assetID, userID, opts.thumbnailWidth, opts.thumbnailHeight, opts.thumbnailFit)),
	}
	if videoProcessing := c.videoProcessingView(userID, roomID, messageBodyID, attachment); videoProcessing != nil {
		view.VideoProcessing = videoProcessing
	}
	return view
}

func (c *wireConn) videoProcessingView(userID, roomID, _ string, attachment *corev1.Attachment) *apiv1.VideoProcessingView {
	if attachment == nil || (!strings.HasPrefix(attachment.GetContentType(), "video/") && attachment.GetContentType() != "image/gif") {
		return nil
	}
	manifest, ok := c.server.core.Assets.VideoAttachmentManifest(attachment.GetId())
	if !ok || manifest == nil {
		return nil
	}
	if succeeded := manifest.Succeeded; succeeded != nil {
		video := succeeded.GetVideo()
		if video == nil {
			return nil
		}
		view := &apiv1.VideoProcessingView{
			Status:          apiv1.VideoProcessingStatus_VIDEO_PROCESSING_STATUS_COMPLETED,
			SourceAvailable: c.assetSourceAvailable(attachment.GetId(), true),
		}
		if video.GetDurationMs() > 0 {
			value := video.GetDurationMs()
			view.DurationMs = &value
		}
		if video.GetWidth() > 0 {
			value := video.GetWidth()
			view.Width = &value
		}
		if video.GetHeight() > 0 {
			value := video.GetHeight()
			view.Height = &value
		}
		if thumbID := video.GetThumbnailAssetId(); thumbID != "" {
			view.ThumbnailAssetUrl = c.assetURLView(c.server.core.GetStableAttachmentAssetURL(thumbID, userID))
		}
		for _, variant := range video.GetVariants() {
			if variant == nil {
				continue
			}
			var width, height int32
			var size int64
			if created, ok := c.server.core.Assets.AssetCreation(variant.GetAssetId()); ok {
				asset := created.GetAsset()
				width = asset.GetWidth()
				height = asset.GetHeight()
				size = asset.GetSize()
			}
			view.Variants = append(view.Variants, &apiv1.VideoVariantView{
				Quality:  variant.GetQuality(),
				Width:    width,
				Height:   height,
				Size:     size,
				AssetUrl: c.assetURLView(c.server.core.GetStableAttachmentAssetURL(variant.GetAssetId(), userID)),
			})
		}
		return view
	}
	if failed := manifest.Failed; failed != nil {
		reasonCode := assetProcessingFailureReasonCode(failed.GetFailureCode())
		sourceAvailable := reasonCode != "original_missing" && c.assetSourceAvailable(attachment.GetId(), true)
		return &apiv1.VideoProcessingView{
			Status:          apiv1.VideoProcessingStatus_VIDEO_PROCESSING_STATUS_FAILED,
			ReasonCode:      optionalString(reasonCode),
			SourceAvailable: sourceAvailable,
		}
	}
	if manifest.Started != nil {
		return &apiv1.VideoProcessingView{
			Status:          apiv1.VideoProcessingStatus_VIDEO_PROCESSING_STATUS_PROCESSING,
			SourceAvailable: c.assetSourceAvailable(attachment.GetId(), true),
		}
	}
	return nil
}

func (c *wireConn) linkPreviewView(preview *corev1.LinkPreview) *apiv1.LinkPreviewView {
	if preview == nil || preview.GetUrl() == "" {
		return nil
	}
	view := &apiv1.LinkPreviewView{
		Url:         preview.GetUrl(),
		Title:       preview.GetTitle(),
		Description: preview.GetDescription(),
		SiteName:    preview.GetSiteName(),
		EmbedType:   preview.GetEmbedType(),
		EmbedId:     optionalString(preview.GetEmbedId()),
	}
	if imageAssetID := preview.GetImageAssetId(); imageAssetID != "" {
		view.ImageAssetId = optionalString(imageAssetID)
		view.ImageUrl = optionalString(c.server.core.GetTransformedServerAssetURL(imageAssetID, wireLinkPreviewWidth, wireLinkPreviewHeight, "contain"))
	}
	return view
}

func (c *wireConn) assetProcessingView(assetID, messageEventID string) *apiv1.AssetProcessingEventView {
	roomID, _ := c.server.core.Assets.AssetRoomID(assetID)
	return &apiv1.AssetProcessingEventView{
		RoomId:         roomID,
		AssetId:        assetID,
		MessageEventId: messageEventID,
	}
}

func (c *wireConn) assetURLView(assetURL core.StableAssetURL) *apiv1.AssetUrl {
	if assetURL.URL == "" {
		return nil
	}
	return &apiv1.AssetUrl{
		Url:       assetURL.URL,
		ExpiresAt: timestamppb.New(assetURL.ExpiresAt),
	}
}

func (c *wireConn) assetSourceAvailable(assetID string, fallback bool) bool {
	created, ok := c.server.core.Assets.AssetCreation(assetID)
	if !ok || created == nil {
		return fallback
	}
	return created.GetOriginalBinaryAvailable()
}

func (c *wireConn) optionalUser(ctx context.Context, userID string) *corev1.User {
	if userID == "" {
		return nil
	}
	user, err := c.server.core.GetUser(ctx, userID)
	if err != nil {
		if !errors.Is(err, core.ErrNotFound) {
			c.server.logger.Debug("Wire failed to hydrate user", "error", err, "user_id", userID)
		}
		return nil
	}
	return cloneUser(user)
}

func roomScopedView(roomID string) *apiv1.RoomScopedEventView {
	return &apiv1.RoomScopedEventView{RoomId: roomID}
}

func callEventView(roomID, callID string) *apiv1.CallEventView {
	return &apiv1.CallEventView{RoomId: roomID, CallId: callID}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalStringAllowEmpty(value string) *string {
	return &value
}

func cloneTimestamp(value *timestamppb.Timestamp) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return proto.Clone(value).(*timestamppb.Timestamp)
}

func containsString(values []string, target string) bool {
	if target == "" {
		return false
	}
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func assetProcessingFailureReasonCode(code corev1.AssetProcessingFailureCode) string {
	switch code {
	case corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING:
		return "original_missing"
	case corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED:
		return "processing_failed"
	default:
		return "processing_failed"
	}
}
