package graph

import (
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// TestJoinGroup_JoinsAllJoinableRooms covers the "Join all" affordance:
// calling joinGroup creates explicit memberships for every room in the
// group that the caller can self-join AND isn't already in. Returned
// IDs reflect the rooms that transitioned from not-joined to joined.
func TestJoinGroup_JoinsAllJoinableRooms(t *testing.T) {
	env := setupTestResolver(t)
	mut := env.resolver.Mutation()

	member := env.createVerifiedUser(t, "joinall", "Member", "password123")

	// Add two extra rooms in the same seed group as env.testRoom.
	roomA, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "alpha", "")
	if err != nil {
		t.Fatalf("CreateRoom alpha: %v", err)
	}
	roomB, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "beta", "")
	if err != nil {
		t.Fatalf("CreateRoom beta: %v", err)
	}

	groupID := env.testRoom.GroupId
	if groupID == "" {
		t.Fatal("expected env.testRoom to have a GroupId")
	}

	joined, err := mut.JoinGroup(env.authContextForUser(member), model.JoinGroupInput{GroupID: groupID})
	if err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}

	// All three rooms should be reported as newly joined.
	want := map[string]bool{env.testRoom.Id: true, roomA.Id: true, roomB.Id: true}
	if len(joined) != len(want) {
		t.Errorf("joined count = %d, want %d (%v)", len(joined), len(want), joined)
	}
	for _, id := range joined {
		if !want[id] {
			t.Errorf("unexpected room %q in joined list", id)
		}
	}

	// And actually be members afterward.
	for r := range want {
		in, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, member.Id, r)
		if err != nil {
			t.Fatalf("RoomMembershipExists %s: %v", r, err)
		}
		if !in {
			t.Errorf("member is not actually in room %s after JoinGroup", r)
		}
	}
}

// TestJoinGroup_SkipsAlreadyJoinedAndNonJoinable verifies that
// already-joined rooms (no double-join) and rooms the caller can't
// self-join (room.join denied) are silently skipped — joinGroup is
// best-effort, the returned list reports only transitions.
func TestJoinGroup_SkipsAlreadyJoinedAndNonJoinable(t *testing.T) {
	env := setupTestResolver(t)
	mut := env.resolver.Mutation()

	member := env.createVerifiedUser(t, "skip-target", "Member", "password123")

	// Pre-join one room so it appears in "already in".
	preJoined, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "pre", "")
	if err != nil {
		t.Fatalf("CreateRoom pre: %v", err)
	}
	if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, preJoined.Id); err != nil {
		t.Fatalf("JoinRoom pre: %v", err)
	}

	// Create a room and deny room.join on the user at the room — they
	// shouldn't be auto-joined.
	restricted, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "restricted", "")
	if err != nil {
		t.Fatalf("CreateRoom restricted: %v", err)
	}
	if err := env.core.DenyUserRoomPermission(env.ctx, restricted.Id, member.Id, core.PermRoomJoin); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}

	// And one freely-joinable room to confirm the path still works.
	fresh, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "fresh", "")
	if err != nil {
		t.Fatalf("CreateRoom fresh: %v", err)
	}

	joined, err := mut.JoinGroup(env.authContextForUser(member), model.JoinGroupInput{GroupID: env.testRoom.GroupId})
	if err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}

	// Pre-joined room shouldn't appear (no transition).
	for _, id := range joined {
		if id == preJoined.Id {
			t.Errorf("already-joined room %q was reported as newly joined", id)
		}
		if id == restricted.Id {
			t.Errorf("non-joinable room %q was reported as newly joined", id)
		}
	}

	// Restricted room: no membership record after the call.
	in, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, member.Id, restricted.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists restricted: %v", err)
	}
	if in {
		t.Error("member was added to restricted room despite room.join deny")
	}

	// Fresh room: actually joined.
	in, err = env.core.RoomMembershipExists(env.ctx, core.KindChannel, member.Id, fresh.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists fresh: %v", err)
	}
	if !in {
		t.Error("expected member to be in the freely-joinable room")
	}
}

// TestJoinGroup_SkipsArchivedRooms verifies that archived rooms inside
// the group are silently skipped rather than aborting the loop. The
// previous behavior partial-succeeded and then returned "cannot join
// archived room", leaving some rooms joined and the caller with an
// error toast.
func TestJoinGroup_SkipsArchivedRooms(t *testing.T) {
	env := setupTestResolver(t)
	mut := env.resolver.Mutation()

	member := env.createVerifiedUser(t, "archive-skip", "Member", "password123")

	// A joinable room before the archived one.
	before, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "before", "")
	if err != nil {
		t.Fatalf("CreateRoom before: %v", err)
	}

	// An archived room — caller still has room.join, but JoinRoom would
	// error on archive state. JoinGroup must not propagate that error.
	archived, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "archived", "")
	if err != nil {
		t.Fatalf("CreateRoom archived: %v", err)
	}
	if _, err := env.core.ArchiveRoom(env.ctx, env.testUser.Id, core.KindChannel, archived.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}

	// A joinable room after the archived one — proves the loop continued
	// past the archived entry instead of aborting.
	after, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "after", "")
	if err != nil {
		t.Fatalf("CreateRoom after: %v", err)
	}

	joined, err := mut.JoinGroup(env.authContextForUser(member), model.JoinGroupInput{GroupID: env.testRoom.GroupId})
	if err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}

	gotJoined := map[string]bool{}
	for _, id := range joined {
		gotJoined[id] = true
	}
	if !gotJoined[before.Id] || !gotJoined[after.Id] {
		t.Errorf("expected both non-archived rooms in joined list, got %v", joined)
	}
	if gotJoined[archived.Id] {
		t.Errorf("archived room %q was reported as joined", archived.Id)
	}

	// Archived room: no membership record was created.
	in, err := env.core.RoomMembershipExists(env.ctx, core.KindChannel, member.Id, archived.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists archived: %v", err)
	}
	if in {
		t.Error("member was added to the archived room")
	}
}

// TestJoinGroup_RequiresAuth checks that the mutation rejects
// unauthenticated callers.
func TestJoinGroup_RequiresAuth(t *testing.T) {
	env := setupTestResolver(t)
	mut := env.resolver.Mutation()

	_, err := mut.JoinGroup(env.unauthContext(), model.JoinGroupInput{GroupID: env.testRoom.GroupId})
	if err == nil {
		t.Error("expected error for unauthenticated caller")
	}
}
