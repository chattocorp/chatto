package core

import (
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestChattoCore_GetRoomLastEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")

	// Initially: no last event
	id, _, exists, err := core.GetRoomLastEvent(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event: %v", err)
	}
	if exists || id != "" {
		t.Errorf("Expected no last event for empty room, got id=%q exists=%v", id, exists)
	}

	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	first, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "First message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	id, ts, exists, err := core.GetRoomLastEvent(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event after post: %v", err)
	}
	if !exists {
		t.Fatal("Expected room last event to exist after a post")
	}
	if id != first.Id {
		t.Errorf("Expected last event id %q, got %q", first.Id, id)
	}
	if ts.IsZero() {
		t.Error("Expected non-zero timestamp")
	}

	second, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Second message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second message: %v", err)
	}

	id, _, _, err = core.GetRoomLastEvent(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event after second post: %v", err)
	}
	if id != second.Id {
		t.Errorf("Expected last event id %q after second post, got %q", second.Id, id)
	}
}

func TestChattoCore_LastReadEventID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// This test only exercises the RUNTIME_STATE read marker. Using fresh IDs
	// keeps it independent of room/user creation and projection timing.
	roomID := NewRoomID()
	userID := NewUserID()

	// Initially: empty (never read)
	id, err := core.GetLastReadEventID(ctx, KindChannel, userID, roomID)
	if err != nil {
		t.Fatalf("Failed to get last read event id: %v", err)
	}
	if id != "" {
		t.Errorf("Expected empty for unread room, got %q", id)
	}

	// Set and read back
	if err := core.SetLastReadEventID(ctx, KindChannel, userID, roomID, "Eabcdefghij012"); err != nil {
		t.Fatalf("Failed to set last read event id: %v", err)
	}
	id, err = core.GetLastReadEventID(ctx, KindChannel, userID, roomID)
	if err != nil {
		t.Fatalf("Failed to get last read event id after set: %v", err)
	}
	if id != "Eabcdefghij012" {
		t.Errorf("Expected %q, got %q", "Eabcdefghij012", id)
	}

	// Overwrite
	if err := core.SetLastReadEventID(ctx, KindChannel, userID, roomID, "Exyzxyzxyzxyz9"); err != nil {
		t.Fatalf("Failed to update last read event id: %v", err)
	}
	id, err = core.GetLastReadEventID(ctx, KindChannel, userID, roomID)
	if err != nil {
		t.Fatalf("Failed to get last read event id after update: %v", err)
	}
	if id != "Exyzxyzxyzxyz9" {
		t.Errorf("Expected %q, got %q", "Exyzxyzxyzxyz9", id)
	}
}

func TestChattoCore_AdvanceLastReadEventIDDoesNotRegress(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "poster", "poster", "password123")
	reader, _ := core.CreateUser(ctx, "system", "reader", "reader", "password123")
	core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id)

	first, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "First message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage first: %v", err)
	}
	second, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "Second message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage second: %v", err)
	}
	if _, err := core.JoinRoom(ctx, reader.Id, KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	advance, err := core.AdvanceLastReadEventID(ctx, KindChannel, reader.Id, room.Id, first.Id)
	if err != nil {
		t.Fatalf("AdvanceLastReadEventID stale first: %v", err)
	}
	if advance.Updated {
		t.Fatal("AdvanceLastReadEventID updated stale marker, want no update")
	}
	if advance.CurrentEventID != second.Id {
		t.Fatalf("CurrentEventID = %q, want %q", advance.CurrentEventID, second.Id)
	}
	got, err := core.GetLastReadEventID(ctx, KindChannel, reader.Id, room.Id)
	if err != nil {
		t.Fatalf("GetLastReadEventID after stale advance: %v", err)
	}
	if got != second.Id {
		t.Fatalf("last read marker regressed to %q, want %q", got, second.Id)
	}

	if err := core.SetLastReadEventID(ctx, KindChannel, reader.Id, room.Id, "Edoesnotexist"); err != nil {
		t.Fatalf("SetLastReadEventID stale marker: %v", err)
	}
	advance, err = core.AdvanceLastReadEventID(ctx, KindChannel, reader.Id, room.Id, first.Id)
	if err != nil {
		t.Fatalf("AdvanceLastReadEventID stale recovery: %v", err)
	}
	if !advance.Updated {
		t.Fatal("AdvanceLastReadEventID did not repair stale marker")
	}
	if advance.CurrentEventID != first.Id {
		t.Fatalf("CurrentEventID after stale recovery = %q, want %q", advance.CurrentEventID, first.Id)
	}
}

// TestChattoCore_LastReadEventID_LazyInitCaughtUp verifies that a user with
// no read marker yet (e.g. a pre-existing user encountering this code path
// for the first time post-deploy) is lazy-initialized as caught up to the
// room's current last root event, so they don't see a wall of unreads.
func TestChattoCore_LastReadEventID_LazyInitCaughtUp(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "poster", "poster", "password123")
	core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id)

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "msg", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage error: %v", err)
	}

	// A user that has never written a marker (simulating post-deploy upgrade
	// where no `room_read_event` entry exists for them) should be lazy-
	// initialized to the room's current last event, not treated as unread.
	stranger, _ := core.CreateUser(ctx, "system", "stranger", "stranger", "password123")
	got, err := core.GetLastReadEventID(ctx, KindChannel, stranger.Id, room.Id)
	if err != nil {
		t.Fatalf("GetLastReadEventID error: %v", err)
	}
	if got != posted.Id {
		t.Errorf("Expected lazy init to current last event %q, got %q", posted.Id, got)
	}

	// The marker should now be persisted — a second read returns the same
	// value without re-running the init.
	got2, err := core.GetLastReadEventID(ctx, KindChannel, stranger.Id, room.Id)
	if err != nil {
		t.Fatalf("Second GetLastReadEventID error: %v", err)
	}
	if got2 != posted.Id {
		t.Errorf("Expected persisted marker %q, got %q", posted.Id, got2)
	}
}

// TestChattoCore_LastReadEventID_LazyInitEmptyRoom verifies that lazy init
// against an empty room returns "" without writing a marker.
func TestChattoCore_LastReadEventID_LazyInitEmptyRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "empty-user", "empty-user", "password123")

	got, err := core.GetLastReadEventID(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("GetLastReadEventID error: %v", err)
	}
	if got != "" {
		t.Errorf("Expected empty event id for empty room, got %q", got)
	}
}

func TestChattoCore_HasUnread_NoMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Room with no messages should have no unread
	hasUnread, err := core.HasUnread(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Error("Expected no unread for room with no messages")
	}
}

func TestChattoCore_HasUnread_NewMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)

	// User1 posts a message
	_, err := core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Hello!", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	// User2 should have unread (hasn't read the room yet)
	hasUnread, err := core.HasUnread(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for user2: %v", err)
	}
	if !hasUnread {
		t.Error("Expected user2 to have unread messages")
	}

	// User1 should NOT have unread (they posted, so they've "read" up to that point)
	hasUnread, err = core.HasUnread(ctx, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for user1: %v", err)
	}
	if hasUnread {
		t.Error("Expected user1 to have NO unread (posting auto-marks as read)")
	}
}

func TestChattoCore_PostMessageClearsExistingRoomBadge(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "Poster clears Badge", "")
	if err != nil {
		t.Fatal(err)
	}
	poster, err := core.CreateUser(ctx, SystemActorID, "poster-clears-badge", "Poster Clears Badge", "password123")
	if err != nil {
		t.Fatal(err)
	}
	other, err := core.CreateUser(ctx, SystemActorID, "poster-badge-source", "Poster Badge Source", "password123")
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{poster.Id, other.Id} {
		if _, err := core.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, other.Id, "unread source", nil, "", "", nil, false); err != nil {
		t.Fatal(err)
	}
	if err := core.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if unread, err := core.HasUnread(ctx, KindChannel, poster.Id, room.Id); err != nil || !unread {
		t.Fatalf("Badge before poster reply = (%v, %v), want (true, nil)", unread, err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "I have caught up", nil, "", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	if unread, err := core.HasUnread(ctx, KindChannel, poster.Id, room.Id); err != nil || unread {
		t.Fatalf("Badge after poster reply = (%v, %v), want (false, nil)", unread, err)
	}
	readID, exists, err := core.PeekLastReadEventID(ctx, poster.Id, room.Id)
	if err != nil || !exists || readID != posted.Id {
		t.Fatalf("poster Message Read Cursor = (%q, %v, %v), want %q", readID, exists, err, posted.Id)
	}
}

func TestChattoCore_HasUnread_RoomMessageOffKeepsCursorWithoutBadge(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "Off room", "")
	author, _ := core.CreateUser(ctx, "system", "off-room-author", "Off Room Author", "password123")
	recipient, _ := core.CreateUser(ctx, "system", "off-room-recipient", "Off Room Recipient", "password123")
	for _, userID := range []string{author.Id, recipient.Id} {
		if _, err := core.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
	}
	if _, err := core.NotificationPolicy().SetRoomNotificationMode(
		ctx, recipient.Id, room.Id, notificationTestSignalRoomMessage,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	); err != nil {
		t.Fatalf("disable room-message attention: %v", err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "No dot", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := core.notificationMaterializer.WaitCurrent(ctx); err != nil {
		t.Fatalf("wait for notification materializer: %v", err)
	}
	if hasUnread, err := core.HasUnread(ctx, KindChannel, recipient.Id, room.Id); err != nil || hasUnread {
		t.Fatalf("Badge attention with Room messages Off = (%v, %v), want (false, nil)", hasUnread, err)
	}
	readID, exists, err := core.PeekLastReadEventID(ctx, recipient.Id, room.Id)
	if err != nil || !exists || readID != "" {
		t.Fatalf("stored read cursor = (%q, %v, %v), want empty join sentinel", readID, exists, err)
	}
	if _, err := core.ReadState().MarkRoomAsRead(ctx, recipient.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("advance independent read cursor: %v", err)
	}
	readID, exists, err = core.PeekLastReadEventID(ctx, recipient.Id, room.Id)
	if err != nil || !exists || readID != posted.Id {
		t.Fatalf("advanced read cursor = (%q, %v, %v), want %q", readID, exists, err, posted.Id)
	}
}

func TestChattoCore_ReadStateUsesLatestReadableInteractionRoot(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chatto.CreateUser(ctx, SystemActorID, "interaction-unread-author", "Interaction Unread Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reader, err := chatto.CreateUser(ctx, SystemActorID, "interaction-unread-reader", "Interaction Unread Reader", "password123")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, author.GetId(), KindChannel, "", "interaction-unread", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.GetId(), reader.GetId()} {
		if _, err := chatto.JoinRoom(ctx, userID, KindChannel, userID, room.GetId()); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.GetId(), RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission message.read: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read-interactions: %v", err)
	}
	first, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "@interaction-unread-reader first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage first related root: %v", err)
	}
	if _, err := chatto.ReadState().MarkRoomAsRead(ctx, reader.GetId(), room.GetId(), first.GetId()); err != nil {
		t.Fatalf("MarkRoomAsRead first related root: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "unrelated newer root", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage unrelated root: %v", err)
	}
	if latest, _, exists, err := chatto.GetRoomLastReadableEvent(ctx, KindChannel, reader.GetId(), room.GetId()); err != nil || !exists || latest != first.GetId() {
		t.Fatalf("latest readable event after unrelated root = %q, %v, %v; want %q", latest, exists, err, first.GetId())
	}
	second, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "@interaction-unread-reader second", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage second related root: %v", err)
	}
	if latest, _, exists, err := chatto.GetRoomLastReadableEvent(ctx, KindChannel, reader.GetId(), room.GetId()); err != nil || !exists || latest != second.GetId() {
		t.Fatalf("latest readable event after related root = %q, %v, %v; want %q", latest, exists, err, second.GetId())
	}
	if _, err := chatto.ReadState().MarkRoomAsRead(ctx, reader.GetId(), room.GetId(), ""); err != nil {
		t.Fatalf("MarkRoomAsRead latest readable root: %v", err)
	}
	if marker, err := chatto.GetLastReadEventID(ctx, KindChannel, reader.GetId(), room.GetId()); err != nil || marker != second.GetId() {
		t.Fatalf("read marker = %q, %v; want %q (first was %q)", marker, err, second.GetId(), first.GetId())
	}
	reply, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "related reply echoed to the channel", nil, second.GetId(), "", nil, true)
	if err != nil {
		t.Fatalf("PostMessage related channel echo: %v", err)
	}
	echoID, ok := chatto.roomModel.channelEchoEventID(reply.GetId())
	if !ok {
		t.Fatal("channel echo was not projected")
	}
	if latest, _, exists, err := chatto.GetRoomLastReadableEvent(ctx, KindChannel, reader.GetId(), room.GetId()); err != nil || !exists || latest != echoID {
		t.Fatalf("latest readable event = %q, %v, %v; want channel echo %q", latest, exists, err, echoID)
	}
	if _, err := chatto.ReadState().MarkRoomAsRead(ctx, reader.GetId(), room.GetId(), ""); err != nil {
		t.Fatalf("MarkRoomAsRead related channel echo: %v", err)
	}
	unrelated, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "unrelated root for an echo", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage unrelated echo root: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "unrelated reply echoed to the channel", nil, unrelated.GetId(), "", nil, true); err != nil {
		t.Fatalf("PostMessage unrelated channel echo: %v", err)
	}
	if latest, _, exists, err := chatto.GetRoomLastReadableEvent(ctx, KindChannel, reader.GetId(), room.GetId()); err != nil || !exists || latest != echoID {
		t.Fatalf("latest readable event after unrelated echo = %q, %v, %v; want %q", latest, exists, err, echoID)
	}
}

func TestChattoCore_HasUnread_AfterMarkingRead(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup with two users
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)

	// User2 posts a message
	_, err := core.PostMessage(ctx, KindChannel, room.Id, user2.Id, "Hello!", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	// User1 should have unread (someone else posted)
	hasUnread, err := core.HasUnread(ctx, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if !hasUnread {
		t.Error("Expected user1 to have unread from user2's message")
	}

	// Get the room's last event
	lastID, _, exists, err := core.GetRoomLastEvent(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event: %v", err)
	}
	if !exists {
		t.Fatal("Expected room to have a last event")
	}

	// User1 marks as read up to the last event. This advances the cursor and
	// clears notification attention through the same user operation.
	if _, err := core.ReadState().MarkRoomAsRead(ctx, user1.Id, room.Id, lastID); err != nil {
		t.Fatalf("Failed to set last read event id: %v", err)
	}

	// User1 should have no unread now
	hasUnread, err = core.HasUnread(ctx, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Error("Expected no unread after marking as read")
	}

	// User2 posts another message
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user2.Id, "Another message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second message: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	// User1 should have unread again (user2 posted new message)
	hasUnread, err = core.HasUnread(ctx, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after new message: %v", err)
	}
	if !hasUnread {
		t.Error("Expected unread after new message posted by another user")
	}
}

func TestChattoCore_HasUnread_NonMember(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	member, _ := core.CreateUser(ctx, "system", "member", "member", "password123")
	nonMember, _ := core.CreateUser(ctx, "system", "nonmember", "nonmember", "password123")

	// Only member joins
	core.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id)

	// Post a message
	_, err := core.PostMessage(ctx, KindChannel, room.Id, member.Id, "Hello!", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Non-member should NOT have unread (returns false, not error)
	hasUnread, err := core.HasUnread(ctx, KindChannel, nonMember.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for non-member: %v", err)
	}
	if hasUnread {
		t.Error("Non-member should not have unread messages")
	}
}

func TestChattoCore_HasUnread_MultipleRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup with two users
	room1, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-1", "Room 1")
	room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-2", "Room 2")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room1.Id)
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room2.Id)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room1.Id)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room2.Id)

	// User2 posts to room1 only
	_, err := core.PostMessage(ctx, KindChannel, room1.Id, user2.Id, "Message in room1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post to room1: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	// Room1 should have unread for user1 (user2 posted)
	hasUnread, err := core.HasUnread(ctx, KindChannel, user1.Id, room1.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for room1: %v", err)
	}
	if !hasUnread {
		t.Error("Expected room1 to have unread for user1")
	}

	// Room2 should NOT have unread for user1 (no messages)
	hasUnread, err = core.HasUnread(ctx, KindChannel, user1.Id, room2.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for room2: %v", err)
	}
	if hasUnread {
		t.Error("Expected room2 to have no unread (no messages)")
	}

	// User1 marks room1 as read
	lastID, _, _, _ := core.GetRoomLastEvent(ctx, KindChannel, room1.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, user1.Id, room1.Id, lastID); err != nil {
		t.Fatalf("Failed to mark room1 read: %v", err)
	}

	// Room1 should now have no unread for user1
	hasUnread, err = core.HasUnread(ctx, KindChannel, user1.Id, room1.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for room1 after read: %v", err)
	}
	if hasUnread {
		t.Error("Expected room1 to have no unread after marking as read")
	}
}

func TestChattoCore_HasUnread_JoiningRoomWithExistingMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: user1 creates space and room
	room, _ := core.CreateRoom(ctx, "user1", KindChannel, "", "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")

	// user1 joins and posts messages BEFORE user2 joins
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)

	_, err := core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 1: %v", err)
	}
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 2", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 2: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	// Now user2 joins (after messages already exist)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)

	// user2 should NOT have unread - they just joined and haven't "been there" before
	// Existing messages should be considered "caught up" at join time
	hasUnread, err := core.HasUnread(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Error("Expected new member to NOT have unread for pre-existing messages")
	}

	// But if user1 posts a NEW message after user2 joined, that should be unread
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "New message after user2 joined", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post new message: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	hasUnread, err = core.HasUnread(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after new message: %v", err)
	}
	if !hasUnread {
		t.Error("Expected user2 to have unread for message posted after they joined")
	}
}

// TestChattoCore_HasUnread_StaleMarker verifies that a stale message cursor
// does not create Badge attention. The cursor remains available to the room
// timeline and is corrected by the next mark-read operation.
func TestChattoCore_HasUnread_StaleMarker(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "stale-marker-user", "stale-marker-user", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "real msg", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage error: %v", err)
	}

	// Force the read marker to reference a non-existent event ID — the
	// "marker pointed at a deleted message" scenario.
	if err := core.SetLastReadEventID(ctx, KindChannel, user.Id, room.Id, "Edoesnotexist"); err != nil {
		t.Fatalf("SetLastReadEventID error: %v", err)
	}

	hasUnread, err := core.HasUnread(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("HasUnread error: %v", err)
	}
	if hasUnread {
		t.Error("Expected stale read marker not to create Badge attention")
	}
}

// TestChattoCore_LastReadEventID_LazyInitRespectsExistingMarker verifies the
// invariant the Create-not-Put fix is there to guarantee: a marker written by
// any other path (MarkRoomAsRead, JoinRoom, PostMessage auto-mark) takes
// precedence over the lazy-init fallback. The race itself isn't directly
// exercisable without controlling KV timing, but the visible contract is
// "if a marker exists, GetLastReadEventID returns *that* value, never
// lazy-init's value."
func TestChattoCore_LastReadEventID_LazyInitRespectsExistingMarker(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "race-poster", "race-poster", "password123")
	core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id)
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "msg", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage error: %v", err)
	}

	// "Stranger" has no marker yet — the post-deploy / deploy-era case that
	// drives the lazy-init path. Pre-write the key directly with a marker
	// the stranger never wrote, simulating a concurrent winner.
	stranger, _ := core.CreateUser(ctx, "system", "race-stranger", "race-stranger", "password123")
	const concurrentWinner = "Eraceconcurwin"
	bucket := core.storage.runtimeStateKV
	key := roomReadEventKey(stranger.Id, room.Id)
	revision, err := bucket.Put(ctx, key, []byte(concurrentWinner))
	if err != nil {
		t.Fatalf("seed marker error: %v", err)
	}
	if err := core.readStateModel.index.waitForRevision(ctx, key, revision); err != nil {
		t.Fatalf("wait for seeded marker: %v", err)
	}

	got, err := core.GetLastReadEventID(ctx, KindChannel, stranger.Id, room.Id)
	if err != nil {
		t.Fatalf("GetLastReadEventID error: %v", err)
	}
	if got != concurrentWinner {
		t.Errorf("Expected concurrent winner %q, got %q (lazy-init clobbered)", concurrentWinner, got)
	}
}

func TestChattoCore_HasUnread_ThreadReplyDoesNotCauseUnread(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: two users in a room
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1-thread", "user1-thread", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2-thread", "user2-thread", "password123")
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)

	// User1 posts a root message
	rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post root message: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	// User2 reads the room (marks as read up to root message)
	lastID, _, _, err := core.GetRoomLastEvent(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get last event: %v", err)
	}
	if _, err := core.ReadState().MarkRoomAsRead(ctx, user2.Id, room.Id, lastID); err != nil {
		t.Fatalf("Failed to set last read: %v", err)
	}

	// Verify user2 has no unread
	hasUnread, err := core.HasUnread(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Fatal("Expected no unread after marking as read")
	}

	// User1 posts a thread reply to the root message
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Thread reply", nil, rootEvent.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post thread reply: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	// User2 should still NOT have unread — thread replies don't affect room-level unread
	hasUnread, err = core.HasUnread(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after thread reply: %v", err)
	}
	if hasUnread {
		t.Error("Thread reply should NOT cause room-level unread")
	}

	// But a new ROOT message should still cause unread
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Another root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second root message: %v", err)
	}
	waitForNotificationMaterializer(t, core)

	hasUnread, err = core.HasUnread(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after second root message: %v", err)
	}
	if !hasUnread {
		t.Error("New root message should cause room-level unread")
	}
}
