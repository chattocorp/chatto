package core

import (
	"errors"
	"strings"
	"testing"
)

func TestRoomDirectoryReadModelVisibilityAndJoinGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	reads := core.RoomDirectoryReads()

	actor, err := core.CreateUser(ctx, SystemActorID, "directory-read-actor", "Directory Read Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	group, err := core.CreateRoomGroup(ctx, SystemActorID, "Directory Read", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	visible, err := core.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "directory-read-visible", "")
	if err != nil {
		t.Fatalf("CreateRoom visible: %v", err)
	}
	hidden, err := core.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "directory-read-hidden", "")
	if err != nil {
		t.Fatalf("CreateRoom hidden: %v", err)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, hidden.Id, RoleEveryone, PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission room.list: %v", err)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, hidden.Id, RoleEveryone, PermRoomJoin); err != nil {
		t.Fatalf("DenyRoomPermission room.join: %v", err)
	}

	rooms, err := reads.ListRooms(ctx, actor.Id, RoomDirectoryListOptions{IncludeChannels: true})
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	if !directoryRoomsContain(rooms, visible.Id) {
		t.Fatalf("visible room %s missing from directory reads", visible.Id)
	}
	if directoryRoomsContain(rooms, hidden.Id) {
		t.Fatalf("hidden room %s appeared in directory reads", hidden.Id)
	}
	if _, err := reads.GetRoom(ctx, actor.Id, hidden.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetRoom hidden error = %v, want ErrPermissionDenied", err)
	}

	joined, err := reads.JoinGroup(ctx, actor.Id, group.Id)
	if err != nil {
		t.Fatalf("JoinGroup: %v", err)
	}
	if got, want := strings.Join(joined, ","), visible.Id; got != want {
		t.Fatalf("joined room ids = %q, want %q", got, want)
	}
	if isMember, err := core.RoomMembershipExists(ctx, KindChannel, actor.Id, visible.Id); err != nil || !isMember {
		t.Fatalf("visible membership = %v, %v; want true, nil", isMember, err)
	}
	if isMember, err := core.RoomMembershipExists(ctx, KindChannel, actor.Id, hidden.Id); err != nil || isMember {
		t.Fatalf("hidden membership = %v, %v; want false, nil", isMember, err)
	}
}

func directoryRoomsContain(rooms []*DirectoryRoom, roomID string) bool {
	for _, room := range rooms {
		if room != nil && room.Room != nil && room.Room.Id == roomID {
			return true
		}
	}
	return false
}
