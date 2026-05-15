package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// TestAuthorAlwaysCanEditOrDeleteOwnMessage pins down the post-revision
// invariant: authors editing or deleting their own messages do not need
// message.manage (or any other permission) — that's always allowed, subject
// only to the edit window for edits and room membership.
//
// We deny message.manage on every role we can reach (everyone at server
// scope and the room's group) to prove the author path is independent.
func TestAuthorAlwaysCanEditOrDeleteOwnMessage(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Deny message.manage on everyone at every tier so no role grant could
	// possibly allow author moderation. Authors must still pass.
	if err := env.core.DenyInstancePermission(env.ctx, core.RoleEveryone, core.PermMessageManage); err != nil {
		t.Fatalf("DenyInstancePermission: %v", err)
	}
	groupID := env.testRoom.GroupId
	if err := env.core.DenyGroupPermission(env.ctx, groupID, core.RoleEveryone, core.PermMessageManage); err != nil {
		t.Fatalf("DenyGroupPermission: %v", err)
	}

	// Create a non-admin member and have them join the room.
	member := env.createVerifiedUser(t, "author-invariant", "Author", "password123")
	if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, env.testRoom.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Sanity: confirm message.manage is denied for this member.
	can, err := env.core.CanManageOthersMessage(env.ctx, member.Id, core.KindChannel, env.testRoom.Id)
	if err != nil {
		t.Fatalf("CanManageOthersMessage: %v", err)
	}
	if can {
		t.Fatal("setup error: member should not have message.manage")
	}

	t.Run("author can delete own message without message.manage", func(t *testing.T) {
		event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, member.Id, "self-delete target", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		success, err := mutation.DeleteMessage(env.authContextForUser(member), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: event.Id,
		})
		if err != nil {
			t.Fatalf("DeleteMessage: %v", err)
		}
		if !success {
			t.Error("expected success=true for author self-delete")
		}
	})

	t.Run("author can edit own message within edit window without message.manage", func(t *testing.T) {
		event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, member.Id, "self-edit target", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		success, err := mutation.EditMessage(env.authContextForUser(member), model.EditMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: event.Id,
			Body:    "edited body",
		})
		if err != nil {
			t.Fatalf("EditMessage: %v", err)
		}
		if !success {
			t.Error("expected success=true for author self-edit")
		}
	})

	t.Run("non-author still cannot delete: message.manage is denied", func(t *testing.T) {
		// Post as `member`; another member tries to delete.
		event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, member.Id, "cross-delete target", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		other := env.createVerifiedUser(t, "author-invariant-other", "Other", "password123")
		if _, err := env.core.JoinRoom(env.ctx, other.Id, core.KindChannel, other.Id, env.testRoom.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
		_, err = mutation.DeleteMessage(env.authContextForUser(other), model.DeleteMessageInput{
			RoomID:  env.testRoom.Id,
			EventID: event.Id,
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied for non-author with message.manage denied, got %v", err)
		}
	})
}

// TestRoomManageOpensPerRoomPermissionEditor verifies the new
// requireRoomManageAuth gate: a user without role.manage but WITH room.manage
// on a specific room can edit that room's permissions, and can't reach
// another room.
func TestRoomManageOpensPerRoomPermissionEditor(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Two channel rooms for the "can't reach another room" assertion.
	targetRoom := env.testRoom
	otherRoom, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "other-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	member := env.createVerifiedUser(t, "rm-editor", "RM Editor", "password123")
	if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, targetRoom.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	input := func(roomID string) model.GrantRoomPermissionInput {
		return model.GrantRoomPermissionInput{
			RoomID:     roomID,
			Role:       core.RoleEveryone,
			Permission: string(core.PermMessageReact),
		}
	}

	t.Run("member without role.manage or room.manage is denied", func(t *testing.T) {
		_, err := mutation.GrantRoomPermission(env.authContextForUser(member), input(targetRoom.Id))
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied for unprivileged member, got %v", err)
		}
	})

	t.Run("member with room.manage on target room can edit that room's permissions", func(t *testing.T) {
		// Grant room.manage to this user directly on the target room.
		if err := env.core.GrantUserRoomPermission(env.ctx, targetRoom.Id, member.Id, core.PermRoomManage); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = env.core.ClearUserRoomPermissionState(env.ctx, targetRoom.Id, member.Id, core.PermRoomManage)
		})

		_, err := mutation.GrantRoomPermission(env.authContextForUser(member), input(targetRoom.Id))
		if err != nil {
			t.Errorf("expected room.manage on target room to allow editing its permissions, got %v", err)
		}
	})

	t.Run("same member denied on a different room", func(t *testing.T) {
		// Re-grant room.manage on targetRoom so the user has it somewhere
		// (and shouldn't be confused with "no room.manage anywhere").
		if err := env.core.GrantUserRoomPermission(env.ctx, targetRoom.Id, member.Id, core.PermRoomManage); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = env.core.ClearUserRoomPermissionState(env.ctx, targetRoom.Id, member.Id, core.PermRoomManage)
		})

		_, err := mutation.GrantRoomPermission(env.authContextForUser(member), input(otherRoom.Id))
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied on a different room, got %v", err)
		}
	})
}
