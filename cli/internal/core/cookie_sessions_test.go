package core

import (
	"context"
	"encoding/json"
	"errors"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestChattoCore_CreateAndValidateCookieSession(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := WithAuditRequestMetadata(testContext(t), &evtv1.AuditRequestMetadata{
		UserAgent: "cookie-session-test",
		IpHash:    "hashed-ip",
	})

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-session-user", "Cookie Session User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	sessionID, created, err := core.CreateCookieSession(ctx, user.Id, "test_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	if sessionID == "" {
		t.Fatalf("expected session ID")
	}
	if created.GetUserId() != user.Id || created.GetSource() != "test_login" {
		t.Fatalf("unexpected created session: %#v", created)
	}
	if created.GetRequest().GetUserAgent() != "cookie-session-test" || created.GetRequest().GetIpHash() != "hashed-ip" {
		t.Fatalf("unexpected request metadata: %#v", created.GetRequest())
	}
	if created.GetFreshAuthAt() == nil || created.GetFreshAuthMethod() == "" || created.GetFreshAuthSource() != "test_login" {
		t.Fatalf("unexpected fresh auth metadata: %#v", created)
	}

	key := core.authTokenKey(sessionID)
	assertRuntimeKVHasTTL(t, core, key)
	assertRawRuntimeTokenKeyAbsent(t, core, authTokenKeyPrefix+sessionID)

	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get cookie session: %v", err)
	}
	var stored AuthTokenData
	if err := json.Unmarshal(entry.Value(), &stored); err != nil {
		t.Fatalf("unmarshal cookie session token: %v", err)
	}
	if stored.UserID != user.Id ||
		stored.Kind != AuthTokenKindFirstPartySession ||
		stored.Presentation != AuthTokenPresentationCookie ||
		stored.Source != "test_login" {
		t.Fatalf("unexpected stored session: %#v", &stored)
	}
	if _, err := core.ValidatePresentedRuntimeCredential(ctx, sessionID, AuthTokenPresentationBearer); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("cookie session presented as bearer err = %v, want ErrAuthTokenNotFound", err)
	}

	validated, err := core.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
	if validated.GetUserId() != user.Id ||
		validated.GetSource() != "test_login" ||
		validated.GetRequest().GetUserAgent() != "cookie-session-test" ||
		validated.GetAuthGeneration() != stored.AuthGeneration {
		t.Fatalf("validated session differs from stored session token: %#v", validated)
	}

	bearerToken, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, bearerToken); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("bearer token presented as cookie err = %v, want ErrCookieSessionNotFound", err)
	}
}

func TestChattoCore_ValidatingCookieSessionDoesNotRewriteRuntimeState(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "cookie-no-rewrite-user", "Cookie No Rewrite User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, _, err := core.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}

	key := core.authTokenKey(sessionID)
	before, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get cookie session before validation: %v", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, sessionID); err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
	after, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get cookie session after validation: %v", err)
	}
	if after.Revision() != before.Revision() {
		t.Fatalf("cookie validation changed revision from %d to %d", before.Revision(), after.Revision())
	}
}

func TestChattoCore_MigrateLegacyCookieSessionAddsExpiryOnce(t *testing.T) {
	core, _ := setupTestCore(t)
	core.config.AuthTokenTTL = 4 * time.Hour
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "legacy-cookie-user", "Legacy Cookie User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.GetId())
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	sessionID := NewAuthToken()
	legacy := AuthTokenData{
		UserID:          user.GetId(),
		Kind:            AuthTokenKindFirstPartySession,
		Presentation:    AuthTokenPresentationCookie,
		Source:          "password_login",
		CreatedAt:       now.Add(-time.Hour),
		AuthGeneration:  authGeneration,
		FreshAuthAt:     now.Add(-time.Hour),
		FreshAuthMethod: "password",
		FreshAuthSource: "password_login",
	}
	value, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy cookie session: %v", err)
	}
	key := core.authTokenKey(sessionID)
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, value, jetstream.KeyTTL(3*time.Hour)); err != nil {
		t.Fatalf("store legacy cookie session: %v", err)
	}

	const callers = 8
	results := make(chan *runtimestatev1.CookieSession, callers)
	errs := make(chan error, callers)
	var wait sync.WaitGroup
	for range callers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			record, migrateErr := core.MigrateLegacyCookieSession(ctx, sessionID, now)
			results <- record
			errs <- migrateErr
		}()
	}
	wait.Wait()
	close(results)
	close(errs)
	for migrateErr := range errs {
		if migrateErr != nil {
			t.Fatalf("MigrateLegacyCookieSession: %v", migrateErr)
		}
	}
	wantExpiry := now.Add(4 * time.Hour)
	for record := range results {
		if record.GetUserId() != user.GetId() || !record.GetExpiresAt().AsTime().Equal(wantExpiry) {
			t.Fatalf("migrated cookie session = %#v, want user %q and expiry %v", record, user.GetId(), wantExpiry)
		}
	}

	storedEntry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get migrated cookie session: %v", err)
	}
	var stored AuthTokenData
	if err := json.Unmarshal(storedEntry.Value(), &stored); err != nil {
		t.Fatalf("unmarshal migrated cookie session: %v", err)
	}
	if !stored.ExpiresAt.Equal(wantExpiry) || !stored.CreatedAt.Equal(legacy.CreatedAt) || !stored.FreshAuthAt.Equal(legacy.FreshAuthAt) {
		t.Fatalf("migrated cookie session changed preserved fields: %#v", stored)
	}
	assertRuntimeKVHasTTL(t, core, key)
	if _, err := core.ValidateCookieCredential(ctx, sessionID); err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}

	retried, err := core.MigrateLegacyCookieSession(ctx, sessionID, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("MigrateLegacyCookieSession retry: %v", err)
	}
	if !retried.GetExpiresAt().AsTime().Equal(wantExpiry) {
		t.Fatalf("retry extended expiry to %v, want %v", retried.GetExpiresAt(), wantExpiry)
	}
}

func TestChattoCore_MigrateLegacyCookieSessionRejectsRevokedAuthority(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "revoked-legacy-cookie-user", "Revoked Legacy Cookie User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.GetId())
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	if err := core.SetPasswordHash(ctx, user.GetId(), "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	now := time.Now()
	sessionID := NewAuthToken()
	value, err := json.Marshal(AuthTokenData{
		UserID:         user.GetId(),
		Kind:           AuthTokenKindFirstPartySession,
		Presentation:   AuthTokenPresentationCookie,
		CreatedAt:      now,
		AuthGeneration: authGeneration,
	})
	if err != nil {
		t.Fatalf("marshal legacy cookie session: %v", err)
	}
	key := core.authTokenKey(sessionID)
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, value, jetstream.KeyTTL(time.Hour)); err != nil {
		t.Fatalf("store legacy cookie session: %v", err)
	}

	if _, err := core.MigrateLegacyCookieSession(ctx, sessionID, now); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("MigrateLegacyCookieSession err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); !isRuntimeStateKeyAbsent(err) {
		t.Fatalf("revoked legacy cookie session still stored: %v", err)
	}
}

func TestChattoCore_ConcurrentCookieRenewalKeepsOneStableHandle(t *testing.T) {
	core, _ := setupTestCore(t)
	core.config.AuthTokenTTL = 4 * time.Hour
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "cookie-concurrent-renew-user", "Cookie Concurrent Renew User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, created, err := core.CreateCookieSession(ctx, user.GetId(), "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	now := created.GetExpiresAt().AsTime().Add(-30 * time.Minute)
	wantExpiry := now.Add(4 * time.Hour)

	var wait sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _, renewErr := core.RenewCookieSession(ctx, sessionID, now)
			errs <- renewErr
		}()
	}
	wait.Wait()
	close(errs)
	for renewErr := range errs {
		if renewErr != nil {
			t.Fatalf("RenewCookieSession: %v", renewErr)
		}
	}

	renewed, err := core.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
	if !renewed.GetExpiresAt().AsTime().Equal(wantExpiry) {
		t.Fatalf("renewed expiry = %v, want %v", renewed.GetExpiresAt(), wantExpiry)
	}
	assertRuntimeKVHasTTL(t, core, core.authTokenKey(sessionID))
}

func TestChattoCore_LogoutDeleteFencesConcurrentCookieRenewal(t *testing.T) {
	core, _ := setupTestCore(t)
	core.config.AuthTokenTTL = 4 * time.Hour
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "cookie-renew-revoke-user", "Cookie Renew Revoke User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	for attempt := 0; attempt < 20; attempt++ {
		sessionID, created, err := core.CreateCookieSession(ctx, user.GetId(), "password_login")
		if err != nil {
			t.Fatalf("CreateCookieSession attempt %d: %v", attempt, err)
		}
		now := created.GetExpiresAt().AsTime().Add(-30 * time.Minute)
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, _, _ = core.RenewCookieSession(ctx, sessionID, now)
		}()
		go func() {
			defer wait.Done()
			<-start
			_ = core.RevokeCookieSession(ctx, sessionID)
		}()
		close(start)
		wait.Wait()
		if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
			t.Fatalf("attempt %d validation error = %v, want ErrCookieSessionNotFound", attempt, err)
		}
	}
}

func TestChattoCore_CookieSessionExplicitExpiryIsAuthoritative(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "cookie-expiry-user", "Cookie Expiry User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, _, err := core.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}

	key := core.authTokenKey(sessionID)
	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get cookie session: %v", err)
	}
	var data AuthTokenData
	if err := json.Unmarshal(entry.Value(), &data); err != nil {
		t.Fatalf("unmarshal cookie session: %v", err)
	}
	data.ExpiresAt = time.Now().Add(-time.Minute)
	value, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal expired cookie session: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Update(ctx, key, value, entry.Revision()); err != nil {
		t.Fatalf("store expired cookie session: %v", err)
	}

	if err := core.MarkCookieSessionFresh(ctx, sessionID, "password", "current_password"); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("MarkCookieSessionFresh err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("ValidateCookieCredential err = %v, want ErrCookieSessionNotFound", err)
	}
}

func TestMutableCookieSessionRetainsPhysicalTTL(t *testing.T) {
	core, _ := setupTestCore(t)
	core.config.AuthTokenTTL = 2 * time.Second
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "cookie-ttl-user", "Cookie TTL User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, created, err := core.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	if err := core.MarkCookieSessionFresh(ctx, sessionID, "password", "current_password"); err != nil {
		t.Fatalf("MarkCookieSessionFresh: %v", err)
	}
	updated, err := core.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieCredential: %v", err)
	}
	if !updated.GetExpiresAt().AsTime().Equal(created.GetExpiresAt().AsTime()) {
		t.Fatalf("fresh-auth update changed expiry from %v to %v", created.GetExpiresAt(), updated.GetExpiresAt())
	}
	key := core.authTokenKey(sessionID)
	assertRuntimeKVHasTTL(t, core, key)

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("mutable cookie session did not expire")
		case <-ticker.C:
			_, err := core.storage.runtimeStateKV.Get(context.Background(), key)
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				return
			}
			if err != nil {
				t.Fatalf("get mutable cookie session: %v", err)
			}
		}
	}
}

func TestChattoCore_CreateCookieSessionRejectsEmptyUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if sessionID, _, err := core.CreateCookieSessionForGeneration(ctx, "", "password_login", 0); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("CreateCookieSessionForGeneration err = %v, sessionID = %q, want ErrCookieSessionNotFound", err, sessionID)
	}
}

func TestChattoCore_CookieSessionFreshAuth(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-fresh-auth-user", "Cookie Fresh Auth User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, created, err := core.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	if err := core.RequireFreshAuthForCookieSession(ctx, sessionID); err != nil {
		t.Fatalf("new cookie session should be fresh: %v", err)
	}

	renewed, didRenew, err := core.RenewCookieSession(ctx, sessionID, created.GetExpiresAt().AsTime().Add(-time.Minute))
	if err != nil {
		t.Fatalf("RenewCookieSession: %v", err)
	}
	if !didRenew {
		t.Fatal("RenewCookieSession didRenew = false, want true")
	}
	if renewed.GetFreshAuthAt() == nil || !renewed.GetFreshAuthAt().AsTime().Equal(created.GetFreshAuthAt().AsTime()) {
		t.Fatalf("renewed fresh auth at = %v, want %v", renewed.GetFreshAuthAt(), created.GetFreshAuthAt())
	}
	if renewed.GetFreshAuthMethod() != created.GetFreshAuthMethod() || renewed.GetFreshAuthSource() != created.GetFreshAuthSource() {
		t.Fatalf("renewed fresh-auth provenance = %q/%q, want %q/%q", renewed.GetFreshAuthMethod(), renewed.GetFreshAuthSource(), created.GetFreshAuthMethod(), created.GetFreshAuthSource())
	}
	if err := core.MarkCookieSessionFresh(ctx, sessionID, "password", "current_password"); err != nil {
		t.Fatalf("MarkCookieSessionFresh: %v", err)
	}
	if err := core.RequireFreshAuthForCookieSession(ctx, sessionID); err != nil {
		t.Fatalf("marked cookie session should be fresh: %v", err)
	}
}

func TestChattoCore_CookieSessionRevocation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-revoke-user", "Cookie Revoke User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	session1, _, err := core.CreateCookieSession(ctx, user.Id, "test")
	if err != nil {
		t.Fatalf("CreateCookieSession 1: %v", err)
	}
	session2, _, err := core.CreateCookieSession(ctx, user.Id, "test")
	if err != nil {
		t.Fatalf("CreateCookieSession 2: %v", err)
	}
	session3, _, err := core.CreateCookieSession(ctx, user.Id, "test")
	if err != nil {
		t.Fatalf("CreateCookieSession 3: %v", err)
	}

	if err := core.RevokeCookieSession(ctx, session1); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, session1); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("Validate revoked session err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, session2); err != nil {
		t.Fatalf("second session should remain valid: %v", err)
	}
	if userID, existed, err := core.RevokePresentedRuntimeCredentialWithReason(
		ctx,
		session2,
		AuthTokenPresentationCookie,
		"logout",
	); err != nil || !existed || userID != user.Id {
		t.Fatalf("RevokePresentedRuntimeCredentialWithReason = %q, %t, %v", userID, existed, err)
	}
	if _, err := core.ValidateCookieCredential(ctx, session3); err != nil {
		t.Fatalf("third session should remain valid: %v", err)
	}

	deleted, err := core.RevokeCookieSessionsForUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("RevokeCookieSessionsForUser: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, err := core.ValidateCookieCredential(ctx, session3); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("Validate user-revoked session err = %v, want ErrCookieSessionNotFound", err)
	}
}

func TestChattoCore_CookieSessionGenerationRejectsStaleAuthentication(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-generation-user", "Cookie Generation User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	sessionID, _, err := core.CreateCookieSessionForGeneration(ctx, user.Id, "password_login", authGeneration)
	if err != nil {
		t.Fatalf("CreateCookieSessionForGeneration: %v", err)
	}

	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("ValidateCookieCredential err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, _, err := core.CreateCookieSessionForGeneration(ctx, user.Id, "password_login", authGeneration); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("stale CreateCookieSessionForGeneration err = %v, want ErrCookieSessionNotFound", err)
	}
	freshGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration fresh: %v", err)
	}
	if fresh, _, err := core.CreateCookieSessionForGeneration(ctx, user.Id, "password_login", freshGeneration); err != nil {
		t.Fatalf("fresh CreateCookieSessionForGeneration should succeed: %v", err)
	} else if _, err := core.ValidateCookieCredential(ctx, fresh); err != nil {
		t.Fatalf("fresh cookie session should validate: %v", err)
	}
}

func TestChattoCore_LegacyCookieSessionRecordIsIgnored(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-legacy-user", "Cookie Legacy User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	sessionID := "cht_CSlegacy-session"
	key := core.runtimeTokenKey("cookie_session."+user.Id+".", sessionID)
	legacyRecord, err := proto.Marshal(&runtimestatev1.CookieSession{
		UserId:    user.Id,
		CreatedAt: timestamppb.Now(),
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		Source:    "legacy_login",
	})
	if err != nil {
		t.Fatalf("marshal legacy session: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, legacyRecord, jetstream.KeyTTL(core.cookieSessionTTL())); err != nil {
		t.Fatalf("store legacy session: %v", err)
	}

	if _, err := core.ValidateCookieCredential(ctx, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("ValidateCookieCredential err = %v, want ErrCookieSessionNotFound", err)
	}
	if err := core.RevokeCookieSession(ctx, sessionID); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
		t.Fatalf("single typed revoke touched legacy record: %v", err)
	}
	deleted, err := core.RevokeCookieSessionsForUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("RevokeCookieSessionsForUser: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("RevokeCookieSessionsForUser deleted = %d, want 0", deleted)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
		t.Fatalf("legacy record should remain untouched until its TTL expires: %v", err)
	}
}
