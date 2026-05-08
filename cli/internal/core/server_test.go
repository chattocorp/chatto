package core

import (
	"strings"
	"testing"
)

func TestResolveServerSpaceID_ZeroSpaces(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Fresh-install case: only the DM space exists (created by core init).
	got, err := core.resolveServerSpaceID(ctx)
	if err != nil {
		t.Fatalf("resolveServerSpaceID: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty server space on fresh install, got %q", got)
	}
}

func TestResolveServerSpaceID_SingleSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Engineering", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	got, err := core.resolveServerSpaceID(ctx)
	if err != nil {
		t.Fatalf("resolveServerSpaceID: %v", err)
	}
	if got != space.Id {
		t.Errorf("expected resolved %q, got %q", space.Id, got)
	}
}

func TestResolveServerSpaceID_MultipleSpaces(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if _, err := core.CreateSpace(ctx, "test-user", "Engineering", ""); err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if _, err := core.CreateSpace(ctx, "test-user", "Lounge", ""); err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	_, err := core.resolveServerSpaceID(ctx)
	if err == nil {
		t.Fatal("expected error for multiple user-facing spaces, got nil")
	}
	if !strings.Contains(err.Error(), "multiple user-facing spaces") {
		t.Errorf("expected 'multiple user-facing spaces' in error, got %v", err)
	}
}

func TestInitServerSpaceID_CachesResult(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Engineering", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	if err := core.InitServerSpaceID(ctx); err != nil {
		t.Fatalf("InitServerSpaceID: %v", err)
	}
	if got := core.ServerSpaceID(); got != space.Id {
		t.Errorf("expected cached %q, got %q", space.Id, got)
	}
}
