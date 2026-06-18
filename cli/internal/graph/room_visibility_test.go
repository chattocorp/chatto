package graph

import (
	"testing"

	"hmans.de/chatto/internal/core"
)

// TestRoomResolver_ListableButNotJoinable verifies that a room with
// `room.list` allowed but `room.join` denied returns
// viewerCanListRoom=true / viewerCanJoinRoom=false through the GraphQL
// resolver. This is the state a future request-to-join flow keys off:
// the directory should surface the room and disable the Join button.
func TestRoomResolver_ListableButNotJoinable(t *testing.T) {
	env := setupTestResolver(t)
	roomResolver := env.resolver.Room()

	// A non-owner viewer. `room.list` defaults to allow at server scope
	// for everyone, and `room.join` likewise — we explicitly deny
	// `room.join` on the room itself for the everyone role to create
	// the listable-but-not-joinable state.
	viewer := env.createVerifiedUser(t, "list-only-viewer", "Viewer", "password123")

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleEveryone, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyRoomPermission(room.join, core.SystemActorID, everyone): %v", err)
	}

	ctx := env.authContextForUser(viewer)

	canList, err := roomResolver.ViewerCanListRoom(ctx, env.testRoom)
	if err != nil {
		t.Fatalf("ViewerCanListRoom: %v", err)
	}
	if !canList {
		t.Error("viewer with default room.list should be able to list a restricted room")
	}

	canJoin, err := roomResolver.ViewerCanJoinRoom(ctx, env.testRoom)
	if err != nil {
		t.Fatalf("ViewerCanJoinRoom: %v", err)
	}
	if canJoin {
		t.Error("viewer should NOT be able to join a room with room.join denied at room scope")
	}

	isMember, err := roomResolver.ViewerIsMember(ctx, env.testRoom)
	if err != nil {
		t.Fatalf("ViewerIsMember: %v", err)
	}
	if isMember {
		t.Error("non-member viewer should not be reported as a room member")
	}
}

// TestRoomResolver_HiddenWhenListDenied verifies the opposite end of the
// spectrum: a room with `room.list` denied at room scope is invisible to
// the viewer (viewerCanListRoom=false), even though they're not a member
// and the directory query would normally include it.
func TestRoomResolver_HiddenWhenListDenied(t *testing.T) {
	env := setupTestResolver(t)
	roomResolver := env.resolver.Room()

	viewer := env.createVerifiedUser(t, "hidden-viewer", "Viewer", "password123")

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission(room.list, core.SystemActorID, everyone): %v", err)
	}

	ctx := env.authContextForUser(viewer)
	canList, err := roomResolver.ViewerCanListRoom(ctx, env.testRoom)
	if err != nil {
		t.Fatalf("ViewerCanListRoom: %v", err)
	}
	if canList {
		t.Error("viewer should NOT be able to list a room with room.list denied at room scope")
	}
}

// TestRoomResolver_ListableForMemberEvenWhenListDenied verifies the
// member-aware short-circuit in CanSeeRoom: an explicit room member sees
// the room regardless of how `room.list` resolves, so they don't lose
// access to a room they're already in just because an operator added a
// deny-list afterward.
func TestRoomResolver_ListableForMemberEvenWhenListDenied(t *testing.T) {
	env := setupTestResolver(t)
	roomResolver := env.resolver.Room()

	viewer := env.createVerifiedUser(t, "member-viewer", "Viewer", "password123")
	if _, err := env.core.JoinRoom(env.ctx, viewer.Id, core.KindChannel, viewer.Id, env.testRoom.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	if err := env.core.DenyRoomPermission(env.ctx, core.SystemActorID, env.testRoom.Id, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission(room.list, core.SystemActorID, everyone): %v", err)
	}

	ctx := env.authContextForUser(viewer)
	canList, err := roomResolver.ViewerCanListRoom(ctx, env.testRoom)
	if err != nil {
		t.Fatalf("ViewerCanListRoom: %v", err)
	}
	if !canList {
		t.Error("explicit room member should still see the room after a room.list deny")
	}

	isMember, err := roomResolver.ViewerIsMember(ctx, env.testRoom)
	if err != nil {
		t.Fatalf("ViewerIsMember: %v", err)
	}
	if !isMember {
		t.Error("explicit room member should be reported as a room member")
	}
}
