package core

import (
	"errors"
	"sync"
	"testing"

	"hmans.de/chatto/internal/config"
)

// ============================================================================
// Email Verification Tests
// ============================================================================

func TestChattoCore_VerifyEmailCode(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("verifies email successfully", func(t *testing.T) {
		user, err := core.CreateUser(ctx, "system", "verify-test-user", "Test User", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		code, err := core.CreateEmailVerificationCode(ctx, user.Id, "verify@example.com")
		if err != nil {
			t.Fatalf("Failed to create code: %v", err)
		}

		userID, err := core.VerifyEmailCode(ctx, user.Id, "verify@example.com", code)
		if err != nil {
			t.Fatalf("Failed to verify email: %v", err)
		}
		if userID != user.Id {
			t.Errorf("Expected userID %s, got %s", user.Id, userID)
		}

		// Check email is now verified
		hasVerified, err := core.HasVerifiedEmail(ctx, user.Id)
		if err != nil {
			t.Fatalf("Failed to check verified email: %v", err)
		}
		if !hasVerified {
			t.Error("Expected user to have verified email")
		}
	})

	t.Run("returns error for missing code record", func(t *testing.T) {
		_, err := core.VerifyEmailCode(ctx, "missing-user", "missing@example.com", "123456")
		if !errors.Is(err, ErrTokenNotFound) {
			t.Errorf("Expected ErrTokenNotFound, got %v", err)
		}
	})

	t.Run("returns error when email already claimed by another user", func(t *testing.T) {
		// Create first user and verify email
		user1, _ := core.CreateUser(ctx, "system", "claim-test-user1", "User 1", "password123")
		if err := core.AddVerifiedEmailDirect(ctx, user1.Id, "claimed@example.com"); err != nil {
			t.Fatalf("Failed to verify email for user1: %v", err)
		}

		// Create second user and try to verify same email
		user2, _ := core.CreateUser(ctx, "system", "claim-test-user2", "User 2", "password123")
		code, _ := core.CreateEmailVerificationCode(ctx, user2.Id, "claimed@example.com")

		_, err := core.VerifyEmailCode(ctx, user2.Id, "claimed@example.com", code)
		if err != ErrEmailAlreadyVerified {
			t.Errorf("Expected ErrEmailAlreadyVerified, got %v", err)
		}
	})

	t.Run("idempotent verification does not delete existing claim on error", func(t *testing.T) {
		// This test verifies the fix for Issue 1:
		// If a user already has a claim and addVerifiedEmail fails,
		// we should not delete the existing claim.

		user, _ := core.CreateUser(ctx, "system", "idempotent-test-user", "Test User", "password123")

		// First verification succeeds
		code1, _ := core.CreateEmailVerificationCode(ctx, user.Id, "idempotent@example.com")
		_, err := core.VerifyEmailCode(ctx, user.Id, "idempotent@example.com", code1)
		if err != nil {
			t.Fatalf("First verification failed: %v", err)
		}

		// Verify email is claimed
		claimed, _ := core.IsEmailClaimed(ctx, "idempotent@example.com")
		if !claimed {
			t.Fatal("Email should be claimed after first verification")
		}

		// Second verification with same email (idempotent case)
		// This should succeed without deleting the claim
		code2, _ := core.CreateEmailVerificationCode(ctx, user.Id, "idempotent@example.com")
		_, err = core.VerifyEmailCode(ctx, user.Id, "idempotent@example.com", code2)
		if err != nil {
			t.Fatalf("Idempotent verification failed: %v", err)
		}

		// Email should still be claimed
		claimed, _ = core.IsEmailClaimed(ctx, "idempotent@example.com")
		if !claimed {
			t.Error("Email should still be claimed after idempotent verification")
		}
	})

	t.Run("unknown code does not consume valid code", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "unknown-email-user", "Unknown User", "password123")
		code, err := core.CreateEmailVerificationCode(ctx, user.Id, "unknown-email@example.com")
		if err != nil {
			t.Fatalf("CreateEmailVerificationCode: %v", err)
		}
		wrongCode := "000000"
		if code == wrongCode {
			wrongCode = "111111"
		}

		_, err = core.VerifyEmailCode(ctx, user.Id, "unknown-email@example.com", wrongCode)
		if !errors.Is(err, ErrEmailVerificationCodeInvalid) {
			t.Fatalf("wrong code error = %v, want ErrEmailVerificationCodeInvalid", err)
		}
		if _, err := core.VerifyEmailCode(ctx, user.Id, "unknown-email@example.com", code); err != nil {
			t.Fatalf("valid code should still verify after wrong code: %v", err)
		}
	})

	t.Run("invalid attempts exhaust code", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "attempt-email-user", "Attempt User", "password123")
		code, err := core.CreateEmailVerificationCode(ctx, user.Id, "attempt-email@example.com")
		if err != nil {
			t.Fatalf("CreateEmailVerificationCode: %v", err)
		}
		wrongCode := "000000"
		if code == wrongCode {
			wrongCode = "111111"
		}

		for i := 1; i < core.emailOTPMaxAttempts(); i++ {
			_, err := core.VerifyEmailCode(ctx, user.Id, "attempt-email@example.com", wrongCode)
			if !errors.Is(err, ErrEmailVerificationCodeInvalid) {
				t.Fatalf("attempt %d error = %v, want ErrEmailVerificationCodeInvalid", i, err)
			}
		}
		_, err = core.VerifyEmailCode(ctx, user.Id, "attempt-email@example.com", wrongCode)
		if !errors.Is(err, ErrEmailVerificationCodeExhausted) {
			t.Fatalf("exhaustion error = %v, want ErrEmailVerificationCodeExhausted", err)
		}
		if _, err := core.VerifyEmailCode(ctx, user.Id, "attempt-email@example.com", code); !errors.Is(err, ErrEmailVerificationCodeExhausted) {
			t.Fatalf("valid code after exhaustion error = %v, want ErrEmailVerificationCodeExhausted", err)
		}
		if _, err := core.CreateEmailVerificationCode(ctx, user.Id, "attempt-email@example.com"); !errors.Is(err, ErrEmailVerificationCodeExhausted) {
			t.Fatalf("new code after exhaustion error = %v, want ErrEmailVerificationCodeExhausted", err)
		}
	})

	t.Run("concurrent valid code consumes once", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "parallel-email-user", "Parallel User", "password123")
		code, err := core.CreateEmailVerificationCode(ctx, user.Id, "parallel-email@example.com")
		if err != nil {
			t.Fatalf("CreateEmailVerificationCode: %v", err)
		}

		var wg sync.WaitGroup
		errs := make(chan error, 5)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := core.VerifyEmailCode(ctx, user.Id, "parallel-email@example.com", code)
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
			case errors.Is(err, ErrEmailVerificationCodeInvalid):
			case errors.Is(err, ErrTokenNotFound):
			default:
				t.Fatalf("unexpected concurrent verification error: %v", err)
			}
		}
		if successes != 1 {
			t.Fatalf("successful verifications = %d, want exactly one", successes)
		}
	})

	t.Run("multiple requests remain valid until success", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "resend-email-user", "Resend User", "password123")
		firstCode, err := core.CreateEmailVerificationCode(ctx, user.Id, "resend-email@example.com")
		if err != nil {
			t.Fatalf("first CreateEmailVerificationCode: %v", err)
		}
		secondCode, err := core.CreateEmailVerificationCode(ctx, user.Id, "resend-email@example.com")
		if err != nil {
			t.Fatalf("second CreateEmailVerificationCode: %v", err)
		}

		if _, err := core.VerifyEmailCode(ctx, user.Id, "resend-email@example.com", firstCode); err != nil {
			t.Fatalf("first code should verify: %v", err)
		}
		if _, err := core.VerifyEmailCode(ctx, user.Id, "resend-email@example.com", secondCode); !errors.Is(err, ErrTokenNotFound) {
			t.Fatalf("second code after challenge success error = %v, want ErrTokenNotFound", err)
		}
	})

	t.Run("active code limit", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "limit-email-user", "Limit User", "password123")

		for i := 0; i < core.emailOTPMaxDeliveredCodes(); i++ {
			if _, err := core.CreateEmailVerificationCode(ctx, user.Id, "limit-email@example.com"); err != nil {
				t.Fatalf("CreateEmailVerificationCode %d: %v", i+1, err)
			}
		}
		if _, err := core.CreateEmailVerificationCode(ctx, user.Id, "limit-email@example.com"); !errors.Is(err, ErrEmailVerificationCodeLimitExceeded) {
			t.Fatalf("extra CreateEmailVerificationCode error = %v, want ErrEmailVerificationCodeLimitExceeded", err)
		}
	})
}

func TestChattoCore_ListUsersWithVerifiedEmail(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty list when no users have verified emails", func(t *testing.T) {
		// Create user without verified email
		_, err := core.CreateUser(ctx, "system", "no-email-user", "No Email", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		users, err := core.ListUsersWithVerifiedEmail(ctx)
		if err != nil {
			t.Fatalf("Failed to list users: %v", err)
		}

		// Should not include the user without verified email
		for _, u := range users {
			if u == "no-email-user" {
				t.Error("User without verified email should not be in list")
			}
		}
	})

	t.Run("returns users with verified emails", func(t *testing.T) {
		user1, _ := core.CreateUser(ctx, "system", "list-email-user1", "User 1", "password123")
		user2, _ := core.CreateUser(ctx, "system", "list-email-user2", "User 2", "password123")

		// Verify emails for both users
		if err := core.AddVerifiedEmailDirect(ctx, user1.Id, "list1@example.com"); err != nil {
			t.Fatalf("Failed to verify email for user1: %v", err)
		}
		if err := core.AddVerifiedEmailDirect(ctx, user2.Id, "list2@example.com"); err != nil {
			t.Fatalf("Failed to verify email for user2: %v", err)
		}

		users, err := core.ListUsersWithVerifiedEmail(ctx)
		if err != nil {
			t.Fatalf("Failed to list users: %v", err)
		}

		// Check both users are in the list
		userSet := make(map[string]bool)
		for _, u := range users {
			userSet[u] = true
		}

		if !userSet[user1.Id] {
			t.Errorf("User1 (%s) not found in verified users list", user1.Id)
		}
		if !userSet[user2.Id] {
			t.Errorf("User2 (%s) not found in verified users list", user2.Id)
		}
	})
}

func TestChattoCore_ApplyConfigOwners(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, err := core.CreateVerifiedUser(ctx, SystemActorID, "config-owner", "Config Owner", "password123", "Owner@Example.com")
	if err != nil {
		t.Fatalf("create owner candidate: %v", err)
	}
	regular, err := core.CreateVerifiedUser(ctx, SystemActorID, "config-regular", "Config Regular", "password123", "regular@example.com")
	if err != nil {
		t.Fatalf("create regular user: %v", err)
	}

	if isOwner, err := core.IsServerOwner(ctx, owner.Id); err != nil || isOwner {
		t.Fatalf("owner candidate should not start as owner, owner=%v err=%v", isOwner, err)
	}

	core.config.Owners = config.OwnersConfig{Emails: []string{" owner@example.com "}}
	if err := core.applyConfigOwners(ctx); err != nil {
		t.Fatalf("apply config owners: %v", err)
	}

	if isOwner, err := core.IsServerOwner(ctx, owner.Id); err != nil || !isOwner {
		t.Fatalf("matching verified email should get owner role, owner=%v err=%v", isOwner, err)
	}
	if isOwner, err := core.IsServerOwner(ctx, regular.Id); err != nil || isOwner {
		t.Fatalf("non-matching verified email should not get owner role, owner=%v err=%v", isOwner, err)
	}

	eventsAfterApply := eventStreamMsgCount(t, core)
	if err := core.applyConfigOwners(ctx); err != nil {
		t.Fatalf("second apply config owners: %v", err)
	}
	eventsAfterSecondApply := eventStreamMsgCount(t, core)
	if eventsAfterSecondApply != eventsAfterApply {
		t.Fatalf("expected applyConfigOwners to be idempotent, got %d -> %d events", eventsAfterApply, eventsAfterSecondApply)
	}
}

func TestChattoCore_AddVerifiedEmailDirect(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("adds verified email directly", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "direct-verify-user", "Direct User", "password123")

		err := core.AddVerifiedEmailDirect(ctx, user.Id, "direct@example.com")
		if err != nil {
			t.Fatalf("Failed to add verified email: %v", err)
		}

		hasVerified, _ := core.HasVerifiedEmail(ctx, user.Id)
		if !hasVerified {
			t.Error("Expected user to have verified email")
		}
	})

	t.Run("is idempotent for same user", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "idempotent-direct-user", "Idempotent User", "password123")

		// Add same email twice
		err := core.AddVerifiedEmailDirect(ctx, user.Id, "idempotent-direct@example.com")
		if err != nil {
			t.Fatalf("First add failed: %v", err)
		}

		err = core.AddVerifiedEmailDirect(ctx, user.Id, "idempotent-direct@example.com")
		if err != nil {
			t.Fatalf("Second add should succeed (idempotent): %v", err)
		}
	})

	t.Run("returns error when email claimed by another user", func(t *testing.T) {
		user1, _ := core.CreateUser(ctx, "system", "direct-claim-user1", "User 1", "password123")
		user2, _ := core.CreateUser(ctx, "system", "direct-claim-user2", "User 2", "password123")

		// First user claims email
		err := core.AddVerifiedEmailDirect(ctx, user1.Id, "direct-claimed@example.com")
		if err != nil {
			t.Fatalf("First user failed to add email: %v", err)
		}

		// Second user tries to claim same email
		err = core.AddVerifiedEmailDirect(ctx, user2.Id, "direct-claimed@example.com")
		if err != ErrEmailAlreadyVerified {
			t.Errorf("Expected ErrEmailAlreadyVerified, got %v", err)
		}
	})

	t.Run("returns error when email claimed by another user (case-insensitive)", func(t *testing.T) {
		user1, _ := core.CreateUser(ctx, "system", "case-claim-user1", "User 1", "password123")
		user2, _ := core.CreateUser(ctx, "system", "case-claim-user2", "User 2", "password123")

		// First user claims email with uppercase
		err := core.AddVerifiedEmailDirect(ctx, user1.Id, "CASE-CLAIMED@EXAMPLE.COM")
		if err != nil {
			t.Fatalf("First user failed to add email: %v", err)
		}

		// Second user tries to claim same email with lowercase
		err = core.AddVerifiedEmailDirect(ctx, user2.Id, "case-claimed@example.com")
		if err != ErrEmailAlreadyVerified {
			t.Errorf("Expected ErrEmailAlreadyVerified for case-insensitive duplicate, got %v", err)
		}
	})

}

func TestChattoCore_GetUserByVerifiedEmail(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns user when email is verified", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "lookup-by-email-user", "Lookup User", "password123")
		err := core.AddVerifiedEmailDirect(ctx, user.Id, "lookup@example.com")
		if err != nil {
			t.Fatalf("Failed to add verified email: %v", err)
		}

		found, err := core.GetUserByVerifiedEmail(ctx, "lookup@example.com")
		if err != nil {
			t.Fatalf("Failed to lookup user: %v", err)
		}
		if found.Id != user.Id {
			t.Errorf("Expected user ID %s, got %s", user.Id, found.Id)
		}
	})

	t.Run("returns error when email not found", func(t *testing.T) {
		_, err := core.GetUserByVerifiedEmail(ctx, "nonexistent@example.com")
		if err == nil {
			t.Error("Expected error for nonexistent email")
		}
	})

	t.Run("lookup is case-insensitive", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "case-insensitive-user", "Case User", "password123")
		err := core.AddVerifiedEmailDirect(ctx, user.Id, "CaseTest@Example.COM")
		if err != nil {
			t.Fatalf("Failed to add verified email: %v", err)
		}

		// Lookup with different casing
		found, err := core.GetUserByVerifiedEmail(ctx, "casetest@example.com")
		if err != nil {
			t.Fatalf("Failed to lookup user: %v", err)
		}
		if found.Id != user.Id {
			t.Errorf("Expected user ID %s, got %s", user.Id, found.Id)
		}
	})
}
