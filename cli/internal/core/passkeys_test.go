package core

import (
	"errors"
	"testing"
)

func TestChattoCore_PasskeysAreDurableAndUniquelyLinked(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	first, err := core.CreateUser(ctx, SystemActorID, "passkey-first", "Passkey First", "password123")
	if err != nil {
		t.Fatalf("CreateUser first: %v", err)
	}
	second, err := core.CreateUser(ctx, SystemActorID, "passkey-second", "Passkey Second", "password123")
	if err != nil {
		t.Fatalf("CreateUser second: %v", err)
	}

	credentialID := []byte("credential-id")
	if _, err := core.LinkPasskey(ctx, first.GetId(), credentialID, []byte("serialized-credential"), "Laptop"); err != nil {
		t.Fatalf("LinkPasskey: %v", err)
	}
	passkeys, err := core.PasskeysForUser(ctx, first.GetId())
	if err != nil {
		t.Fatalf("PasskeysForUser: %v", err)
	}
	if len(passkeys) != 1 || passkeys[0].Label != "Laptop" || passkeys[0].CredentialHash == "" {
		t.Fatalf("passkeys = %#v", passkeys)
	}
	if _, err := core.LinkPasskey(ctx, second.GetId(), credentialID, []byte("serialized-credential"), "Duplicate"); !errors.Is(err, ErrPasskeyClaimed) {
		t.Fatalf("LinkPasskey duplicate error = %v, want ErrPasskeyClaimed", err)
	}
	if err := core.UnlinkPasskey(ctx, first.GetId(), passkeys[0].CredentialHash); err != nil {
		t.Fatalf("UnlinkPasskey: %v", err)
	}
	passkeys, err = core.PasskeysForUser(ctx, first.GetId())
	if err != nil {
		t.Fatalf("PasskeysForUser after unlink: %v", err)
	}
	if len(passkeys) != 0 {
		t.Fatalf("passkeys after unlink = %#v, want none", passkeys)
	}
}
