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
