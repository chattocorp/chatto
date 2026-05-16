package graph

import (
	"testing"

	"hmans.de/chatto/internal/core"
)

// TestRoomMembers_ExplicitMemberAfterJoinDeny is the regression test for
// the reported bug: a user who joined a room (explicit membership record)
// and then lost room.join was still seeing the room in their sidebar
// (`User.rooms` correctly honors the explicit record) but was NOT being
// listed in `Room.members`. The two views must stay consistent — anyone
// `RoomMembershipExists` treats as a member must show up here too.
//
// Covers both auto-join rooms (where the implicit/permission-derived path
// runs alongside the explicit list) and plain rooms (where only the
// explicit list applies).
func TestRoomMembers_ExplicitMemberAfterJoinDeny(t *testing.T) {
	for _, autoJoin := range []bool{false, true} {
		name := "regular-room"
		if autoJoin {
			name = "auto-join-room"
		}
		t.Run(name, func(t *testing.T) {
			env := setupTestResolver(t)

			member := env.createVerifiedUser(t, "explicit-"+name, "Member", "password123")

			room, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", name, "")
			if err != nil {
				t.Fatalf("CreateRoom: %v", err)
			}
			if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, room.Id); err != nil {
				t.Fatalf("JoinRoom: %v", err)
			}
			if autoJoin {
				if _, err := env.core.SetRoomAutoJoin(env.ctx, env.testUser.Id, core.KindChannel, room.Id, true); err != nil {
					t.Fatalf("SetRoomAutoJoin: %v", err)
				}
				// Reload so the resolver sees AutoJoin=true.
				room, err = env.core.GetRoom(env.ctx, core.KindChannel, room.Id)
				if err != nil {
					t.Fatalf("GetRoom: %v", err)
				}
			}

			// Deny room.join on the user — this is the action that
			// reportedly removed them from Room.members.
			if err := env.core.DenyUserPermission(env.ctx, member.Id, core.PermRoomJoin); err != nil {
				t.Fatalf("DenyUserPermission: %v", err)
			}

			// Sanity: explicit membership still exists per the source of truth.
			exists, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, member.Id, room.Id)
			if err != nil {
				t.Fatalf("RoomMembershipExists: %v", err)
			}
			if !exists {
				t.Fatal("baseline: RoomMembershipExists should still be true (explicit record wins)")
			}

			// Resolver-level check: the explicit member must show up.
			members, err := env.resolver.Room().Members(env.authContextForUser(member), room)
			if err != nil {
				t.Fatalf("Room.Members: %v", err)
			}
			var sawMember bool
			for _, u := range members {
				if u.Id == member.Id {
					sawMember = true
					break
				}
			}
			if !sawMember {
				t.Errorf("expected explicit member %q to remain in Room.members after room.join deny (autoJoin=%v); got %d members", member.Id, autoJoin, len(members))
			}
		})
	}
}
