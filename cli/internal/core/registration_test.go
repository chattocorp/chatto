package core

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestNewVerificationCode(t *testing.T) {
	code, err := NewVerificationCode()
	if err != nil {
		t.Fatalf("NewVerificationCode: %v", err)
	}
	if !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
		t.Fatalf("code = %q, want six digits", code)
	}
}

func TestChattoCore_CreateRegistrationCode(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	code, err := core.CreateRegistrationCode(ctx, "NewUser@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationCode: %v", err)
	}
	if !verificationCodePattern.MatchString(code) {
		t.Fatalf("code = %q, want six digits", code)
	}

	key := core.registrationCodeKey("newuser@example.com")
	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("registration code record missing: %v", err)
	}
	assertRuntimeKVHasTTL(t, core, key)

	var record RegistrationCode
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if record.Email != "newuser@example.com" {
		t.Fatalf("email = %q, want normalized address", record.Email)
	}
	if record.CodeHash == "" || record.CodeHash == code {
		t.Fatalf("code verifier was not hashed safely: %#v", record)
	}
	if strings.Contains(string(entry.Value()), code) {
		t.Fatalf("runtime state leaked raw code: %s", entry.Value())
	}

	if RegistrationCodeTTL != 15*time.Minute {
		t.Fatalf("RegistrationCodeTTL = %v, want 15m", RegistrationCodeTTL)
	}
}

func TestChattoCore_VerifyRegistrationCode(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	code, err := core.CreateRegistrationCode(ctx, "complete@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationCode: %v", err)
	}

	token, err := core.VerifyRegistrationCode(ctx, "complete@example.com", code)
	if err != nil {
		t.Fatalf("VerifyRegistrationCode: %v", err)
	}
	if token == "" {
		t.Fatal("expected completion token")
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.registrationCodeKey("complete@example.com")); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("registration code should be consumed, got %v", err)
	}

	tokenData, err := core.GetRegistrationToken(ctx, token)
	if err != nil {
		t.Fatalf("GetRegistrationToken: %v", err)
	}
	if tokenData.Email != "complete@example.com" {
		t.Fatalf("completion token email = %q", tokenData.Email)
	}
	assertRuntimeKVHasTTL(t, core, core.registrationTokenKey(token))
}

func TestChattoCore_VerifyRegistrationCodeInvalidAttempts(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	code, err := core.CreateRegistrationCode(ctx, "attempts@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationCode: %v", err)
	}
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "111111"
	}

	for i := 1; i < RegistrationCodeMaxAttempts; i++ {
		_, err := core.VerifyRegistrationCode(ctx, "attempts@example.com", wrongCode)
		if !errors.Is(err, ErrRegistrationCodeInvalid) {
			t.Fatalf("attempt %d error = %v, want ErrRegistrationCodeInvalid", i, err)
		}
	}

	_, err = core.VerifyRegistrationCode(ctx, "attempts@example.com", wrongCode)
	if !errors.Is(err, ErrRegistrationCodeExhausted) {
		t.Fatalf("exhaustion error = %v, want ErrRegistrationCodeExhausted", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.registrationCodeKey("attempts@example.com")); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("exhausted code should be deleted, got %v", err)
	}

	_, err = core.VerifyRegistrationCode(ctx, "attempts@example.com", code)
	if !errors.Is(err, ErrRegistrationCodeNotFound) {
		t.Fatalf("consumed exhausted code error = %v, want ErrRegistrationCodeNotFound", err)
	}
}

func TestChattoCore_RegistrationCodeResendInvalidatesPreviousCode(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	firstCode, err := core.CreateRegistrationCode(ctx, "resend@example.com")
	if err != nil {
		t.Fatalf("first CreateRegistrationCode: %v", err)
	}
	secondCode, err := core.CreateRegistrationCode(ctx, "resend@example.com")
	if err != nil {
		t.Fatalf("second CreateRegistrationCode: %v", err)
	}

	_, err = core.VerifyRegistrationCode(ctx, "resend@example.com", firstCode)
	if !errors.Is(err, ErrRegistrationCodeInvalid) {
		t.Fatalf("first code error = %v, want ErrRegistrationCodeInvalid", err)
	}
	if _, err := core.VerifyRegistrationCode(ctx, "resend@example.com", secondCode); err != nil {
		t.Fatalf("second code should verify: %v", err)
	}
}

func TestChattoCore_RegistrationCompletionToken(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	token, err := core.CreateRegistrationToken(ctx, "token@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationToken: %v", err)
	}
	if len(token) != 16 {
		t.Fatalf("token length = %d, want 16", len(token))
	}

	tokenData, err := core.GetRegistrationToken(ctx, token)
	if err != nil {
		t.Fatalf("GetRegistrationToken: %v", err)
	}
	if tokenData.Email != "token@example.com" {
		t.Fatalf("email = %q", tokenData.Email)
	}
	if RegistrationCompletionTokenTTL != 15*time.Minute {
		t.Fatalf("RegistrationCompletionTokenTTL = %v, want 15m", RegistrationCompletionTokenTTL)
	}

	if err := core.DeleteRegistrationToken(ctx, token); err != nil {
		t.Fatalf("DeleteRegistrationToken: %v", err)
	}
	_, err = core.GetRegistrationToken(ctx, token)
	if !errors.Is(err, ErrRegistrationTokenNotFound) {
		t.Fatalf("deleted token error = %v, want ErrRegistrationTokenNotFound", err)
	}
}
