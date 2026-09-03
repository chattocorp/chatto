// SPDX-FileCopyrightText: 2026-present Chatto contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package http_server

import (
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// projectRealtimeEvent explicitly maps one durable EVT event to the independent
// public catalogue. EVT field numbers and public field numbers need not match.
func projectRealtimeEvent(source *evtv1.Event) *realtimev1.RealtimeEvent {
	if source == nil || source.GetEvent() == nil {
		return nil
	}
	target := &realtimev1.RealtimeEvent{Id: source.GetId(), CreatedAt: source.GetCreatedAt()}
	if source.GetActorId() != "" {
		target.ActorId = proto.String(source.GetActorId())
	}
	switch e := source.GetEvent().(type) {
	case *evtv1.Event_RoomCreated:
		v := e.RoomCreated
		target.Event = &realtimev1.RealtimeEvent_RoomCreated{RoomCreated: &realtimev1.RoomCreatedEvent{RoomId: v.GetRoomId(), Name: v.GetName(), Description: v.GetDescription(), Kind: realtimeRoomKind(v.GetKind()), Universal: v.GetUniversal(), ThreadingMode: realtimeRoomThreadingMode(v.GetThreadingMode())}}
	case *evtv1.Event_RoomUpdated:
		v := e.RoomUpdated
		target.Event = &realtimev1.RealtimeEvent_RoomUpdated{RoomUpdated: &realtimev1.RoomUpdatedEvent{RoomId: v.GetRoomId(), Name: v.GetName(), Description: v.GetDescription()}}
	case *evtv1.Event_RoomDeleted:
		target.Event = &realtimev1.RealtimeEvent_RoomDeleted{RoomDeleted: &realtimev1.RoomDeletedEvent{RoomId: e.RoomDeleted.GetRoomId()}}
	case *evtv1.Event_RoomArchived:
		target.Event = &realtimev1.RealtimeEvent_RoomArchived{RoomArchived: &realtimev1.RoomArchivedEvent{RoomId: e.RoomArchived.GetRoomId()}}
	case *evtv1.Event_RoomUnarchived:
		target.Event = &realtimev1.RealtimeEvent_RoomUnarchived{RoomUnarchived: &realtimev1.RoomUnarchivedEvent{RoomId: e.RoomUnarchived.GetRoomId()}}
	case *evtv1.Event_RoomUniversalChanged:
		v := e.RoomUniversalChanged
		target.Event = &realtimev1.RealtimeEvent_RoomUniversalChanged{RoomUniversalChanged: &realtimev1.RoomUniversalChangedEvent{RoomId: v.GetRoomId(), Universal: v.GetUniversal()}}
	case *evtv1.Event_RoomSlowModeChanged:
		v := e.RoomSlowModeChanged
		target.Event = &realtimev1.RealtimeEvent_RoomSlowModeChanged{RoomSlowModeChanged: &realtimev1.RoomSlowModeChangedEvent{RoomId: v.GetRoomId(), SlowModeSeconds: v.GetSlowModeSeconds()}}
	case *evtv1.Event_RoomThreadingModeChanged:
		v := e.RoomThreadingModeChanged
		target.Event = &realtimev1.RealtimeEvent_RoomThreadingModeChanged{RoomThreadingModeChanged: &realtimev1.RoomThreadingModeChangedEvent{RoomId: v.GetRoomId(), ThreadingMode: realtimeRoomThreadingMode(v.GetThreadingMode())}}
	case *evtv1.Event_UserJoinedRoom:
		target.Event = &realtimev1.RealtimeEvent_UserJoinedRoom{UserJoinedRoom: &realtimev1.UserJoinedRoomEvent{RoomId: e.UserJoinedRoom.GetRoomId()}}
	case *evtv1.Event_UserLeftRoom:
		target.Event = &realtimev1.RealtimeEvent_UserLeftRoom{UserLeftRoom: &realtimev1.UserLeftRoomEvent{RoomId: e.UserLeftRoom.GetRoomId()}}
	case *evtv1.Event_RoomGroupCreated:
		v := e.RoomGroupCreated
		target.Event = &realtimev1.RealtimeEvent_RoomGroupCreated{RoomGroupCreated: &realtimev1.RoomGroupCreatedEvent{GroupId: v.GetGroupId(), Name: v.GetName(), Description: v.GetDescription()}}
	case *evtv1.Event_RoomGroupUpdated:
		v := e.RoomGroupUpdated
		target.Event = &realtimev1.RealtimeEvent_RoomGroupUpdated{RoomGroupUpdated: &realtimev1.RoomGroupUpdatedEvent{GroupId: v.GetGroupId(), Name: v.GetName(), Description: v.GetDescription()}}
	case *evtv1.Event_RoomGroupDeleted:
		target.Event = &realtimev1.RealtimeEvent_RoomGroupDeleted{RoomGroupDeleted: &realtimev1.RoomGroupDeletedEvent{GroupId: e.RoomGroupDeleted.GetGroupId()}}
	case *evtv1.Event_RoomAddedToGroup:
		v := e.RoomAddedToGroup
		target.Event = &realtimev1.RealtimeEvent_RoomAddedToGroup{RoomAddedToGroup: &realtimev1.RoomAddedToGroupEvent{GroupId: v.GetGroupId(), RoomId: v.GetRoomId()}}
	case *evtv1.Event_RoomRemovedFromGroup:
		v := e.RoomRemovedFromGroup
		target.Event = &realtimev1.RealtimeEvent_RoomRemovedFromGroup{RoomRemovedFromGroup: &realtimev1.RoomRemovedFromGroupEvent{GroupId: v.GetGroupId(), RoomId: v.GetRoomId()}}
	case *evtv1.Event_RoomsInGroupReordered:
		v := e.RoomsInGroupReordered
		target.Event = &realtimev1.RealtimeEvent_RoomsInGroupReordered{RoomsInGroupReordered: &realtimev1.RoomsInGroupReorderedEvent{GroupId: v.GetGroupId(), RoomIds: append([]string(nil), v.GetRoomIds()...)}}
	case *evtv1.Event_SidebarLinkAddedToGroup:
		v := e.SidebarLinkAddedToGroup
		target.Event = &realtimev1.RealtimeEvent_SidebarLinkAddedToGroup{SidebarLinkAddedToGroup: &realtimev1.SidebarLinkAddedToGroupEvent{GroupId: v.GetGroupId(), LinkId: v.GetLinkId(), Label: v.GetLabel(), Url: v.GetUrl()}}
	case *evtv1.Event_SidebarLinkUpdated:
		v := e.SidebarLinkUpdated
		target.Event = &realtimev1.RealtimeEvent_SidebarLinkUpdated{SidebarLinkUpdated: &realtimev1.SidebarLinkUpdatedEvent{GroupId: v.GetGroupId(), LinkId: v.GetLinkId(), Label: v.GetLabel(), Url: v.GetUrl()}}
	case *evtv1.Event_SidebarLinkRemovedFromGroup:
		v := e.SidebarLinkRemovedFromGroup
		target.Event = &realtimev1.RealtimeEvent_SidebarLinkRemovedFromGroup{SidebarLinkRemovedFromGroup: &realtimev1.SidebarLinkRemovedFromGroupEvent{GroupId: v.GetGroupId(), LinkId: v.GetLinkId()}}
	case *evtv1.Event_SidebarGroupEntriesReordered:
		v := e.SidebarGroupEntriesReordered
		entries := make([]*realtimev1.SidebarGroupEntryReference, 0, len(v.GetEntries()))
		for _, entry := range v.GetEntries() {
			if entry.GetId() != "" {
				entries = append(entries, &realtimev1.SidebarGroupEntryReference{
					Kind: realtimeSidebarGroupEntryKind(entry.GetKind()),
					Id:   entry.GetId(),
				})
			}
		}
		target.Event = &realtimev1.RealtimeEvent_SidebarGroupEntriesReordered{SidebarGroupEntriesReordered: &realtimev1.SidebarGroupEntriesReorderedEvent{GroupId: v.GetGroupId(), Entries: entries}}
	case *evtv1.Event_RoomGroupsReordered:
		target.Event = &realtimev1.RealtimeEvent_RoomGroupsReordered{RoomGroupsReordered: &realtimev1.RoomGroupsReorderedEvent{GroupIds: append([]string(nil), e.RoomGroupsReordered.GetGroupIds()...)}}
	case *evtv1.Event_VoiceCallParticipantJoined:
		v := e.VoiceCallParticipantJoined
		target.Event = &realtimev1.RealtimeEvent_VoiceCallParticipantJoined{VoiceCallParticipantJoined: &realtimev1.VoiceCallParticipantJoinedEvent{RoomId: v.GetRoomId(), Source: realtimeCallParticipantSource(v.GetSource()), CallId: v.GetCallId()}}
	case *evtv1.Event_VoiceCallParticipantLeft:
		v := e.VoiceCallParticipantLeft
		target.Event = &realtimev1.RealtimeEvent_VoiceCallParticipantLeft{VoiceCallParticipantLeft: &realtimev1.VoiceCallParticipantLeftEvent{RoomId: v.GetRoomId(), Source: realtimeCallParticipantSource(v.GetSource()), CallId: v.GetCallId()}}
	case *evtv1.Event_VoiceCallStarted:
		v := e.VoiceCallStarted
		target.Event = &realtimev1.RealtimeEvent_VoiceCallStarted{VoiceCallStarted: &realtimev1.VoiceCallStartedEvent{RoomId: v.GetRoomId(), CallId: v.GetCallId(), Source: realtimeCallParticipantSource(v.GetSource())}}
	case *evtv1.Event_VoiceCallEnded:
		v := e.VoiceCallEnded
		target.Event = &realtimev1.RealtimeEvent_VoiceCallEnded{VoiceCallEnded: &realtimev1.VoiceCallEndedEvent{RoomId: v.GetRoomId(), CallId: v.GetCallId(), Source: realtimeCallParticipantSource(v.GetSource())}}
	case *evtv1.Event_MessagePosted:
		v := e.MessagePosted
		target.Event = &realtimev1.RealtimeEvent_MessagePosted{MessagePosted: &realtimev1.MessagePostedEvent{RoomId: v.GetRoomId(), InReplyTo: v.GetInReplyTo(), InThread: v.GetInThread(), MentionedUserIds: append([]string(nil), v.GetMentionedUserIds()...), EchoOfEventId: v.GetEchoOfEventId(), EchoFromThreadRootEventId: v.GetEchoFromThreadRootEventId(), Mentions: realtimeMentions(v.GetMentions())}}
	case *evtv1.Event_MessageEdited:
		v := e.MessageEdited
		target.Event = &realtimev1.RealtimeEvent_MessageEdited{MessageEdited: &realtimev1.MessageEditedEvent{RoomId: v.GetRoomId(), EventId: v.GetEventId()}}
	case *evtv1.Event_MessageRetracted:
		v := e.MessageRetracted
		target.Event = &realtimev1.RealtimeEvent_MessageRetracted{MessageRetracted: &realtimev1.MessageRetractedEvent{RoomId: v.GetRoomId(), EventId: v.GetEventId()}}
	case *evtv1.Event_MessagePinned:
		v := e.MessagePinned
		target.Event = &realtimev1.RealtimeEvent_MessagePinned{MessagePinned: &realtimev1.MessagePinnedEvent{RoomId: v.GetRoomId(), MessageEventId: v.GetMessageEventId()}}
	case *evtv1.Event_MessageUnpinned:
		v := e.MessageUnpinned
		target.Event = &realtimev1.RealtimeEvent_MessageUnpinned{MessageUnpinned: &realtimev1.MessageUnpinnedEvent{RoomId: v.GetRoomId(), MessageEventId: v.GetMessageEventId()}}
	case *evtv1.Event_ThreadCreated:
		v := e.ThreadCreated
		target.Event = &realtimev1.RealtimeEvent_ThreadCreated{ThreadCreated: &realtimev1.ThreadCreatedEvent{RoomId: v.GetRoomId(), ThreadRootEventId: v.GetThreadRootEventId()}}
	case *evtv1.Event_AssetProcessingStarted:
		v := e.AssetProcessingStarted
		target.Event = &realtimev1.RealtimeEvent_AssetProcessingStarted{AssetProcessingStarted: &realtimev1.AssetProcessingStartedEvent{AssetId: v.GetAssetId(), MessageEventId: v.GetMessageEventId()}}
	case *evtv1.Event_AssetProcessingSucceeded:
		v := e.AssetProcessingSucceeded
		target.Event = &realtimev1.RealtimeEvent_AssetProcessingSucceeded{AssetProcessingSucceeded: &realtimev1.AssetProcessingSucceededEvent{AssetId: v.GetAssetId(), Video: realtimeProcessedVideo(v.GetVideo()), MessageEventId: v.GetMessageEventId()}}
	case *evtv1.Event_AssetProcessingFailed:
		v := e.AssetProcessingFailed
		target.Event = &realtimev1.RealtimeEvent_AssetProcessingFailed{AssetProcessingFailed: &realtimev1.AssetProcessingFailedEvent{AssetId: v.GetAssetId(), FailureCode: realtimeAssetProcessingFailureCode(v.GetFailureCode()), MessageEventId: v.GetMessageEventId()}}
	case *evtv1.Event_AssetDeleted:
		target.Event = &realtimev1.RealtimeEvent_AssetDeleted{AssetDeleted: &realtimev1.AssetDeletedEvent{AssetId: e.AssetDeleted.GetAssetId()}}
	case *evtv1.Event_ServerMotdChanged:
		target.Event = &realtimev1.RealtimeEvent_ServerMotdChanged{ServerMotdChanged: &realtimev1.ServerMotdChangedEvent{Motd: e.ServerMotdChanged.GetMotd()}}
	case *evtv1.Event_UserAccountCreated:
		v := e.UserAccountCreated
		target.Event = &realtimev1.RealtimeEvent_UserAccountCreated{UserAccountCreated: &realtimev1.UserAccountCreatedEvent{UserId: v.GetUserId(), IsBot: v.GetIsBot(), BotOwnerUserId: v.GetBotOwnerUserId()}}
	case *evtv1.Event_UserLoginChanged:
		target.Event = &realtimev1.RealtimeEvent_UserLoginChanged{UserLoginChanged: &realtimev1.UserLoginChangedEvent{UserId: e.UserLoginChanged.GetUserId()}}
	case *evtv1.Event_UserDisplayNameChanged:
		target.Event = &realtimev1.RealtimeEvent_UserDisplayNameChanged{UserDisplayNameChanged: &realtimev1.UserDisplayNameChangedEvent{UserId: e.UserDisplayNameChanged.GetUserId()}}
	case *evtv1.Event_UserAvatarSet:
		target.Event = &realtimev1.RealtimeEvent_UserAvatarSet{UserAvatarSet: &realtimev1.UserAvatarSetEvent{UserId: e.UserAvatarSet.GetUserId()}}
	case *evtv1.Event_UserAvatarCleared:
		target.Event = &realtimev1.RealtimeEvent_UserAvatarCleared{UserAvatarCleared: &realtimev1.UserAvatarClearedEvent{UserId: e.UserAvatarCleared.GetUserId()}}
	case *evtv1.Event_UserAccountDeleted:
		target.Event = &realtimev1.RealtimeEvent_UserAccountDeleted{UserAccountDeleted: &realtimev1.UserAccountDeletedEvent{UserId: e.UserAccountDeleted.GetUserId()}}
	case *evtv1.Event_UserCustomStatusSet:
		v := e.UserCustomStatusSet
		target.Event = &realtimev1.RealtimeEvent_UserCustomStatusSet{UserCustomStatusSet: &realtimev1.UserCustomStatusSetEvent{UserId: v.GetUserId(), Status: realtimeCustomStatus(v.GetStatus())}}
	case *evtv1.Event_UserCustomStatusCleared:
		target.Event = &realtimev1.RealtimeEvent_UserCustomStatusCleared{UserCustomStatusCleared: &realtimev1.UserCustomStatusClearedEvent{UserId: e.UserCustomStatusCleared.GetUserId()}}
	case *evtv1.Event_UserBioChanged:
		target.Event = &realtimev1.RealtimeEvent_UserBioChanged{UserBioChanged: &realtimev1.UserBioChangedEvent{UserId: e.UserBioChanged.GetUserId()}}
	case *evtv1.Event_RoomMemberBanned:
		v := e.RoomMemberBanned
		target.Event = &realtimev1.RealtimeEvent_RoomMemberBanned{RoomMemberBanned: &realtimev1.RoomMemberBannedEvent{RoomId: v.GetRoomId(), UserId: v.GetUserId(), ExpiresAt: v.GetExpiresAt()}}
	case *evtv1.Event_RoomMemberUnbanned:
		v := e.RoomMemberUnbanned
		target.Event = &realtimev1.RealtimeEvent_RoomMemberUnbanned{RoomMemberUnbanned: &realtimev1.RoomMemberUnbannedEvent{RoomId: v.GetRoomId(), UserId: v.GetUserId()}}
	case *evtv1.Event_RoomMemberAdded:
		v := e.RoomMemberAdded
		target.Event = &realtimev1.RealtimeEvent_RoomMemberAdded{RoomMemberAdded: &realtimev1.RoomMemberAddedEvent{RoomId: v.GetRoomId(), UserId: v.GetUserId()}}
	case *evtv1.Event_RoomMemberRemoved:
		v := e.RoomMemberRemoved
		target.Event = &realtimev1.RealtimeEvent_RoomMemberRemoved{RoomMemberRemoved: &realtimev1.RoomMemberRemovedEvent{RoomId: v.GetRoomId(), UserId: v.GetUserId()}}
	case *evtv1.Event_ReactionAdded:
		v := e.ReactionAdded
		target.Event = &realtimev1.RealtimeEvent_ReactionAdded{ReactionAdded: &realtimev1.ReactionAddedEvent{RoomId: v.GetRoomId(), MessageEventId: v.GetMessageEventId(), Emoji: v.GetEmoji()}}
	case *evtv1.Event_ReactionRemoved:
		v := e.ReactionRemoved
		target.Event = &realtimev1.RealtimeEvent_ReactionRemoved{ReactionRemoved: &realtimev1.ReactionRemovedEvent{RoomId: v.GetRoomId(), MessageEventId: v.GetMessageEventId(), Emoji: v.GetEmoji()}}
	default:
		return nil
	}
	return target
}

// projectRealtimePubSubEvent admits one restricted pubsub variant into the
// public union. Pubsub variants use public payload types directly, but the
// returned event is a deep copy because later authorization can remove fields
// for one viewer.
func projectRealtimePubSubEvent(source *pubsubv1.PubSubEvent) *realtimev1.RealtimeEvent {
	if source == nil || source.GetEvent() == nil {
		return nil
	}
	target := &realtimev1.RealtimeEvent{Id: source.GetId(), CreatedAt: source.GetCreatedAt()}
	if source.GetActorId() != "" {
		target.ActorId = proto.String(source.GetActorId())
	}
	switch e := source.GetEvent().(type) {
	case *pubsubv1.PubSubEvent_UserProfileChanged:
		target.Event = &realtimev1.RealtimeEvent_UserProfileChanged{UserProfileChanged: e.UserProfileChanged}
	case *pubsubv1.PubSubEvent_ViewerPreferencesChanged:
		target.Event = &realtimev1.RealtimeEvent_ViewerPreferencesChanged{ViewerPreferencesChanged: e.ViewerPreferencesChanged}
	case *pubsubv1.PubSubEvent_ThreadViewerStateChanged:
		target.Event = &realtimev1.RealtimeEvent_ThreadViewerStateChanged{ThreadViewerStateChanged: e.ThreadViewerStateChanged}
	case *pubsubv1.PubSubEvent_ServerProfileChanged:
		target.Event = &realtimev1.RealtimeEvent_ServerProfileChanged{ServerProfileChanged: e.ServerProfileChanged}
	case *pubsubv1.PubSubEvent_UserTyping:
		target.Event = &realtimev1.RealtimeEvent_UserTyping{UserTyping: e.UserTyping}
	case *pubsubv1.PubSubEvent_PresenceChanged:
		target.Event = &realtimev1.RealtimeEvent_PresenceChanged{PresenceChanged: e.PresenceChanged}
	case *pubsubv1.PubSubEvent_NotificationOccurrencesInvalidated:
		target.Event = &realtimev1.RealtimeEvent_NotificationOccurrencesInvalidated{NotificationOccurrencesInvalidated: e.NotificationOccurrencesInvalidated}
	case *pubsubv1.PubSubEvent_NotificationUnreadChanged:
		target.Event = &realtimev1.RealtimeEvent_NotificationUnreadChanged{NotificationUnreadChanged: e.NotificationUnreadChanged}
	case *pubsubv1.PubSubEvent_RoomReadStateChanged:
		target.Event = &realtimev1.RealtimeEvent_RoomReadStateChanged{RoomReadStateChanged: e.RoomReadStateChanged}
	default:
		return nil
	}
	return proto.Clone(target).(*realtimev1.RealtimeEvent)
}

func realtimeRoomKind(value evtv1.RoomKind) apiv1.RoomKind {
	switch value {
	case evtv1.RoomKind_ROOM_KIND_CHANNEL:
		return apiv1.RoomKind_ROOM_KIND_CHANNEL
	case evtv1.RoomKind_ROOM_KIND_DM:
		return apiv1.RoomKind_ROOM_KIND_DM
	default:
		return apiv1.RoomKind_ROOM_KIND_UNSPECIFIED
	}
}

func realtimeRoomThreadingMode(value evtv1.RoomThreadingMode) apiv1.RoomThreadingMode {
	switch value {
	case evtv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED:
		return apiv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED
	case evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED:
		return apiv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED
	case evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED:
		return apiv1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED
	case evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED:
		return apiv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED
	default:
		return apiv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED
	}
}

func realtimeSidebarGroupEntryKind(value evtv1.SidebarGroupEntry_Kind) realtimev1.SidebarGroupEntryKind {
	switch value {
	case evtv1.SidebarGroupEntry_ROOM:
		return realtimev1.SidebarGroupEntryKind_SIDEBAR_GROUP_ENTRY_KIND_ROOM
	case evtv1.SidebarGroupEntry_SIDEBAR_LINK:
		return realtimev1.SidebarGroupEntryKind_SIDEBAR_GROUP_ENTRY_KIND_SIDEBAR_LINK
	default:
		return realtimev1.SidebarGroupEntryKind_SIDEBAR_GROUP_ENTRY_KIND_UNSPECIFIED
	}
}

func realtimeCallParticipantSource(value evtv1.CallParticipantEventSource) realtimev1.CallParticipantEventSource {
	switch value {
	case evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER:
		return realtimev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER
	case evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT:
		return realtimev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_LIVEKIT
	case evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION:
		return realtimev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_RECONCILIATION
	default:
		return realtimev1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_UNSPECIFIED
	}
}

func realtimeAssetProcessingFailureCode(value evtv1.AssetProcessingFailureCode) realtimev1.AssetProcessingFailureCode {
	switch value {
	case evtv1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED:
		return realtimev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_PROCESSING_FAILED
	case evtv1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING:
		return realtimev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_SOURCE_MISSING
	default:
		return realtimev1.AssetProcessingFailureCode_ASSET_PROCESSING_FAILURE_CODE_UNSPECIFIED
	}
}

func realtimeMentions(values []*evtv1.MessageMention) []*realtimev1.MessageMention {
	result := make([]*realtimev1.MessageMention, 0, len(values))
	for _, v := range values {
		if v == nil {
			continue
		}
		m := &realtimev1.MessageMention{UserId: v.GetUserId()}
		switch c := v.GetCause().(type) {
		case *evtv1.MessageMention_Direct:
			m.Cause = &realtimev1.MessageMention_Direct{Direct: &realtimev1.DirectUserMention{}}
		case *evtv1.MessageMention_Role:
			m.Cause = &realtimev1.MessageMention_Role{Role: &realtimev1.RoleMessageMention{RoleName: c.Role.GetRoleName()}}
		case *evtv1.MessageMention_Here:
			m.Cause = &realtimev1.MessageMention_Here{Here: &realtimev1.HereMessageMention{}}
		case *evtv1.MessageMention_All:
			m.Cause = &realtimev1.MessageMention_All{All: &realtimev1.AllMessageMention{}}
		}
		result = append(result, m)
	}
	return result
}

func realtimeProcessedVideo(v *evtv1.AssetProcessedVideo) *realtimev1.AssetProcessedVideo {
	if v == nil {
		return nil
	}
	r := &realtimev1.AssetProcessedVideo{DurationMs: v.GetDurationMs(), Width: v.GetWidth(), Height: v.GetHeight(), ThumbnailAssetId: v.GetThumbnailAssetId()}
	for _, x := range v.GetVariants() {
		r.Variants = append(r.Variants, &realtimev1.AssetVideoVariant{Quality: x.GetQuality(), AssetId: x.GetAssetId()})
	}
	if v.GetHls() != nil {
		r.Hls = &realtimev1.AssetProcessedHLS{}
		for _, x := range v.GetHls().GetRenditions() {
			y := &realtimev1.AssetHLSRendition{Width: x.GetWidth(), Height: x.GetHeight(), Bandwidth: x.GetBandwidth()}
			for _, s := range x.GetSegments() {
				y.Segments = append(y.Segments, &realtimev1.AssetHLSSegment{AssetId: s.GetAssetId(), DurationMs: s.GetDurationMs()})
			}
			r.Hls.Renditions = append(r.Hls.Renditions, y)
		}
	}
	return r
}

func realtimeCustomStatus(v *evtv1.CustomUserStatus) *apiv1.CustomUserStatus {
	if v == nil {
		return nil
	}
	return &apiv1.CustomUserStatus{Emoji: v.GetEmoji(), Text: v.GetText(), ExpiresAt: v.GetExpiresAt()}
}

func applyRealtimePlaintext(target *realtimev1.RealtimeEvent, plaintext *core.EventPlaintext) {
	if target == nil || plaintext == nil {
		return
	}
	switch p := target.GetEvent().(type) {
	case *realtimev1.RealtimeEvent_MessagePosted:
		p.MessagePosted.BodyPlaintext = plaintext.MessageBody
	case *realtimev1.RealtimeEvent_UserAccountCreated:
		p.UserAccountCreated.LoginPlaintext = plaintext.Login
		p.UserAccountCreated.DisplayNamePlaintext = plaintext.DisplayName
	case *realtimev1.RealtimeEvent_UserLoginChanged:
		p.UserLoginChanged.LoginPlaintext = plaintext.Login
	case *realtimev1.RealtimeEvent_UserDisplayNameChanged:
		p.UserDisplayNameChanged.DisplayNamePlaintext = plaintext.DisplayName
	case *realtimev1.RealtimeEvent_UserBioChanged:
		p.UserBioChanged.BioPlaintext = plaintext.Bio
	}
}
