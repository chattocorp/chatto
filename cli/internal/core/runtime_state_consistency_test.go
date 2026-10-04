package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// These tests simulate a NATS follower that lags behind committed writes.
// RUNTIME_STATE allows direct gets, so any replica can answer a fast read.

// laggingReplicaKV answers Get as a DirectGet served by a NATS follower that
// has not applied later writes: a key in stale returns its older entry, or
// jetstream.ErrKeyNotFound when that entry is nil.
type laggingReplicaKV struct {
	jetstream.KeyValue
	stale map[string]jetstream.KeyValueEntry
}

func (kv laggingReplicaKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	if entry, ok := kv.stale[key]; ok {
		if entry == nil {
			return nil, jetstream.ErrKeyNotFound
		}
		return entry, nil
	}
	return kv.KeyValue.Get(ctx, key)
}

// lagReplica makes later direct reads of RUNTIME_STATE use stale. Reads
// through the stream leader are unaffected.
func lagReplica(t *testing.T, core *ChattoCore, stale map[string]jetstream.KeyValueEntry) {
	t.Helper()
	core.storage.runtimeStateKV = bindTestKeyValue(t, core.js, laggingReplicaKV{KeyValue: core.storage.runtimeStateKV.KeyValue, stale: stale})
}

func TestChattoCore_RefreshBearerSessionIgnoresLaggingReplicaReads(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "lagging-replica-user", "Lagging Replica User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	sessionID, _, ok := chattoCore.parseRefreshToken(initial.RefreshToken)
	if !ok {
		t.Fatal("initial refresh credential did not parse")
	}
	sessionKey := chattoCore.renewableSessionKey(sessionID)
	stale, err := chattoCore.storage.runtimeStateKV.Get(ctx, sessionKey)
	if err != nil {
		t.Fatalf("get initial renewable session: %v", err)
	}
	lagReplica(t, chattoCore, map[string]jetstream.KeyValueEntry{sessionKey: stale})

	// A follower that still holds generation 0 must neither fail the rotation's
	// confirmation nor make the next refresh look like token reuse.
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession with a lagging replica: %v", err)
	}
	if _, generation, ok := chattoCore.parseRefreshToken(rotated.RefreshToken); !ok || generation != 1 {
		t.Fatalf("rotated refresh generation = %d, %v; want 1", generation, ok)
	}
	next, err := chattoCore.RefreshBearerSession(ctx, rotated.RefreshToken, testRefreshRequestIDB, "")
	if err != nil {
		t.Fatalf("second RefreshBearerSession with a lagging replica: %v", err)
	}
	if got, err := chattoCore.ValidateAuthToken(ctx, next.AccessToken); err != nil || got != user.Id {
		t.Fatalf("ValidateAuthToken after second rotation = %q, %v", got, err)
	}
}

func TestChattoCore_AccessValidationIgnoresLaggingReplicaReads(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "lagging-access-user", "Lagging Access User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	initial, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	sessionID, _, ok := chattoCore.parseRefreshToken(initial.RefreshToken)
	if !ok {
		t.Fatal("initial refresh credential did not parse")
	}
	sessionKey := chattoCore.renewableSessionKey(sessionID)
	stale, err := chattoCore.storage.runtimeStateKV.Get(ctx, sessionKey)
	if err != nil {
		t.Fatalf("get initial renewable session: %v", err)
	}
	rotated, err := chattoCore.RefreshBearerSession(ctx, initial.RefreshToken, testRefreshRequestIDA, "")
	if err != nil {
		t.Fatalf("RefreshBearerSession: %v", err)
	}

	// The follower has neither the new access record nor the new generation.
	accessKey := chattoCore.authTokenKey(rotated.AccessToken)
	lagReplica(t, chattoCore, map[string]jetstream.KeyValueEntry{sessionKey: stale, accessKey: nil})

	if got, err := chattoCore.ValidateAuthToken(ctx, rotated.AccessToken); err != nil || got != user.Id {
		t.Fatalf("ValidateAuthToken with a lagging replica = %q, %v", got, err)
	}
	if _, err := chattoCore.storage.runtimeStateKV.Get(ctx, accessKey); err != nil {
		t.Fatalf("validation removed the new access record: %v", err)
	}
}

func TestChattoCore_ExchangeAuthCodeIgnoresLaggingReplicaMiss(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "lagging-code-user", "Lagging Code User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	redirectURI := "https://example.com/callback"
	code, err := chattoCore.CreateAuthCode(ctx, user.Id, redirectURI, GenerateCodeChallenge(verifier), "S256")
	if err != nil {
		t.Fatalf("CreateAuthCode: %v", err)
	}
	lagReplica(t, chattoCore, map[string]jetstream.KeyValueEntry{chattoCore.authCodeKey(code): nil})

	if _, _, err := chattoCore.ExchangeAuthCode(ctx, code, verifier, redirectURI); err != nil {
		t.Fatalf("ExchangeAuthCode with a lagging replica: %v", err)
	}
}

// staleRuntimeEntry returns the current entry for key with change applied to
// its JSON value, as a lagging replica can answer.
func staleRuntimeEntry[T any](t *testing.T, core *ChattoCore, key string, change func(*T)) jetstream.KeyValueEntry {
	t.Helper()
	entry, err := core.storage.runtimeStateKV.Get(testContext(t), key)
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	var value T
	if err := json.Unmarshal(entry.Value(), &value); err != nil {
		t.Fatalf("decode %s: %v", key, err)
	}
	change(&value)
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode %s: %v", key, err)
	}
	return benchmarkKVEntry{key: key, value: data, revision: entry.Revision()}
}

func TestChattoCore_CookieValidationIgnoresLaggingExpiry(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "lagging-cookie-user", "Lagging Cookie User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, _, err := chattoCore.CreateCookieSession(ctx, user.Id, "test_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	key := chattoCore.authTokenKey(sessionID)
	current, err := chattoCore.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get cookie session: %v", err)
	}
	// The follower still shows the window that a renewal already moved.
	lagReplica(t, chattoCore, map[string]jetstream.KeyValueEntry{
		key: staleRuntimeEntry(t, chattoCore, key, func(data *AuthTokenData) {
			data.ExpiresAt = time.Now().Add(-time.Minute)
		}),
	})

	if _, err := chattoCore.ValidateCookieCredential(ctx, sessionID); err != nil {
		t.Fatalf("ValidateCookieCredential with a lagging expiry: %v", err)
	}
	if after, err := chattoCore.storage.runtimeStateKV.Get(ctx, key); err != nil || after.Revision() != current.Revision() {
		t.Fatalf("cookie session after validation = %v, %v; want it unchanged", after, err)
	}
	loaded, err := chattoCore.LoadCookieSessionValue(ctx, sessionID, time.Now())
	if err != nil || loaded.Revision != current.Revision() {
		t.Fatalf("LoadCookieSessionValue = revision %d, %v; want %d", loaded.Revision, err, current.Revision())
	}
}

func TestChattoCore_FreshAuthIgnoresLaggingReplica(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "lagging-fresh-user", "Lagging Fresh User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bearer, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	sessionID, _, ok := chattoCore.parseRefreshToken(bearer.RefreshToken)
	if !ok {
		t.Fatal("refresh credential did not parse")
	}
	cookieID, _, err := chattoCore.CreateCookieSession(ctx, user.Id, "test_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}

	// The follower has not applied the re-verification that just made both
	// sessions fresh.
	expired := time.Now().Add(-FreshAuthWindow - time.Minute)
	sessionKey := chattoCore.renewableSessionKey(sessionID)
	cookieKey := chattoCore.authTokenKey(cookieID)
	lagReplica(t, chattoCore, map[string]jetstream.KeyValueEntry{
		sessionKey: staleRuntimeEntry(t, chattoCore, sessionKey, func(session *RenewableSession) { session.FreshAuthAt = expired }),
		cookieKey:  staleRuntimeEntry(t, chattoCore, cookieKey, func(data *AuthTokenData) { data.FreshAuthAt = expired }),
	})

	if err := chattoCore.RequireFreshAuthForBearerToken(ctx, bearer.AccessToken); err != nil {
		t.Fatalf("RequireFreshAuthForBearerToken with a lagging replica: %v", err)
	}
	if err := chattoCore.RequireFreshAuthForCookieSession(ctx, cookieID); err != nil {
		t.Fatalf("RequireFreshAuthForCookieSession with a lagging replica: %v", err)
	}
}

func TestChattoCore_ValidAccessTokenDoesNotReadThroughLeader(t *testing.T) {
	chattoCore, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := chattoCore.CreateUser(ctx, SystemActorID, "fast-access-user", "Fast Access User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bearer, err := chattoCore.CreateBearerSessionWithSource(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateBearerSessionWithSource: %v", err)
	}
	accessKey := chattoCore.authTokenKey(bearer.AccessToken)
	leaderReads, _ := countKeyValueReads(t, nc, "RUNTIME_STATE", accessKey)

	for range 3 {
		if got, err := chattoCore.ValidateAuthToken(ctx, bearer.AccessToken); err != nil || got != user.Id {
			t.Fatalf("ValidateAuthToken = %q, %v", got, err)
		}
	}
	// The counter sees one leader read; a validation read would add more.
	if _, err := chattoCore.storage.runtimeStateKV.Get(ctx, accessKey); err != nil {
		t.Fatalf("leader read: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for leaderReads() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if reads := leaderReads(); reads != 1 {
		t.Fatalf("leader reads of the access record = %d, want only the control read", reads)
	}
}
