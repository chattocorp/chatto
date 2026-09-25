package core

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// withoutAuthGenerationKey returns a stored-record JSON value without the
// auth_generation key, like a record written before auth generations existed.
func withoutAuthGenerationKey(t *testing.T, data []byte) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("unmarshal record fields: %v", err)
	}
	delete(fields, "auth_generation")
	stripped, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal record fields: %v", err)
	}
	return stripped
}

func TestAuthTokenDataRecordsAuthGenerationPresence(t *testing.T) {
	data, err := json.Marshal(AuthTokenData{UserID: "user"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var current AuthTokenData
	if err := json.Unmarshal(data, &current); err != nil {
		t.Fatalf("unmarshal current: %v", err)
	}
	if current.runtimeCredential().MayPredateAuthGeneration {
		t.Fatalf("record with explicit generation 0 is treated as legacy: %s", data)
	}

	var legacy AuthTokenData
	if err := json.Unmarshal(withoutAuthGenerationKey(t, data), &legacy); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if !legacy.runtimeCredential().MayPredateAuthGeneration {
		t.Fatal("record without auth_generation is not treated as legacy")
	}
	if legacy.UserID != "user" {
		t.Fatalf("legacy UserID = %q, want user", legacy.UserID)
	}

	// A session store decodes and encodes records again. That must not turn a
	// legacy record into a current generation-0 record.
	reencoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	var roundTripped AuthTokenData
	if err := json.Unmarshal(reencoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round trip: %v", err)
	}
	if !roundTripped.runtimeCredential().MayPredateAuthGeneration {
		t.Fatalf("re-encoded legacy record lost its legacy form: %s", reencoded)
	}

	legacy.AuthGeneration = 42
	upgraded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal upgraded: %v", err)
	}
	var decodedUpgrade AuthTokenData
	if err := json.Unmarshal(upgraded, &decodedUpgrade); err != nil {
		t.Fatalf("unmarshal upgraded: %v", err)
	}
	if decodedUpgrade.AuthGeneration != 42 || decodedUpgrade.runtimeCredential().MayPredateAuthGeneration {
		t.Fatalf("upgraded record = %s, want generation 42 and not legacy", upgraded)
	}
}

func TestAuthCodeDataRecordsAuthGenerationPresence(t *testing.T) {
	data, err := json.Marshal(AuthCodeData{UserID: "user", AuthGeneration: 7})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var current AuthCodeData
	if err := json.Unmarshal(data, &current); err != nil {
		t.Fatalf("unmarshal current: %v", err)
	}
	if current.legacyAuthGeneration || current.AuthGeneration != 7 {
		t.Fatalf("current code = legacy %v generation %d, want false 7", current.legacyAuthGeneration, current.AuthGeneration)
	}

	var legacy AuthCodeData
	if err := json.Unmarshal(withoutAuthGenerationKey(t, data), &legacy); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if !legacy.legacyAuthGeneration || legacy.AuthGeneration != 0 {
		t.Fatalf("legacy code = legacy %v generation %d, want true 0", legacy.legacyAuthGeneration, legacy.AuthGeneration)
	}
}

// A login that checks the old password while a reset is in progress can store
// a generation-0 cookie session after the reset event timestamp. The reset must
// revoke that session. Only records without the auth_generation key can use the
// legacy CreatedAt comparison.
func TestChattoCore_PasswordResetRevokesGenerationZeroSessionFromConcurrentLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "generation-zero-user", "Generation Zero User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.SetInitialPasswordHash(ctx, user.Id, "oldpassword123"); err != nil {
		t.Fatalf("SetInitialPasswordHash: %v", err)
	}
	if generation := mustCurrentAuthGeneration(t, core, user.Id); generation != 0 {
		t.Fatalf("auth generation before reset = %d, want 0", generation)
	}
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	storeSession := func(t *testing.T, legacy bool) string {
		t.Helper()
		now := time.Now()
		data, err := json.Marshal(AuthTokenData{
			UserID:       user.Id,
			Kind:         AuthTokenKindFirstPartySession,
			Presentation: AuthTokenPresentationCookie,
			CreatedAt:    now,
			ExpiresAt:    now.Add(time.Hour),
		})
		if err != nil {
			t.Fatalf("marshal session: %v", err)
		}
		if legacy {
			data = withoutAuthGenerationKey(t, data)
		}
		sessionID := NewAuthToken()
		if _, err := core.storage.runtimeStateKV.Create(ctx, core.authTokenKey(sessionID), data, jetstream.KeyTTL(time.Hour)); err != nil {
			t.Fatalf("store session: %v", err)
		}
		return sessionID
	}

	raced := storeSession(t, false)
	if _, err := core.ValidateCookieCredential(ctx, raced); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("generation-0 session after reset: err = %v, want ErrCookieSessionNotFound", err)
	}

	legacy := storeSession(t, true)
	if _, err := core.ValidateCookieCredential(ctx, legacy); err != nil {
		t.Fatalf("legacy session created after the password event should stay valid: %v", err)
	}
	if upgraded := readAuthTokenData(t, core, legacy); upgraded.AuthGeneration == 0 || upgraded.runtimeCredential().MayPredateAuthGeneration {
		t.Fatalf("legacy session was not upgraded: generation %d", upgraded.AuthGeneration)
	}
}

// An authorization code that a login writes at generation 0 while a reset is in
// progress must not be exchanged after the reset.
func TestChattoCore_PasswordResetRevokesGenerationZeroAuthCodeFromConcurrentLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "generation-zero-code-user", "Generation Zero Code User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.SetInitialPasswordHash(ctx, user.Id, "oldpassword123"); err != nil {
		t.Fatalf("SetInitialPasswordHash: %v", err)
	}
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	redirectURI := "https://example.com/callback"
	code := NewAuthCode()
	data, err := json.Marshal(AuthCodeData{
		UserID:              user.Id,
		RedirectURI:         redirectURI,
		CodeChallenge:       GenerateCodeChallenge(verifier),
		CodeChallengeMethod: "S256",
		CreatedAt:           time.Now(),
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
