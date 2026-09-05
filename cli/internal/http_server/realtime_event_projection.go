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
func projectRealtimeEvent(viewerID string, source *evtv1.Event) *realtimev1.RealtimeEvent {
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
	case *evtv1.Event_RoomGroupCreated,
		*evtv1.Event_RoomGroupUpdated,
		*evtv1.Event_RoomGroupDeleted,
		*evtv1.Event_RoomAddedToGroup,
		*evtv1.Event_RoomRemovedFromGroup,
		*evtv1.Event_RoomsInGroupReordered,
		*evtv1.Event_SidebarLinkAddedToGroup,
		*evtv1.Event_SidebarLinkUpdated,
		*evtv1.Event_SidebarLinkRemovedFromGroup,
		*evtv1.Event_SidebarGroupEntriesReordered,
		*evtv1.Event_RoomGroupsReordered:
		target.Event = &realtimev1.RealtimeEvent_RoomLayoutChanged{RoomLayoutChanged: &realtimev1.RoomLayoutChangedEvent{}}
	case *evtv1.Event_VoiceCallParticipantJoined:
		v := e.VoiceCallParticipantJoined
		target.Event = &realtimev1.RealtimeEvent_VoiceCallParticipantJoined{VoiceCallParticipantJoined: &realtimev1.VoiceCallParticipantJoinedEvent{RoomId: v.GetRoomId(), CallId: v.GetCallId()}}
	case *evtv1.Event_VoiceCallParticipantLeft:
		v := e.VoiceCallParticipantLeft
		target.Event = &realtimev1.RealtimeEvent_VoiceCallParticipantLeft{VoiceCallParticipantLeft: &realtimev1.VoiceCallParticipantLeftEvent{RoomId: v.GetRoomId(), CallId: v.GetCallId()}}
	case *evtv1.Event_VoiceCallStarted:
		v := e.VoiceCallStarted
		target.Event = &realtimev1.RealtimeEvent_VoiceCallStarted{VoiceCallStarted: &realtimev1.VoiceCallStartedEvent{RoomId: v.GetRoomId(), CallId: v.GetCallId()}}
	case *evtv1.Event_VoiceCallEnded:
		v := e.VoiceCallEnded
		target.Event = &realtimev1.RealtimeEvent_VoiceCallEnded{VoiceCallEnded: &realtimev1.VoiceCallEndedEvent{RoomId: v.GetRoomId(), CallId: v.GetCallId()}}
	case *evtv1.Event_MessagePosted:
		v := e.MessagePosted
		target.Event = &realtimev1.RealtimeEvent_MessagePosted{MessagePosted: &realtimev1.MessagePostedEvent{RoomId: v.GetRoomId(), InReplyTo: v.GetInReplyTo(), ThreadRootEventId: v.GetInThread(), EchoOfEventId: v.GetEchoOfEventId(), EchoFromThreadRootEventId: v.GetEchoFromThreadRootEventId(), Mentions: realtimeMentions(viewerID, v.GetMentions())}}
	case *evtv1.Event_MessageEdited:
		v := e.MessageEdited
		target.Event = &realtimev1.RealtimeEvent_MessageEdited{MessageEdited: &realtimev1.MessageEditedEvent{RoomId: v.GetRoomId(), MessageEventId: v.GetEventId()}}
	case *evtv1.Event_MessageRetracted:
		v := e.MessageRetracted
		target.Event = &realtimev1.RealtimeEvent_MessageRetracted{MessageRetracted: &realtimev1.MessageRetractedEvent{RoomId: v.GetRoomId(), MessageEventId: v.GetEventId()}}
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
		target.Event = &realtimev1.RealtimeEvent_AssetProcessingSucceeded{AssetProcessingSucceeded: &realtimev1.AssetProcessingSucceededEvent{AssetId: v.GetAssetId(), MessageEventId: v.GetMessageEventId()}}
	case *evtv1.Event_AssetProcessingFailed:
		v := e.AssetProcessingFailed
		target.Event = &realtimev1.RealtimeEvent_AssetProcessingFailed{AssetProcessingFailed: &realtimev1.AssetProcessingFailedEvent{AssetId: v.GetAssetId(), FailureCode: realtimeAssetProcessingFailureCode(v.GetFailureCode()), MessageEventId: v.GetMessageEventId()}}
	case *evtv1.Event_AssetDeleted:
		target.Event = &realtimev1.RealtimeEvent_AssetDeleted{AssetDeleted: &realtimev1.AssetDeletedEvent{AssetId: e.AssetDeleted.GetAssetId()}}
	case *evtv1.Event_ServerMotdChanged:
		target.Event = &realtimev1.RealtimeEvent_ServerMotdChanged{ServerMotdChanged: &realtimev1.ServerMotdChangedEvent{Motd: e.ServerMotdChanged.GetMotd()}}
	case *evtv1.Event_ServerNameChanged,
		*evtv1.Event_ServerDescriptionChanged,
		*evtv1.Event_ServerLogoSet,
		*evtv1.Event_ServerLogoCleared,
		*evtv1.Event_ServerBannerSet,
		*evtv1.Event_ServerBannerCleared:
		target.Event = &realtimev1.RealtimeEvent_ServerProfileChanged{ServerProfileChanged: &realtimev1.ServerProfileChangedEvent{}}
	case *evtv1.Event_UserAccountCreated:
		v := e.UserAccountCreated
		target.Event = &realtimev1.RealtimeEvent_UserAccountCreated{UserAccountCreated: &realtimev1.UserAccountCreatedEvent{UserId: v.GetUserId(), IsBot: v.GetIsBot()}}
	case *evtv1.Event_UserLoginChanged,
		*evtv1.Event_UserDisplayNameChanged,
		*evtv1.Event_UserAvatarSet,
		*evtv1.Event_UserAvatarCleared,
		*evtv1.Event_UserCustomStatusSet,
		*evtv1.Event_UserCustomStatusCleared,
		*evtv1.Event_UserBioChanged,
		*evtv1.Event_AssetCreated:
		userID := core.UserIDOfPublicProfileEvent(source)
		target.Event = &realtimev1.RealtimeEvent_UserProfileChanged{UserProfileChanged: &realtimev1.UserProfileChangedEvent{UserId: userID}}
	case *evtv1.Event_UserAccountDeleted:
		target.Event = &realtimev1.RealtimeEvent_UserAccountDeleted{UserAccountDeleted: &realtimev1.UserAccountDeletedEvent{UserId: e.UserAccountDeleted.GetUserId()}}
	case *evtv1.Event_ThreadFollowed:
		v := e.ThreadFollowed
		target.Event = &realtimev1.RealtimeEvent_ThreadViewerStateChanged{ThreadViewerStateChanged: &realtimev1.ThreadViewerStateChangedEvent{RoomId: v.GetRoomId(), ThreadRootEventId: v.GetThreadRootEventId(), IsFollowing: true}}
	case *evtv1.Event_ThreadUnfollowed:
		v := e.ThreadUnfollowed
		target.Event = &realtimev1.RealtimeEvent_ThreadViewerStateChanged{ThreadViewerStateChanged: &realtimev1.ThreadViewerStateChangedEvent{RoomId: v.GetRoomId(), ThreadRootEventId: v.GetThreadRootEventId(), IsFollowing: false}}
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
	case *pubsubv1.PubSubEvent_ThreadViewerStateChanged:
		target.Event = &realtimev1.RealtimeEvent_ThreadViewerStateChanged{ThreadViewerStateChanged: e.ThreadViewerStateChanged}
	case *pubsubv1.PubSubEvent_UserTyping:
		target.Event = &realtimev1.RealtimeEvent_UserTyping{UserTyping: e.UserTyping}
	case *pubsubv1.PubSubEvent_PresenceChanged:
		target.Event = &realtimev1.RealtimeEvent_PresenceChanged{PresenceChanged: e.PresenceChanged}
	case *pubsubv1.PubSubEvent_NotificationOccurrencesChanged:
		target.Event = &realtimev1.RealtimeEvent_NotificationOccurrencesChanged{NotificationOccurrencesChanged: e.NotificationOccurrencesChanged}
	case *pubsubv1.PubSubEvent_NotificationUnreadStateChanged:
		target.Event = &realtimev1.RealtimeEvent_NotificationUnreadStateChanged{NotificationUnreadStateChanged: e.NotificationUnreadStateChanged}
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

// realtimeMentions keeps each target once and retains the viewer's original
// recipient decision. It never expands recipients from current room state.
func realtimeMentions(viewerID string, values []*evtv1.MessageMention) []*realtimev1.MessageMention {
	var result []*realtimev1.MessageMention
	type targetKey struct{ kind, id string }
	targets := make(map[targetKey]*realtimev1.MessageMention)
	for _, v := range values {
		if v.GetUserId() == "" {
			continue
		}
		var key targetKey
		m := &realtimev1.MessageMention{}
		switch c := v.GetCause().(type) {
		case *evtv1.MessageMention_Direct:
			key = targetKey{"direct", v.GetUserId()}
			m.Cause = &realtimev1.MessageMention_Direct{Direct: &realtimev1.DirectUserMention{UserId: v.GetUserId()}}
		case *evtv1.MessageMention_Role:
			if c.Role.GetRoleName() == "" {
				continue
			}
			key = targetKey{"role", c.Role.GetRoleName()}
			m.Cause = &realtimev1.MessageMention_Role{Role: &realtimev1.RoleMessageMention{RoleName: c.Role.GetRoleName()}}
		case *evtv1.MessageMention_Here:
			key = targetKey{kind: "here"}
			m.Cause = &realtimev1.MessageMention_Here{Here: &realtimev1.HereMessageMention{}}
		case *evtv1.MessageMention_All:
			key = targetKey{kind: "all"}
			m.Cause = &realtimev1.MessageMention_All{All: &realtimev1.AllMessageMention{}}
		default:
			continue
		}
		if existing := targets[key]; existing != nil {
			m = existing
		} else {
			targets[key] = m
			result = append(result, m)
		}
		m.IncludesViewer = m.GetIncludesViewer() || v.GetUserId() == viewerID
	}
	return result
}
