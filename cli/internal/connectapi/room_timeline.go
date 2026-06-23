package connectapi

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const roomTimelineCursorSeqPrefix = "seq:"

type roomTimelineService struct {
	api *API
}

func (s *roomTimelineService) GetRoomEvents(ctx context.Context, req *connect.Request[apiv1.GetRoomEventsRequest]) (*connect.Response[apiv1.GetRoomEventsResponse], error) {
	user, room, err := s.requireRoomMember(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, err
	}
	kind := core.KindOfRoom(room)

	var page *core.RoomEventsResult
	switch cursor := req.Msg.Cursor.(type) {
	case *apiv1.GetRoomEventsRequest_After:
		seq, err := parseRoomTimelineCursor(cursor.After)
		if err != nil {
			return nil, err
		}
		page, err = s.api.core.GetRoomEventsAfter(ctx, kind, room.Id, seq, int(req.Msg.Limit))
	case *apiv1.GetRoomEventsRequest_Before:
		seq, err := parseRoomTimelineCursor(cursor.Before)
		if err != nil {
			return nil, err
		}
		page, err = s.api.core.GetRoomEvents(ctx, kind, room.Id, int(req.Msg.Limit), &seq)
	default:
		page, err = s.api.core.GetRoomEvents(ctx, kind, room.Id, int(req.Msg.Limit), nil)
	}
	if err != nil {
		return nil, connectError(err)
	}

	resp, err := s.buildPage(ctx, user.Id, kind, page.Events, page.HasOlder, page.HasNewer)
	if err != nil {
		return nil, connectError(err)
	}
	resp.StartCursor = formatRoomTimelineCursor(page.StartCursorSeq)
	resp.EndCursor = formatRoomTimelineCursor(page.EndCursorSeq)
	return connect.NewResponse(&apiv1.GetRoomEventsResponse{Page: resp}), nil
}

func (s *roomTimelineService) GetRoomEventsAround(ctx context.Context, req *connect.Request[apiv1.GetRoomEventsAroundRequest]) (*connect.Response[apiv1.GetRoomEventsAroundResponse], error) {
	user, room, err := s.requireRoomMember(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Msg.EventId) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("event_id is required"))
	}
	kind := core.KindOfRoom(room)

	result, err := s.api.core.GetRoomEventsAround(ctx, kind, room.Id, req.Msg.EventId, int(req.Msg.Limit))
	if err != nil {
		return nil, connectError(err)
	}
	page, err := s.buildPage(ctx, user.Id, kind, result.Events, result.HasOlder, result.HasNewer)
	if err != nil {
		return nil, connectError(err)
	}
	if len(result.Events) > 0 {
		page.StartCursor = formatRoomTimelineCursor(result.Events[0].Sequence)
		page.EndCursor = formatRoomTimelineCursor(result.Events[len(result.Events)-1].Sequence)
	}

	return connect.NewResponse(&apiv1.GetRoomEventsAroundResponse{
		Page:        page,
		TargetIndex: int32(result.TargetIndex),
	}), nil
}

func (s *roomTimelineService) requireRoomMember(ctx context.Context, roomID string) (*corev1.User, *corev1.Room, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, errors.New("room_id is required"))
	}

	room, err := s.api.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, nil, connectError(err)
	}
	kind := core.KindOfRoom(room)
	ok, err := s.api.core.RoomMembershipExists(ctx, kind, user.Id, room.Id)
	if err != nil {
		return nil, nil, connectError(err)
	}
	if !ok {
		return nil, nil, connectError(core.ErrNotRoomMember)
	}
	return user, room, nil
}

// buildPage turns projected room timeline entries into the public Connect view.
// The projected event log intentionally stores facts, not UI rows: message
// bodies, reactions, thread metadata, and users live in sibling projections.
// Hydrating them here keeps the public API free of GraphQL-style resolver N+1s
// and gives future clients one renderable page per request.
func (s *roomTimelineService) buildPage(ctx context.Context, viewerID string, kind core.RoomKind, events []*core.RoomEvent, hasOlder, hasNewer bool) (*apiv1.RoomTimelinePage, error) {
	messageIDs := make([]string, 0, len(events))
	for _, event := range events {
		if event.GetMessagePosted() != nil {
			messageIDs = append(messageIDs, event.Id)
		}
	}

	reactionsByMessageID, err := s.api.core.GetReactionsBatch(ctx, messageIDs)
	if err != nil {
		return nil, err
	}

	h := &timelineHydrator{
		api:                  s.api,
		ctx:                  ctx,
		viewerID:             viewerID,
		kind:                 kind,
		reactionsByMessageID: reactionsByMessageID,
		userIDs:              make(map[string]struct{}),
	}

	apiEvents := make([]*apiv1.RoomTimelineEvent, 0, len(events))
	for _, event := range events {
		apiEvent, err := h.event(event)
		if err != nil {
			return nil, err
		}
		if apiEvent != nil {
			apiEvents = append(apiEvents, apiEvent)
		}
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

type timelineHydrator struct {
	api                  *API
	ctx                  context.Context
	viewerID             string
	kind                 core.RoomKind
	reactionsByMessageID map[string][]core.ReactionSummary
	userIDs              map[string]struct{}
}

func (h *timelineHydrator) event(event *core.RoomEvent) (*apiv1.RoomTimelineEvent, error) {
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
		message, err := h.messagePosted(event, payload.MessagePosted)
		if err != nil {
			return nil, err
		}
		apiEvent.Event = &apiv1.RoomTimelineEvent_MessagePosted{MessagePosted: message}
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
		return nil, nil
	}

	return apiEvent, nil
}

func (h *timelineHydrator) messagePosted(event *core.RoomEvent, payload *corev1.MessagePostedEvent) (*apiv1.RoomTimelineMessagePosted, error) {
	message := &apiv1.RoomTimelineMessagePosted{
		RoomId:                         payload.GetRoomId(),
		InReplyTo:                      payload.GetInReplyTo(),
		ThreadRootEventId:              payload.GetInThread(),
		EchoOfEventId:                  payload.GetEchoOfEventId(),
		EchoFromThreadRootEventId:      payload.GetEchoFromThreadRootEventId(),
		ViewerIsFollowingThread:        false,
		ViewerIsFollowingThreadPresent: false,
		Reactions:                      h.reactions(event.Id),
	}

	if echoID, ok := h.api.core.RoomTimeline.ChannelEchoEventID(event.Id); ok {
		message.ChannelEchoEventId = echoID
	}

	body, err := h.api.core.GetFullMessageBodyByEventID(h.ctx, event.Id)
	if err != nil {
		return nil, err
	}
	if body != nil {
		message.Body = body.Body
		message.BodyPresent = true
		message.Attachments = h.attachments(payload.GetRoomId(), event.Id, body.Attachments)
		message.LinkPreview = h.linkPreview(body.LinkPreview)
		if body.UpdatedAt != nil {
			message.UpdatedAt = timestamppb.New(*body.UpdatedAt)
		}
	}

	if payload.GetInThread() == "" {
		metadata, err := h.api.core.GetThreadMetadata(h.ctx, h.kind, payload.GetRoomId(), event.Id)
		if err != nil && !errors.Is(err, core.ErrNotFound) {
			return nil, err
		}
		if metadata != nil {
			message.ReplyCount = int32(metadata.ReplyCount)
			if metadata.LastReplyAt != nil {
				message.LastReplyAt = timestamppb.New(*metadata.LastReplyAt)
			}
			message.ThreadParticipantUserIds = firstN(metadata.ParticipantIDs, 5)
			h.addUserIDs(message.ThreadParticipantUserIds)
		}
		following, err := h.api.core.IsFollowingThread(h.ctx, h.kind, h.viewerID, payload.GetRoomId(), event.Id)
		if err != nil {
			return nil, err
		}
		message.ViewerIsFollowingThread = following
		message.ViewerIsFollowingThreadPresent = true
	}

	return message, nil
}

func (h *timelineHydrator) attachments(roomID, messageEventID string, attachments []*corev1.Attachment) []*apiv1.RoomTimelineAttachment {
	result := make([]*apiv1.RoomTimelineAttachment, 0, len(attachments))
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
		thumbnailURL := h.api.core.GetStableTransformedAttachmentAssetURL(attachment.Id, h.viewerID, 960, 800, "contain")
		result = append(result, &apiv1.RoomTimelineAttachment{
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

func (h *timelineHydrator) videoProcessing(attachment *corev1.Attachment) *apiv1.RoomTimelineVideoProcessing {
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
		result := &apiv1.RoomTimelineVideoProcessing{
			Status:          apiv1.RoomTimelineVideoProcessingStatus_ROOM_TIMELINE_VIDEO_PROCESSING_STATUS_COMPLETED,
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
			result.Variants = append(result.Variants, &apiv1.RoomTimelineVideoVariant{
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
		return &apiv1.RoomTimelineVideoProcessing{
			Status:          apiv1.RoomTimelineVideoProcessingStatus_ROOM_TIMELINE_VIDEO_PROCESSING_STATUS_FAILED,
			SourceAvailable: reasonCode != "original_missing" && h.assetSourceAvailable(attachment.GetId(), true),
			ReasonCode:      reasonCode,
		}
	}

	if manifest.Started != nil {
		return &apiv1.RoomTimelineVideoProcessing{
			Status:          apiv1.RoomTimelineVideoProcessingStatus_ROOM_TIMELINE_VIDEO_PROCESSING_STATUS_PROCESSING,
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

func (h *timelineHydrator) linkPreview(preview *corev1.LinkPreview) *apiv1.RoomTimelineLinkPreview {
	if preview == nil {
		return nil
	}
	imageAssetID := preview.GetImageAssetId()
	if image := preview.GetImageAsset(); image != nil && image.GetId() != "" {
		imageAssetID = image.GetId()
	}
	imageURL := ""
	if imageAssetID != "" {
		imageURL = h.api.core.GetTransformedServerAssetURL(imageAssetID, 600, 314, "contain")
	}
	return &apiv1.RoomTimelineLinkPreview{
		Url:         preview.GetUrl(),
		Title:       preview.GetTitle(),
		Description: preview.GetDescription(),
		SiteName:    preview.GetSiteName(),
		ImageUrl:    imageURL,
		EmbedType:   preview.GetEmbedType(),
		EmbedId:     preview.GetEmbedId(),
	}
}

func (h *timelineHydrator) reactions(messageEventID string) []*apiv1.RoomTimelineReactionSummary {
	summaries := h.reactionsByMessageID[messageEventID]
	result := make([]*apiv1.RoomTimelineReactionSummary, 0, len(summaries))
	for _, summary := range summaries {
		previewUserIDs := firstN(summary.UserIDs, 5)
		h.addUserIDs(previewUserIDs)
		result = append(result, &apiv1.RoomTimelineReactionSummary{
			Emoji:      summary.Emoji,
			Count:      int32(len(summary.UserIDs)),
			HasReacted: containsString(summary.UserIDs, h.viewerID),
			UserIds:    previewUserIDs,
		})
	}
	return result
}

func (h *timelineHydrator) users() (map[string]*apiv1.RoomTimelineUser, error) {
	ids := make([]string, 0, len(h.userIDs))
	for id := range h.userIDs {
		ids = append(ids, id)
	}
	coreUsers, err := h.api.core.GetUsers(h.ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*apiv1.RoomTimelineUser, len(ids))
	avatarWidth, avatarHeight := 96, 96
	for i, id := range ids {
		user := coreUsers[i]
		if user == nil {
			user = core.DeletedUserReference(id)
		}
		avatarURL, _ := h.api.core.GetUserAvatarURL(h.ctx, user.Id, &avatarWidth, &avatarHeight, "cover")
		result[id] = &apiv1.RoomTimelineUser{
			Id:          user.Id,
			Login:       user.Login,
			DisplayName: user.DisplayName,
			Deleted:     user.Deleted,
			AvatarUrl:   avatarURL,
		}
	}
	return result, nil
}

func (h *timelineHydrator) addUserID(userID string) {
	if userID != "" {
		h.userIDs[userID] = struct{}{}
	}
}

func (h *timelineHydrator) addUserIDs(userIDs []string) {
	for _, userID := range userIDs {
		h.addUserID(userID)
	}
}

func roomEvent(roomID string) *apiv1.RoomTimelineRoomEvent {
	return &apiv1.RoomTimelineRoomEvent{RoomId: roomID}
}

func assetURLView(assetURL core.StableAssetURL) *apiv1.RoomTimelineAssetUrl {
	return &apiv1.RoomTimelineAssetUrl{
		Url:       assetURL.URL,
		ExpiresAt: timestamppb.New(assetURL.ExpiresAt),
	}
}

func formatRoomTimelineCursor(seq uint64) string {
	if seq == 0 {
		return ""
	}
	return roomTimelineCursorSeqPrefix + strconv.FormatUint(seq, 10)
}

func parseRoomTimelineCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	rest, ok := strings.CutPrefix(cursor, roomTimelineCursorSeqPrefix)
	if !ok {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid cursor format"))
	}
	seq, err := strconv.ParseUint(rest, 10, 64)
	if err != nil {
		return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor sequence: %w", err))
	}
	return seq, nil
}

func firstN(values []string, n int) []string {
	if len(values) <= n {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[:n]...)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
