package core

import (
	"context"
	"slices"
	"strings"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// grantTestRank assigns userID a new role without permissions and places it
// directly below admin. The user then outranks moderators, custom roles, and
// accounts without roles, but not admins or owners.
func grantTestRank(t *testing.T, c *ChattoCore, ctx context.Context, userID string) {
	t.Helper()
	roleName := "rank-" + strings.ToLower(userID)
	if _, err := c.CreateServerRole(ctx, SystemActorID, roleName, "Test rank", ""); err != nil {
		t.Fatalf("CreateServerRole %s: %v", roleName, err)
	}
	moveTestRoleBelow(t, c, ctx, roleName, RoleAdmin)
	if err := c.AssignServerRole(ctx, SystemActorID, userID, roleName); err != nil {
		t.Fatalf("AssignServerRole %s: %v", roleName, err)
	}
}

// moveTestRoleBelow places roleName directly below upperRoleName.
func moveTestRoleBelow(t *testing.T, c *ChattoCore, ctx context.Context, roleName, upperRoleName string) {
	t.Helper()
	order := slices.DeleteFunc(c.orderableRoleNames(), func(name string) bool { return name == roleName })
	before := ""
	if upper := slices.Index(order, upperRoleName); upper > 0 {
		before = order[upper-1]
	}
	if _, err := c.MoveServerRole(ctx, SystemActorID, roleName, before); err != nil {
		t.Fatalf("MoveServerRole %s below %s: %v", roleName, upperRoleName, err)
	}
}

// rbacRolePermissionDeniedEvent builds a role deny. Roles only grant
// permissions since 0.5, so only an earlier version could store one (ADR-116).
func rbacRolePermissionDeniedEvent(scope PermissionScope, scopeID, roleName string, perm Permission) *evtv1.RbacPermissionDeniedEvent {
	return rbacPermissionDeniedEvent(scope, scopeID, evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, roleName, perm)
}

// appendStoredRoleDeny stores a role deny as an earlier version could, and
// waits for the RBAC projection.
func appendStoredRoleDeny(t *testing.T, c *ChattoCore, ctx context.Context, scope PermissionScope, scopeID, roleName string, perm Permission) {
	t.Helper()
	event := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacRolePermissionDeniedEvent(scope, scopeID, roleName, perm),
	}})
	if _, err := c.appendRBACEvent(ctx, event, nil); err != nil {
		t.Fatalf("append stored %s deny: %v", roleName, err)
	}
}

// setStoredUserDeny stores a deny of a user at scope, or clears the user's
// setting when deny is false. It skips the authority checks of an acting user.
func setStoredUserDeny(t *testing.T, c *ChattoCore, ctx context.Context, scope PermissionScope, scopeID, userID string, perm Permission, deny bool) {
	t.Helper()
	event := &evtv1.Event{Event: &evtv1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacUserPermissionClearedEvent(scope, scopeID, userID, perm),
	}}
	if deny {
		event = &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{
			RbacPermissionDenied: rbacUserPermissionDeniedEvent(scope, scopeID, userID, perm),
		}}
	}
	if _, err := c.appendRBACEvent(ctx, newEvent(SystemActorID, event), nil); err != nil {
		t.Fatalf("append user permission state: %v", err)
	}
}
