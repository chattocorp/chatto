package graph

import (
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// TestUserRooms_ExplicitMembershipSurvivesRoomJoinDeny is the repro for the
// reported bug: "I have rooms I'm already in; if I deny room.join the entire
// list disappears." An explicit membership record always counts — once a
// user has joined a room, the persistent record stays valid until they
// leave (or are removed). This must hold both for plain channel rooms AND
// for rooms that are later flipped to global, since per-room global-flag
// changes shouldn't invalidate prior memberships.
func TestUserRooms_ExplicitMembershipSurvivesRoomJoinDeny(t *testing.T) {
	env := setupTestResolver(t)

	// Member joins a non-global room (explicit membership record).
	member := env.createVerifiedUser(t, "explicit-room-member", "Member", "password123")
	room, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "non-global-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if room.IsGlobal {
		t.Fatalf("setup expected a non-global room, got isGlobal=true")
	}
	if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Sanity: the room appears before any deny.
	channel := model.RoomTypeChannel
	roomsBefore, err := env.resolver.User().Rooms(env.authContextForUser(member), member, &channel)
	if err != nil {
		t.Fatalf("User.Rooms (before deny): %v", err)
	}
	var sawBefore bool
	for _, r := range roomsBefore {
		if r.Id == room.Id {
			sawBefore = true
			break
		}
	}
	if !sawBefore {
		t.Fatal("baseline: expected joined room to appear in viewer.user.rooms")
	}

	// Deny room.join on the user. This is the action that reportedly
	// empties the sidebar.
	if err := env.core.DenyUserPermission(env.ctx, member.Id, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyUserPermission: %v", err)
	}

	// The room should still appear — explicit membership survives.
	roomsAfter, err := env.resolver.User().Rooms(env.authContextForUser(member), member, &channel)
	if err != nil {
		t.Fatalf("User.Rooms (after deny): %v", err)
	}
	var sawAfter bool
	for _, r := range roomsAfter {
		if r.Id == room.Id {
			sawAfter = true
			break
		}
	}
	if !sawAfter {
		t.Errorf("expected joined room to still appear in viewer.user.rooms after room.join deny; got %d rooms", len(roomsAfter))
	}

	// The same room is then flipped to global. The user still has the
	// explicit membership record. With the persistent record honored, they
	// must STILL see the room despite their existing room.join deny — the
	// global flag changes the implicit-membership rules, it doesn't strip
	// existing explicit memberships.
	if _, err := env.core.SetRoomGlobal(env.ctx, env.testUser.Id, core.KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomGlobal: %v", err)
	}
	roomsAfterGlobal, err := env.resolver.User().Rooms(env.authContextForUser(member), member, &channel)
	if err != nil {
		t.Fatalf("User.Rooms (after global flip): %v", err)
	}
	var sawAfterGlobal bool
	for _, r := range roomsAfterGlobal {
		if r.Id == room.Id {
			sawAfterGlobal = true
			break
		}
	}
	if !sawAfterGlobal {
		t.Errorf("expected room with explicit membership to remain visible after being flipped to global; got %d rooms", len(roomsAfterGlobal))
	}
}
