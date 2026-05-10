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
	results, err := query.RolePermissions(env.authContext(), "owner", &env.testRoom.Id)
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
	t.Skip("Phase 5 collapsed the instance tier; the legacy 'instance-admin' role with an instance-only permission set no longer exists.")
	env := setupTestResolver(t)
	query := env.resolver.Query()

	results, err := query.RolePermissions(env.authContext(), "instance-admin", nil)
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
	if results.Space != nil {
		t.Error("expected no space tier when no roomId is provided (instance scope only)")
	}
	if results.Room != nil {
		t.Error("expected no room tier when roomId is absent")
	}
}

func TestRolePermissions_NonAdminCannotInspectInstanceScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	regular := env.createVerifiedUser(t, "regular-rp", "Regular", "password123")
	_, err := query.RolePermissions(env.authContextForUser(regular), "instance-admin", nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}

