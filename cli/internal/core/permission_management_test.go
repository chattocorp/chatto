package core

import "testing"

func TestRoleHolderResolutionAppliesMessageReadInclusion(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	roomID := createPermissionEditRoom(t, c, ctx, "role-holder-inclusion")
	for _, perm := range []Permission{PermMessageRead, PermMessageReadInteractions} {
		if err := c.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, perm); err != nil {
			t.Fatalf("ClearServerPermissionState %s: %v", perm, err)
		}
	}
	// An everyone deny that an earlier version stored has no effect (ADR-116).
	appendStoredRoleDeny(t, c, ctx, ScopeRoom, roomID, RoleEveryone, PermMessageReadInteractions)
	if err := c.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessageRead); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}

	// The broad allow of the role includes the narrow permission.
	if got := c.PermResolver().resolveRoleHolder(RoleModerator, KindChannel, roomID, "", PermMessageReadInteractions); got != DecisionAllow {
		t.Fatalf("moderator holder message.read-interactions = %s, want allow", got)
	}
	// Without an allow, a member with only everyone has no access.
	if got := c.PermResolver().resolveRoleHolder(RoleEveryone, KindChannel, roomID, "", PermMessageReadInteractions); got != DecisionNone {
		t.Fatalf("everyone message.read-interactions = %s, want none", got)
	}
}

// This test does not call t.Parallel: installTestPermissionInclusion changes
// the package-wide permission catalog.
func TestRoleHolderResolutionAppliesExplicitInclusion(t *testing.T) {
	broad, narrow := installTestPermissionInclusion(t)
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	if err := c.GrantServerPermission(ctx, SystemActorID, RoleModerator, broad); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}
	if got := c.PermResolver().resolveRoleHolder(RoleModerator, KindChannel, "", "", narrow); got != DecisionAllow {
		t.Fatalf("moderator holder %s = %s, want allow included by %s", narrow, got, broad)
	}
}
