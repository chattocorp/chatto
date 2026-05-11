package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
)

func TestRolePermissions_RoomTierIncludesAllAppliedTiers(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// env.testUser is the bootstrap owner -> server admin, can read everything.
	results, err := query.RolePermissions(env.authContext(), "owner", &env.testRoom.Id)
	if err != nil {
		t.Fatalf("RolePermissions: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil result for the owner role")
	}
	if results.RoleName != "owner" {
		t.Errorf("RoleName = %s, want owner", results.RoleName)
	}
	if results.Server == nil {
		t.Error("expected server tier")
	}
	if results.Room == nil {
		t.Error("expected room tier")
	}
}

func TestRolePermissions_ServerTierWithoutRoomScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	results, err := query.RolePermissions(env.authContext(), "admin", nil)
	if err != nil {
		t.Fatalf("RolePermissions: %v", err)
	}
	if results == nil {
		t.Fatal("expected non-nil result for admin")
	}
	if results.Server == nil {
		t.Error("expected server tier on every result")
	}
	if results.Room != nil {
		t.Error("expected no room tier when roomId is absent")
	}
}

func TestRolePermissions_NonAdminCannotInspectServerScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	regular := env.createVerifiedUser(t, "regular-rp", "Regular", "password123")
	_, err := query.RolePermissions(env.authContextForUser(regular), "admin", nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}
