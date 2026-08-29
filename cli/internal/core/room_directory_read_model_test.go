package core

import (
	"errors"
	"strings"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	if dirGroup.ViewerState.CanManageRoomGroup {
		t.Fatal("CanManageRoomGroup = true before grant")
	}
	if err := core.GrantUserGroupPermission(ctx, SystemActorID, group.Id, actor.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission room.manage: %v", err)
	}
	dirGroup, err = reads.GetRoomGroup(ctx, actor.Id, group.Id, RoomDirectoryGroupOptions{})
	if err != nil {
		t.Fatalf("GetRoomGroup after room.manage grant: %v", err)
	}
	if !dirGroup.ViewerState.CanManageRoomGroup {
		t.Fatal("CanManageRoomGroup = false after grant")
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

func TestRoomDirectoryReadModelSortsMemberDMsByActivityDespiteMessageReadDenial(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := chattoCore.CreateUser(ctx, SystemActorID, "directory-sort-actor", "Directory Sort Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	createDM := func(login string) (*evtv1.User, *evtv1.Room) {
		t.Helper()
		other, err := chattoCore.CreateUser(ctx, SystemActorID, login, login, "password")
		if err != nil {
			t.Fatalf("CreateUser %s: %v", login, err)
		}
		dm, _, err := chattoCore.FindOrCreateDM(ctx, actor.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("FindOrCreateDM %s: %v", login, err)
		}
		return other, dm
	}
	olderUser, olderDM := createDM("directory-sort-older")
	newerUser, newerDM := createDM("directory-sort-newer")
	deniedUser, deniedDM := createDM("directory-sort-denied")
	if _, err := chattoCore.PostMessage(ctx, KindDM, olderDM.Id, olderUser.Id, "older", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage older: %v", err)
	}
	if _, err := chattoCore.PostMessage(ctx, KindDM, newerDM.Id, newerUser.Id, "newer", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage newer: %v", err)
	}
	if err := chattoCore.DenyUserRoomPermission(ctx, SystemActorID, deniedDM.Id, actor.Id, PermMessageRead); err != nil {
		t.Fatalf("DenyUserRoomPermission: %v", err)
	}
	if _, err := chattoCore.PostMessage(ctx, KindDM, deniedDM.Id, deniedUser.Id, "denied permission but newest", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage denied DM: %v", err)
	}

	rooms, err := chattoCore.RoomDirectoryReads().ListRooms(ctx, actor.Id, RoomDirectoryListOptions{IncludeDMs: true, IncludeEmptyDMs: true})
	if err != nil {
		t.Fatalf("ListRooms exhaustive projection: %v", err)
	}
	if len(rooms) != 3 {
		t.Fatalf("exhaustive DM count = %d, want 3", len(rooms))
	}
	if rooms[0].Room.GetId() != deniedDM.Id || rooms[1].Room.GetId() != newerDM.Id || rooms[2].Room.GetId() != olderDM.Id {
		t.Fatalf("exhaustive DM order = [%s %s %s], want newest-first [%s %s %s]", rooms[0].Room.GetId(), rooms[1].Room.GetId(), rooms[2].Room.GetId(), deniedDM.Id, newerDM.Id, olderDM.Id)
	}
}

func TestRoomDirectoryReadModelKeepsActiveDMsWhenMessageReadIsDenied(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := chattoCore.CreateUser(ctx, SystemActorID, "directory-no-read-actor", "Directory No Read Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	otherA, err := chattoCore.CreateUser(ctx, SystemActorID, "directory-no-read-a", "Directory No Read A", "password")
	if err != nil {
		t.Fatalf("CreateUser other A: %v", err)
	}
	otherB, err := chattoCore.CreateUser(ctx, SystemActorID, "directory-no-read-b", "Directory No Read B", "password")
	if err != nil {
		t.Fatalf("CreateUser other B: %v", err)
	}
	dmA, _, err := chattoCore.FindOrCreateDM(ctx, actor.Id, []string{otherA.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM A: %v", err)
	}
	dmB, _, err := chattoCore.FindOrCreateDM(ctx, actor.Id, []string{otherB.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM B: %v", err)
	}
	for _, roomID := range []string{dmA.Id, dmB.Id} {
		if err := chattoCore.DenyUserRoomPermission(ctx, SystemActorID, roomID, actor.Id, PermMessageRead); err != nil {
			t.Fatalf("DenyUserRoomPermission %s: %v", roomID, err)
		}
	}
	if _, err := chattoCore.NotificationPolicy().SetRoomNotificationMode(
		ctx, actor.Id, dmA.Id, notificationTestSignalDirectMessage,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE,
	); err != nil {
		t.Fatalf("SetRoomNotificationMode: %v", err)
	}

	list := func(wantIDs ...string) {
		t.Helper()
		rooms, err := chattoCore.RoomDirectoryReads().ListRooms(ctx, actor.Id, RoomDirectoryListOptions{IncludeDMs: true})
		if err != nil {
			t.Fatalf("ListRooms: %v", err)
		}
		if len(rooms) != len(wantIDs) {
			t.Fatalf("active DM room count = %d, want %d", len(rooms), len(wantIDs))
		}
		for i, wantID := range wantIDs {
			if rooms[i].Room.GetId() != wantID {
				t.Fatalf("active DM room %d = %s, want %s", i, rooms[i].Room.GetId(), wantID)
			}
		}
	}
	list()
	if _, err := chattoCore.PostMessage(ctx, KindDM, dmA.Id, otherA.Id, "activity visible to DM member", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	waitForNotificationMaterializer(t, chattoCore)
	list(dmA.Id)
	room, err := chattoCore.RoomDirectoryReads().GetRoom(ctx, actor.Id, dmA.Id)
	if err != nil {
		t.Fatalf("GetRoom: %v", err)
	}
	if !room.ViewerState.CanReadMessages || !room.ViewerState.HasUnread {
		t.Fatalf("viewer state for readable DM %s = %+v", room.Room.GetId(), room.ViewerState)
	}
}
