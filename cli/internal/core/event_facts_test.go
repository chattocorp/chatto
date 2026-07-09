package core

import (
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestEventFactsRoomIDAndVisibility(t *testing.T) {
	tests := []struct {
		name    string
		event   *corev1.Event
		roomID  string
		visible bool
	}{
		{
			name: "root message",
			event: &corev1.Event{Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "thread reply",
			event: &corev1.Event{Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1", InThread: "ROOT"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "edit",
			event: &corev1.Event{Event: &corev1.Event_MessageEdited{
				MessageEdited: &corev1.MessageEditedEvent{RoomId: "R1", EventId: "M1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "asset creation is resolved by asset projections",
			event: &corev1.Event{Event: &corev1.Event_AssetCreated{
				AssetCreated: &corev1.AssetCreatedEvent{RoomId: "R1"},
			}},
			roomID:  "",
			visible: false,
		},
		{
			name: "room member joined",
			event: &corev1.Event{Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "room member left",
			event: &corev1.Event{Event: &corev1.Event_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: true,
		},
		{
			name: "voice call started",
			event: &corev1.Event{Event: &corev1.Event_VoiceCallStarted{
				VoiceCallStarted: &corev1.CallStartedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "voice call ended",
			event: &corev1.Event{Event: &corev1.Event_VoiceCallEnded{
				VoiceCallEnded: &corev1.CallEndedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "voice call participant joined",
			event: &corev1.Event{Event: &corev1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "voice call participant left",
			event: &corev1.Event{Event: &corev1.Event_VoiceCallParticipantLeft{
				VoiceCallParticipantLeft: &corev1.CallParticipantLeftEvent{RoomId: "R1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "thread followed",
			event: &corev1.Event{Event: &corev1.Event_ThreadFollowed{
				ThreadFollowed: &corev1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "thread unfollowed",
			event: &corev1.Event{Event: &corev1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &corev1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
			}},
			roomID:  "R1",
			visible: false,
		},
		{
			name: "unlisted event variant is hidden by default",
			event: &corev1.Event{Event: &corev1.Event_RoomGroupCreated{
				RoomGroupCreated: &corev1.RoomGroupCreatedEvent{GroupId: "G1"},
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

func TestEventFactsAssetLifecycle(t *testing.T) {
	tests := []struct {
		name        string
		event       *corev1.Event
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
			event: &corev1.Event{Event: &corev1.Event_AssetCreated{
				AssetCreated: &corev1.AssetCreatedEvent{Asset: &corev1.AssetRecord{Id: "A1"}},
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
			event: &corev1.Event{Event: &corev1.Event_AssetProcessingStarted{
				AssetProcessingStarted: &corev1.AssetProcessingStartedEvent{AssetId: "A1"},
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
			event: &corev1.Event{Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"},
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
			name: "thread reply",
			event: &corev1.Event{Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1", InThread: "ROOT"},
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
			event: &corev1.Event{Event: &corev1.Event_MessageEdited{
				MessageEdited: &corev1.MessageEditedEvent{RoomId: "R1", EventId: "M1"},
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
			event: &corev1.Event{Event: &corev1.Event_ThreadCreated{
				ThreadCreated: &corev1.ThreadCreatedEvent{RoomId: "R1", ThreadRootEventId: "ROOT"},
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
			event: &corev1.Event{Event: &corev1.Event_ThreadFollowed{
				ThreadFollowed: &corev1.ThreadFollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
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
			event: &corev1.Event{Event: &corev1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &corev1.ThreadUnfollowedEvent{RoomId: "R1", ThreadRootEventId: "ROOT", UserId: "U1"},
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
			event: &corev1.Event{Event: &corev1.Event_ReactionAdded{
				ReactionAdded: &corev1.ReactionAddedEvent{RoomId: "R1"},
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
			name: "room member joined",
			event: &corev1.Event{Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
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
			event: &corev1.Event{Event: &corev1.Event_VoiceCallParticipantJoined{
				VoiceCallParticipantJoined: &corev1.CallParticipantJoinedEvent{RoomId: "R1"},
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
			event: &corev1.Event{Event: &corev1.Event_UserCustomStatusSet{
				UserCustomStatusSet: &corev1.UserCustomStatusSetEvent{UserId: "U1", Status: &corev1.CustomUserStatus{Emoji: "🌿"}},
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
		event *corev1.Event
		want  bool
	}{
		{
			name: "custom status set",
			event: &corev1.Event{Event: &corev1.Event_UserCustomStatusSet{
				UserCustomStatusSet: &corev1.UserCustomStatusSetEvent{UserId: "U1", Status: &corev1.CustomUserStatus{Emoji: "🌿"}},
			}},
			want: true,
		},
		{
			name: "custom status cleared",
			event: &corev1.Event{Event: &corev1.Event_UserCustomStatusCleared{
				UserCustomStatusCleared: &corev1.UserCustomStatusClearedEvent{UserId: "U1"},
			}},
			want: true,
		},
		{
			name: "login change is not delivered",
			event: &corev1.Event{Event: &corev1.Event_UserLoginChanged{
				UserLoginChanged: &corev1.UserLoginChangedEvent{UserId: "U1"},
			}},
			want: false,
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
