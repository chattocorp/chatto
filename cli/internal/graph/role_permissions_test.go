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

// TestRolePermissions_InstanceRoleHasInstanceTier tested the
// "instance-admin"-vs-"admin" distinction, which is gone after the role
// rename per ADR-028. With a unified namespace and the dual-engine model
// still alive (until PR 4), names like "admin" exist in both engines and
// querying with a spaceId resolves to the space-tier role. The full
// tier-distinction story collapses with the engines in PR 4 and the test
// is rewritten there.

func TestRolePermissions_NonAdminCannotInspectInstanceScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	regular := env.createVerifiedUser(t, "regular-rp", "Regular", "password123")
	_, err := query.RolePermissions(env.authContextForUser(regular), core.RoleAdmin, nil, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestRolePermissions_CrossSpaceLeakRejected(t *testing.T) {
	t.Skip("Per ADR-021 / ADR-028 (PR 4) RBAC is server-wide; the " +
		"cross-space leakage gate this test exercised no longer exists.")
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
func TestRolePermissions_RoomMustBelongToSpace(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// Create a second space + room. env.testUser is bootstrap owner
	// (instance admin) so they pass the auth gate for both spaces.
	otherSpace, err := env.core.CreateSpace(env.ctx, env.testUser.Id, "Other", "")
	if err != nil {
		t.Fatalf("create other space: %v", err)
	}
	otherRoom, err := env.core.CreateRoom(env.ctx, env.testUser.Id, otherSpace.Id, "general", "")
	if err != nil {
		t.Fatalf("create other room: %v", err)
	}

	_, err = query.RolePermissions(env.authContext(), "owner", &env.testSpace.Id, &otherRoom.Id)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied for cross-space roomId, got %v", err)
	}
}
