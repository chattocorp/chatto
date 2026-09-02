package core

import (
	"errors"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	if core.rbacModel.hasRole(target.Id, RoleModerator) {
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
	if !core.rbacModel.hasRole(target.Id, RoleModerator) {
		t.Fatal("target did not receive moderator within bounded authority")
	}
	if err := core.AdminAssignServerRole(ctx, assigner.Id, target.Id, RoleOwner); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("assign owner as non-owner error = %v, want permission denied", err)
	}
}

func TestImplicitEveryoneRoleIsNeverAssignableOrRevocable(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := core.CreateUser(ctx, SystemActorID, "implicit-role-actor", "Implicit Role Actor", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, actor.Id, PermRoleAssign); err != nil {
		t.Fatalf("GrantUserPermission role.assign: %v", err)
	}

	if can, err := core.CanAssignRole(ctx, actor.Id, RoleEveryone); err != nil || can {
		t.Fatalf("CanAssignRole(everyone) = %v, %v; want false, nil", can, err)
	}
	if can, err := core.CanRevokeRole(ctx, actor.Id, RoleEveryone); err != nil || can {
		t.Fatalf("CanRevokeRole(everyone) = %v, %v; want false, nil", can, err)
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

func TestRoleAssignmentIgnoresUnrelatedChatTraffic(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	assigner, err := core.CreateUser(ctx, SystemActorID, "chat-independent-assigner", "Chat Independent Assigner", "password123")
	if err != nil {
		t.Fatalf("CreateUser assigner: %v", err)
	}
	target, err := core.CreateUser(ctx, SystemActorID, "chat-independent-target", "Chat Independent Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil || len(groups) == 0 {
		t.Fatalf("ListRoomGroupsOrdered groups=%d err=%v", len(groups), err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, groups[0].GetId(), "assignment-chat-traffic", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, assigner.Id, KindChannel, assigner.Id, room.GetId()); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	for _, permission := range []Permission{PermRoleAssign, PermMessageManage, PermRoomMemberBan} {
		if err := core.GrantUserPermission(ctx, SystemActorID, assigner.Id, permission); err != nil {
			t.Fatalf("GrantUserPermission %s: %v", permission, err)
		}
	}

	if _, err := core.PostMessage(ctx, KindChannel, room.GetId(), assigner.Id, "unrelated traffic", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := core.AdminAssignServerRole(ctx, assigner.Id, target.Id, RoleModerator); err != nil {
		t.Fatalf("AdminAssignServerRole after chat traffic: %v", err)
	}
}

func TestRoleAssignmentRechecksTargetExistenceWithStableInputs(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	target, err := core.CreateUser(ctx, SystemActorID, "deleted-role-target", "Deleted Role Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{
		RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: target.GetId(), RoleName: RoleModerator},
	}})
	authorizationChecks := 0

	_, err = core.appendRoleAssignmentEvent(ctx, target.GetId(), true, event, func() error {
		authorizationChecks++
		if authorizationChecks == 1 {
			return core.DeleteUser(ctx, SystemActorID, target.GetId())
		}
		return nil
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("appendRoleAssignmentEvent error = %v, want ErrNotFound", err)
	}
	if core.rbacModel.hasRole(target.GetId(), RoleModerator) {
		t.Fatal("role assignment was stored for a deleted target")
	}
}
