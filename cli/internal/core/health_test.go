package core

import (
	"testing"
)

func TestSpaceRBACHealthCheck(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a test user
	user, err := core.CreateUser(ctx, SystemActorID, "health-test-user", "Health Test", "password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	t.Run("no spaces returns zero counts", func(t *testing.T) {
		report, err := core.SpaceRBACHealthCheck(ctx)
		if err != nil {
			t.Fatalf("SpaceRBACHealthCheck failed: %v", err)
		}

		// DM space exists but should be skipped
		if report.SpacesChecked != 0 {
			t.Errorf("expected 0 spaces checked (DM space skipped), got %d", report.SpacesChecked)
		}
		if report.SpacesInitialized != 0 {
			t.Errorf("expected 0 spaces initialized, got %d", report.SpacesInitialized)
		}
		if len(report.Errors) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(report.Errors), report.Errors)
		}
	})

	t.Run("space with RBAC initialized reports as checked but not initialized", func(t *testing.T) {
		// Create a space (which initializes RBAC)
		_, err := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")
		if err != nil {
			t.Fatalf("CreateSpace failed: %v", err)
		}

		report, err := core.SpaceRBACHealthCheck(ctx)
		if err != nil {
			t.Fatalf("SpaceRBACHealthCheck failed: %v", err)
		}

		if report.SpacesChecked != 1 {
			t.Errorf("expected 1 space checked, got %d", report.SpacesChecked)
		}
		if report.SpacesInitialized != 0 {
			t.Errorf("expected 0 spaces initialized (already initialized), got %d", report.SpacesInitialized)
		}
		if len(report.Errors) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(report.Errors), report.Errors)
		}

		// Verify owner role exists via public API
		_, err = core.GetInstanceRole(ctx, RoleOwner)
		if err != nil {
			t.Errorf("expected owner role to exist, got error: %v", err)
		}
	})

	t.Run("multiple spaces are all checked", func(t *testing.T) {
		// Create another space
		_, err := core.CreateSpace(ctx, user.Id, "Test Space 2", "Another test space")
		if err != nil {
			t.Fatalf("CreateSpace failed: %v", err)
		}

		report, err := core.SpaceRBACHealthCheck(ctx)
		if err != nil {
			t.Fatalf("SpaceRBACHealthCheck failed: %v", err)
		}

		// Should have checked 2 spaces
		if report.SpacesChecked != 2 {
			t.Errorf("expected 2 spaces checked, got %d", report.SpacesChecked)
		}
		if report.SpacesInitialized != 0 {
			t.Errorf("expected 0 spaces initialized (all already initialized), got %d", report.SpacesInitialized)
		}
	})

	t.Run("is idempotent - running twice produces same results", func(t *testing.T) {
		// Run health check twice
		report1, err := core.SpaceRBACHealthCheck(ctx)
		if err != nil {
			t.Fatalf("SpaceRBACHealthCheck (1) failed: %v", err)
		}

		report2, err := core.SpaceRBACHealthCheck(ctx)
		if err != nil {
			t.Fatalf("SpaceRBACHealthCheck (2) failed: %v", err)
		}

		if report1.SpacesChecked != report2.SpacesChecked {
			t.Errorf("expected same spaces checked, got %d vs %d", report1.SpacesChecked, report2.SpacesChecked)
		}
		if report1.SpacesInitialized != report2.SpacesInitialized {
			t.Errorf("expected same spaces initialized, got %d vs %d", report1.SpacesInitialized, report2.SpacesInitialized)
		}
	})
}

func TestSpaceRBACHealthCheck_SkipsDMSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// DM space is automatically initialized by setupTestCore via initDMSpace
	// Create a regular space to verify it's checked while DM is skipped
	user, err := core.CreateUser(ctx, SystemActorID, "dm-test-user", "DM Test", "password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	_, err = core.CreateSpace(ctx, user.Id, "Regular Space", "A regular space")
	if err != nil {
		t.Fatalf("CreateSpace failed: %v", err)
	}

	// Run health check
	report, err := core.SpaceRBACHealthCheck(ctx)
	if err != nil {
		t.Fatalf("SpaceRBACHealthCheck failed: %v", err)
	}

	// Should only check 1 space (not the DM space)
	if report.SpacesChecked != 1 {
		t.Errorf("expected 1 space checked (DM space should be skipped), got %d", report.SpacesChecked)
	}
}

// TestSpaceRBACHealthCheck_InitializesUninitializedSpace tests that the health check
// initializes RBAC for spaces that are missing their default roles.
// This simulates the migration scenario for existing spaces.
func TestSpaceRBACHealthCheck_InitializesUninitializedSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a test user
	user, err := core.CreateUser(ctx, SystemActorID, "uninit-test-user", "Test", "password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create a space normally (which initializes RBAC)
	_, err = core.CreateSpace(ctx, user.Id, "Test Space", "A test space")
	if err != nil {
		t.Fatalf("CreateSpace failed: %v", err)
	}

	// Manually delete the owner role entry from the KV bucket to simulate an uninitialized space
	// This directly accesses the underlying storage, bypassing the public API
	rbacKV := core.storage.serverRBACKV

	// Delete the owner role key
	if err := rbacKV.Delete(ctx, "role.owner"); err != nil {
		t.Fatalf("Failed to delete owner role key: %v", err)
	}

	// Verify owner role is gone
	_, err = core.GetInstanceRole(ctx, RoleOwner)
	if err == nil {
		t.Fatal("expected owner role to not exist after deletion")
	}

	// Run health check - should detect and initialize the missing RBAC
	report, err := core.SpaceRBACHealthCheck(ctx)
	if err != nil {
		t.Fatalf("SpaceRBACHealthCheck failed: %v", err)
	}

	if report.SpacesChecked != 1 {
		t.Errorf("expected 1 space checked, got %d", report.SpacesChecked)
	}
	if report.SpacesInitialized != 1 {
		t.Errorf("expected 1 space initialized, got %d", report.SpacesInitialized)
	}
	if len(report.Errors) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(report.Errors), report.Errors)
	}

	// Verify owner role now exists
	_, err = core.GetInstanceRole(ctx, RoleOwner)
	if err != nil {
		t.Errorf("expected owner role to exist after health check, got error: %v", err)
	}
}
