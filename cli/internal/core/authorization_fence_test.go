package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestAuthorizedGroupMutationRechecksAfterPermissionRevocation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := core.CreateUser(ctx, SystemActorID, "fenced-group-manager", "Fenced Group Manager", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, actor.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserPermission room.manage: %v", err)
	}
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil || len(groups) == 0 {
		t.Fatalf("ListRoomGroupsOrdered groups=%d err=%v", len(groups), err)
	}
	group := groups[0]
	event := newEvent(actor.Id, &corev1.Event{Event: &corev1.Event_RoomGroupUpdated{
		RoomGroupUpdated: &corev1.RoomGroupUpdatedEvent{
			GroupId:     group.GetId(),
			Name:        "must-not-commit",
			Description: group.GetDescription(),
		},
	}})

	checks := 0
	authorize := func() error {
		checks++
		if checks == 1 {
			if err := core.ClearUserPermissionState(ctx, SystemActorID, actor.Id, PermRoomManage); err != nil {
				return err
			}
			return nil
		}
		return core.requireCanManageRoomGroup(ctx, actor.Id, group.GetId())
	}

	if _, err := core.appendGroupLayoutMutation(ctx, events.GroupAggregate(group.GetId()), event, authorize); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("appendGroupLayoutMutation err = %v, want ErrPermissionDenied", err)
	}
	updated, err := core.GetRoomGroup(ctx, group.GetId())
	if err != nil {
		t.Fatalf("GetRoomGroup: %v", err)
	}
	if updated.GetName() == "must-not-commit" {
		t.Fatal("group mutation committed after room.manage was revoked")
	}
}

func TestScopedPermissionMutationRechecksAfterRoleManageRevocation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := core.CreateUser(ctx, SystemActorID, "fenced-role-manager", "Fenced Role Manager", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, actor.Id, PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}

	checks := 0
	authorize := func() error {
		checks++
		if checks == 1 {
			if err := core.ClearUserPermissionState(ctx, SystemActorID, actor.Id, PermRoleManage); err != nil {
				return err
			}
			return nil
		}
		return core.requireCanManageAdminRoles(ctx, actor.Id)
	}

	err = core.applyRolePermissionState(ctx, actor.Id, ScopeServer, "", RoleModerator, PermRoomCreate, PermissionStateAllow, authorize)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("applyRolePermissionState err = %v, want ErrPermissionDenied", err)
	}
	if got := core.RBAC.GetDecision(ScopeServer, "", RoleModerator, PermRoomCreate); got == DecisionAllow {
		t.Fatal("permission mutation committed after role.manage was revoked")
	}
}
