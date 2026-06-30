package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
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
	if data := readAuthTokenData(t, core, token); data.Kind != AuthTokenKindFirstPartySession || data.Presentation != AuthTokenPresentationBearer {
		t.Fatalf("auth token kind/presentation = %q/%q, want %q/%q", data.Kind, data.Presentation, AuthTokenKindFirstPartySession, AuthTokenPresentationBearer)
	}
	assertRuntimeKVHasTTL(t, core, key)
	assertRawRuntimeTokenKeyAbsent(t, core, authTokenKeyPrefix+token)
}

func TestChattoCore_BearerTokenFreshAuth(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "fresh-auth-token-user", "Fresh Auth Token User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := core.CreateAuthTokenWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource: %v", err)
	}
	if err := core.RequireFreshAuthForBearerToken(ctx, token); err != nil {
		t.Fatalf("new token should be fresh: %v", err)
	}

	key := core.authTokenKey(token)
	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	var data AuthTokenData
	if err := json.Unmarshal(entry.Value(), &data); err != nil {
		t.Fatalf("unmarshal token: %v", err)
	}
	data.FreshAuthAt = time.Now().Add(-FreshAuthWindow - time.Minute)
	staleValue, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal stale token: %v", err)
	}
	if _, err := core.updateRuntimeStateTokenTTL(ctx, key, staleValue, entry.Revision(), core.authTokenTTL()); err != nil {
		t.Fatalf("write stale token: %v", err)
	}
	if err := core.RequireFreshAuthForBearerToken(ctx, token); !errors.Is(err, ErrFreshAuthRequired) {
		t.Fatalf("stale token fresh auth err = %v, want ErrFreshAuthRequired", err)
	}
	if err := core.MarkBearerTokenFresh(ctx, token, "password", "current_password"); err != nil {
		t.Fatalf("MarkBearerTokenFresh: %v", err)
	}
	if err := core.RequireFreshAuthForBearerToken(ctx, token); err != nil {
		t.Fatalf("marked token should be fresh: %v", err)
	}
}

func TestChattoCore_OAuthAccessTokenCannotBecomeFresh(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "oauth-access-token-user", "OAuth Access Token User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := core.CreateAuthTokenWithSource(ctx, user.Id, "oauth_code_exchange")
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSource: %v", err)
	}
	if data := readAuthTokenData(t, core, token); data.Kind != AuthTokenKindOAuthAccessToken || data.Presentation != AuthTokenPresentationBearer {
		t.Fatalf("auth token kind/presentation = %q/%q, want %q/%q", data.Kind, data.Presentation, AuthTokenKindOAuthAccessToken, AuthTokenPresentationBearer)
	}
	if err := core.MarkBearerTokenFresh(ctx, token, "password", "current_password"); !errors.Is(err, ErrFreshAuthRequired) {
		t.Fatalf("MarkBearerTokenFresh err = %v, want ErrFreshAuthRequired", err)
	}
	if err := core.RequireFreshAuthForBearerToken(ctx, token); !errors.Is(err, ErrFreshAuthRequired) {
		t.Fatalf("RequireFreshAuthForBearerToken err = %v, want ErrFreshAuthRequired", err)
	}
	if data := readAuthTokenData(t, core, token); !data.FreshAuthAt.IsZero() || data.FreshAuthMethod != "" || data.FreshAuthSource != "" {
		t.Fatalf("OAuth access token was marked fresh: %+v", data)
	}
}

func TestChattoCore_LegacyUntypedBearerTokenCannotBecomeFresh(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "legacy-untyped-token-user", "Legacy Untyped Token User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token := NewAuthToken()
	data, err := json.Marshal(AuthTokenData{
		UserID:         user.Id,
		CreatedAt:      time.Now(),
		AuthGeneration: mustCurrentAuthGeneration(t, core, user.Id),
	})
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, core.authTokenKey(token), data, jetstream.KeyTTL(core.authTokenTTL())); err != nil {
		t.Fatalf("store token: %v", err)
	}

	if err := core.MarkBearerTokenFresh(ctx, token, "password", "current_password"); !errors.Is(err, ErrFreshAuthRequired) {
		t.Fatalf("MarkBearerTokenFresh err = %v, want ErrFreshAuthRequired", err)
	}
	if err := core.RequireFreshAuthForBearerToken(ctx, token); !errors.Is(err, ErrFreshAuthRequired) {
		t.Fatalf("RequireFreshAuthForBearerToken err = %v, want ErrFreshAuthRequired", err)
	}
}

func TestChattoCore_LegacyFreshBearerTokenCanSatisfyExistingFreshWindow(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "legacy-fresh-token-user", "Legacy Fresh Token User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token := NewAuthToken()
	data, err := json.Marshal(AuthTokenData{
		UserID:          user.Id,
		CreatedAt:       time.Now(),
		AuthGeneration:  mustCurrentAuthGeneration(t, core, user.Id),
		FreshAuthAt:     time.Now(),
		FreshAuthMethod: "password",
		FreshAuthSource: "password_login",
	})
	if err != nil {
		t.Fatalf("marshal token: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, core.authTokenKey(token), data, jetstream.KeyTTL(core.authTokenTTL())); err != nil {
		t.Fatalf("store token: %v", err)
	}

	if err := core.RequireFreshAuthForBearerToken(ctx, token); err != nil {
		t.Fatalf("RequireFreshAuthForBearerToken: %v", err)
	}
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

func TestChattoCore_RevokeAllAuthTokensForUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "revoke-all-user", "Revoke All User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	otherUser, err := core.CreateUser(ctx, "", "revoke-all-other", "Revoke All Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}

	token1, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken 1: %v", err)
	}
	token2, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken 2: %v", err)
	}
	otherToken, err := core.CreateAuthToken(ctx, otherUser.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken other: %v", err)
	}

	revoked, err := core.RevokeAllAuthTokensForUserWithReason(ctx, user.Id, "password_reset")
	if err != nil {
		t.Fatalf("RevokeAllAuthTokensForUserWithReason: %v", err)
	}
	if revoked != 2 {
		t.Fatalf("revoked = %d, want 2", revoked)
	}

	if _, err := core.ValidateAuthToken(ctx, token1); err != ErrAuthTokenNotFound {
		t.Fatalf("token1 ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
	}
	if _, err := core.ValidateAuthToken(ctx, token2); err != ErrAuthTokenNotFound {
		t.Fatalf("token2 ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
	}
	if gotUserID, err := core.ValidateAuthToken(ctx, otherToken); err != nil {
		t.Fatalf("other token should remain valid: %v", err)
	} else if gotUserID != otherUser.Id {
		t.Fatalf("other token user ID = %q, want %q", gotUserID, otherUser.Id)
	}
}

func TestChattoCore_AuthTokenGenerationRejectsStaleAuthentication(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "generation-token-user", "Generation Token User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	token, err := core.CreateAuthTokenWithSourceGeneration(ctx, user.Id, "password_login", authGeneration)
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSourceGeneration: %v", err)
	}

	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	if _, err := core.ValidateAuthToken(ctx, token); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
	}
	if _, err := core.CreateAuthTokenWithSourceGeneration(ctx, user.Id, "password_login", authGeneration); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("stale CreateAuthTokenWithSourceGeneration err = %v, want ErrAuthTokenNotFound", err)
	}
	freshGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration fresh: %v", err)
	}
	if fresh, err := core.CreateAuthTokenWithSourceGeneration(ctx, user.Id, "password_login", freshGeneration); err != nil {
		t.Fatalf("fresh CreateAuthTokenWithSourceGeneration should succeed: %v", err)
	} else if gotUserID, err := core.ValidateAuthToken(ctx, fresh); err != nil {
		t.Fatalf("fresh token should validate: %v", err)
	} else if gotUserID != user.Id {
		t.Fatalf("fresh token user ID = %q, want %q", gotUserID, user.Id)
	}
}

func TestChattoCore_ValidateAuthTokenGrandfathersLegacyGeneration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "legacy-token-user", "Legacy Token User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}

	token := NewAuthToken()
	key := core.authTokenKey(token)
	data, err := json.Marshal(AuthTokenData{
		UserID:    user.Id,
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("marshal legacy token: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(core.authTokenTTL())); err != nil {
		t.Fatalf("store legacy token: %v", err)
	}

	gotUserID, err := core.ValidateAuthToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateAuthToken: %v", err)
	}
	if gotUserID != user.Id {
		t.Fatalf("ValidateAuthToken user ID = %q, want %q", gotUserID, user.Id)
	}

	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get upgraded token: %v", err)
	}
	var upgraded AuthTokenData
	if err := json.Unmarshal(entry.Value(), &upgraded); err != nil {
		t.Fatalf("unmarshal upgraded token: %v", err)
	}
	if upgraded.AuthGeneration != authGeneration {
		t.Fatalf("upgraded auth generation = %d, want %d", upgraded.AuthGeneration, authGeneration)
	}
}

func TestChattoCore_ValidateAuthTokenRejectsLegacyGenerationBeforePasswordChange(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "", "legacy-token-stale-user", "Legacy Token Stale User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	legacyCreatedAt := time.Now()
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	token := NewAuthToken()
	key := core.authTokenKey(token)
	data, err := json.Marshal(AuthTokenData{
		UserID:    user.Id,
		CreatedAt: legacyCreatedAt,
	})
	if err != nil {
		t.Fatalf("marshal legacy token: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(core.authTokenTTL())); err != nil {
		t.Fatalf("store legacy token: %v", err)
	}

	if _, err := core.ValidateAuthToken(ctx, token); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("legacy stale token lookup error = %v, want ErrKeyNotFound", err)
	}
}

func readAuthTokenData(t *testing.T, core *ChattoCore, token string) AuthTokenData {
	t.Helper()
	entry, err := core.storage.runtimeStateKV.Get(testContext(t), core.authTokenKey(token))
	if err != nil {
		t.Fatalf("get auth token: %v", err)
	}
	var data AuthTokenData
	if err := json.Unmarshal(entry.Value(), &data); err != nil {
		t.Fatalf("unmarshal auth token: %v", err)
	}
	return data
}

func mustCurrentAuthGeneration(t *testing.T, core *ChattoCore, userID string) uint64 {
	t.Helper()
	authGeneration, err := core.CurrentAuthGeneration(testContext(t), userID)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	return authGeneration
}
