package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
)

func TestPermissionExplanation_InstanceAdminAtInstanceScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// env.testUser is auto-promoted to instance owner.
	results, err := query.PermissionExplanation(env.authContext(), env.testUser.Id, nil)
	if err != nil {
		t.Fatalf("PermissionExplanation: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty explanations at instance scope")
	}
}

func TestPermissionExplanation_NonAdminCannotInspectThemselves(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// The inspector is admin-only — non-admins can't even inspect themselves.
	regular := env.createVerifiedUser(t, "regular-self", "Regular", "password123")
	_, err := query.PermissionExplanation(env.authContextForUser(regular), regular.Id, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied for non-admin self-inspection, got %v", err)
	}
}

func TestPermissionExplanation_AdminInspectsAnotherUser(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	query := env.resolver.Query()

	target := env.createVerifiedUser(t, "target", "Target", "password123")

	results, err := query.PermissionExplanation(env.authContext(), target.Id, nil)
	if err != nil {
		t.Fatalf("PermissionExplanation: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty explanations from admin inspecting another user")
	}
}

func TestPermissionExplanation_NonAdminCannotInspectAnotherUser(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// env.testUser is the bootstrap owner (auto-promoted instance owner) and so
	// has admin access. Use freshly-created users instead — neither is admin.
	regular := env.createVerifiedUser(t, "regular", "Regular", "password123")
	target := env.createVerifiedUser(t, "target2", "Target 2", "password123")

	_, err := query.PermissionExplanation(env.authContextForUser(regular), target.Id, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied when non-admin inspects another user, got %v", err)
	}
}

func TestPermissionExplanation_SpaceAdminCannotInspectAnotherSpace(t *testing.T) {
	t.Skip("Phase 5 collapsed instance/space tiers; multi-space cross-tier scenarios no longer apply.")
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// env.testUser is the bootstrap owner (instance admin) — using them as the
	// caller would skip the cross-space gate via the instance-admin path. So
	// we set up two non-admin space owners and verify the gate triggers.
	spaceAOwner := env.createVerifiedUser(t, "spacea-owner", "A Owner", "password123")
	spaceA, err := env.core.CreateSpace(env.ctx, spaceAOwner.Id, "Space A", "")
	if err != nil {
		t.Fatalf("create space A: %v", err)
	}
	_ = spaceA // spaceAOwner has roles.manage in spaceA but not spaceB

	spaceBOwner := env.createVerifiedUser(t, "spaceb-owner", "B Owner", "password123")
	_, err = env.core.CreateSpace(env.ctx, spaceBOwner.Id, "Space B", "")
	if err != nil {
		t.Fatalf("create space B: %v", err)
	}
	target := env.createVerifiedUser(t, "spaceb-target", "B Target", "password123")

	_, err = query.PermissionExplanation(
		env.authContextForUser(spaceAOwner), target.Id, nil,
	)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied for cross-space inspection, got %v", err)
	}
}

// TestPermissionExplanation_RoomMustBelongToServer verifies that passing a
// roomID that does not exist on the deployment is rejected. Without this
// check, the API would silently return an empty trace for a nonexistent
// room — confusing and an authorization-shaped contract gap.

func TestPermissionExplanation_Unauthenticated(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	_, err := query.PermissionExplanation(env.unauthContext(), env.testUser.Id, nil)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("expected ErrNotAuthenticated, got %v", err)
	}
}
