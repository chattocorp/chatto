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
	dirGroup, err := reads.GetRoomGroup(ctx, actor.Id, group.Id, RoomDirectoryGroupOptions{})
	if err != nil {
		t.Fatalf("GetRoomGroup: %v", err)
	}
	if !directoryRoomsContain(dirGroup.Rooms, visible.Id) {
		t.Fatalf("visible room %s missing from GetRoomGroup", visible.Id)
	}
	if directoryRoomsContain(dirGroup.Rooms, hidden.Id) {
		t.Fatalf("hidden room %s appeared in GetRoomGroup", hidden.Id)
	}
	if _, err := reads.GetRoomGroup(ctx, actor.Id, "missing-group", RoomDirectoryGroupOptions{}); !errors.Is(err, ErrRoomGroupNotFound) {
		t.Fatalf("GetRoomGroup missing error = %v, want ErrRoomGroupNotFound", err)
	}
	batchGroups, err := reads.BatchGetRoomGroups(ctx, actor.Id, []string{group.Id, "missing-group", group.Id}, RoomDirectoryGroupOptions{})
	if err != nil {
		t.Fatalf("BatchGetRoomGroups: %v", err)
	}
	if len(batchGroups) != 1 || batchGroups[0].Group.GetId() != group.Id {
		t.Fatalf("BatchGetRoomGroups = %+v, want single group %s", batchGroups, group.Id)
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

func TestRoomDirectoryReadModelCanIncludeEmptyDMsForExhaustiveProjection(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := chattoCore.CreateUser(ctx, SystemActorID, "directory-empty-dm-actor", "Directory Empty DM Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	other, err := chattoCore.CreateUser(ctx, SystemActorID, "directory-empty-dm-other", "Directory Empty DM Other", "password")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	dm, _, err := chattoCore.FindOrCreateDM(ctx, actor.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}

	publicRooms, err := chattoCore.RoomDirectoryReads().ListRooms(ctx, actor.Id, RoomDirectoryListOptions{IncludeDMs: true})
	if err != nil {
		t.Fatalf("ListRooms public policy: %v", err)
	}
	if directoryRoomsContain(publicRooms, dm.Id) {
		t.Fatalf("empty DM %s appeared without IncludeEmptyDMs", dm.Id)
	}

	projectedRooms, err := chattoCore.RoomDirectoryReads().ListRooms(ctx, actor.Id, RoomDirectoryListOptions{IncludeDMs: true, IncludeEmptyDMs: true})
	if err != nil {
		t.Fatalf("ListRooms exhaustive projection: %v", err)
	}
	if !directoryRoomsContain(projectedRooms, dm.Id) {
		t.Fatalf("empty DM %s missing with IncludeEmptyDMs", dm.Id)
	}
}
