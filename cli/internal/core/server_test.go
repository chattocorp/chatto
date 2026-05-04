package core

import (
	"strings"
	"testing"
)

func TestResolvePrimarySpaceID_ConfiguredAndExists(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Engineering", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	got, err := core.ResolvePrimarySpaceID(ctx, space.Id)
	if err != nil {
		t.Fatalf("ResolvePrimarySpaceID: %v", err)
	}
	if got != space.Id {
		t.Errorf("expected %q, got %q", space.Id, got)
	}
}

func TestResolvePrimarySpaceID_ConfiguredButMissing(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.ResolvePrimarySpaceID(ctx, "Sdoesnotexist123")
	if err == nil {
		t.Fatal("expected error for missing configured space, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got %v", err)
	}
}

func TestResolvePrimarySpaceID_UnsetZeroSpaces(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Fresh-install case: only the DM space exists (created by core init).
	got, err := core.ResolvePrimarySpaceID(ctx, "")
	if err != nil {
		t.Fatalf("ResolvePrimarySpaceID: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty primary on fresh install, got %q", got)
	}
}

func TestResolvePrimarySpaceID_UnsetSingleSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Engineering", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	got, err := core.ResolvePrimarySpaceID(ctx, "")
	if err != nil {
		t.Fatalf("ResolvePrimarySpaceID: %v", err)
	}
	if got != space.Id {
		t.Errorf("expected auto-derived %q, got %q", space.Id, got)
	}
}

func TestResolvePrimarySpaceID_UnsetMultipleSpaces(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if _, err := core.CreateSpace(ctx, "test-user", "Engineering", ""); err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if _, err := core.CreateSpace(ctx, "test-user", "Lounge", ""); err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	_, err := core.ResolvePrimarySpaceID(ctx, "")
	if err == nil {
		t.Fatal("expected error for ambiguous unset primary, got nil")
	}
	if !strings.Contains(err.Error(), "multiple spaces") {
		t.Errorf("expected 'multiple spaces' in error, got %v", err)
	}
}
