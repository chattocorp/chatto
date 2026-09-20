// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package http_server

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func TestRealtimeStateHydratesCurrentViewerStateAndNotifications(t *testing.T) {
	env := setupWebSocketTestServer(t)
	alice, err := env.core.CreateUser(env.ctx, core.SystemActorID, "state-alice", "Alice", "password123")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := env.core.CreateUser(env.ctx, core.SystemActorID, "state-bob", "Bob", "password123")
	if err != nil {
		t.Fatal(err)
	}
	dm, _, err := env.core.RoomCommands().StartDM(env.ctx, core.RoomStartDMInput{ActorID: alice.Id, ParticipantIDs: []string{bob.Id}})
	if err != nil {
		t.Fatal(err)
	}
	posted, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, alice.Id, "State delivery", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	notifications := &realtimev1.RealtimeEvent{Event: &realtimev1.RealtimeEvent_NotificationOccurrencesChanged{NotificationOccurrencesChanged: &realtimev1.NotificationOccurrencesChangedEvent{}}}
	env.httpServer.hydrateRealtimeState(env.ctx, bob.Id, notifications)
	page := notifications.GetNotificationOccurrencesChanged().GetNotifications()
	if page == nil || page.GetUnreadCount() != 1 || len(page.GetOccurrences()) != 1 {
		t.Fatalf("receiver notification snapshot = %v", page)
	}
	if page.GetOccurrences()[0].GetActor().GetId() != alice.Id {
		t.Fatal("notification actor missing")
	}
	readHint := func(viewerID string) *realtimev1.RoomReadStateChangedEvent {
		event := &realtimev1.RealtimeEvent{Event: &realtimev1.RealtimeEvent_RoomReadStateChanged{RoomReadStateChanged: &realtimev1.RoomReadStateChangedEvent{RoomId: dm.Id}}}
		env.httpServer.hydrateRealtimeState(env.ctx, viewerID, event)
		return event.GetRoomReadStateChanged()
	}
	room := readHint(bob.Id).GetRoom()
	if room.GetRoom().GetId() != dm.Id || !room.GetViewerState().GetIsMember() || !room.GetHasMessageHistory() || len(room.GetMemberUserIds()) != 2 {
		t.Fatalf("receiver DM state = %v", room)
	}
	if _, err := env.core.ReadState().MarkRoomAsRead(env.ctx, bob.Id, dm.Id, posted.Id); err != nil {
		t.Fatal(err)
	}
	// Reusing an old hint must hydrate current state, not restore its old counts.
	env.httpServer.hydrateRealtimeState(env.ctx, bob.Id, notifications)
	if notifications.GetNotificationOccurrencesChanged().GetNotifications().GetUnreadCount() != 0 {
		t.Fatal("old hint restored notification unread state")
	}
	if readHint(bob.Id).GetRoom().GetViewerState().GetHasUnread() {
		t.Fatal("read acknowledgement restored Badge attention")
	}
	// A non-participant must not receive the room, even with a known room ID.
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "state-outsider", "Outsider", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if readHint(outsider.Id).GetRoom() != nil {
		t.Fatal("private DM leaked to non-participant")
	}
	other := proto.Clone(notifications).(*realtimev1.RealtimeEvent)
	env.httpServer.hydrateRealtimeState(env.ctx, outsider.Id, other)
	if len(other.GetNotificationOccurrencesChanged().GetNotifications().GetOccurrences()) != 0 {
		t.Fatal("notification page leaked between viewers")
	}
}

func TestRealtimeStateHydrationFailureLeavesFallbackHint(t *testing.T) {
	env := setupWebSocketTestServer(t)
	ctx, cancel := context.WithCancel(env.ctx)
	cancel()
	event := &realtimev1.RealtimeEvent{Event: &realtimev1.RealtimeEvent_RoomReadStateChanged{RoomReadStateChanged: &realtimev1.RoomReadStateChangedEvent{RoomId: "missing"}}}
	env.httpServer.hydrateRealtimeState(ctx, "viewer", event)
	if event.GetRoomReadStateChanged().GetRoom() != nil || event.GetRoomReadStateChanged().GetRoomId() != "missing" {
		t.Fatal("failed hydration must preserve the fallback hint")
	}
}
