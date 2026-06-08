package core

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func TestChattoCore_WorkflowTokensUseRuntimeState(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "runtime-token-user", "Runtime Token User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.AddVerifiedEmailDirect(ctx, user.Id, "runtime-token@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}

	registrationToken, err := core.CreateRegistrationToken(ctx, "runtime-registration@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationToken: %v", err)
	}
	assertRuntimeTokenOnly(t, core, core.registrationTokenKey(registrationToken), registrationCompletionTokenKeyPrefix+registrationToken)

	registrationCode, err := core.CreateRegistrationCode(ctx, "runtime-registration-code@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationCode: %v", err)
	}
	assertRuntimeTokenOnly(t, core, core.registrationCodeKey("runtime-registration-code@example.com", registrationCode), emailOTPKeyPrefix+registrationCode)

	emailCode, err := core.CreateEmailVerificationCode(ctx, user.Id, "runtime-verify@example.com")
	if err != nil {
		t.Fatalf("CreateEmailVerificationCode: %v", err)
	}
	assertRuntimeTokenOnly(t, core, core.emailVerificationCodeKey(user.Id, "runtime-verify@example.com", emailCode), emailOTPKeyPrefix+emailCode)

	resetToken, err := core.CreatePasswordResetToken(ctx, "runtime-token@example.com")
	if err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}
	assertRuntimeTokenOnly(t, core, core.passwordResetTokenKey(resetToken), passwordResetTokenKeyPrefix+resetToken)

	deletionToken, err := core.CreateAccountDeletionToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAccountDeletionToken: %v", err)
	}
	assertRuntimeTokenOnly(t, core, core.accountDeletionTokenKey(deletionToken), accountDeletionTokenKeyPrefix+deletionToken)
}

func assertRuntimeTokenOnly(t *testing.T, core *ChattoCore, key, rawKey string) {
	t.Helper()
	ctx := testContext(t)
	if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
		t.Fatalf("expected %s in RUNTIME_STATE: %v", key, err)
	}
	assertRuntimeKVHasTTL(t, core, key)
	assertLegacyKeyAbsent(t, core.storage.serverKV, key, "legacy INSTANCE token key")
	assertRawRuntimeTokenKeyAbsent(t, core, rawKey)
	assertLegacyKeyAbsent(t, core.storage.serverKV, rawKey, "raw legacy INSTANCE token key")
}

func assertRuntimeKVHasTTL(t *testing.T, core *ChattoCore, key string) {
	t.Helper()
	ctx := testContext(t)

	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get runtime state key %s: %v", key, err)
	}
	stream, err := core.js.Stream(ctx, "KV_RUNTIME_STATE")
	if err != nil {
		t.Fatalf("get RUNTIME_STATE stream: %v", err)
	}
	msg, err := stream.GetMsg(ctx, entry.Revision())
	if err != nil {
		t.Fatalf("get raw runtime state message for %s: %v", key, err)
	}
	if msg.Header.Get("Nats-TTL") == "" {
		t.Fatalf("expected %s to carry per-key TTL, got headers: %v", key, msg.Header)
	}
}

func assertRawRuntimeTokenKeyAbsent(t *testing.T, core *ChattoCore, rawKey string) {
	t.Helper()
	ctx := testContext(t)
	if _, err := core.storage.runtimeStateKV.Get(ctx, rawKey); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("raw runtime token key %s lookup error = %v, want ErrKeyNotFound", rawKey, err)
	}
}
