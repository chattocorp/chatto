package core

import (
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestEventFactsRoomIDAndVisibility(t *testing.T) {
	tests := []struct {
		name    string
		event   *evtv1.Event
		roomID  string
		visible bool
	}{
		{
			name: "root message",
			event: &evtv1.Event{Event: &evtv1.Event_MessagePosted{
				MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "thread reply",
			event: &evtv1.Event{Event: &evtv1.Event_MessagePosted{
				MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", InThread: "ROOT"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "edit",
			event: &evtv1.Event{Event: &evtv1.Event_MessageEdited{
				MessageEdited: &evtv1.MessageEditedEvent{RoomId: "R1", EventId: "M1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "asset creation is resolved by asset projections",
			event: &evtv1.Event{Event: &evtv1.Event_AssetCreated{
				AssetCreated: &evtv1.AssetCreatedEvent{RoomId: "R1"},
			}},
			roomID:  "",
			visible: false,
		},
		{
			name: "threading mode changed",
			event: &evtv1.Event{Event: &evtv1.Event_RoomThreadingModeChanged{
				RoomThreadingModeChanged: &evtv1.RoomThreadingModeChangedEvent{
					RoomId: "R1", ThreadingMode: evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED,
				},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "room member joined",
			event: &evtv1.Event{Event: &evtv1.Event_UserJoinedRoom{
				UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "room member left",
			event: &evtv1.Event{Event: &evtv1.Event_UserLeftRoom{
				UserLeftRoom: &evtv1.UserLeftRoomEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "voice call started",
			event: &evtv1.Event{Event: &evtv1.Event_VoiceCallStarted{
				VoiceCallStarted: &evtv1.CallStartedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "voice call ended",
			event: &evtv1.Event{Event: &evtv1.Event_VoiceCallEnded{
				VoiceCallEnded: &evtv1.CallEndedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "voice call participant joined",
			event: &evtv1.Event{Event: &evtv1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &evtv1.CallParticipantJoinedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "voice call participant left",
			event: &evtv1.Event{Event: &evtv1.Event_VoiceCallParticipantLeft{
				VoiceCallParticipantLeft: &evtv1.CallParticipantLeftEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "thread followed",
			event: &evtv1.Event{Event: &evtv1.Event_ThreadFollowed{
				ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "thread unfollowed",
			event: &evtv1.Event{Event: &evtv1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "unlisted event variant is hidden by default",
			event: &evtv1.Event{Event: &evtv1.Event_RoomGroupCreated{
				RoomGroupCreated: &evtv1.RoomGroupCreatedEvent{GroupId: "G1"},
			}},
			roomID:  "",
			visible: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := roomIDOfEvent(tt.event); got != tt.roomID {
				t.Fatalf("roomIDOfEvent = %q, want %q", got, tt.roomID)
			}
			if got := isVisibleRoomTimelineEntry(tt.event); got != tt.visible {
				t.Fatalf("isVisibleRoomTimelineEntry = %v, want %v", got, tt.visible)
			}
		})
	}
}

func TestMessageEventSourceMessageID(t *testing.T) {
	core := &ChattoCore{}
	tests := []struct {
		name  string
		event *evtv1.Event
		want  string
		ok    bool
	}{
		{
			name: "posted message",
			event: &evtv1.Event{Id: "M1", Event: &evtv1.Event_MessagePosted{
				MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"},
			}},
			want: "M1", ok: true,
		},
		{
			name: "reaction target",
			event: &evtv1.Event{Event: &evtv1.Event_ReactionAdded{
				ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1", MessageEventId: "M2"},
			}},
			want: "M2", ok: true,
		},
		{
			name: "pin target",
			event: &evtv1.Event{Event: &evtv1.Event_MessagePinned{
				MessagePinned: &evtv1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M3"},
			}},
			want: "M3", ok: true,
		},
		{
			name: "attached asset target",
			event: &evtv1.Event{Event: &evtv1.Event_AssetAttached{
				AssetAttached: &evtv1.AssetAttachedEvent{RoomId: "R1", MessageEventId: "M4"},
			}},
			want: "M4", ok: true,
		},
		{
			name: "asset creation has no message source",
			event: &evtv1.Event{Event: &evtv1.Event_AssetCreated{
				AssetCreated: &evtv1.AssetCreatedEvent{RoomId: "R1"},
			}},
		},
		{
			name: "wrong room fails closed",
			event: &evtv1.Event{Event: &evtv1.Event_MessagePinned{
				MessagePinned: &evtv1.MessagePinnedEvent{RoomId: "R2", MessageEventId: "M5"},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := core.MessageEventSourceMessageID("R1", tt.event)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("MessageEventSourceMessageID = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestEventFactsAssetLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		event       *evtv1.Event
		assetID     string
		lifecycle   bool
		liveAsset   bool
		liveRoomEVT bool
		reactions   bool
		threads     bool
		directory   bool
		callState   bool
	}{
		{
			name: "created",
			event: &evtv1.Event{Event: &evtv1.Event_AssetCreated{
				AssetCreated: &evtv1.AssetCreatedEvent{Asset: &evtv1.AssetRecord{Id: "A1"}},
			}},
			assetID:     "A1",
			lifecycle:   true,
			liveAsset:   false,
			liveRoomEVT: false,
			reactions:   false,
			threads:     false,
			directory:   false,
			callState:   false,
		},
		{
			name: "attached",
			event: &evtv1.Event{Event: &evtv1.Event_AssetAttached{
				AssetAttached: &evtv1.AssetAttachedEvent{AssetId: "A1", RoomId: "R1", MessageEventId: "M1", UserId: "U1"},
			}},
			assetID:     "A1",
			lifecycle:   true,
			liveAsset:   false,
			liveRoomEVT: false,
			reactions:   false,
			threads:     false,
			directory:   false,
			callState:   false,
		},
		{
			name: "processing started",
			event: &evtv1.Event{Event: &evtv1.Event_AssetProcessingStarted{
				AssetProcessingStarted: &evtv1.AssetProcessingStartedEvent{AssetId: "A1"},
			}},
			assetID:     "A1",
			lifecycle:   true,
			liveAsset:   true,
			liveRoomEVT: true,
			reactions:   false,
			threads:     false,
			directory:   false,
			callState:   false,
		},
		{
			name: "message posted",
			event: &evtv1.Event{Event: &evtv1.Event_MessagePosted{
				MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     true,
			directory:   false,
			callState:   false,
		},
		{
			name: "thread reply",
			event: &evtv1.Event{Event: &evtv1.Event_MessagePosted{
				MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", InThread: "ROOT"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     true,
			directory:   false,
			callState:   false,
		},
		{
			name: "message edited",
			event: &evtv1.Event{Event: &evtv1.Event_MessageEdited{
				MessageEdited: &evtv1.MessageEditedEvent{RoomId: "R1", EventId: "M1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     true,
			directory:   false,
			callState:   false,
		},
		{
			name: "thread created",
			event: &evtv1.Event{Event: &evtv1.Event_ThreadCreated{
				ThreadCreated: &evtv1.ThreadCreatedEvent{RoomId: "R1", ThreadRootEventId: "ROOT"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     true,
			directory:   false,
			callState:   false,
		},
		{
			name: "thread followed",
			event: &evtv1.Event{Event: &evtv1.Event_ThreadFollowed{
				ThreadFollowed: &evtv1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: false,
			reactions:   false,
			threads:     true,
			directory:   false,
			callState:   false,
		},
		{
			name: "thread unfollowed",
			event: &evtv1.Event{Event: &evtv1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: false,
			reactions:   false,
			threads:     true,
			directory:   false,
			callState:   false,
		},
		{
			name: "reaction added",
			event: &evtv1.Event{Event: &evtv1.Event_ReactionAdded{
				ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "R1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   true,
			threads:     false,
			directory:   false,
			callState:   false,
		},
		{
			name: "message pinned",
			event: &evtv1.Event{Event: &evtv1.Event_MessagePinned{
				MessagePinned: &evtv1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     false,
			directory:   false,
			callState:   false,
		},
		{
			name: "message unpinned",
			event: &evtv1.Event{Event: &evtv1.Event_MessageUnpinned{
				MessageUnpinned: &evtv1.MessageUnpinnedEvent{RoomId: "R1", MessageEventId: "M1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     false,
			directory:   false,
			callState:   false,
		},
		{
			name: "room member joined",
			event: &evtv1.Event{Event: &evtv1.Event_UserJoinedRoom{
				UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: "R1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     false,
			directory:   true,
			callState:   false,
		},
		{
			name: "voice call participant joined",
			event: &evtv1.Event{Event: &evtv1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &evtv1.CallParticipantJoinedEvent{RoomId: "R1"},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: true,
			reactions:   false,
			threads:     false,
			directory:   false,
			callState:   true,
		},
		{
			name: "custom user status set",
			event: &evtv1.Event{Event: &evtv1.Event_UserCustomStatusSet{
				UserCustomStatusSet: &evtv1.UserCustomStatusSetEvent{UserId: "U1", Status: &evtv1.CustomUserStatus{Emoji: "🌿"}},
			}},
			lifecycle:   false,
			liveAsset:   false,
			liveRoomEVT: false,
			reactions:   false,
			threads:     false,
			directory:   false,
			callState:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := assetIDOfLifecycleEvent(tt.event); got != tt.assetID {
				t.Fatalf("assetIDOfLifecycleEvent = %q, want %q", got, tt.assetID)
			}
			if got := isAssetLifecycleEvent(tt.event); got != tt.lifecycle {
				t.Fatalf("isAssetLifecycleEvent = %v, want %v", got, tt.lifecycle)
			}
			if got := isDeliverableLiveEVTAssetEvent(tt.event); got != tt.liveAsset {
				t.Fatalf("isDeliverableLiveEVTAssetEvent = %v, want %v", got, tt.liveAsset)
			}
			if got := isDeliverableLiveEVTRoomEvent(tt.event); got != tt.liveRoomEVT {
				t.Fatalf("isDeliverableLiveEVTRoomEvent = %v, want %v", got, tt.liveRoomEVT)
			}
			if got := eventNeedsReactionProjection(tt.event); got != tt.reactions {
				t.Fatalf("eventNeedsReactionProjection = %v, want %v", got, tt.reactions)
			}
			if got := eventNeedsThreadProjection(tt.event); got != tt.threads {
				t.Fatalf("eventNeedsThreadProjection = %v, want %v", got, tt.threads)
			}
			if got := eventNeedsRoomDirectoryProjection(tt.event); got != tt.directory {
				t.Fatalf("eventNeedsRoomDirectoryProjection = %v, want %v", got, tt.directory)
			}
			if got := eventNeedsCallStateProjection(tt.event); got != tt.callState {
				t.Fatalf("eventNeedsCallStateProjection = %v, want %v", got, tt.callState)
			}
		})
	}
}

func TestEventFactsUserLiveEVT(t *testing.T) {
	tests := []struct {
		name  string
		event *evtv1.Event
		want  bool
	}{
		{
			name: "custom status set",
			event: &evtv1.Event{Event: &evtv1.Event_UserCustomStatusSet{
				UserCustomStatusSet: &evtv1.UserCustomStatusSetEvent{UserId: "U1", Status: &evtv1.CustomUserStatus{Emoji: "🌿"}},
			}},
			want: true,
		},
		{
			name: "custom status cleared",
			event: &evtv1.Event{Event: &evtv1.Event_UserCustomStatusCleared{
				UserCustomStatusCleared: &evtv1.UserCustomStatusClearedEvent{UserId: "U1"},
			}},
			want: true,
		},
		{
			name: "login change refreshes the public user projection",
			event: &evtv1.Event{Event: &evtv1.Event_UserLoginChanged{
				UserLoginChanged: &evtv1.UserLoginChangedEvent{UserId: "U1"},
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDeliverableLiveEVTUserEvent(tt.event); got != tt.want {
				t.Fatalf("isDeliverableLiveEVTUserEvent = %v, want %v", got, tt.want)
			}
		})
	}
}
