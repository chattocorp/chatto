package core

import "testing"

func TestRoleHolderResolutionAppliesMessageReadInclusion(t *testing.T) {
	t.Parallel()

	c, _ := setupTestCore(t)
	ctx := testContext(t)
	if err := c.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("ClearServerPermissionState: %v", err)
	}
	if err := c.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessageReadInteractions); err != nil {
		t.Fatalf("DenyServerPermission: %v", err)
	}
	if err := c.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessageRead); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}

	// The broad allow of the role includes the narrow permission, also over
	// the everyone deny of the narrow permission.
	if got := c.PermResolver().resolveRoleHolder(RoleModerator, KindChannel, "", "", PermMessageReadInteractions); got != DecisionAllow {
		t.Fatalf("moderator holder message.read-interactions = %s, want allow", got)
	}
	if got := c.PermResolver().resolveRoleHolder(RoleEveryone, KindChannel, "", "", PermMessageReadInteractions); got != DecisionDeny {
		t.Fatalf("everyone message.read-interactions = %s, want deny", got)
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
