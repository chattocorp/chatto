package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
)

func TestRolePermissions_RoomTierIncludesAllAppliedTiers(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// env.testUser is the bootstrap owner -> instance admin, can read everything.
	results, err := query.RolePermissions(env.authContext(), "owner", &env.testSpace.Id, &env.testRoom.Id)
	if err != nil {
		t.Fatalf("RolePermissions: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil result for the space's owner role")
	}
	if results.RoleName != "owner" {
		t.Errorf("RoleName = %s, want owner", results.RoleName)
	}
	if results.IsInstanceRole {
		t.Error("expected isInstanceRole=false for owner")
	}
	// Space tier present, instance tier absent (space role).
	if results.Space == nil {
		t.Error("expected space tier")
	}
	if results.Instance != nil {
		t.Error("space role should not expose an instance tier")
	}
	if results.Room == nil {
		t.Error("expected room tier")
	}
}

func TestRolePermissions_InstanceRoleHasInstanceTier(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	results, err := query.RolePermissions(env.authContext(), "instance-admin", &env.testSpace.Id, nil)
	if err != nil {
		t.Fatalf("RolePermissions: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil result for instance-admin")
	}
	if !results.IsInstanceRole {
		t.Error("expected isInstanceRole=true")
	}
	if results.Instance == nil {
		t.Error("instance role should expose an instance tier")
	}
	if results.Space == nil {
		t.Error("expected space tier when spaceId provided")
	}
	if results.Room != nil {
		t.Error("expected no room tier when roomId is absent")
	}
}

func TestRolePermissions_NonAdminCannotInspectInstanceScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	regular := env.createVerifiedUser(t, "regular-rp", "Regular", "password123")
	_, err := query.RolePermissions(env.authContextForUser(regular), "instance-admin", nil, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestRolePermissions_CrossSpaceLeakRejected(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	spaceAOwner := env.createVerifiedUser(t, "spacea-owner-rp", "A Owner", "password123")
	if _, err := env.core.CreateSpace(env.ctx, spaceAOwner.Id, "Space A", ""); err != nil {
		t.Fatalf("create space A: %v", err)
	}
	spaceBOwner := env.createVerifiedUser(t, "spaceb-owner-rp", "B Owner", "password123")
	spaceB, err := env.core.CreateSpace(env.ctx, spaceBOwner.Id, "Space B", "")
	if err != nil {
		t.Fatalf("create space B: %v", err)
	}

	// spaceAOwner has role.manage in spaceA but not spaceB.
	_, err = query.RolePermissions(env.authContextForUser(spaceAOwner), "owner", &spaceB.Id, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied for cross-space lookup, got %v", err)
	}
}

func TestRolePermissions_RoomIDWithoutSpaceIDFails(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	_, err := query.RolePermissions(env.authContext(), "owner", nil, &env.testRoom.Id)
	if err == nil {
		t.Error("expected error when roomId provided without spaceId")
	}
}

// TestRolePermissions_RoomMustBelongToSpace verifies that passing a roomID
// that does not exist in the requested space is rejected, even if the
// caller has role.manage in that space. Without this check the API
// silently returned an empty room tier for a nonsensical (space, room)
// pair, which is confusing and an authorization-shaped contract gap.
