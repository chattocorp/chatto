package core

import (
	"bytes"
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

func TestChattoCore_NewCredentialsRecordAuthGeneration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createGenerationZeroPasswordUser(t, core, "recorded-generation-user")

	cookieSession, _, err := core.CreateCookieSession(ctx, userID, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	if cookie := readAuthTokenData(t, core, cookieSession); !cookie.AuthGenerationRecorded {
		t.Fatal("new cookie session does not record its auth generation")
	}

	code, err := core.CreateAuthCode(ctx, userID, "https://example.com/callback", GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"), "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	entry, err := core.storage.runtimeStateKV.Get(ctx, core.authCodeKey(code))
	if err != nil {
		t.Fatalf("get auth code: %v", err)
	}
	var codeData AuthCodeData
	if err := json.Unmarshal(entry.Value(), &codeData); err != nil {
		t.Fatalf("unmarshal auth code: %v", err)
	}
	if !codeData.AuthGenerationRecorded {
		t.Fatal("new auth code does not record its auth generation")
	}

	// Refresh retries compare stored access records byte for byte, also with
	// records from older replicas. Access records must keep the older encoding.
	bearer, err := core.CreateBearerSessionWithSource(ctx, userID, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	accessEntry, err := core.storage.runtimeStateKV.Get(ctx, core.authTokenKey(bearer.AccessToken))
	if err != nil {
		t.Fatalf("get access record: %v", err)
	}
	if bytes.Contains(accessEntry.Value(), []byte("auth_generation")) {
		t.Fatalf("generation-0 access record encoding changed: %s", accessEntry.Value())
	}
}

// A login that checks the old password while a reset is in progress can store
// a generation-0 cookie session after the password event timestamp. The new
// generation must revoke that session. Only records that do not record their
// auth generation can use the legacy CreatedAt comparison.
func TestChattoCore_PasswordChangeRevokesGenerationZeroSessionFromConcurrentLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createGenerationZeroPasswordUser(t, core, "generation-zero-user")
	if err := core.SetPasswordHash(ctx, userID, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	storeSession := func(t *testing.T, recorded bool) string {
		t.Helper()
		now := time.Now()
		data, err := json.Marshal(AuthTokenData{
			UserID:                 userID,
			Kind:                   AuthTokenKindFirstPartySession,
			Presentation:           AuthTokenPresentationCookie,
			CreatedAt:              now,
			ExpiresAt:              now.Add(time.Hour),
			AuthGenerationRecorded: recorded,
		})
		if err != nil {
			t.Fatalf("marshal session: %v", err)
		}
		sessionID := NewAuthToken()
		if _, err := core.storage.runtimeStateKV.Create(ctx, core.authTokenKey(sessionID), data, jetstream.KeyTTL(time.Hour)); err != nil {
			t.Fatalf("store session: %v", err)
		}
		return sessionID
	}

	raced := storeSession(t, true)
	if _, err := core.ValidateCookieCredential(ctx, raced); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("generation-0 session after password change: err = %v, want ErrCookieSessionNotFound", err)
	}

	legacy := storeSession(t, false)
	if _, err := core.ValidateCookieCredential(ctx, legacy); err != nil {
		t.Fatalf("legacy session created after the password event should stay valid: %v", err)
	}
	if upgraded := readAuthTokenData(t, core, legacy); upgraded.AuthGeneration == 0 {
		t.Fatal("legacy session was not upgraded to the current auth generation")
	}
}

// An authorization code that a login writes at generation 0 while a reset is in
// progress must not be exchanged after the reset.
func TestChattoCore_PasswordChangeRevokesGenerationZeroAuthCodeFromConcurrentLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	userID := createGenerationZeroPasswordUser(t, core, "generation-zero-code-user")
	if err := core.SetPasswordHash(ctx, userID, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	redirectURI := "https://example.com/callback"
	code := NewAuthCode()
	data, err := json.Marshal(AuthCodeData{
		UserID:                 userID,
		RedirectURI:            redirectURI,
		CodeChallenge:          GenerateCodeChallenge(verifier),
		CodeChallengeMethod:    "S256",
		CreatedAt:              time.Now(),
		AuthGenerationRecorded: true,
	})
	if err != nil {
		t.Fatalf("marshal auth code: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, core.authCodeKey(code), data, jetstream.KeyTTL(authCodeTTL)); err != nil {
		t.Fatalf("store auth code: %v", err)
	}

	if _, _, err := core.ExchangeAuthCode(ctx, code, verifier, redirectURI); !errors.Is(err, ErrAuthCodeNotFound) {
		t.Fatalf("ExchangeAuthCode err = %v, want ErrAuthCodeNotFound", err)
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
