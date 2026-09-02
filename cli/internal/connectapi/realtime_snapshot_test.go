// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package connectapi

import (
	"testing"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestBuildRealtimeSnapshotLimitsUsersAndOmitsRuntimePresence(t *testing.T) {
	env := newConnectAPITestEnv(t)
	peer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "snapshot-peer", "Snapshot Peer", "password")
	if err != nil {
		t.Fatalf("CreateUser peer: %v", err)
	}
	unreferenced, err := env.core.CreateUser(env.ctx, core.SystemActorID, "snapshot-unreferenced", "Snapshot Unreferenced", "password")
	if err != nil {
		t.Fatalf("CreateUser unreferenced: %v", err)
	}
	if _, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{peer.Id}); err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if err := env.core.SetPresence(env.ctx, env.viewer.Id, core.PresenceStatusAway); err != nil {
		t.Fatalf("SetPresence viewer: %v", err)
	}
	if err := env.core.SetPresence(env.ctx, peer.Id, core.PresenceStatusDoNotDisturb); err != nil {
		t.Fatalf("SetPresence peer: %v", err)
	}

	env.api.config.LiveKit = config.LiveKitConfig{
		Enabled:   true,
		URL:       "ws://livekit.test",
		APIKey:    "test-key",
		APISecret: "test-secret",
		ServerID:  "test-server",
	}
	room := env.createJoinedRoom("snapshot-active-call")
	if err := env.core.RecordCallParticipantJoined(env.ctx, room.Id, env.viewer.Id, evtv1.CallParticipantEventSource_CALL_PARTICIPANT_EVENT_SOURCE_USER); err != nil {
		t.Fatalf("RecordCallParticipantJoined: %v", err)
	}

	snapshot, err := env.api.BuildRealtimeSnapshot(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeSnapshot: %v", err)
	}
	users := make(map[string]*apiv1.User, len(snapshot.Users.GetUsers()))
	for _, member := range snapshot.Users.GetUsers() {
		users[member.GetUser().GetId()] = member.GetUser()
	}
	for _, userID := range []string{env.viewer.Id, peer.Id} {
		user := users[userID]
		if user == nil {
			t.Fatalf("referenced user %q is absent", userID)
		}
		if user.GetPresenceStatus() != apiv1.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED {
			t.Fatalf("snapshot user %q presence = %v, want UNSPECIFIED", userID, user.GetPresenceStatus())
		}
	}
	if users[unreferenced.Id] != nil {
		t.Fatalf("unreferenced directory user %q is present", unreferenced.Id)
	}
	if calls := snapshot.ActiveCalls.GetCalls(); len(calls) != 1 || len(calls[0].GetParticipants()) != 1 {
		t.Fatalf("active calls = %+v, want one call with one participant", calls)
	} else if got := calls[0].GetParticipants()[0].GetUser().GetPresenceStatus(); got != apiv1.PresenceStatus_PRESENCE_STATUS_UNSPECIFIED {
		t.Fatalf("snapshot call participant presence = %v, want UNSPECIFIED", got)
	}
}
