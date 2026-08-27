package core

import (
	"errors"
	"testing"
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
