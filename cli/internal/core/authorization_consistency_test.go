package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestAuthorizeAtStableInputsRepeatsChangedDecision(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	staleDecision := errors.New("stale authorization decision")
	attempts := 0

	err := chattoCore.authorizeAtStableInputs(ctx, func() error {
		attempts++
		if attempts != 1 {
			return nil
		}
		if _, err := chattoCore.CreateUser(ctx, SystemActorID, "stable-auth-race", "Stable Auth Race", "password123"); err != nil {
			return err
		}
		return staleDecision
	})
	if err != nil {
		t.Fatalf("authorizeAtStableInputs: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("authorization attempts = %d, want 2", attempts)
	}
}

func TestAuthorizeAtStableInputsReturnsStableDecision(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	want := errors.New("stable denial")

	err := chattoCore.authorizeAtStableInputs(ctx, func() error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("authorizeAtStableInputs error = %v, want %v", err, want)
	}
}

func TestAuthorityChangesDoNotWriteLegacyFence(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	before, err := chattoCore.EventPublisher.LastSubjectSeq(ctx, evtstream.AuthorizationSubjectFilter())
	if err != nil {
		t.Fatalf("read legacy authorization fence before mutations: %v", err)
	}

	user, err := chattoCore.CreateUser(ctx, SystemActorID, "no-legacy-fence", "No Legacy Fence", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := chattoCore.GrantUserPermission(ctx, SystemActorID, user.GetId(), PermMessagePost); err != nil {
		t.Fatalf("GrantUserPermission: %v", err)
	}
	group, err := chattoCore.CreateRoomGroup(ctx, SystemActorID, "No Legacy Fence Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, SystemActorID, KindChannel, group.GetId(), "no-legacy-fence-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chattoCore.JoinRoom(ctx, user.GetId(), KindChannel, user.GetId(), room.GetId()); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if removed, err := chattoCore.RemoveMember(ctx, SystemActorID, KindChannel, room.GetId(), user.GetId()); err != nil || !removed {
		t.Fatalf("RemoveMember removed=%v error=%v", removed, err)
	}

	after, err := chattoCore.EventPublisher.LastSubjectSeq(ctx, evtstream.AuthorizationSubjectFilter())
	if err != nil {
		t.Fatalf("read legacy authorization fence after mutations: %v", err)
	}
	if after != before {
		t.Fatalf("legacy authorization fence advanced from %d to %d", before, after)
	}
}

func TestAuthorizedGroupMutationRechecksAfterPermissionRevocation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor, err := core.CreateUser(ctx, SystemActorID, "stable-group-manager", "Stable Group Manager", "password123")
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
	event := newEvent(actor.Id, &evtv1.Event{Event: &evtv1.Event_RoomGroupUpdated{
		RoomGroupUpdated: &evtv1.RoomGroupUpdatedEvent{
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

	if _, err := core.appendGroupLayoutMutation(ctx, evtstream.GroupAggregate(group.GetId()), event, authorize); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("appendGroupLayoutMutation error = %v, want ErrPermissionDenied", err)
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
	actor, err := core.CreateUser(ctx, SystemActorID, "stable-role-manager", "Stable Role Manager", "password123")
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
		t.Fatalf("applyRolePermissionState error = %v, want ErrPermissionDenied", err)
	}
	if got := core.rbacModel.decision(ScopeServer, "", RoleModerator, PermRoomCreate); got == DecisionAllow {
		t.Fatal("permission mutation committed after role.manage was revoked")
	}
}
