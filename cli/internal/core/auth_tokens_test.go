package core

import (
	"strings"
	"testing"
)

func TestChattoCore_CreateAuthToken(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user first
	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create an auth token
	token, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}

	// Validate the token returns the correct user ID
	userID, err := core.ValidateAuthToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateAuthToken failed: %v", err)
	}
	if userID != user.Id {
		t.Errorf("ValidateAuthToken returned userID %q, want %q", userID, user.Id)
	}

	key := core.authTokenKey(token)
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
		t.Fatalf("expected auth token in RUNTIME_STATE: %v", err)
	}
	assertRuntimeKVHasTTL(t, core, key)
	assertRawRuntimeTokenKeyAbsent(t, core, authTokenKeyPrefix+token)
}

func TestChattoCore_ValidateAuthToken_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.ValidateAuthToken(ctx, "cht_ATnonexistent1234")
	if err == nil {
		t.Fatal("ValidateAuthToken should have returned an error for non-existent token")
	}
	if err != ErrAuthTokenNotFound {
		t.Errorf("ValidateAuthToken returned error %v, want ErrAuthTokenNotFound", err)
	}
}

func TestChattoCore_RevokeAuthToken(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create and then revoke a token
	token, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}

	err = core.RevokeAuthToken(ctx, token)
	if err != nil {
		t.Fatalf("RevokeAuthToken failed: %v", err)
	}

	// Token should no longer be valid
	_, err = core.ValidateAuthToken(ctx, token)
	if err != ErrAuthTokenNotFound {
		t.Errorf("ValidateAuthToken after revoke returned error %v, want ErrAuthTokenNotFound", err)
	}
}

func TestChattoCore_RevokeAuthToken_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Revoking a non-existent token should not error
	err := core.RevokeAuthToken(ctx, "cht_ATnonexistent1234")
	if err != nil {
		t.Errorf("RevokeAuthToken for non-existent token returned error: %v", err)
	}
}

func TestChattoCore_AuthTokenFormat(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	token, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken failed: %v", err)
	}

	if !strings.HasPrefix(token, "cht_AT") {
		t.Errorf("Token %q does not start with 'cht_AT'", token)
	}

	// cht_ (4) + AT (2) + nanoid (14) = 20 chars
	if len(token) != 20 {
		t.Errorf("Token length is %d, want 20", len(token))
	}
}

func TestChattoCore_MultipleTokensPerUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create multiple tokens for the same user
	token1, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken (1) failed: %v", err)
	}

	token2, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken (2) failed: %v", err)
	}

	// Both should be valid
	userID1, err := core.ValidateAuthToken(ctx, token1)
	if err != nil {
		t.Fatalf("ValidateAuthToken (1) failed: %v", err)
	}
	userID2, err := core.ValidateAuthToken(ctx, token2)
	if err != nil {
		t.Fatalf("ValidateAuthToken (2) failed: %v", err)
	}

	if userID1 != user.Id || userID2 != user.Id {
		t.Errorf("Tokens should both map to user %q, got %q and %q", user.Id, userID1, userID2)
	}

	// Revoking one should not affect the other
	err = core.RevokeAuthToken(ctx, token1)
	if err != nil {
		t.Fatalf("RevokeAuthToken failed: %v", err)
	}

	_, err = core.ValidateAuthToken(ctx, token1)
	if err != ErrAuthTokenNotFound {
		t.Error("Token1 should be invalid after revocation")
	}

	userID2, err = core.ValidateAuthToken(ctx, token2)
	if err != nil {
		t.Fatalf("Token2 should still be valid, got error: %v", err)
	}
	if userID2 != user.Id {
		t.Errorf("Token2 returned wrong user ID %q, want %q", userID2, user.Id)
	}
}
