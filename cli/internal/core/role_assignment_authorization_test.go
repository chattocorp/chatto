package core

import (
	"errors"
	"testing"
)

func TestDelegatedRoleAssignmentCannotGrantBroaderAuthority(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	assigner, err := core.CreateUser(ctx, SystemActorID, "bounded-role-assigner", "Bounded Role Assigner", "password")
	if err != nil {
		t.Fatalf("CreateUser assigner: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "bounded-role-target", "Bounded Role Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, assigner.Id, PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}

	if err := core.AdminAssignServerRole(ctx, assigner.Id, target.Id, RoleModerator); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("assign moderator beyond authority error = %v, want permission denied", err)
	}
	if core.RBAC.HasRole(target.Id, RoleModerator) {
		t.Fatal("target received moderator despite bounded assignment denial")
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, assigner.Id, PermMessageManage); err != nil {
		t.Fatalf("GrantUserPermission message.manage: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, assigner.Id, PermRoomMemberBan); err != nil {
		t.Fatalf("GrantUserPermission room.ban-member: %v", err)
	}
	if err := core.AdminAssignServerRole(ctx, assigner.Id, target.Id, RoleModerator); err != nil {
		t.Fatalf("assign moderator within authority: %v", err)
	}
	if !core.RBAC.HasRole(target.Id, RoleModerator) {
		t.Fatal("target did not receive moderator within bounded authority")
	}
	if err := core.AdminAssignServerRole(ctx, assigner.Id, target.Id, RoleOwner); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("assign owner as non-owner error = %v, want permission denied", err)
	}
}

func TestDelegatedRoleRevocationCannotRemoveBroaderRestriction(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	assigner, err := core.CreateUser(ctx, SystemActorID, "bounded-role-revoker", "Bounded Role Revoker", "password")
	if err != nil {
		t.Fatalf("CreateUser assigner: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "bounded-role-revoke-target", "Bounded Role Revoke Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "restricted", "Restricted", "", false); err != nil {
		t.Fatalf("CreateServerRole restricted: %v", err)
	}
	if err := core.DenyServerPermission(ctx, SystemActorID, "restricted", PermRoomCreate); err != nil {
		t.Fatalf("DenyServerPermission room.create: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, target.Id, "restricted"); err != nil {
		t.Fatalf("AssignServerRole restricted: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, assigner.Id, PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}

	if err := core.AdminRevokeServerRole(ctx, assigner.Id, target.Id, "restricted"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("revoke restriction beyond authority error = %v, want permission denied", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, assigner.Id, PermRoomCreate); err != nil {
		t.Fatalf("GrantUserPermission room.create: %v", err)
	}
	if err := core.AdminRevokeServerRole(ctx, assigner.Id, target.Id, "restricted"); err != nil {
		t.Fatalf("revoke restriction within authority: %v", err)
	}
}

func TestDelegatedRoleAssignmentChecksScopedAuthority(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	assigner, err := core.CreateUser(ctx, SystemActorID, "scoped-role-assigner", "Scoped Role Assigner", "password")
	if err != nil {
		t.Fatalf("CreateUser assigner: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "scoped-role-target", "Scoped Role Target", "password")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil || len(groups) == 0 {
		t.Fatalf("ListRoomGroupsOrdered: groups=%d err=%v", len(groups), err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, groups[0].GetId(), "scoped-assignment-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "room-moderator", "Room Moderator", "", false); err != nil {
		t.Fatalf("CreateServerRole room-moderator: %v", err)
	}
	if err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, "room-moderator", PermMessageManage); err != nil {
		t.Fatalf("GrantRoomPermission message.manage: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, assigner.Id, PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}

	if err := core.AdminAssignServerRole(ctx, assigner.Id, target.Id, "room-moderator"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("assign room-moderator without scoped authority error = %v, want permission denied", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, assigner.Id, PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission message.manage: %v", err)
	}
	if err := core.AdminAssignServerRole(ctx, assigner.Id, target.Id, "room-moderator"); err != nil {
		t.Fatalf("assign room-moderator within scoped authority: %v", err)
	}
}
