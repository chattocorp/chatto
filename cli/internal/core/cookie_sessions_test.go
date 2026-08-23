package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_CreateAndValidateCookieSession(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := WithAuditRequestMetadata(testContext(t), &corev1.AuditRequestMetadata{
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
	assertRuntimeKVHasTTL(t, core, runtimeCredentialExpiryMarkerKey(key))
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

func TestRuntimeCredentialExpiryReconcileRepairsAndRemovesRecords(t *testing.T) {
	core, _ := newTestCore(t)
	ctx := testContext(t)
	now := time.Now()

	futureKey := core.authTokenKey("future-cookie")
	future, err := json.Marshal(AuthTokenData{
		UserID:       "future-user",
		Kind:         AuthTokenKindFirstPartySession,
		Presentation: AuthTokenPresentationCookie,
		CreatedAt:    now,
		ExpiresAt:    now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("marshal future cookie session: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, futureKey, future); err != nil {
		t.Fatalf("create future cookie session: %v", err)
	}

	expiredKey := core.renewableSessionKey("expired-renewable")
	expired, err := json.Marshal(RenewableSession{
		UserID:    "expired-user",
		Kind:      AuthTokenKindFirstPartySession,
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("marshal expired renewable session: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, expiredKey, expired); err != nil {
		t.Fatalf("create expired renewable session: %v", err)
	}

	if err := core.runtimeCredentialExpiry.reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, futureKey); err != nil {
		t.Fatalf("future cookie session removed: %v", err)
	}
	assertRuntimeKVHasTTL(t, core, runtimeCredentialExpiryMarkerKey(futureKey))
	if _, err := core.storage.runtimeStateKV.Get(ctx, expiredKey); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("expired renewable session lookup error = %v, want ErrKeyNotFound", err)
	}
}

func TestRuntimeCredentialExpiryMarkerRemovesMutableCookieSession(t *testing.T) {
	core, _ := setupTestCore(t)
	core.config.AuthTokenTTL = 2 * time.Second
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "cookie-marker-user", "Cookie Marker User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID, _, err := core.CreateCookieSession(ctx, user.Id, "password_login")
	if err != nil {
		t.Fatalf("CreateCookieSession: %v", err)
	}
	if err := core.MarkCookieSessionFresh(ctx, sessionID, "password", "current_password"); err != nil {
		t.Fatalf("MarkCookieSessionFresh: %v", err)
	}
	key := core.authTokenKey(sessionID)
	assertRuntimeKVHasNoTTL(t, core, key)

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("expiry marker did not remove mutable cookie session")
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

	created.FreshAuthAt = timestamppb.New(time.Now().Add(-FreshAuthWindow - time.Minute))
	rotatedID, rotated, err := core.CreateCookieSessionForGenerationPreservingFreshAuth(ctx, user.Id, "session_rotation", created.GetAuthGeneration(), created)
	if err != nil {
		t.Fatalf("CreateCookieSessionForGenerationPreservingFreshAuth: %v", err)
	}
	if rotated.GetFreshAuthAt() == nil || !rotated.GetFreshAuthAt().AsTime().Equal(created.GetFreshAuthAt().AsTime()) {
		t.Fatalf("rotated fresh auth at = %v, want %v", rotated.GetFreshAuthAt(), created.GetFreshAuthAt())
	}
	if err := core.RequireFreshAuthForCookieSession(ctx, rotatedID); !errors.Is(err, ErrFreshAuthRequired) {
		t.Fatalf("rotated stale session fresh auth err = %v, want ErrFreshAuthRequired", err)
	}
	if err := core.MarkCookieSessionFresh(ctx, rotatedID, "password", "current_password"); err != nil {
		t.Fatalf("MarkCookieSessionFresh: %v", err)
	}
	if err := core.RequireFreshAuthForCookieSession(ctx, rotatedID); err != nil {
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

	if err := core.RevokeCookieSession(ctx, session1); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, session1); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("Validate revoked session err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, err := core.ValidateCookieCredential(ctx, session2); err != nil {
		t.Fatalf("second session should remain valid: %v", err)
	}

	deleted, err := core.RevokeCookieSessionsForUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("RevokeCookieSessionsForUser: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, err := core.ValidateCookieCredential(ctx, session2); !errors.Is(err, ErrCookieSessionNotFound) {
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
	legacyRecord, err := proto.Marshal(&corev1.CookieSession{
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
