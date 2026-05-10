package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// ptr returns a pointer to the given string value
func ptr(s string) *string {
	return &s
}

// ============================================================================
// CreateRoom Authorization Tests
// ============================================================================

func TestCreateRoom_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	input := model.CreateRoomInput{
		Name:    "new-room",
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.CreateRoom(env.unauthContext(), input)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected with permission denied", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-create", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.CreateRoom(env.authContextForUser(outsider), input)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("space admin can create room", func(t *testing.T) {
		// testUser is the space creator (admin)
		room, err := mutation.CreateRoom(env.authContext(), model.CreateRoomInput{
			Name:    "admin-created-room",
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if room == nil {
			t.Fatal("expected room, got nil")
		}
		if room.Name != "admin-created-room" {
			t.Errorf("expected name 'admin-created-room', got %s", room.Name)
		}
	})

	t.Run("regular member cannot create room by default", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-create", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}

		// room.create is not granted to everyone role by default
		_, err = mutation.CreateRoom(env.authContextForUser(member), model.CreateRoomInput{
			Name:    "member-created-room",
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("member with room.create permission can create room", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-create-granted", "Member Granted", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}

		// Grant room.create to the everyone role
		err = env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to grant permission: %v", err)
		}

		room, err := mutation.CreateRoom(env.authContextForUser(member), model.CreateRoomInput{
			Name:    "member-created-room-granted",
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if room == nil {
			t.Fatal("expected room, got nil")
		}
	})
}

// ============================================================================
// PostMessage Authorization Tests
// ============================================================================

func TestPostMessage_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	input := model.PostMessageInput{
		RoomID:  env.testRoom.Id,
		Body:    ptr("Hello, world!"),
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.PostMessage(env.unauthContext(), input)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-space-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-post", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.PostMessage(env.authContextForUser(outsider), input)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-post", "Space Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, spaceMember.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}
		// Note: not joining the room

		_, err = mutation.PostMessage(env.authContextForUser(spaceMember), input)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can post message", func(t *testing.T) {
		// testUser is a room member
		event, err := mutation.PostMessage(env.authContext(), input)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})
}

// ============================================================================
// PostMessage Thread Permission Tests
// ============================================================================

func TestPostMessage_ThreadPermissions(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a regular member (not admin/owner — affected by "everyone" role denials)
	member, err := env.core.CreateUser(env.ctx, "system", "thread-member", "Thread Member", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
	if err != nil {
		t.Fatalf("failed to join space: %v", err)
	}
	_, err = env.core.JoinRoom(env.ctx, member.Id, env.testSpace.Id, member.Id, env.testRoom.Id)
	if err != nil {
		t.Fatalf("failed to join room: %v", err)
	}

	t.Run("member can post first thread reply with default permissions", func(t *testing.T) {
		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for thread test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:   env.testRoom.Id,
			Body:     ptr("Starting a thread"),
			InThread: ptr(root.Id),
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})

	t.Run("member with post-in-thread denied cannot post any thread reply", func(t *testing.T) {
		if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessagePostInThread); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessagePostInThread)

		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for deny test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		// First reply (no existing replies) — still requires post-in-thread
		_, err = mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:   env.testRoom.Id,
			Body:     ptr("Trying to start thread"),
			InThread: ptr(root.Id),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied for first reply, got %v", err)
		}

		// Subsequent reply (existing replies) — also denied
		_, err = env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "First reply by owner", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("failed to create thread: %v", err)
		}

		_, err = mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:   env.testRoom.Id,
			Body:     ptr("Trying to post in existing thread"),
			InThread: ptr(root.Id),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied for subsequent reply, got %v", err)
		}
	})

	t.Run("denying message.post does not affect thread replies", func(t *testing.T) {
		if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessagePost)

		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for independence test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:   env.testRoom.Id,
			Body:     ptr("Thread reply still works"),
			InThread: ptr(root.Id),
		})
		if err != nil {
			t.Fatalf("expected success for thread reply, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})
}

func TestPostMessage_ReplyPermissions(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a regular member
	member, err := env.core.CreateUser(env.ctx, "system", "reply-member", "Reply Member", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
	if err != nil {
		t.Fatalf("failed to join space: %v", err)
	}
	_, err = env.core.JoinRoom(env.ctx, member.Id, env.testSpace.Id, member.Id, env.testRoom.Id)
	if err != nil {
		t.Fatalf("failed to join room: %v", err)
	}

	t.Run("member can reply in room with default permissions", func(t *testing.T) {
		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for reply test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:    env.testRoom.Id,
			Body:      ptr("Replying in room"),
			InReplyTo: ptr(root.Id),
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})

	t.Run("member without message.reply denied cannot use inReplyTo in room", func(t *testing.T) {
		if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReply); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReply)

		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for reply deny test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		_, err = mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:    env.testRoom.Id,
			Body:      ptr("Trying to reply"),
			InReplyTo: ptr(root.Id),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("member without message.reply can still post without inReplyTo", func(t *testing.T) {
		if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReply); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReply)

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:  env.testRoom.Id,
			Body:    ptr("Root message without reply"),
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})

	t.Run("member can reply in thread with default permissions", func(t *testing.T) {
		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for thread reply test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}
		firstReply, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "First reply", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("failed to create thread: %v", err)
		}

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:    env.testRoom.Id,
			Body:      ptr("Replying in thread"),
			InThread:  ptr(root.Id),
			InReplyTo: ptr(firstReply.Id),
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})

	t.Run("member without message.reply-in-thread denied cannot use inReplyTo in thread", func(t *testing.T) {
		if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReplyInThread); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReplyInThread)

		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for thread reply deny test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}
		firstReply, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "First reply", nil, root.Id, "", nil, false)
		if err != nil {
			t.Fatalf("failed to create thread: %v", err)
		}

		_, err = mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:    env.testRoom.Id,
			Body:      ptr("Trying to reply in thread"),
			InThread:  ptr(root.Id),
			InReplyTo: ptr(firstReply.Id),
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("member without message.reply-in-thread can still post in thread without inReplyTo", func(t *testing.T) {
		if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReplyInThread); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		defer env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermMessageReplyInThread)

		root, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Root for thread no-reply test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post root: %v", err)
		}

		event, err := mutation.PostMessage(env.authContextForUser(member), model.PostMessageInput{
			RoomID:   env.testRoom.Id,
			Body:     ptr("Thread reply without attribution"),
			InThread: ptr(root.Id),
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})
}

// ============================================================================
// UpdateSpace Authorization Tests
// ============================================================================

func TestUpdateInstance_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	newName := "Updated Instance Name"

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.UpdateInstance(env.unauthContext(), model.UpdateInstanceInput{Name: newName})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected with permission denied", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-update", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.UpdateInstance(env.authContextForUser(outsider), model.UpdateInstanceInput{Name: newName})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("regular member is rejected with permission denied", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-update", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}

		_, err = mutation.UpdateInstance(env.authContextForUser(member), model.UpdateInstanceInput{Name: newName})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("admin can update instance", func(t *testing.T) {
		// testUser is the space creator (admin)
		instance, err := mutation.UpdateInstance(env.authContext(), model.UpdateInstanceInput{Name: newName})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if instance == nil {
			t.Fatal("expected instance, got nil")
		}
		// Verify the underlying space was renamed
		space, err := env.core.GetSpace(env.ctx, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to fetch space: %v", err)
		}
		if space.Name != newName {
			t.Errorf("expected space name %q, got %q", newName, space.Name)
		}
	})
}

// ============================================================================
// JoinRoom Authorization Tests
// ============================================================================

func TestJoinRoom_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a new room for join tests
	newRoom, err := env.core.CreateRoom(env.ctx, env.testUser.Id, env.testSpace.Id, "join-test-room", "Room for join tests")
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.JoinRoom(env.unauthContext(), model.JoinRoomInput{RoomID: newRoom.Id})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-space-member is rejected with permission denied", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-join", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.JoinRoom(env.authContextForUser(outsider), model.JoinRoomInput{RoomID: newRoom.Id})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("space member can join room (default member permission)", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-join", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}

		success, err := mutation.JoinRoom(env.authContextForUser(member), model.JoinRoomInput{RoomID: newRoom.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}

		// Verify membership
		exists, err := env.core.RoomMembershipExists(env.ctx, env.testSpace.Id, member.Id, newRoom.Id)
		if err != nil {
			t.Fatalf("failed to check membership: %v", err)
		}
		if !exists {
			t.Error("expected user to be room member")
		}
	})
}

// ============================================================================
// LeaveRoom Authorization Tests
// ============================================================================

func TestLeaveRoom_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.LeaveRoom(env.unauthContext(), model.LeaveRoomInput{RoomID: env.testRoom.Id})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("room member can leave room", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "room-leaver", "Room Leaver", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member.Id, env.testSpace.Id, member.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		success, err := mutation.LeaveRoom(env.authContextForUser(member), model.LeaveRoomInput{RoomID: env.testRoom.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}

		// Verify no longer a member
		exists, err := env.core.RoomMembershipExists(env.ctx, env.testSpace.Id, member.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to check membership: %v", err)
		}
		if exists {
			t.Error("expected user to not be room member")
		}
	})
}

// ============================================================================
// AddReaction Authorization Tests
// ============================================================================

func TestAddReaction_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message to react to
	event, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Test message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	messageEventID := event.Id

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.AddReaction(env.unauthContext(), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-react", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.AddReaction(env.authContextForUser(outsider), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-react", "Space Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, spaceMember.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}

		_, err = mutation.AddReaction(env.authContextForUser(spaceMember), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can add reaction", func(t *testing.T) {
		success, err := mutation.AddReaction(env.authContext(), model.AddReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})
}

// ============================================================================
// RemoveReaction Authorization Tests
// ============================================================================

func TestRemoveReaction_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message and add a reaction to remove
	event, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Test message for removal", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	messageEventID := event.Id

	// Add a reaction first
	_, err = env.core.AddReaction(env.ctx, env.testSpace.Id, env.testRoom.Id, messageEventID, "thumbsup", env.testUser.Id)
	if err != nil {
		t.Fatalf("failed to add reaction: %v", err)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.RemoveReaction(env.unauthContext(), model.RemoveReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-unreact", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.RemoveReaction(env.authContextForUser(outsider), model.RemoveReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can remove reaction", func(t *testing.T) {
		success, err := mutation.RemoveReaction(env.authContext(), model.RemoveReactionInput{RoomID: env.testRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})
}

// ============================================================================
// MarkRoomAsRead Authorization Tests
// ============================================================================

func TestMarkRoomAsRead_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.MarkRoomAsRead(env.unauthContext(), model.MarkRoomAsReadInput{RoomID: env.testRoom.Id})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-read", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.MarkRoomAsRead(env.authContextForUser(outsider), model.MarkRoomAsReadInput{RoomID: env.testRoom.Id})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can mark room as read", func(t *testing.T) {
		result, err := mutation.MarkRoomAsRead(env.authContext(), model.MarkRoomAsReadInput{RoomID: env.testRoom.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Error("expected result, got nil")
		}
	})
}

// ============================================================================
// MarkThreadAsOpened Authorization Tests
// ============================================================================

func TestMarkThreadAsOpened_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message to use as thread root
	event, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Thread root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	threadRootEventId := event.Id

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.MarkThreadAsOpened(env.unauthContext(), model.MarkThreadAsOpenedInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-thread", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.MarkThreadAsOpened(env.authContextForUser(outsider), model.MarkThreadAsOpenedInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-thread", "Space Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, spaceMember.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}
		// Note: not joining the room

		_, err = mutation.MarkThreadAsOpened(env.authContextForUser(spaceMember), model.MarkThreadAsOpenedInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("room member can mark thread as opened", func(t *testing.T) {
		// testUser is a room member
		result, err := mutation.MarkThreadAsOpened(env.authContext(), model.MarkThreadAsOpenedInput{RoomID: env.testRoom.Id, ThreadRootEventID: threadRootEventId})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Error("expected result, got nil")
		}
	})

	t.Run("first open returns nil previous time", func(t *testing.T) {
		// Create a new message for this test
		newEvent, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Another root message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		result, err := mutation.MarkThreadAsOpened(env.authContext(), model.MarkThreadAsOpenedInput{RoomID: env.testRoom.Id, ThreadRootEventID: newEvent.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result.PreviousOpenedAt != nil {
			t.Errorf("expected nil previous time on first open, got %v", result.PreviousOpenedAt)
		}
	})

	t.Run("second open returns previous time", func(t *testing.T) {
		// Create a new message for this test
		newEvent, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Yet another root message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		// First open
		_, err = mutation.MarkThreadAsOpened(env.authContext(), model.MarkThreadAsOpenedInput{RoomID: env.testRoom.Id, ThreadRootEventID: newEvent.Id})
		if err != nil {
			t.Fatalf("first open failed: %v", err)
		}

		// Second open
		result, err := mutation.MarkThreadAsOpened(env.authContext(), model.MarkThreadAsOpenedInput{RoomID: env.testRoom.Id, ThreadRootEventID: newEvent.Id})
		if err != nil {
			t.Fatalf("second open failed: %v", err)
		}
		if result.PreviousOpenedAt == nil {
			t.Error("expected non-nil previous time on second open")
		}
	})
}

// ============================================================================
// DeleteInstanceLogo Authorization Tests
// ============================================================================

func TestDeleteInstanceLogo_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.DeleteInstanceLogo(env.unauthContext())
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected with permission denied", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-logo", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.DeleteInstanceLogo(env.authContextForUser(outsider))
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("regular member is rejected with permission denied", func(t *testing.T) {
		member, err := env.core.CreateUser(env.ctx, "system", "member-logo", "Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}

		_, err = mutation.DeleteInstanceLogo(env.authContextForUser(member))
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("admin can delete logo (even if none exists)", func(t *testing.T) {
		// testUser is the instance admin. Should succeed even if no logo exists - it's a no-op.
		instance, err := mutation.DeleteInstanceLogo(env.authContext())
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if instance == nil {
			t.Fatal("expected instance, got nil")
		}
	})
}

// ============================================================================
// DeleteMessage Authorization Tests
// ============================================================================

func TestDeleteMessage_Authorization(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a message to delete
	event, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Message to delete", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post message: %v", err)
	}
	eventID := event.Id

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.DeleteMessage(env.unauthContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: eventID})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("non-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-delete", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		_, err = mutation.DeleteMessage(env.authContextForUser(outsider), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: eventID})
		if !errors.Is(err, core.ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("member cannot delete another user's message", func(t *testing.T) {
		// Create another member
		otherMember, err := env.core.CreateUser(env.ctx, "system", "othermember-delete", "Other Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, otherMember.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, otherMember.Id, env.testSpace.Id, otherMember.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Try to delete testUser's message as otherMember
		_, err = mutation.DeleteMessage(env.authContextForUser(otherMember), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: eventID})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("author can delete own message", func(t *testing.T) {
		// Post a new message as testUser
		newEvent, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Another message to delete", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		// Delete own message
		success, err := mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: newEvent.Id})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}

		// Verify message body is deleted
		messageBodyKey := newEvent.GetMessagePosted().MessageBodyId
		body, err := env.core.GetMessageBody(env.ctx, env.testSpace.Id, messageBodyKey)
		if err != nil {
			t.Fatalf("failed to get message body: %v", err)
		}
		if body != "" {
			t.Error("expected empty body after deletion")
		}
	})

	// NOTE: Moderator delete (admin deleting any message) is no longer supported.
	// Only the author can delete their own messages.

	t.Run("deleting already deleted message succeeds (idempotent)", func(t *testing.T) {
		// Post and delete a message
		newEvent, err := env.core.PostMessage(env.ctx, env.testSpace.Id, env.testRoom.Id, env.testUser.Id, "Message for idempotent test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("failed to post message: %v", err)
		}

		// Delete once
		_, err = mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: newEvent.Id})
		if err != nil {
			t.Fatalf("first delete failed: %v", err)
		}

		// Delete again - should succeed
		success, err := mutation.DeleteMessage(env.authContext(), model.DeleteMessageInput{RoomID: env.testRoom.Id, EventID: newEvent.Id})
		if err != nil {
			t.Fatalf("second delete failed: %v", err)
		}
		if !success {
			t.Error("expected success=true for idempotent delete")
		}
	})
}

// ============================================================================
// DM Reactions Tests
// ============================================================================

func TestAddReaction_DM(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a second user for DM
	user2, err := env.core.CreateUser(env.ctx, "system", "dm-react2", "DM React 2", "password123")
	if err != nil {
		t.Fatalf("failed to create user2: %v", err)
	}

	// Create a DM conversation
	dmRoom, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{user2.Id})
	if err != nil {
		t.Fatalf("failed to create DM: %v", err)
	}

	// Post a message in the DM
	event, err := env.core.PostMessage(env.ctx, core.DMSpaceID, dmRoom.Id, env.testUser.Id, "DM message for reaction test", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("failed to post DM message: %v", err)
	}
	messageEventID := event.Id

	t.Run("DM participant can add reaction", func(t *testing.T) {
		success, err := mutation.AddReaction(env.authContext(), model.AddReactionInput{RoomID: dmRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})

	t.Run("other DM participant can add reaction", func(t *testing.T) {
		success, err := mutation.AddReaction(env.authContextForUser(user2), model.AddReactionInput{RoomID: dmRoom.Id, MessageEventID: messageEventID, Emoji: "heart"})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})

	t.Run("non-participant cannot add reaction to DM", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "dm-outsider", "Outsider", "password123")
		if err != nil {
			t.Fatalf("failed to create outsider: %v", err)
		}

		_, err = mutation.AddReaction(env.authContextForUser(outsider), model.AddReactionInput{RoomID: dmRoom.Id, MessageEventID: messageEventID, Emoji: "thumbsup"})
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("expected ErrNotRoomMember, got %v", err)
		}
	})
}

// ============================================================================
// PostMessage Echo Permission Tests
// ============================================================================

func TestPostMessage_EchoPermission(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Post a root message to reply to
	rootInput := model.PostMessageInput{
		RoomID:  env.testRoom.Id,
		Body:    ptr("Thread root for echo test"),
	}
	rootEvent, err := mutation.PostMessage(env.authContext(), rootInput)
	if err != nil {
		t.Fatalf("failed to post root message: %v", err)
	}

	t.Run("user with echo permission can post with alsoSendToChannel", func(t *testing.T) {
		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Reply echoed to channel"),
			InThread:          &rootEvent.Id,
			AlsoSendToChannel: &alsoSend,
		}

		event, err := mutation.PostMessage(env.authContext(), input)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})

	t.Run("alsoSendToChannel without inThread is rejected", func(t *testing.T) {
		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Not a reply but trying to echo"),
			AlsoSendToChannel: &alsoSend,
		}

		_, err := mutation.PostMessage(env.authContext(), input)
		if err == nil {
			t.Error("expected error when alsoSendToChannel is true without inThread")
		}
	})

	t.Run("user without echo permission is denied", func(t *testing.T) {
		// Create a user and deny echo permission on the everyone role
		member, err := env.core.CreateUser(env.ctx, "system", "no-echo-user", "No Echo", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member.Id, env.testSpace.Id, member.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Deny echo at room level for everyone role (testUser is space owner, has roles.manage)
		err = env.core.DenyRoomRolePermission(env.ctx, env.testUser.Id, env.testSpace.Id, env.testRoom.Id, core.RoleEveryone, core.PermMessageEcho)
		if err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}

		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Reply trying to echo"),
			InThread:          &rootEvent.Id,
			AlsoSendToChannel: &alsoSend,
		}

		_, err = mutation.PostMessage(env.authContextForUser(member), input)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("user without message.post permission is denied echo", func(t *testing.T) {
		// Create a user and deny message.post on the everyone role (announcements pattern)
		member3, err := env.core.CreateUser(env.ctx, "system", "no-post-echo-user", "No Post Echo", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member3.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member3.Id, env.testSpace.Id, member3.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Deny message.post at room level for everyone role
		err = env.core.DenyRoomRolePermission(env.ctx, env.testUser.Id, env.testSpace.Id, env.testRoom.Id, core.RoleEveryone, core.PermMessagePost)
		if err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}

		alsoSend := true
		input := model.PostMessageInput{
			RoomID:            env.testRoom.Id,
			Body:              ptr("Reply trying to echo without post permission"),
			InThread:          &rootEvent.Id,
			AlsoSendToChannel: &alsoSend,
		}

		_, err = mutation.PostMessage(env.authContextForUser(member3), input)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("user without echo permission can still post normal thread reply", func(t *testing.T) {
		// Create a fresh user — the previous test denied echo on everyone role for the room
		member2, err := env.core.CreateUser(env.ctx, "system", "normal-reply-user", "Normal Reply", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, member2.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, member2.Id, env.testSpace.Id, member2.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("failed to join room: %v", err)
		}

		// Post normal thread reply without echo — should succeed even though echo is denied
		input := model.PostMessageInput{
			RoomID:   env.testRoom.Id,
			Body:     ptr("Normal thread reply"),
			InThread: &rootEvent.Id,
		}

		event, err := mutation.PostMessage(env.authContextForUser(member2), input)
		if err != nil {
			t.Fatalf("expected success for normal reply, got error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
	})
}
