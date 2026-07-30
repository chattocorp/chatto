package connectapi

import (
	"testing"
	"time"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestRealtimeProjectionSnapshotHonorsMessageDeletionAndCryptoErasure(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("realtime-projection-deletion")
	author, err := env.core.CreateUser(env.ctx, core.SystemActorID, "projection-shredded-author", "Projection Shredded Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	retracted := env.post(room.Id, env.viewer.Id, "retracted secret", "")
	shredded := env.post(room.Id, author.Id, "crypto-erased secret", "")
	if err := env.core.DeleteMessage(env.ctx, env.viewer.Id, core.KindChannel, room.Id, retracted.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if err := env.core.DeleteUser(env.ctx, env.viewer.Id, author.Id); err != nil {
		t.Fatalf("DeleteUser author: %v", err)
	}

	snapshot, err := env.api.BuildRealtimeProjectionSnapshot(env.ctx, env.viewer.Id, []string{room.Id})
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionSnapshot: %v", err)
	}
	var page *apiv1.RoomTimelinePage
	var eventCursors map[string]string
	for _, timeline := range snapshot.Timelines {
		if timeline.RoomID == room.Id {
			page = timeline.Page
			eventCursors = timeline.EventCursors
			break
		}
	}
	if page == nil {
		t.Fatalf("room %q missing from realtime snapshot", room.Id)
	}
	for _, eventID := range []string{retracted.Id, shredded.Id} {
		if eventCursors[eventID] == "" {
			t.Fatalf("message %q has no realtime timeline cursor", eventID)
		}
		message := timelinePageEvent(page, eventID).GetMessagePosted().GetMessage()
		if message.GetDeletedAt() == nil {
			t.Fatalf("message %q deleted_at is absent", eventID)
		}
		if message.GetBody() != "" {
			t.Fatalf("message %q exposed deleted body %q", eventID, message.GetBody())
		}
	}
}

func TestBuildRealtimeProjectionPresencesReturnsLatestServerDirectoryState(t *testing.T) {
	env := newConnectAPITestEnv(t)
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "projection-presence", "Projection Presence", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := env.core.SetPresenceWithOptions(env.ctx, member.Id, core.PresenceStatusAway, true); err != nil {
		t.Fatalf("SetPresenceWithOptions: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		statuses, err := env.api.BuildRealtimeProjectionPresences(env.ctx)
		if err != nil {
			t.Fatalf("BuildRealtimeProjectionPresences: %v", err)
		}
		if statuses[member.Id] == apiv1.PresenceStatus_PRESENCE_STATUS_AWAY {
			if _, ok := statuses[env.viewer.Id]; !ok {
				t.Fatalf("viewer %q missing from complete presence map", env.viewer.Id)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("presence for %q never converged to AWAY: %v", member.Id, statuses[member.Id])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRealtimeProjectionSnapshotIncludesEmptyJoinedDM(t *testing.T) {
	env := newConnectAPITestEnv(t)
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "projection-empty-dm", "Projection Empty DM", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}

	snapshot, err := env.api.BuildRealtimeProjectionSnapshot(env.ctx, env.viewer.Id, nil)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionSnapshot: %v", err)
	}
	for _, room := range snapshot.Rooms {
		if room.Room.GetRoom().GetId() == dm.Id {
			if len(room.MemberUserIDs) != 2 {
				t.Fatalf("empty DM member references = %v, want two members", room.MemberUserIDs)
			}
			if room.HasMessageHistory == nil || *room.HasMessageHistory {
				t.Fatalf("empty DM has_message_history = %v, want explicit false", room.HasMessageHistory)
			}
			goto foundEmpty
		}
	}
	t.Fatalf("empty joined DM %s missing from realtime projection snapshot", dm.Id)

foundEmpty:
	message, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.viewer.Id, "first DM message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := env.core.DeleteMessage(env.ctx, env.viewer.Id, core.KindDM, dm.Id, message.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	room, err := env.api.BuildRealtimeProjectionRoomSummary(env.ctx, env.viewer.Id, dm.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionRoomSummary: %v", err)
	}
	if room.HasMessageHistory == nil || !*room.HasMessageHistory {
		t.Fatalf("used DM has_message_history = %v, want true after message retraction", room.HasMessageHistory)
	}
}

func TestRealtimeProjectionLatestValueViewerStatesConverge(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("projection-latest-viewer-state")
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "projection-state-other", "Projection State Other", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom other: %v", err)
	}
	root, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage root: %v", err)
	}
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, other.Id, "unread reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply: %v", err)
	}
	roomMessage, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, other.Id, "unread room message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage room: %v", err)
	}

	threadStates, err := env.api.BuildRealtimeProjectionThreadViewerStates(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionThreadViewerStates unread: %v", err)
	}
	foundUnreadThread := false
	for _, state := range threadStates {
		if state.ThreadRootEventID == root.Id {
			foundUnreadThread = state.ViewerState.GetIsFollowing() && state.ViewerState.GetHasUnread()
		}
	}
	if !foundUnreadThread {
		t.Fatalf("followed thread %s missing unread viewer state: %+v", root.Id, threadStates)
	}

	roomStates, err := env.api.BuildRealtimeProjectionRoomViewerStates(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionRoomViewerStates unread: %v", err)
	}
	if !realtimeRoomViewerState(roomStates, room.Id).GetHasUnread() {
		t.Fatalf("room %s should be unread before latest-value reconciliation", room.Id)
	}

	if _, err := env.core.ReadState().MarkRoomAsRead(env.ctx, env.viewer.Id, room.Id, roomMessage.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}
	if _, err := env.core.SetThreadLastReadEventID(env.ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id, reply.Id); err != nil {
		t.Fatalf("SetThreadLastReadEventID: %v", err)
	}
	roomStates, err = env.api.BuildRealtimeProjectionRoomViewerStates(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionRoomViewerStates read: %v", err)
	}
	if realtimeRoomViewerState(roomStates, room.Id).GetHasUnread() {
		t.Fatalf("room %s remained unread after cross-client marker advance", room.Id)
	}
	threadStates, err = env.api.BuildRealtimeProjectionThreadViewerStates(env.ctx, env.viewer.Id)
	if err != nil {
		t.Fatalf("BuildRealtimeProjectionThreadViewerStates read: %v", err)
	}
	for _, state := range threadStates {
		if state.ThreadRootEventID == root.Id && state.ViewerState.GetHasUnread() {
			t.Fatalf("thread %s remained unread after marker advance", root.Id)
		}
	}
}

func realtimeRoomViewerState(states []*RealtimeProjectionRoomViewerState, roomID string) *apiv1.RoomViewerState {
	for _, state := range states {
		if state.RoomID == roomID {
			return state.ViewerState
		}
	}
	return nil
}
