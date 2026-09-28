// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package connectapi

import (
	"testing"

	"connectrpc.com/connect"

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

func TestBuildRealtimeSnapshotHidesDMHistoryWithoutReadPermission(t *testing.T) {
	env := newConnectAPITestEnv(t)
	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "snapshot-dm-admin", "Snapshot DM Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, admin.GetId()); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	peer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "snapshot-dm-peer", "Snapshot DM Peer", "password")
	if err != nil {
		t.Fatalf("CreateUser peer: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.GetId(), []string{peer.GetId()})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.GetId(), env.viewer.GetId(), "private message", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.SetUserPermissionState(env.ctx, admin.GetId(), env.viewer.GetId(), core.PermissionTargetScope{Kind: core.MatrixScopeDM}, core.PermMessageRead, core.PermissionStateDeny); err != nil {
		t.Fatalf("deny DM message.read: %v", err)
	}

	snapshot, err := env.api.BuildRealtimeSnapshot(env.ctx, env.viewer.GetId())
	if err != nil {
		t.Fatalf("BuildRealtimeSnapshot: %v", err)
	}
	for _, room := range snapshot.Rooms.GetRooms() {
		if room.GetRoom().GetId() != dm.GetId() {
			continue
		}
		if room.HasMessageHistory == nil || room.GetHasMessageHistory() {
			t.Fatalf("DM has_message_history = %v, want explicit false", room.HasMessageHistory)
		}
		if len(room.GetMemberUserIds()) != 2 {
			t.Fatalf("DM member IDs = %v, want two participants", room.GetMemberUserIds())
		}
		return
	}
	t.Fatalf("DM %q is absent from realtime snapshot", dm.GetId())
}

func TestDMWithDeletedParticipantKeepsTheParticipant(t *testing.T) {
	env := newConnectAPITestEnv(t)
	peer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "deleted-dm-peer", "Deleted DM Peer", "password")
	if err != nil {
		t.Fatalf("CreateUser peer: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.GetId(), []string{peer.GetId()})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if err := env.core.DeleteUser(env.ctx, peer.GetId(), peer.GetId()); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	assertParticipants := func(t *testing.T, source string, rooms []*apiv1.RoomWithViewerState) {
		t.Helper()
		room := directoryRoomsByID(rooms)[dm.GetId()]
		if room == nil {
			t.Fatalf("%s: DM %q is absent", source, dm.GetId())
		}
		got := map[string]bool{}
		for _, id := range room.GetMemberUserIds() {
			got[id] = true
		}
		if len(got) != 2 || !got[env.viewer.GetId()] || !got[peer.GetId()] {
			t.Fatalf("%s: DM member IDs = %v, want the viewer and the deleted participant", source, room.GetMemberUserIds())
		}
	}

	resp, err := env.directory.ListRooms(withCaller(env.ctx, env.viewer), connect.NewRequest(&apiv1.ListRoomsRequest{
		Scope: apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_DMS,
	}))
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	assertParticipants(t, "ListRooms", resp.Msg.GetRooms())

	snapshot, err := env.api.BuildRealtimeSnapshot(env.ctx, env.viewer.GetId())
	if err != nil {
		t.Fatalf("BuildRealtimeSnapshot: %v", err)
	}
	assertParticipants(t, "BuildRealtimeSnapshot", snapshot.Rooms.GetRooms())
	var deleted *apiv1.User
	for _, member := range snapshot.Users.GetUsers() {
		if member.GetUser().GetId() == peer.GetId() {
			deleted = member.GetUser()
		}
	}
	if deleted == nil || !deleted.GetDeleted() || deleted.GetLogin() != "" {
		t.Fatalf("snapshot user for deleted participant = %+v, want a tombstone without PII", deleted)
	}
}
