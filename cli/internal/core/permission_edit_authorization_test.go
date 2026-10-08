package core

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/internal/authctx"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func createPermissionEditUser(t *testing.T, core *ChattoCore, ctx context.Context, login string) string {
	t.Helper()
	user, err := core.CreateUser(ctx, SystemActorID, login, login, "password")
	if err != nil {
		t.Fatalf("CreateUser %s: %v", login, err)
	}
	return user.Id
}

func createPermissionEditRoom(t *testing.T, core *ChattoCore, ctx context.Context, name string) string {
	t.Helper()
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil || len(groups) == 0 {
		t.Fatalf("ListRoomGroupsOrdered: groups=%d err=%v", len(groups), err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, groups[0].GetId(), name, "")
	if err != nil {
		t.Fatalf("CreateRoom %s: %v", name, err)
	}
	return room.Id
}

func TestDelegatedUserPermissionEditsStayWithinAuthority(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor := createPermissionEditUser(t, core, ctx, "user-permission-editor")
	target := createPermissionEditUser(t, core, ctx, "user-permission-target")
	roomID := createPermissionEditRoom(t, core, ctx, "user-permission-edit-room")
	peer := createPermissionEditUser(t, core, ctx, "user-permission-peer")
	grantTestRank(t, core, ctx, actor)
	if err := core.AssignServerRole(ctx, SystemActorID, peer, "rank-"+strings.ToLower(actor)); err != nil {
		t.Fatalf("AssignServerRole peer rank: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, actor, PermUserManagePermissions); err != nil {
		t.Fatalf("GrantUserPermission user.manage-permissions: %v", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, roomID, actor, PermMessageManage); err != nil {
		t.Fatalf("GrantUserRoomPermission message.manage: %v", err)
	}
	server := PermissionTargetScope{Kind: MatrixScopeServer}
	room := PermissionTargetScope{Kind: MatrixScopeRoom, ID: roomID}

	tests := []struct {
		name    string
		subject string
		scope   PermissionTargetScope
		perm    Permission
		state   PermissionState
		allowed bool
	}{
		{"grant beyond authority", target, server, PermRoleManage, PermissionStateAllow, false},
		{"deny beyond authority", target, server, PermServerManage, PermissionStateDeny, false},
		{"clear beyond authority", target, server, PermUserDeleteAny, PermissionStateNone, false},
		{"grant room authority at server scope", target, server, PermMessageManage, PermissionStateAllow, false},
		{"grant room authority at its room", target, room, PermMessageManage, PermissionStateAllow, true},
		{"grant held permission", target, server, PermMessageReact, PermissionStateAllow, true},
		{"deny held permission", target, server, PermMessagePost, PermissionStateDeny, true},
		{"clear held permission", target, server, PermMessagePost, PermissionStateNone, true},
		{"grant own held permission", actor, server, PermMessageReact, PermissionStateAllow, true},
		{"grant own room authority", actor, room, PermMessageManage, PermissionStateAllow, true},
		{"grant own permission beyond authority", actor, server, PermRoleManage, PermissionStateAllow, false},
		{"grant held permission to a peer", peer, server, PermMessageReact, PermissionStateAllow, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scopeKind, scopeID := ScopeServer, ""
			if tt.scope.Kind == MatrixScopeRoom {
				scopeKind, scopeID = ScopeRoom, tt.scope.ID
			}
			before := core.rbacModel.decision(scopeKind, scopeID, tt.subject, tt.perm)
			err := core.SetUserPermissionState(ctx, actor, tt.subject, tt.scope, tt.perm, tt.state)
			if tt.allowed {
				if err != nil {
					t.Fatalf("SetUserPermissionState error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrPermissionDenied) {
				t.Fatalf("SetUserPermissionState error = %v, want permission denied", err)
			}
			if after := core.rbacModel.decision(scopeKind, scopeID, tt.subject, tt.perm); after != before {
				t.Fatalf("decision changed from %s to %s despite denial", before, after)
			}
		})
	}
}

func TestPermissionEditsUseTheActorsPrivilegedModeState(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	owner := createPermissionEditUser(t, core, ctx, "privileged-permission-owner")
	target := createPermissionEditUser(t, core, ctx, "privileged-permission-target")
	if err := core.AssignOwnerRole(ctx, owner); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	credential := func(deadline time.Time) context.Context {
		return authctx.WithCredential(ctx, authctx.RuntimeCredential{
			Kind:                    authctx.RuntimeCredentialKindBearerToken,
			UserID:                  owner,
			Handle:                  "permission-edit-session",
			PrivilegedModeExpiresAt: deadline,
		})
	}
	server := PermissionTargetScope{Kind: MatrixScopeServer}

	if err := core.SetUserPermissionState(credential(time.Time{}), owner, target, server, PermServerManage, PermissionStateAllow); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("inactive owner grant error = %v, want permission denied", err)
	}
	if err := core.SetUserPermissionState(credential(time.Now().Add(time.Minute)), owner, target, server, PermServerManage, PermissionStateAllow); err != nil {
		t.Fatalf("active owner grant error = %v, want nil", err)
	}
}

func TestDelegatedRolePermissionEditsStayWithinAuthority(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roleManager := createPermissionEditUser(t, core, ctx, "role-permission-editor")
	roomManager := createPermissionEditUser(t, core, ctx, "room-permission-editor")
	roomID := createPermissionEditRoom(t, core, ctx, "role-permission-edit-room")
	grantTestRank(t, core, ctx, roleManager)
	grantTestRank(t, core, ctx, roomManager)
	if _, err := core.CreateServerRole(ctx, SystemActorID, "edited", "Edited", "", false); err != nil {
		t.Fatalf("CreateServerRole edited: %v", err)
	}
	if err := core.GrantServerPermission(ctx, SystemActorID, "edited", PermUserDeleteAny); err != nil {
		t.Fatalf("GrantServerPermission user.delete-any: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, roleManager, PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	if err := core.GrantUserRoomPermission(ctx, SystemActorID, roomID, roomManager, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}
	server := PermissionTargetScope{Kind: MatrixScopeServer}
	room := PermissionTargetScope{Kind: MatrixScopeRoom, ID: roomID}

	tests := []struct {
		name    string
		actor   string
		role    string
		scope   PermissionTargetScope
		perm    Permission
		state   PermissionState
		allowed bool
	}{
		{"role manager edits a role above them", roleManager, RoleAdmin, server, PermMessageReact, PermissionStateAllow, true},
		{"room manager edits a role above them", roomManager, RoleAdmin, room, PermMessagePost, PermissionStateAllow, false},
		{"role manager grants beyond authority", roleManager, "edited", server, PermServerManage, PermissionStateAllow, false},
		{"role manager clears a grant beyond authority", roleManager, "edited", server, PermUserDeleteAny, PermissionStateNone, false},
		{"role manager grants held permission", roleManager, "edited", server, PermMessageReact, PermissionStateAllow, true},
		{"role manager grants own management authority", roleManager, "edited", server, PermRoleManage, PermissionStateAllow, true},
		{"room manager grants beyond authority", roomManager, "edited", room, PermMessageManage, PermissionStateAllow, false},
		{"room manager grants held room authority", roomManager, "edited", room, PermRoomManage, PermissionStateAllow, true},
		{"room manager denies a held permission for everyone", roomManager, RoleEveryone, room, PermMessagePost, PermissionStateDeny, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scopeKind, scopeID := ScopeServer, ""
			if tt.scope.Kind == MatrixScopeRoom {
				scopeKind, scopeID = ScopeRoom, tt.scope.ID
			}
			before := core.rbacModel.decision(scopeKind, scopeID, tt.role, tt.perm)
			err := core.SetRolePermissionState(ctx, tt.actor, tt.role, tt.scope, tt.perm, tt.state)
			if tt.allowed {
				if err != nil {
					t.Fatalf("SetRolePermissionState error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrPermissionDenied) {
				t.Fatalf("SetRolePermissionState error = %v, want permission denied", err)
			}
			if after := core.rbacModel.decision(scopeKind, scopeID, tt.role, tt.perm); after != before {
				t.Fatalf("decision changed from %s to %s despite denial", before, after)
			}
		})
	}
}

func TestPermissionEditsReportInvalidScopesBeforeAuthority(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor := createPermissionEditUser(t, core, ctx, "invalid-scope-editor")
	target := createPermissionEditUser(t, core, ctx, "invalid-scope-target")
	if err := core.AssignServerRole(ctx, SystemActorID, actor, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}
	dm := PermissionTargetScope{Kind: MatrixScopeDM}

	if err := core.SetUserPermissionState(ctx, actor, target, dm, PermRoleManage, PermissionStateAllow); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("user DM-scope role.manage error = %v, want invalid argument", err)
	}
	if err := core.SetRolePermissionState(ctx, actor, RoleModerator, dm, PermRoleManage, PermissionStateAllow); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("role DM-scope role.manage error = %v, want invalid argument", err)
	}
	if err := core.SetUserPermissionState(ctx, actor, target, PermissionTargetScope{Kind: MatrixScopeServer}, Permission("unknown.permission"), PermissionStateAllow); !errors.Is(err, ErrInvalidPermission) {
		t.Fatalf("unknown permission error = %v, want invalid permission", err)
	}
}

func TestDelegatedRoleDeletionStaysWithinAuthority(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	actor := createPermissionEditUser(t, core, ctx, "role-deleter")
	grantTestRank(t, core, ctx, actor)
	if err := core.GrantUserPermission(ctx, SystemActorID, actor, PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	for _, role := range []struct {
		name     string
		decision func(roleName string) error
		allowed  bool
	}{
		{"broader-grant", func(name string) error { return core.GrantServerPermission(ctx, SystemActorID, name, PermServerManage) }, false},
		{"held-grant", func(name string) error { return core.GrantServerPermission(ctx, SystemActorID, name, PermMessageReact) }, true},
	} {
		t.Run(role.name, func(t *testing.T) {
			if _, err := core.CreateServerRole(ctx, SystemActorID, role.name, role.name, "", false); err != nil {
				t.Fatalf("CreateServerRole: %v", err)
			}
			if err := role.decision(role.name); err != nil {
				t.Fatalf("set role decision: %v", err)
			}
			err := core.AdminDeleteServerRole(ctx, actor, role.name)
			if role.allowed {
				if err != nil {
					t.Fatalf("AdminDeleteServerRole error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrPermissionDenied) {
				t.Fatalf("AdminDeleteServerRole error = %v, want permission denied", err)
			}
			if !core.rbacModel.roleExists(role.name) {
				t.Fatal("role was deleted despite denial")
			}
		})
	}
}

func TestRoleDeletionIgnoresRetiredPermissionDecisions(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)
	roleManager := createPermissionEditUser(t, core, ctx, "retired-role-deleter")
	owner := createPermissionEditUser(t, core, ctx, "retired-role-owner")
	grantTestRank(t, core, ctx, roleManager)
	if err := core.GrantUserPermission(ctx, SystemActorID, roleManager, PermRoleManage); err != nil {
		t.Fatalf("GrantUserPermission role.manage: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	roomID := createPermissionEditRoom(t, core, ctx, "retired-permission-room")
	const retired = Permission("room.ban-member")

	for _, actor := range []struct{ name, id string }{{"role-manager", roleManager}, {"owner", owner}} {
		t.Run(actor.name, func(t *testing.T) {
			roleName := "retired-" + actor.name
			if _, err := core.CreateServerRole(ctx, SystemActorID, roleName, roleName, "", false); err != nil {
				t.Fatalf("CreateServerRole: %v", err)
			}
			for _, event := range []*evtv1.Event{
				newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{
					RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", roleName, retired),
				}}),
				newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{
					RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeRoom, roomID, roleName, retired),
				}}),
			} {
				if _, err := core.appendRBACEvent(ctx, event, nil); err != nil {
					t.Fatalf("append retired decision: %v", err)
				}
			}
			if err := core.AdminDeleteServerRole(ctx, actor.id, roleName); err != nil {
				t.Fatalf("AdminDeleteServerRole error = %v, want nil", err)
			}
		})
	}
}

// TestUserPermissionMatrixDescribesTheTargetAccount checks that member
// permission cells describe the target account, not the viewer's session.
func TestUserPermissionMatrixDescribesTheTargetAccount(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin := createPermissionEditUser(t, c, ctx, "matrix-target-admin")
	if err := c.AssignServerRole(ctx, SystemActorID, admin, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}
	owner := createPermissionEditUser(t, c, ctx, "matrix-target-owner")
	if err := c.AssignServerRole(ctx, SystemActorID, owner, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole owner: %v", err)
	}
	member := createPermissionEditUser(t, c, ctx, "matrix-target-member")
	roomID := createPermissionEditRoom(t, c, ctx, "matrix-target-room")
	server := PermissionMatrixScope{Kind: MatrixScopeServer, ID: "server"}

	cell := func(userID string, perm Permission, scope PermissionMatrixScope) PermissionMatrixCell {
		t.Helper()
		// No credential in ctx: internal work would see everything as
		// privileged, so the result must not depend on it.
		got, ok, err := c.buildUserPermissionMatrixCell(ctx, userID, perm, scope)
		if err != nil || !ok {
			t.Fatalf("buildUserPermissionMatrixCell(%s, %s): ok=%v err=%v", userID, perm, ok, err)
		}
		return got
	}

	if got := cell(admin, PermRoleManage, server); got.Effective != MatrixDecisionDeny || got.EffectiveWithPrivilegedMode != MatrixDecisionAllow {
		t.Fatalf("admin role.manage = %s / %s, want deny without and allow with privileged mode", got.Effective, got.EffectiveWithPrivilegedMode)
	}
	if got := cell(owner, PermUserDeleteAny, server); got.Effective == MatrixDecisionAllow || got.EffectiveWithPrivilegedMode != MatrixDecisionAllow {
		t.Fatalf("owner user.delete-any = %s / %s, want not allowed without and allowed with privileged mode", got.Effective, got.EffectiveWithPrivilegedMode)
	}

	if _, err := c.JoinRoom(ctx, member, KindChannel, member, roomID); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if err := c.RoomCommands().RemoveUser(ctx, RoomRemoveUserInput{ActorID: owner, RoomID: roomID, UserID: member, Reason: "test", Suspension: true}); err != nil {
		t.Fatalf("RemoveUser with suspension: %v", err)
	}
	room := PermissionMatrixScope{Kind: MatrixScopeRoom, ID: "room:" + roomID}
	if got := cell(member, PermRoomJoin, room); got.Effective != MatrixDecisionDeny || got.EffectiveWithPrivilegedMode != MatrixDecisionDeny {
		t.Fatalf("suspended member room.join = %s / %s, want deny", got.Effective, got.EffectiveWithPrivilegedMode)
	}
}

// TestPermissionMatricesReportWhatTheViewerCanChange checks that the grant
// limit reaches the matrices, so locked cells do not look editable.
func TestPermissionMatricesReportWhatTheViewerCanChange(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	roomManager := createPermissionEditUser(t, c, ctx, "matrix-room-manager")
	roomID := createPermissionEditRoom(t, c, ctx, "matrix-change-room")
	if err := c.GrantUserRoomPermission(ctx, SystemActorID, roomID, roomManager, PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}

	tiers, err := c.GetRolePermissionTierMatrix(ctx, roomManager, roomID, "")
	if err != nil {
		t.Fatalf("GetRolePermissionTierMatrix: %v", err)
	}
	if !slices.Contains(tiers.ViewerChangeablePermissions, string(PermMessagePost)) {
		t.Fatalf("changeable = %v, want the held message.post", tiers.ViewerChangeablePermissions)
	}
	if slices.Contains(tiers.ViewerChangeablePermissions, string(PermMessageManage)) {
		t.Fatalf("changeable = %v, want no message.manage, which the manager lacks", tiers.ViewerChangeablePermissions)
	}
}
