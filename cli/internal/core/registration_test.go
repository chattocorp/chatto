package core

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
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

	key := core.registrationCodeKey("newuser@example.com", code)
	entry, err := core.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("registration code record missing: %v", err)
	}
	assertRuntimeKVHasTTL(t, core, key)
	assertRuntimeKVHasTTL(t, core, core.registrationCodeChallengeKey("newuser@example.com"))

	var record RegistrationCode
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if record.Email != "newuser@example.com" {
		t.Fatalf("email = %q, want normalized address", record.Email)
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
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.registrationCodeKey("complete@example.com", code)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("registration code should be consumed, got %v", err)
	}
	if _, err := core.storage.runtimeStateKV.Get(ctx, core.registrationCodeChallengeKey("complete@example.com")); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("registration challenge should be consumed, got %v", err)
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

func TestChattoCore_VerifyRegistrationCodeUnknownCode(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	code, err := core.CreateRegistrationCode(ctx, "unknown@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationCode: %v", err)
	}
	wrongCode := "000000"
	if code == wrongCode {
		wrongCode = "111111"
	}

	_, err = core.VerifyRegistrationCode(ctx, "unknown@example.com", wrongCode)
	if !errors.Is(err, ErrRegistrationCodeInvalid) {
		t.Fatalf("wrong code error = %v, want ErrRegistrationCodeInvalid", err)
	}
	if _, err := core.VerifyRegistrationCode(ctx, "unknown@example.com", code); err != nil {
		t.Fatalf("valid code should still verify after wrong code: %v", err)
	}
}

func TestChattoCore_VerifyRegistrationCodeInvalidAttemptsExhaust(t *testing.T) {
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

	for i := 1; i < emailOTPMaxAttempts; i++ {
		_, err := core.VerifyRegistrationCode(ctx, "attempts@example.com", wrongCode)
		if !errors.Is(err, ErrRegistrationCodeInvalid) {
			t.Fatalf("attempt %d error = %v, want ErrRegistrationCodeInvalid", i, err)
		}
	}
	_, err = core.VerifyRegistrationCode(ctx, "attempts@example.com", wrongCode)
	if !errors.Is(err, ErrRegistrationCodeExhausted) {
		t.Fatalf("exhaustion error = %v, want ErrRegistrationCodeExhausted", err)
	}
	if _, err := core.VerifyRegistrationCode(ctx, "attempts@example.com", code); !errors.Is(err, ErrRegistrationCodeExhausted) {
		t.Fatalf("valid code after exhaustion error = %v, want ErrRegistrationCodeExhausted", err)
	}
	if _, err := core.CreateRegistrationCode(ctx, "attempts@example.com"); !errors.Is(err, ErrRegistrationCodeExhausted) {
		t.Fatalf("new code after exhaustion error = %v, want ErrRegistrationCodeExhausted", err)
	}
}

func TestChattoCore_VerifyRegistrationCodeConcurrentValidCodeConsumesOnce(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	code, err := core.CreateRegistrationCode(ctx, "parallel-valid@example.com")
	if err != nil {
		t.Fatalf("CreateRegistrationCode: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := core.VerifyRegistrationCode(ctx, "parallel-valid@example.com", code)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrRegistrationCodeInvalid):
		case errors.Is(err, ErrRegistrationCodeNotFound):
		default:
			t.Fatalf("unexpected concurrent verification error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful verifications = %d, want exactly one", successes)
	}
}

func TestChattoCore_RegistrationCodeMultipleRequestsRemainValidUntilSuccess(t *testing.T) {
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

	if _, err := core.VerifyRegistrationCode(ctx, "resend@example.com", firstCode); err != nil {
		t.Fatalf("first code should verify: %v", err)
	}
	if _, err := core.VerifyRegistrationCode(ctx, "resend@example.com", secondCode); !errors.Is(err, ErrRegistrationCodeNotFound) {
		t.Fatalf("second code after challenge success error = %v, want ErrRegistrationCodeNotFound", err)
	}
}

func TestChattoCore_RegistrationCodeActiveLimit(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	for i := 0; i < emailOTPMaxActiveCodes; i++ {
		if _, err := core.CreateRegistrationCode(ctx, "limit@example.com"); err != nil {
			t.Fatalf("CreateRegistrationCode %d: %v", i+1, err)
		}
	}
	if _, err := core.CreateRegistrationCode(ctx, "limit@example.com"); !errors.Is(err, ErrRegistrationCodeLimitExceeded) {
		t.Fatalf("extra CreateRegistrationCode error = %v, want ErrRegistrationCodeLimitExceeded", err)
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
