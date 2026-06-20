package http_server

import (
	"context"
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestClientLiveHistoryRoomEventsHydrateMessageRows(t *testing.T) {
	s := setupServerInfoServer(t, config.AuthConfig{})
	ctx := testContext(t)
	user, room := createLiveHistoryUserAndRoom(t, ctx, s, "history-room-user")
	posted, err := s.core.PostMessage(ctx, core.KindChannel, room.Id, user.Id, "wire history body", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("post message: %v", err)
	}

	session := newClientLiveSession(s, nil, user.Id, func() {})
	page, err := session.handleRoomEventsRequest(ctx, &corev1.ClientRoomEventsRequest{
		RoomId: room.Id,
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("handleRoomEventsRequest: %v", err)
	}

	item := findClientLiveHistoryItem(page.GetEvents(), posted.Id)
	if item == nil {
		t.Fatalf("history page did not include posted event %q", posted.Id)
	}
	if item.GetStreamSequence() == 0 {
		t.Fatal("expected stream sequence on history item")
	}
	if got := item.GetEvent().GetMessagePosted().GetBody(); got != "wire history body" {
		t.Fatalf("hydrated body = %q, want %q", got, "wire history body")
	}
	if page.GetStartCursorSeq() == 0 || page.GetEndCursorSeq() == 0 {
		t.Fatalf("expected non-zero cursors, got start=%d end=%d", page.GetStartCursorSeq(), page.GetEndCursorSeq())
	}
}

func TestClientLiveHistoryRejectsNonMembers(t *testing.T) {
	s := setupServerInfoServer(t, config.AuthConfig{})
	ctx := testContext(t)
	owner, room := createLiveHistoryUserAndRoom(t, ctx, s, "history-owner")
	if _, err := s.core.PostMessage(ctx, core.KindChannel, room.Id, owner.Id, "private room message", nil, "", "", nil, false); err != nil {
		t.Fatalf("post message: %v", err)
	}
	outsider, err := s.core.CreateUser(ctx, "system", "history-outsider", "History Outsider", "password123")
	if err != nil {
		t.Fatalf("create outsider: %v", err)
	}

	session := newClientLiveSession(s, nil, outsider.Id, func() {})
	_, err = session.handleRoomEventsRequest(ctx, &corev1.ClientRoomEventsRequest{
		RoomId: room.Id,
		Limit:  50,
	})
	if !errors.Is(err, core.ErrNotRoomMember) {
		t.Fatalf("error = %v, want ErrNotRoomMember", err)
	}
}

func TestClientLiveHistoryThreadInitialPageIncludesRootButCursorPagesDoNot(t *testing.T) {
	s := setupServerInfoServer(t, config.AuthConfig{})
	ctx := testContext(t)
	user, room := createLiveHistoryUserAndRoom(t, ctx, s, "history-thread-user")
	root, err := s.core.PostMessage(ctx, core.KindChannel, room.Id, user.Id, "thread root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("post root: %v", err)
	}
	reply, err := s.core.PostMessage(ctx, core.KindChannel, room.Id, user.Id, "thread reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("post reply: %v", err)
	}

	session := newClientLiveSession(s, nil, user.Id, func() {})
	initial, err := session.handleThreadEventsRequest(ctx, &corev1.ClientThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             50,
	})
	if err != nil {
		t.Fatalf("initial thread request: %v", err)
	}
	if findClientLiveHistoryItem(initial.GetEvents(), root.Id) == nil {
		t.Fatalf("initial thread page did not include root event %q", root.Id)
	}
	if findClientLiveHistoryItem(initial.GetEvents(), reply.Id) == nil {
		t.Fatalf("initial thread page did not include reply event %q", reply.Id)
	}

	cursorPage, err := session.handleThreadEventsRequest(ctx, &corev1.ClientThreadEventsRequest{
		RoomId:            room.Id,
		ThreadRootEventId: root.Id,
		Limit:             50,
		BeforeSeq:         initial.GetEndCursorSeq() + 1,
	})
	if err != nil {
		t.Fatalf("cursor thread request: %v", err)
	}
	if findClientLiveHistoryItem(cursorPage.GetEvents(), root.Id) != nil {
		t.Fatal("cursor thread page unexpectedly included root event")
	}
	if findClientLiveHistoryItem(cursorPage.GetEvents(), reply.Id) == nil {
		t.Fatalf("cursor thread page did not include reply event %q", reply.Id)
	}
}

func createLiveHistoryUserAndRoom(t *testing.T, ctx context.Context, s *HTTPServer, login string) (*corev1.User, *corev1.Room) {
	t.Helper()
	user, err := s.core.CreateUser(ctx, "system", login, login, "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	room, err := s.core.CreateRoom(ctx, user.Id, core.KindChannel, "", login+"-room", login+" room")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if _, err := s.core.JoinRoom(ctx, user.Id, core.KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("join room: %v", err)
	}
	return user, room
}

func findClientLiveHistoryItem(items []*corev1.ClientRoomEventItem, eventID string) *corev1.ClientRoomEventItem {
	for _, item := range items {
		if item.GetEvent().GetId() == eventID {
			return item
		}
	}
	return nil
}
