package connectapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type roomTimelineAssembler struct {
	api       *API
	thumbnail attachmentThumbnailRequest
}

func newRoomTimelineAssembler(api *API) *roomTimelineAssembler {
	return newRoomTimelineAssemblerWithThumbnail(api, defaultTimelineAttachmentThumbnail())
}

func defaultTimelineAttachmentThumbnail() attachmentThumbnailRequest {
	return attachmentThumbnailRequest{
		width:  960,
		height: 400,
		fit:    "contain",
	}
}

func newRoomTimelineAssemblerWithThumbnail(api *API, thumbnail attachmentThumbnailRequest) *roomTimelineAssembler {
	return &roomTimelineAssembler{api: api, thumbnail: thumbnail}
}

// buildPage turns projected room timeline entries into the public Connect view.
// The projected event log intentionally stores facts, not UI rows: message
// bodies, reactions, thread metadata, and users live in sibling projections.
// Hydrating them here keeps the public API free of per-field resolver N+1s
// and gives future clients one renderable page per request.
func (a *roomTimelineAssembler) buildPage(ctx context.Context, viewerID string, kind core.RoomKind, events []*core.RoomEvent, hasOlder, hasNewer bool) (*apiv1.RoomTimelinePage, error) {
	apiEvents, h, err := a.hydrateEvents(ctx, viewerID, kind, events)
	if err != nil {
		return nil, err
	}

	users, err := h.users()
	if err != nil {
		return nil, err
	}

	return &apiv1.RoomTimelinePage{
		Events:   apiEvents,
		HasOlder: hasOlder,
		HasNewer: hasNewer,
		Includes: &apiv1.RoomTimelineIncludes{Users: users},
	}, nil
}

func (a *roomTimelineAssembler) hydrateEvents(ctx context.Context, viewerID string, kind core.RoomKind, events []*core.RoomEvent) ([]*apiv1.RoomTimelineEvent, *timelineHydrator, error) {
	ctx = core.WithDEKRequestCache(ctx)

	messageIDs := make([]string, 0, len(events))
	for _, event := range events {
		if event.GetMessagePosted() != nil {
			messageIDs = append(messageIDs, event.Id)
		}
	}

	reactionsByMessageID, err := a.api.core.GetReactionsBatch(ctx, messageIDs)
	if err != nil {
		return nil, nil, err
	}

	h := &timelineHydrator{
		api:                  a.api,
		ctx:                  ctx,
		viewerID:             viewerID,
		kind:                 kind,
		reactionsByMessageID: reactionsByMessageID,
		userIDs:              make(map[string]struct{}),
		thumbnail:            a.thumbnail,
	}

	apiEvents, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, events, func(ctx context.Context, _ int, event *core.RoomEvent) (*apiv1.RoomTimelineEvent, error) {
		return h.event(ctx, event)
	})
	if err != nil {
		return nil, nil, err
	}

	return apiEvents, h, nil
}

func (a *roomTimelineAssembler) buildThreadPage(ctx context.Context, viewerID string, kind core.RoomKind, root *core.RoomEvent, replies *core.RoomEventsResult, includeRoot bool) (*apiv1.RoomTimelinePage, error) {
	events := make([]*core.RoomEvent, 0, 1+len(replies.Events))
	if includeRoot {
		events = append(events, root)
	}
	events = append(events, replies.Events...)

	page, err := a.buildPage(ctx, viewerID, kind, events, replies.HasOlder, replies.HasNewer)
	if err != nil {
		return nil, err
	}
	page.StartCursor = formatRoomTimelineCursor(replies.StartCursorSeq)
	page.EndCursor = formatRoomTimelineCursor(replies.EndCursorSeq)
	return page, nil
}

func (a *roomTimelineAssembler) hydrateEvent(ctx context.Context, viewerID string, kind core.RoomKind, event *corev1.Event) (*apiv1.RoomTimelineEvent, *apiv1.RoomTimelineIncludes, error) {
	ctx = core.WithDEKRequestCache(ctx)

	messageIDs := []string(nil)
	if event.GetMessagePosted() != nil {
		messageIDs = append(messageIDs, event.Id)
	}

	reactionsByMessageID, err := a.api.core.GetReactionsBatch(ctx, messageIDs)
	if err != nil {
		return nil, nil, err
	}

	h := &timelineHydrator{
		api:                  a.api,
		ctx:                  ctx,
		viewerID:             viewerID,
		kind:                 kind,
		reactionsByMessageID: reactionsByMessageID,
		userIDs:              make(map[string]struct{}),
		thumbnail:            a.thumbnail,
	}
	apiEvent, err := h.event(ctx, &core.RoomEvent{Event: event})
	if err != nil {
		return nil, nil, err
	}
	users, err := h.users()
	if err != nil {
		return nil, nil, err
	}
	return apiEvent, &apiv1.RoomTimelineIncludes{Users: users}, nil
}

type timelineHydrator struct {
	api                  *API
	ctx                  context.Context
	viewerID             string
	kind                 core.RoomKind
	reactionsByMessageID map[string][]core.ReactionSummary
	userMu               sync.Mutex
	userIDs              map[string]struct{}
	thumbnail            attachmentThumbnailRequest
}

func (h *timelineHydrator) event(ctx context.Context, event *core.RoomEvent) (*apiv1.RoomTimelineEvent, error) {
	if event == nil || event.Event == nil {
		return nil, nil
	}
	h.addUserID(event.ActorId)

	apiEvent := &apiv1.RoomTimelineEvent{
		Id:        event.Id,
		CreatedAt: event.CreatedAt,
		ActorId:   event.ActorId,
	}

	switch payload := event.Event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		message, err := h.messagePosted(ctx, event, payload.MessagePosted)
		if err != nil {
			return nil, err
		}
		apiEvent.Event = &apiv1.RoomTimelineEvent_MessagePosted{
			MessagePosted: &apiv1.RoomMessagePosted{Message: message},
		}
	case *corev1.Event_RoomCreated:
		apiEvent.Event = &apiv1.RoomTimelineEvent_RoomCreated{RoomCreated: roomEvent(payload.RoomCreated.GetRoomId())}
	case *corev1.Event_RoomUpdated:
		apiEvent.Event = &apiv1.RoomTimelineEvent_RoomUpdated{RoomUpdated: roomEvent(payload.RoomUpdated.GetRoomId())}
	case *corev1.Event_RoomDeleted:
		apiEvent.Event = &apiv1.RoomTimelineEvent_RoomDeleted{RoomDeleted: roomEvent(payload.RoomDeleted.GetRoomId())}
	case *corev1.Event_RoomArchived:
		apiEvent.Event = &apiv1.RoomTimelineEvent_RoomArchived{RoomArchived: roomEvent(payload.RoomArchived.GetRoomId())}
	case *corev1.Event_RoomUnarchived:
		apiEvent.Event = &apiv1.RoomTimelineEvent_RoomUnarchived{RoomUnarchived: roomEvent(payload.RoomUnarchived.GetRoomId())}
	case *corev1.Event_UserJoinedRoom:
		apiEvent.Event = &apiv1.RoomTimelineEvent_UserJoinedRoom{UserJoinedRoom: roomEvent(payload.UserJoinedRoom.GetRoomId())}
	case *corev1.Event_UserLeftRoom:
		apiEvent.Event = &apiv1.RoomTimelineEvent_UserLeftRoom{UserLeftRoom: roomEvent(payload.UserLeftRoom.GetRoomId())}
	default:
		return nil, fmt.Errorf("unsupported room timeline event %T", payload)
	}

	return apiEvent, nil
}

func (h *timelineHydrator) messagePosted(ctx context.Context, event *core.RoomEvent, payload *corev1.MessagePostedEvent) (*apiv1.Message, error) {
	message := &apiv1.Message{
		Id:                        event.Id,
		RoomId:                    payload.GetRoomId(),
		CreatedAt:                 event.CreatedAt,
		ActorId:                   event.ActorId,
		InReplyTo:                 payload.GetInReplyTo(),
		ThreadRootEventId:         payload.GetInThread(),
		EchoOfEventId:             payload.GetEchoOfEventId(),
		EchoFromThreadRootEventId: payload.GetEchoFromThreadRootEventId(),
		Reactions:                 h.reactions(event.Id),
	}
	if deletedAt, ok := h.api.core.RoomTimeline.MessageDeletedAt(event.Id); ok {
		message.DeletedAt = timestamppb.New(deletedAt)
	}

	if echoID, ok := h.api.core.RoomTimeline.ChannelEchoEventID(event.Id); ok {
		message.ChannelEchoEventId = echoID
	}

	body, err := h.api.core.GetFullMessageBodyByEventID(ctx, event.Id)
	if err != nil {
		if !errors.Is(err, core.ErrMessageBodyCorrupt) {
			return nil, err
		}
		// A single corrupt body envelope must not make the whole room history
		// unreadable. Keep the message envelope renderable and let clients show
		// the existing unavailable-message state.
		log.Warn("Failed to hydrate room timeline message body",
			"room_id", payload.GetRoomId(),
			"message_event_id", event.Id,
			"error", err)
		body = nil
	}
	if body != nil {
		message.Body = &body.Body
		message.Attachments = h.attachments(payload.GetRoomId(), event.Id, body.Attachments)
		message.LinkPreview = h.linkPreview(body.LinkPreview)
		if body.UpdatedAt != nil {
			message.UpdatedAt = timestamppb.New(*body.UpdatedAt)
		}
	}

	if payload.GetInThread() == "" {
		thread := &apiv1.ThreadSummary{
			ThreadRootEventId: event.Id,
		}
		metadata, err := h.api.core.GetThreadMetadata(ctx, h.kind, payload.GetRoomId(), event.Id)
		if err != nil && !errors.Is(err, core.ErrNotFound) {
			return nil, err
		}
		if metadata != nil {
			thread.ReplyCount = int32(metadata.ReplyCount)
			if metadata.LastReplyAt != nil {
				thread.LastReplyAt = timestamppb.New(*metadata.LastReplyAt)
			}
			thread.ParticipantPreviewUserIds = firstN(metadata.ParticipantIDs, 5)
			thread.ParticipantCount = int32(len(metadata.ParticipantIDs))
			h.addUserIDs(thread.ParticipantPreviewUserIds)
		}
		following, err := h.api.core.IsFollowingThread(ctx, h.kind, h.viewerID, payload.GetRoomId(), event.Id)
		if err != nil {
			return nil, err
		}
		thread.ViewerState = &apiv1.ThreadViewerState{IsFollowing: &following}
		message.Thread = thread
	}

	return message, nil
}

func (h *timelineHydrator) attachments(roomID, messageEventID string, attachments []*corev1.Attachment) []*apiv1.MessageAttachment {
	result := make([]*apiv1.MessageAttachment, 0, len(attachments))
	thumbnail := h.thumbnail
	if thumbnail.width <= 0 || thumbnail.height <= 0 || thumbnail.fit == "" {
		thumbnail = defaultTimelineAttachmentThumbnail()
	}
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if attachment.RoomId == "" {
			attachment.RoomId = roomID
		}
		if attachment.MessageBodyId == "" {
			attachment.MessageBodyId = messageEventID
		}
		assetURL := h.api.core.GetStableAttachmentAssetURL(attachment.Id, h.viewerID)
		thumbnailURL := h.api.core.GetStableTransformedAttachmentAssetURL(attachment.Id, h.viewerID, thumbnail.width, thumbnail.height, thumbnail.fit)
		result = append(result, &apiv1.MessageAttachment{
			Id:                attachment.Id,
			Filename:          attachment.Filename,
			ContentType:       attachment.ContentType,
			Width:             attachment.Width,
			Height:            attachment.Height,
			AssetUrl:          assetURLView(assetURL),
			ThumbnailAssetUrl: assetURLView(thumbnailURL),
			VideoProcessing:   h.videoProcessing(attachment),
		})
	}
	return result
}

func (h *timelineHydrator) videoProcessing(attachment *corev1.Attachment) *apiv1.MessageVideoProcessing {
	if attachment == nil || (!strings.HasPrefix(attachment.GetContentType(), "video/") && attachment.GetContentType() != "image/gif") {
		return nil
	}

	manifest, ok := h.api.core.Assets.VideoAttachmentManifest(attachment.GetId())
	if !ok || manifest == nil {
		return nil
	}

	if succeeded := manifest.Succeeded; succeeded != nil {
		video := succeeded.GetVideo()
		if video == nil {
			return nil
		}
		result := &apiv1.MessageVideoProcessing{
			Status:          apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_COMPLETED,
			DurationMs:      video.GetDurationMs(),
			Width:           video.GetWidth(),
			Height:          video.GetHeight(),
			SourceAvailable: h.assetSourceAvailable(attachment.GetId(), true),
		}
		if thumbnailID := video.GetThumbnailAssetId(); thumbnailID != "" {
			result.ThumbnailAssetUrl = assetURLView(h.api.core.GetStableAttachmentAssetURL(thumbnailID, h.viewerID))
		}
		for _, variant := range video.GetVariants() {
			if variant == nil {
				continue
			}
			var width, height int32
			var size int64
			if created, ok := h.api.core.Assets.AssetCreation(variant.GetAssetId()); ok {
				asset := created.GetAsset()
				if asset != nil {
					width = asset.GetWidth()
					height = asset.GetHeight()
					size = asset.GetSize()
				}
			}
			result.Variants = append(result.Variants, &apiv1.MessageVideoVariant{
				Quality:  variant.GetQuality(),
				Width:    width,
				Height:   height,
				Size:     size,
				AssetUrl: assetURLView(h.api.core.GetStableAttachmentAssetURL(variant.GetAssetId(), h.viewerID)),
			})
		}
		return result
	}

	if failed := manifest.Failed; failed != nil {
		reasonCode := assetProcessingFailureReasonCode(failed.GetFailureCode())
		return &apiv1.MessageVideoProcessing{
			Status:          apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_FAILED,
			SourceAvailable: reasonCode != "original_missing" && h.assetSourceAvailable(attachment.GetId(), true),
			ReasonCode:      reasonCode,
		}
	}

	if manifest.Started != nil {
		return &apiv1.MessageVideoProcessing{
			Status:          apiv1.MessageVideoProcessingStatus_MESSAGE_VIDEO_PROCESSING_STATUS_PROCESSING,
			SourceAvailable: h.assetSourceAvailable(attachment.GetId(), true),
		}
	}

	return nil
}

func (h *timelineHydrator) assetSourceAvailable(assetID string, fallback bool) bool {
	created, ok := h.api.core.Assets.AssetCreation(assetID)
	if !ok || created == nil {
		return fallback
	}
	return created.GetOriginalBinaryAvailable()
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

func (h *timelineHydrator) linkPreview(preview *corev1.LinkPreview) *apiv1.LinkPreview {
	return apiLinkPreview(h.api, preview)
}

func (h *timelineHydrator) reactions(messageEventID string) []*apiv1.MessageReaction {
	summaries := h.reactionsByMessageID[messageEventID]
	result := make([]*apiv1.MessageReaction, 0, len(summaries))
	for _, summary := range summaries {
		previewUserIDs := firstN(summary.UserIDs, 5)
		h.addUserIDs(previewUserIDs)
		result = append(result, &apiv1.MessageReaction{
			Emoji:          summary.Emoji,
			Count:          int32(len(summary.UserIDs)),
			HasReacted:     containsString(summary.UserIDs, h.viewerID),
			PreviewUserIds: previewUserIDs,
		})
	}
	return result
}

func (h *timelineHydrator) users() (map[string]*apiv1.User, error) {
	h.userMu.Lock()
	ids := make([]string, 0, len(h.userIDs))
	for id := range h.userIDs {
		ids = append(ids, id)
	}
	h.userMu.Unlock()

	coreUsers, err := h.api.core.GetUsers(h.ctx, ids)
	if err != nil {
		return nil, err
	}
	presences, err := h.api.core.GetUserPresences(h.ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*apiv1.User, len(ids))
	avatarWidth, avatarHeight := 96, 96
	for i, id := range ids {
		user := coreUsers[i]
		if user == nil {
			user = core.DeletedUserReference(id)
		}
		summary, err := (&userService{api: h.api}).userSummaryWithPresence(h.ctx, user, &apiv1.ImageTransformOptions{
			Width:  int32(avatarWidth),
			Height: int32(avatarHeight),
			Fit:    apiv1.ImageFitMode_IMAGE_FIT_MODE_COVER,
		}, presences[id])
		if err != nil {
			return nil, err
		}
		result[id] = summary
	}
	return result, nil
}

func (h *timelineHydrator) addUserID(userID string) {
	if userID == "" {
		return
	}
	h.userMu.Lock()
	h.userIDs[userID] = struct{}{}
	h.userMu.Unlock()
}

func (h *timelineHydrator) addUserIDs(userIDs []string) {
	h.userMu.Lock()
	defer h.userMu.Unlock()
	for _, userID := range userIDs {
		if userID != "" {
			h.userIDs[userID] = struct{}{}
		}
	}
}

func roomEvent(roomID string) *apiv1.RoomTimelineRoomEvent {
	return &apiv1.RoomTimelineRoomEvent{RoomId: roomID}
}

func assetURLView(assetURL core.StableAssetURL) *apiv1.MessageAssetUrl {
	return &apiv1.MessageAssetUrl{
		Url:       assetURL.URL,
		ExpiresAt: timestamppb.New(assetURL.ExpiresAt),
	}
}
