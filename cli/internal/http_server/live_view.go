package http_server

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (s *clientLiveSession) clientLiveEventForEnvelope(ctx context.Context, event core.EventEnvelope) (*corev1.LiveEvent, error) {
	if event == nil {
		return nil, errors.New("nil event envelope")
	}
	if live := event.LiveEvent(); live != nil {
		return live, nil
	}
	if evt := event.EVTEvent(); evt != nil {
		return s.clientLiveEventForEVT(ctx, event, evt)
	}
	return nil, errors.New("event envelope has no client live payload")
}

func (s *clientLiveSession) clientLiveEventForEVT(ctx context.Context, envelope core.EventEnvelope, evt *corev1.Event) (*corev1.LiveEvent, error) {
	live := &corev1.LiveEvent{
		Id:        envelope.ID(),
		CreatedAt: envelope.CreatedAt(),
		ActorId:   envelope.ActorID(),
	}

	switch e := evt.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		roomEvent, err := s.liveRoomMessagePosted(ctx, evt, e.MessagePosted)
		if err != nil {
			return nil, err
		}
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: roomEvent}
	case *corev1.Event_MessageEdited:
		roomEvent, err := s.liveRoomMessageEdited(ctx, evt, e.MessageEdited)
		if err != nil {
			return nil, err
		}
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: roomEvent}
	case *corev1.Event_MessageRetracted:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_MessageRetracted{
			MessageRetracted: proto.Clone(e.MessageRetracted).(*corev1.MessageRetractedEvent),
		})}
	case *corev1.Event_ReactionAdded:
		update, err := s.liveReactionUpdate(ctx, e.ReactionAdded.GetRoomId(), e.ReactionAdded.GetMessageEventId())
		if err != nil {
			return nil, err
		}
		live.Event = &corev1.LiveEvent_MessageReactionsUpdated{MessageReactionsUpdated: update}
	case *corev1.Event_ReactionRemoved:
		update, err := s.liveReactionUpdate(ctx, e.ReactionRemoved.GetRoomId(), e.ReactionRemoved.GetMessageEventId())
		if err != nil {
			return nil, err
		}
		live.Event = &corev1.LiveEvent_MessageReactionsUpdated{MessageReactionsUpdated: update}
	case *corev1.Event_AssetProcessingStarted:
		update, err := s.liveAttachmentUpdate(ctx, s.assetRoomID(e.AssetProcessingStarted.GetAssetId()), e.AssetProcessingStarted.GetMessageEventId())
		if err != nil {
			return nil, err
		}
		live.Event = &corev1.LiveEvent_MessageAttachmentsUpdated{MessageAttachmentsUpdated: update}
	case *corev1.Event_AssetProcessingSucceeded:
		update, err := s.liveAttachmentUpdate(ctx, s.assetRoomID(e.AssetProcessingSucceeded.GetAssetId()), e.AssetProcessingSucceeded.GetMessageEventId())
		if err != nil {
			return nil, err
		}
		live.Event = &corev1.LiveEvent_MessageAttachmentsUpdated{MessageAttachmentsUpdated: update}
	case *corev1.Event_AssetProcessingFailed:
		update, err := s.liveAttachmentUpdate(ctx, s.assetRoomID(e.AssetProcessingFailed.GetAssetId()), e.AssetProcessingFailed.GetMessageEventId())
		if err != nil {
			return nil, err
		}
		live.Event = &corev1.LiveEvent_MessageAttachmentsUpdated{MessageAttachmentsUpdated: update}
	case *corev1.Event_AssetDeleted:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_AssetDeleted{
			AssetDeleted: proto.Clone(e.AssetDeleted).(*corev1.AssetDeletedEvent),
		})}
	case *corev1.Event_RoomCreated:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_RoomCreated{RoomCreated: proto.Clone(e.RoomCreated).(*corev1.RoomCreatedEvent)})}
	case *corev1.Event_RoomUpdated:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_RoomUpdated{RoomUpdated: proto.Clone(e.RoomUpdated).(*corev1.RoomUpdatedEvent)})}
	case *corev1.Event_RoomDeleted:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_RoomDeleted{RoomDeleted: proto.Clone(e.RoomDeleted).(*corev1.RoomDeletedEvent)})}
	case *corev1.Event_RoomArchived:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_RoomArchived{RoomArchived: proto.Clone(e.RoomArchived).(*corev1.RoomArchivedEvent)})}
	case *corev1.Event_RoomUnarchived:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_RoomUnarchived{RoomUnarchived: proto.Clone(e.RoomUnarchived).(*corev1.RoomUnarchivedEvent)})}
	case *corev1.Event_UserJoinedRoom:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_UserJoinedRoom{UserJoinedRoom: proto.Clone(e.UserJoinedRoom).(*corev1.UserJoinedRoomEvent)})}
	case *corev1.Event_UserLeftRoom:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_UserLeftRoom{UserLeftRoom: proto.Clone(e.UserLeftRoom).(*corev1.UserLeftRoomEvent)})}
	case *corev1.Event_RoomMemberBanned:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_RoomMemberBanned{RoomMemberBanned: proto.Clone(e.RoomMemberBanned).(*corev1.RoomMemberBannedEvent)})}
	case *corev1.Event_RoomMemberUnbanned:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_RoomMemberUnbanned{RoomMemberUnbanned: proto.Clone(e.RoomMemberUnbanned).(*corev1.RoomMemberUnbannedEvent)})}
	case *corev1.Event_ServerMemberDeleted:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_ServerMemberDeleted{ServerMemberDeleted: proto.Clone(e.ServerMemberDeleted).(*corev1.ServerMemberDeletedEvent)})}
	case *corev1.Event_ThreadCreated:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_ThreadCreated{ThreadCreated: proto.Clone(e.ThreadCreated).(*corev1.ThreadCreatedEvent)})}
	case *corev1.Event_VoiceCallStarted:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_CallStarted{CallStarted: proto.Clone(e.VoiceCallStarted).(*corev1.CallStartedEvent)})}
	case *corev1.Event_VoiceCallParticipantJoined:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_CallParticipantJoined{CallParticipantJoined: proto.Clone(e.VoiceCallParticipantJoined).(*corev1.CallParticipantJoinedEvent)})}
	case *corev1.Event_VoiceCallParticipantLeft:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_CallParticipantLeft{CallParticipantLeft: proto.Clone(e.VoiceCallParticipantLeft).(*corev1.CallParticipantLeftEvent)})}
	case *corev1.Event_VoiceCallEnded:
		live.Event = &corev1.LiveEvent_RoomEvent{RoomEvent: s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_CallEnded{CallEnded: proto.Clone(e.VoiceCallEnded).(*corev1.CallEndedEvent)})}
	default:
		return nil, errors.New("unsupported EVT payload for client live")
	}
	return live, nil
}

func (s *clientLiveSession) liveRoomMessagePosted(ctx context.Context, evt *corev1.Event, posted *corev1.MessagePostedEvent) (*corev1.LiveRoomEvent, error) {
	view, err := s.liveMessagePostedView(ctx, evt.GetId(), posted)
	if err != nil {
		return nil, err
	}
	return s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_MessagePosted{MessagePosted: view}), nil
}

func (s *clientLiveSession) liveRoomMessageEdited(ctx context.Context, evt *corev1.Event, edited *corev1.MessageEditedEvent) (*corev1.LiveRoomEvent, error) {
	body, err := s.server.core.GetFullMessageBodyByEventID(ctx, edited.GetEventId())
	if err != nil {
		return nil, err
	}
	view := &corev1.LiveMessageEditedEvent{
		RoomId:         edited.GetRoomId(),
		MessageEventId: edited.GetEventId(),
	}
	if body != nil {
		view.Body = stringPtr(body.Body)
		view.Attachments = s.liveAttachments(body.Attachments)
		view.LinkPreview = s.liveLinkPreview(body.LinkPreview)
		if body.UpdatedAt != nil {
			view.UpdatedAt = stringPtr(body.UpdatedAt.UTC().Format(timeFormatRFC3339Milli))
		}
	}
	return s.liveRoomEvent(ctx, evt, &corev1.LiveRoomEvent_MessageEdited{MessageEdited: view}), nil
}

func (s *clientLiveSession) liveRoomEvent(ctx context.Context, evt *corev1.Event, payload any) *corev1.LiveRoomEvent {
	roomEvent := &corev1.LiveRoomEvent{
		Id:        evt.GetId(),
		CreatedAt: evt.GetCreatedAt(),
		ActorId:   evt.GetActorId(),
		Actor:     s.liveUserView(ctx, evt.GetActorId()),
		RoomId:    s.liveRoomEventRoomID(payload),
	}
	switch p := payload.(type) {
	case *corev1.LiveRoomEvent_MessagePosted:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_MessageEdited:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_MessageRetracted:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_RoomCreated:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_RoomUpdated:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_RoomDeleted:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_RoomArchived:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_RoomUnarchived:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_UserJoinedRoom:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_UserLeftRoom:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_RoomMemberBanned:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_RoomMemberUnbanned:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_ServerMemberDeleted:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_ThreadCreated:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_CallStarted:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_CallParticipantJoined:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_CallParticipantLeft:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_CallEnded:
		roomEvent.Event = p
	case *corev1.LiveRoomEvent_AssetDeleted:
		roomEvent.Event = p
	}
	return roomEvent
}

func (s *clientLiveSession) liveRoomEventRoomID(payload any) string {
	switch p := payload.(type) {
	case *corev1.LiveRoomEvent_MessagePosted:
		return p.MessagePosted.GetRoomId()
	case *corev1.LiveRoomEvent_MessageEdited:
		return p.MessageEdited.GetRoomId()
	case *corev1.LiveRoomEvent_MessageRetracted:
		return p.MessageRetracted.GetRoomId()
	case *corev1.LiveRoomEvent_RoomCreated:
		return p.RoomCreated.GetRoomId()
	case *corev1.LiveRoomEvent_RoomUpdated:
		return p.RoomUpdated.GetRoomId()
	case *corev1.LiveRoomEvent_RoomDeleted:
		return p.RoomDeleted.GetRoomId()
	case *corev1.LiveRoomEvent_RoomArchived:
		return p.RoomArchived.GetRoomId()
	case *corev1.LiveRoomEvent_RoomUnarchived:
		return p.RoomUnarchived.GetRoomId()
	case *corev1.LiveRoomEvent_UserJoinedRoom:
		return p.UserJoinedRoom.GetRoomId()
	case *corev1.LiveRoomEvent_UserLeftRoom:
		return p.UserLeftRoom.GetRoomId()
	case *corev1.LiveRoomEvent_RoomMemberBanned:
		return p.RoomMemberBanned.GetRoomId()
	case *corev1.LiveRoomEvent_RoomMemberUnbanned:
		return p.RoomMemberUnbanned.GetRoomId()
	case *corev1.LiveRoomEvent_ThreadCreated:
		return p.ThreadCreated.GetRoomId()
	case *corev1.LiveRoomEvent_CallStarted:
		return p.CallStarted.GetRoomId()
	case *corev1.LiveRoomEvent_CallParticipantJoined:
		return p.CallParticipantJoined.GetRoomId()
	case *corev1.LiveRoomEvent_CallParticipantLeft:
		return p.CallParticipantLeft.GetRoomId()
	case *corev1.LiveRoomEvent_CallEnded:
		return p.CallEnded.GetRoomId()
	case *corev1.LiveRoomEvent_AssetDeleted:
		return s.assetRoomID(p.AssetDeleted.GetAssetId())
	default:
		return ""
	}
}

func (s *clientLiveSession) liveMessagePostedView(ctx context.Context, eventID string, posted *corev1.MessagePostedEvent) (*corev1.LiveMessagePostedEvent, error) {
	view := &corev1.LiveMessagePostedEvent{
		RoomId:                    posted.GetRoomId(),
		InReplyTo:                 stringPtrIfNotEmpty(posted.GetInReplyTo()),
		ThreadRootEventId:         stringPtrIfNotEmpty(posted.GetInThread()),
		EchoOfEventId:             stringPtrIfNotEmpty(posted.GetEchoOfEventId()),
		EchoFromThreadRootEventId: stringPtrIfNotEmpty(posted.GetEchoFromThreadRootEventId()),
	}
	if posted.GetInThread() != "" && posted.GetEchoOfEventId() == "" {
		if echoID, ok := s.server.core.RoomTimeline.ChannelEchoEventID(eventID); ok {
			view.ChannelEchoEventId = stringPtr(echoID)
		}
	}
	body, err := s.server.core.GetFullMessageBodyByEventID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if body != nil {
		view.Body = stringPtr(body.Body)
		view.Attachments = s.liveAttachments(body.Attachments)
		view.LinkPreview = s.liveLinkPreview(body.LinkPreview)
		if body.UpdatedAt != nil {
			view.UpdatedAt = stringPtr(body.UpdatedAt.UTC().Format(timeFormatRFC3339Milli))
		}
	}
	reactions, err := s.liveReactions(ctx, eventID)
	if err != nil {
		return nil, err
	}
	view.Reactions = reactions

	if posted.GetInThread() == "" {
		kind, err := s.server.core.FindRoomKind(ctx, posted.GetRoomId())
		if err == nil {
			if metadata, err := s.server.core.GetThreadMetadata(ctx, kind, posted.GetRoomId(), eventID); err == nil && metadata != nil {
				view.ReplyCount = int32(metadata.ReplyCount)
				if metadata.LastReplyAt != nil {
					view.LastReplyAt = timestamppb.New(*metadata.LastReplyAt)
				}
				for _, participantID := range metadata.ParticipantIDs {
					if len(view.ThreadParticipants) >= 5 {
						break
					}
					if participant := s.liveUserView(ctx, participantID); participant != nil {
						view.ThreadParticipants = append(view.ThreadParticipants, participant)
					}
				}
			}
			if following, err := s.server.core.IsFollowingThread(ctx, kind, s.userID, posted.GetRoomId(), eventID); err == nil {
				view.ViewerIsFollowingThread = &following
			}
		}
	}

	return view, nil
}

func (s *clientLiveSession) liveReactionUpdate(ctx context.Context, roomID, messageEventID string) (*corev1.LiveMessageReactionsUpdatedEvent, error) {
	reactions, err := s.liveReactions(ctx, messageEventID)
	if err != nil {
		return nil, err
	}
	return &corev1.LiveMessageReactionsUpdatedEvent{
		RoomId:         roomID,
		MessageEventId: messageEventID,
		Reactions:      reactions,
	}, nil
}

func (s *clientLiveSession) liveAttachmentUpdate(ctx context.Context, roomID, messageEventID string) (*corev1.LiveMessageAttachmentsUpdatedEvent, error) {
	body, err := s.server.core.GetFullMessageBodyByEventID(ctx, messageEventID)
	if err != nil {
		return nil, err
	}
	update := &corev1.LiveMessageAttachmentsUpdatedEvent{RoomId: roomID, MessageEventId: messageEventID}
	if body != nil {
		update.Attachments = s.liveAttachments(body.Attachments)
		update.LinkPreview = s.liveLinkPreview(body.LinkPreview)
		if body.UpdatedAt != nil {
			update.UpdatedAt = stringPtr(body.UpdatedAt.UTC().Format(timeFormatRFC3339Milli))
		}
	}
	return update, nil
}

func (s *clientLiveSession) assetRoomID(assetID string) string {
	roomID, _ := s.server.core.Assets.AssetRoomID(assetID)
	return roomID
}

func (s *clientLiveSession) liveReactions(ctx context.Context, messageEventID string) ([]*corev1.LiveReactionSummaryView, error) {
	summaries, err := s.server.core.GetReactions(ctx, messageEventID)
	if err != nil {
		return nil, err
	}
	out := make([]*corev1.LiveReactionSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		view := &corev1.LiveReactionSummaryView{
			Emoji:      summary.Emoji,
			Count:      int32(len(summary.UserIDs)),
			HasReacted: containsString(summary.UserIDs, s.userID),
		}
		for _, userID := range summary.UserIDs {
			if len(view.Users) >= 5 {
				break
			}
			user, err := s.server.core.GetUser(ctx, userID)
			if err != nil || user == nil {
				continue
			}
			view.Users = append(view.Users, &corev1.LiveReactionUserView{Id: user.Id, DisplayName: user.DisplayName})
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *clientLiveSession) liveUserView(ctx context.Context, userID string) *corev1.LiveUserView {
	if userID == "" {
		return nil
	}
	user, err := s.server.core.GetUser(ctx, userID)
	if err != nil || user == nil {
		return nil
	}
	width, height := 96, 96
	avatarURL, _ := s.server.core.GetUserAvatarURL(ctx, userID, &width, &height, "")
	presenceStatus, err := s.server.core.GetUserPresence(ctx, userID)
	if err != nil {
		presenceStatus = core.PresenceStatusOffline
	}
	return &corev1.LiveUserView{
		Id:             user.Id,
		Login:          user.Login,
		DisplayName:    user.DisplayName,
		Deleted:        user.Deleted,
		AvatarUrl:      avatarURL,
		PresenceStatus: presenceStatus,
	}
}

func (s *clientLiveSession) liveAttachments(attachments []*corev1.Attachment) []*corev1.LiveAttachmentView {
	out := make([]*corev1.LiveAttachmentView, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		view := &corev1.LiveAttachmentView{
			Id:                attachment.GetId(),
			Filename:          attachment.GetFilename(),
			ContentType:       attachment.GetContentType(),
			Width:             attachment.GetWidth(),
			Height:            attachment.GetHeight(),
			AssetUrl:          s.liveAssetURL(s.server.core.GetStableAttachmentAssetURL(attachment.GetId(), s.userID)),
			ThumbnailAssetUrl: s.liveAssetURL(s.server.core.GetStableTransformedAttachmentAssetURL(attachment.GetId(), s.userID, 960, 800, "contain")),
			VideoProcessing:   s.liveVideoProcessing(attachment),
		}
		out = append(out, view)
	}
	return out
}

func (s *clientLiveSession) liveVideoProcessing(attachment *corev1.Attachment) *corev1.LiveVideoProcessingView {
	if attachment == nil || (!strings.HasPrefix(attachment.GetContentType(), "video/") && attachment.GetContentType() != "image/gif") {
		return nil
	}
	manifest, ok := s.server.core.Assets.VideoAttachmentManifest(attachment.GetId())
	if !ok || manifest == nil {
		return nil
	}
	result := &corev1.LiveVideoProcessingView{SourceAvailable: true}
	switch {
	case manifest.Succeeded != nil:
		video := manifest.Succeeded.GetVideo()
		if video == nil {
			return nil
		}
		result.Status = "COMPLETED"
		if video.DurationMs > 0 {
			result.DurationMs = &video.DurationMs
		}
		if video.Width > 0 {
			result.Width = &video.Width
		}
		if video.Height > 0 {
			result.Height = &video.Height
		}
		if thumbnailID := video.GetThumbnailAssetId(); thumbnailID != "" {
			result.ThumbnailAssetUrl = s.liveAssetURL(s.server.core.GetStableAttachmentAssetURL(thumbnailID, s.userID))
		}
		for _, variant := range video.Variants {
			if variant == nil {
				continue
			}
			v := &corev1.LiveVideoVariantView{
				Quality:  variant.GetQuality(),
				AssetUrl: s.liveAssetURL(s.server.core.GetStableAttachmentAssetURL(variant.GetAssetId(), s.userID)),
			}
			if created, ok := s.server.core.Assets.AssetCreation(variant.GetAssetId()); ok && created.GetAsset() != nil {
				v.Width = created.GetAsset().GetWidth()
				v.Height = created.GetAsset().GetHeight()
				v.Size = created.GetAsset().GetSize()
			}
			result.Variants = append(result.Variants, v)
		}
	case manifest.Failed != nil:
		result.Status = "FAILED"
		reason := "processing_failed"
		if manifest.Failed.GetFailureCode() == corev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING {
			reason = "original_missing"
			result.SourceAvailable = false
		}
		result.ReasonCode = &reason
	case manifest.Started != nil:
		result.Status = "PROCESSING"
	default:
		return nil
	}
	return result
}

func (s *clientLiveSession) liveLinkPreview(preview *corev1.LinkPreview) *corev1.LiveLinkPreviewView {
	if preview == nil {
		return nil
	}
	view := &corev1.LiveLinkPreviewView{
		Url:         preview.GetUrl(),
		Title:       preview.GetTitle(),
		Description: preview.GetDescription(),
		SiteName:    preview.GetSiteName(),
		EmbedType:   preview.GetEmbedType(),
		EmbedId:     stringPtrIfNotEmpty(preview.GetEmbedId()),
	}
	if imageID := preview.GetImageAssetId(); imageID != "" {
		view.ImageUrl = s.server.core.GetTransformedServerAssetURL(imageID, 600, 314, "contain")
	}
	return view
}

func (s *clientLiveSession) liveAssetURL(assetURL core.StableAssetURL) *corev1.LiveAssetURL {
	if assetURL.URL == "" {
		return nil
	}
	return &corev1.LiveAssetURL{Url: assetURL.URL, ExpiresAt: timestamppb.New(assetURL.ExpiresAt)}
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

const timeFormatRFC3339Milli = "2006-01-02T15:04:05.000Z07:00"
