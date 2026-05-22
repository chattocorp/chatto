package core

import (
	"errors"
	"testing"
)

func TestATProtoDIDLink_LookupAndIdempotence(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "alice", "Alice", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	const did = "did:plc:abc123example"

	// Initially not linked.
	got, err := core.GetUserByATProtoDID(ctx, did)
	if err != nil {
		t.Fatalf("GetUserByATProtoDID pre-link: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil pre-link, got %v", got)
	}

	// Link, then look up.
	if err := core.LinkATProtoDID(ctx, did, user.Id); err != nil {
		t.Fatalf("LinkATProtoDID: %v", err)
	}

	got, err = core.GetUserByATProtoDID(ctx, did)
	if err != nil {
		t.Fatalf("GetUserByATProtoDID post-link: %v", err)
	}
	if got == nil || got.Id != user.Id {
		t.Fatalf("expected user %s, got %v", user.Id, got)
	}

	// Re-linking the same DID to the same user is a no-op.
	if err := core.LinkATProtoDID(ctx, did, user.Id); err != nil {
		t.Fatalf("LinkATProtoDID (idempotent): %v", err)
	}
}

func TestATProtoDIDLink_RejectsRelinkToDifferentUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	alice, err := core.CreateUser(ctx, "system", "alice", "Alice", "")
	if err != nil {
		t.Fatalf("CreateUser alice: %v", err)
	}
	bob, err := core.CreateUser(ctx, "system", "bob", "Bob", "")
	if err != nil {
		t.Fatalf("CreateUser bob: %v", err)
	}

	const did = "did:plc:shareddid"

	if err := core.LinkATProtoDID(ctx, did, alice.Id); err != nil {
		t.Fatalf("LinkATProtoDID alice: %v", err)
	}

	err = core.LinkATProtoDID(ctx, did, bob.Id)
	if !errors.Is(err, ErrATProtoDIDAlreadyClaimed) {
		t.Fatalf("expected ErrATProtoDIDAlreadyClaimed, got %v", err)
	}

	// Alice still owns the DID.
	got, err := core.GetUserByATProtoDID(ctx, did)
	if err != nil {
		t.Fatalf("GetUserByATProtoDID after rejected relink: %v", err)
	}
	if got == nil || got.Id != alice.Id {
		t.Fatalf("expected alice (%s) to still own DID, got %v", alice.Id, got)
	}
}
