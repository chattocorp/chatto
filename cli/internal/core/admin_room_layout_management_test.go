package core

import (
	"errors"
	"slices"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestAdminRoomLayoutManagementAuthorization(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	actor, err := core.CreateUser(ctx, SystemActorID, "admin-layout-actor", "Admin Layout Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected seeded room group")
	}
	sourceGroupID := groups[0].Id

	if _, err := core.AdminCreateRoomGroup(ctx, actor.Id, "Managed", ""); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("AdminCreateRoomGroup without room.manage error = %v, want ErrPermissionDenied", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, actor.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserPermission room.manage: %v", err)
	}
	targetGroup, err := core.AdminCreateRoomGroup(ctx, actor.Id, "Managed", "")
	if err != nil {
		t.Fatalf("AdminCreateRoomGroup with room.manage: %v", err)
	}
	if err := core.ClearUserPermissionState(ctx, SystemActorID, actor.Id, PermRoomManage); err != nil {
		t.Fatalf("ClearUserPermissionState room.manage: %v", err)
	}

	if _, err := core.AdminCreateSidebarLink(ctx, actor.Id, sourceGroupID, "Docs", "/docs"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("AdminCreateSidebarLink without source room.manage error = %v, want ErrPermissionDenied", err)
	}
	if err := core.GrantGroupPermission(ctx, SystemActorID, sourceGroupID, RoleEveryone, PermRoomManage); err != nil {
		t.Fatalf("GrantGroupPermission source room.manage: %v", err)
	}
	link, err := core.AdminCreateSidebarLink(ctx, actor.Id, sourceGroupID, "Docs", "/docs")
	if err != nil {
		t.Fatalf("AdminCreateSidebarLink with source room.manage: %v", err)
	}

	if _, err := core.AdminMoveSidebarLinkToGroup(ctx, actor.Id, link.Id, targetGroup.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("AdminMoveSidebarLinkToGroup without target room.manage error = %v, want ErrPermissionDenied", err)
	}
	if err := core.GrantGroupPermission(ctx, SystemActorID, targetGroup.Id, RoleEveryone, PermRoomManage); err != nil {
		t.Fatalf("GrantGroupPermission target room.manage: %v", err)
	}
	movedLink, err := core.AdminMoveSidebarLinkToGroup(ctx, actor.Id, link.Id, targetGroup.Id)
	if err != nil {
		t.Fatalf("AdminMoveSidebarLinkToGroup with source and target room.manage: %v", err)
	}
	if movedLink.GetId() != link.Id {
		t.Fatalf("moved link id = %q, want %q", movedLink.GetId(), link.Id)
	}

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, sourceGroupID, "layout-managed-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	movedRoom, err := core.AdminMoveRoomToGroup(ctx, actor.Id, room.Id, targetGroup.Id)
	if err != nil {
		t.Fatalf("AdminMoveRoomToGroup with source and target room.manage: %v", err)
	}
	if movedRoom.GetGroupId() != targetGroup.Id {
		t.Fatalf("moved room group = %q, want %q", movedRoom.GetGroupId(), targetGroup.Id)
	}
}

func TestAdminRelativeRoomLayoutMoves(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	actor, err := core.CreateUser(ctx, SystemActorID, "relative-layout-actor", "Relative Layout Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, actor.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserPermission room.manage: %v", err)
	}
	first, err := core.AdminCreateRoomGroup(ctx, actor.Id, "Relative First", "")
	if err != nil {
		t.Fatalf("AdminCreateRoomGroup first: %v", err)
	}
	second, err := core.AdminCreateRoomGroup(ctx, actor.Id, "Relative Second", "")
	if err != nil {
		t.Fatalf("AdminCreateRoomGroup second: %v", err)
	}
	third, err := core.AdminCreateRoomGroup(ctx, actor.Id, "Relative Third", "")
	if err != nil {
		t.Fatalf("AdminCreateRoomGroup third: %v", err)
	}
	if err := core.AdminMoveRoomGroup(ctx, actor.Id, third.Id, first.Id); err != nil {
		t.Fatalf("AdminMoveRoomGroup: %v", err)
	}
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	groupIDs := make([]string, 0, len(groups))
	for _, group := range groups {
		groupIDs = append(groupIDs, group.Id)
	}
	if slices.Index(groupIDs, third.Id)+1 != slices.Index(groupIDs, first.Id) {
		t.Fatalf("group order = %v, want %s immediately before %s", groupIDs, third.Id, first.Id)
	}

	if err := core.ClearUserPermissionState(ctx, SystemActorID, actor.Id, PermRoomManage); err != nil {
		t.Fatalf("ClearUserPermissionState room.manage: %v", err)
	}
	for _, groupID := range []string{first.Id, second.Id} {
		if err := core.GrantGroupPermission(ctx, SystemActorID, groupID, RoleEveryone, PermRoomManage); err != nil {
			t.Fatalf("GrantGroupPermission(%s): %v", groupID, err)
		}
	}

	movingRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, first.Id, "relative-moving-room", "")
	if err != nil {
		t.Fatalf("CreateRoom moving: %v", err)
	}
	firstTargetRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, second.Id, "relative-target-first", "")
	if err != nil {
		t.Fatalf("CreateRoom first target: %v", err)
	}
	secondTargetRoom, err := core.CreateRoom(ctx, SystemActorID, KindChannel, second.Id, "relative-target-second", "")
	if err != nil {
		t.Fatalf("CreateRoom second target: %v", err)
	}
	link, err := core.AdminCreateSidebarLink(ctx, actor.Id, first.Id, "Relative docs", "/relative-docs")
	if err != nil {
		t.Fatalf("AdminCreateSidebarLink: %v", err)
	}

	roomRef := &evtv1.SidebarGroupEntry{Kind: evtv1.SidebarGroupEntry_ROOM, Id: movingRoom.Id}
	beforeSecond := &evtv1.SidebarGroupEntry{Kind: evtv1.SidebarGroupEntry_ROOM, Id: secondTargetRoom.Id}
	if _, err := core.AdminMoveSidebarItem(ctx, actor.Id, roomRef, second.Id, beforeSecond); err != nil {
		t.Fatalf("AdminMoveSidebarItem room across groups: %v", err)
	}
	target, err := core.GetRoomGroup(ctx, second.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup target: %v", err)
	}
	wantTargetOrder := []string{
		"room:" + firstTargetRoom.Id,
		"room:" + movingRoom.Id,
		"room:" + secondTargetRoom.Id,
	}
	if got := sidebarEntryKeys(target.GetEntries()); !slices.Equal(got, wantTargetOrder) {
		t.Fatalf("target order = %v, want %v", got, wantTargetOrder)
	}

	linkRef := &evtv1.SidebarGroupEntry{Kind: evtv1.SidebarGroupEntry_SIDEBAR_LINK, Id: link.Id}
	beforeFirst := &evtv1.SidebarGroupEntry{Kind: evtv1.SidebarGroupEntry_ROOM, Id: firstTargetRoom.Id}
	if _, err := core.AdminMoveSidebarItem(ctx, actor.Id, linkRef, second.Id, beforeFirst); err != nil {
		t.Fatalf("AdminMoveSidebarItem link across groups: %v", err)
	}
	target, err = core.GetRoomGroup(ctx, second.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup target after link move: %v", err)
	}
	wantTargetOrder = append([]string{"link:" + link.Id}, wantTargetOrder...)
	if got := sidebarEntryKeys(target.GetEntries()); !slices.Equal(got, wantTargetOrder) {
		t.Fatalf("target order after link move = %v, want %v", got, wantTargetOrder)
	}

	secondTargetRef := &evtv1.SidebarGroupEntry{Kind: evtv1.SidebarGroupEntry_ROOM, Id: secondTargetRoom.Id}
	if _, err := core.AdminMoveSidebarItem(ctx, actor.Id, secondTargetRef, second.Id, beforeFirst); err != nil {
		t.Fatalf("AdminMoveSidebarItem within group: %v", err)
	}
	wantTargetOrder = []string{
		"link:" + link.Id,
		"room:" + secondTargetRoom.Id,
		"room:" + firstTargetRoom.Id,
		"room:" + movingRoom.Id,
	}
	target, err = core.GetRoomGroup(ctx, second.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup target after intra-group move: %v", err)
	}
	if got := sidebarEntryKeys(target.GetEntries()); !slices.Equal(got, wantTargetOrder) {
		t.Fatalf("target order after intra-group move = %v, want %v", got, wantTargetOrder)
	}

	if err := core.DenyGroupPermission(ctx, SystemActorID, second.Id, RoleEveryone, PermRoomManage); err != nil {
		t.Fatalf("DenyGroupPermission target room.manage: %v", err)
	}
	if _, err := core.AdminMoveSidebarItem(ctx, actor.Id, linkRef, second.Id, beforeSecond); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("no-op placement after authorization loss error = %v, want ErrPermissionDenied", err)
	}
}

func sidebarEntryKeys(entries []*evtv1.SidebarGroupEntry) []string {
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, sidebarEntryKey(entry))
	}
	return keys
}
