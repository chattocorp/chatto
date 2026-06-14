package core

import (
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

	key := core.cookieSessionKey(user.Id, sessionID)
	assertRuntimeKVHasTTL(t, core, key)
	assertRawRuntimeTokenKeyAbsent(t, core, cookieSessionKeyPrefix+user.Id+"."+sessionID)

	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get cookie session: %v", err)
	}
	var stored corev1.CookieSession
	if err := proto.Unmarshal(entry.Value(), &stored); err != nil {
		t.Fatalf("unmarshal cookie session: %v", err)
	}
	if stored.GetUserId() != user.Id || stored.GetExpiresAt() == nil {
		t.Fatalf("unexpected stored session: %#v", &stored)
	}

	validated, err := core.ValidateCookieSession(ctx, user.Id, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieSession: %v", err)
	}
	if !proto.Equal(validated, &stored) {
		t.Fatalf("validated session differs from stored session")
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

	if err := core.RevokeCookieSession(ctx, user.Id, session1); err != nil {
		t.Fatalf("RevokeCookieSession: %v", err)
	}
	if _, err := core.ValidateCookieSession(ctx, user.Id, session1); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("Validate revoked session err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, err := core.ValidateCookieSession(ctx, user.Id, session2); err != nil {
		t.Fatalf("second session should remain valid: %v", err)
	}

	deleted, err := core.RevokeCookieSessionsForUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("RevokeCookieSessionsForUser: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, err := core.ValidateCookieSession(ctx, user.Id, session2); !errors.Is(err, ErrCookieSessionNotFound) {
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
	if _, err := core.ValidateCookieSession(ctx, user.Id, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("ValidateCookieSession err = %v, want ErrCookieSessionNotFound", err)
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
	} else if _, err := core.ValidateCookieSession(ctx, user.Id, fresh); err != nil {
		t.Fatalf("fresh cookie session should validate: %v", err)
	}
}

func TestChattoCore_ValidateCookieSessionGrandfathersLegacyGeneration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-legacy-user", "Cookie Legacy User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}

	sessionID := NewCookieSessionID()
	key := core.cookieSessionKey(user.Id, sessionID)
	legacy := &corev1.CookieSession{
		UserId:    user.Id,
		CreatedAt: timestamppb.New(time.Now()),
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		Source:    "legacy_login",
	}
	data, err := proto.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy session: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(core.cookieSessionTTL())); err != nil {
		t.Fatalf("store legacy session: %v", err)
	}

	validated, err := core.ValidateCookieSession(ctx, user.Id, sessionID)
	if err != nil {
		t.Fatalf("ValidateCookieSession: %v", err)
	}
	if validated.GetAuthGeneration() != authGeneration {
		t.Fatalf("validated auth generation = %d, want %d", validated.GetAuthGeneration(), authGeneration)
	}

	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get upgraded session: %v", err)
	}
	var upgraded corev1.CookieSession
	if err := proto.Unmarshal(entry.Value(), &upgraded); err != nil {
		t.Fatalf("unmarshal upgraded session: %v", err)
	}
	if upgraded.GetAuthGeneration() != authGeneration {
		t.Fatalf("stored auth generation = %d, want %d", upgraded.GetAuthGeneration(), authGeneration)
	}
}

func TestChattoCore_ValidateCookieSessionRejectsLegacyGenerationBeforePasswordChange(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-legacy-stale-user", "Cookie Legacy Stale User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	legacyCreatedAt := time.Now()
	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
	}

	sessionID := NewCookieSessionID()
	key := core.cookieSessionKey(user.Id, sessionID)
	legacy := &corev1.CookieSession{
		UserId:    user.Id,
		CreatedAt: timestamppb.New(legacyCreatedAt),
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		Source:    "legacy_login",
	}
	data, err := proto.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy session: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(core.cookieSessionTTL())); err != nil {
		t.Fatalf("store legacy session: %v", err)
	}

	if _, err := core.ValidateCookieSession(ctx, user.Id, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("ValidateCookieSession err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("legacy stale session lookup error = %v, want ErrKeyNotFound", err)
	}
}

func TestChattoCore_ValidateCookieSessionRejectsExpiredPayload(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "cookie-expired-user", "Cookie Expired User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sessionID := NewCookieSessionID()
	key := core.cookieSessionKey(user.Id, sessionID)
	expired := &corev1.CookieSession{
		UserId:    user.Id,
		CreatedAt: timestamppb.New(time.Now().Add(-2 * time.Hour)),
		ExpiresAt: timestamppb.New(time.Now().Add(-time.Hour)),
		Source:    "test",
	}
	data, err := proto.Marshal(expired)
	if err != nil {
		t.Fatalf("marshal expired session: %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(core.cookieSessionTTL())); err != nil {
		t.Fatalf("store expired session: %v", err)
	}

	if _, err := core.ValidateCookieSession(ctx, user.Id, sessionID); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("ValidateCookieSession err = %v, want ErrCookieSessionNotFound", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("expired session key lookup error = %v, want ErrKeyNotFound", err)
	}
}
