package core

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// createGenerationZeroPasswordUser returns a user whose auth generation stays
// 0 after the first password, because adding a first password preserves
// existing credentials.
func createGenerationZeroPasswordUser(t *testing.T, core *ChattoCore, login string) string {
	t.Helper()
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, login, login, "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.SetInitialPasswordHash(ctx, user.Id, "oldpassword123"); err != nil {
		t.Fatalf("SetInitialPasswordHash: %v", err)
	}
	if generation := mustCurrentAuthGeneration(t, core, user.Id); generation != 0 {
		t.Fatalf("auth generation = %d, want 0", generation)
	}
	return user.Id
}

// A login that checks the old password while a reset is in progress can store
// a generation-0 cookie session after the password event. The new generation
// must revoke that session, whatever its CreatedAt time.
func TestChattoCore_PasswordChangeRevokesGenerationZeroSessionFromConcurrentLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createGenerationZeroPasswordUser(t, core, "generation-zero-user")
	if err := core.SetPasswordHash(ctx, userID, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	now := time.Now()
	data, err := json.Marshal(AuthTokenData{
		UserID:       userID,
		Kind:         AuthTokenKindFirstPartySession,
		Presentation: AuthTokenPresentationCookie,
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	sessionID := NewAuthToken()
	if _, err := core.storage.runtimeStateKV.Create(ctx, core.authTokenKey(sessionID), data, jetstream.KeyTTL(time.Hour)); err != nil {
		t.Fatalf("store session: %v", err)
	}

	if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("generation-0 session after password change: err = %v, want ErrCookieSessionNotFound", err)
	}
}

// A logout that presents a credential from before a password change must not
// count as a live logout, because the handler then terminates every current
// session of the user.
func TestChattoCore_LogoutWithStaleCredentialAfterPasswordChangeIsNotLive(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "stale-logout-user", "Stale Logout User", "oldpassword123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	stale, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource stale: %v", err)
	}
	staleRefresh, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource stale refresh: %v", err)
	}
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	current, err := core.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource current: %v", err)
	}

	if userID, revoked, err := core.RevokePresentedRuntimeCredentialWithReason(ctx, stale.AccessToken, AuthTokenPresentationBearer, "logout"); err != nil || revoked || userID != "" {
		t.Fatalf("stale access logout = (%q, %v, %v), want (\"\", false, nil)", userID, revoked, err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.authTokenKey(stale.AccessToken)); !isRuntimeStateKeyAbsent(err) {
		t.Fatalf("stale access record still stored: %v", err)
	}
	if userID, revoked, err := core.RevokeRefreshTokenWithReasonResult(ctx, staleRefresh.RefreshToken, "logout"); err != nil || revoked || userID != "" {
		t.Fatalf("stale refresh logout = (%q, %v, %v), want (\"\", false, nil)", userID, revoked, err)
	}

	if userID, revoked, err := core.RevokeRefreshTokenWithReasonResult(ctx, current.RefreshToken, "logout"); err != nil || !revoked || userID != user.Id {
		t.Fatalf("current refresh logout = (%q, %v, %v), want (%q, true, nil)", userID, revoked, err, user.Id)
	}
}
