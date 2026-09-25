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
