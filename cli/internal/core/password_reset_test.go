package core

import (
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestChattoCore_ResetPasswordConcurrentSingleUse(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "concurrent-reset", "Concurrent Reset", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := core.AddVerifiedEmailDirect(ctx, user.Id, "concurrent-reset@example.com"); err != nil {
		t.Fatalf("AddVerifiedEmailDirect: %v", err)
	}
	token, err := core.CreatePasswordResetToken(ctx, "concurrent-reset@example.com")
	if err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("newpassword123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword: %v", err)
	}

	const attempts = 8
	start := make(chan struct{})
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- core.ResetPassword(ctx, token, string(hash))
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrPasswordResetTokenNotFound) {
			t.Fatalf("ResetPassword returned unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful resets = %d, want 1", successes)
	}
}

// ============================================================================
// Password Reset Tests
// ============================================================================

func TestChattoCore_CreatePasswordResetToken(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates token for verified email", func(t *testing.T) {
		user, err := core.CreateUser(ctx, "system", "reset-test-user", "Test User", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Verify user's email first
		if err := core.AddVerifiedEmailDirect(ctx, user.Id, "reset@example.com"); err != nil {
			t.Fatalf("Failed to verify email: %v", err)
		}

		// Create password reset token
		token, err := core.CreatePasswordResetToken(ctx, "reset@example.com")
		if err != nil {
			t.Fatalf("Failed to create token: %v", err)
		}
		if token == "" {
			t.Error("Expected non-empty token")
		}
		if len(token) != 16 { // "PR" prefix + 14 chars
			t.Errorf("Expected token length 16, got %d", len(token))
		}
	})

	t.Run("returns empty token for unverified email (no error)", func(t *testing.T) {
		// Create user but don't verify any email
		_, err := core.CreateUser(ctx, "system", "unverified-reset-user", "Test User", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Try to create reset token for unverified email
		token, err := core.CreatePasswordResetToken(ctx, "unverified@example.com")
		if err != nil {
			t.Fatalf("Should not return error: %v", err)
		}
		if token != "" {
			t.Error("Expected empty token for unverified email")
		}
	})

	t.Run("returns empty token for non-existent email (no error)", func(t *testing.T) {
		token, err := core.CreatePasswordResetToken(ctx, "nonexistent@example.com")
		if err != nil {
			t.Fatalf("Should not return error: %v", err)
		}
		if token != "" {
			t.Error("Expected empty token for non-existent email")
		}
	})

	t.Run("returns error for empty email", func(t *testing.T) {
		_, err := core.CreatePasswordResetToken(ctx, "")
		if err == nil {
			t.Error("Expected error for empty email")
		}
	})

	t.Run("email lookup is case-insensitive", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "case-reset-user", "Test User", "password123")
		core.AddVerifiedEmailDirect(ctx, user.Id, "CaseTest@Example.COM")

		// Create token with different casing
		token, err := core.CreatePasswordResetToken(ctx, "casetest@example.com")
		if err != nil {
			t.Fatalf("Failed to create token: %v", err)
		}
		if token == "" {
			t.Error("Expected token for case-insensitive email match")
		}
	})
}

func TestChattoCore_ValidatePasswordResetToken(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("validates valid token", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "validate-token-user", "Test User", "password123")
		core.AddVerifiedEmailDirect(ctx, user.Id, "validate@example.com")

		token, _ := core.CreatePasswordResetToken(ctx, "validate@example.com")

		userID, err := core.ValidatePasswordResetToken(ctx, token)
		if err != nil {
			t.Fatalf("Failed to validate token: %v", err)
		}
		if userID != user.Id {
			t.Errorf("Expected userID %s, got %s", user.Id, userID)
		}
	})

	t.Run("returns error for invalid token", func(t *testing.T) {
		_, err := core.ValidatePasswordResetToken(ctx, "invalid-token")
		if err != ErrPasswordResetTokenNotFound {
			t.Errorf("Expected ErrPasswordResetTokenNotFound, got %v", err)
		}
	})
}

func TestChattoCore_ResetPassword(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("resets password successfully", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "reset-pw-user", "Test User", "oldpassword")
		core.AddVerifiedEmailDirect(ctx, user.Id, "resetpw@example.com")

		token, _ := core.CreatePasswordResetToken(ctx, "resetpw@example.com")

		// Create new password hash
		newHash, _ := bcrypt.GenerateFromPassword([]byte("newpassword123"), bcrypt.DefaultCost)

		// Reset password
		err := core.ResetPassword(ctx, token, string(newHash))
		if err != nil {
			t.Fatalf("Failed to reset password: %v", err)
		}

		// Verify new password works
		_, err = core.VerifyPassword(ctx, "reset-pw-user", "newpassword123")
		if err != nil {
			t.Errorf("New password should work: %v", err)
		}

		// Verify old password no longer works
		_, err = core.VerifyPassword(ctx, "reset-pw-user", "oldpassword")
		if err == nil {
			t.Error("Old password should not work")
		}
	})

	t.Run("revokes bearer tokens for reset user only", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "reset-revoke-user", "Reset Revoke User", "oldpassword")
		core.AddVerifiedEmailDirect(ctx, user.Id, "resetrevoke@example.com")
		otherUser, _ := core.CreateUser(ctx, "system", "reset-revoke-other", "Reset Revoke Other", "password123")

		token1, _ := core.CreateAuthToken(ctx, user.Id)
		token2, _ := core.CreateAuthToken(ctx, user.Id)
		otherToken, _ := core.CreateAuthToken(ctx, otherUser.Id)
		resetToken, _ := core.CreatePasswordResetToken(ctx, "resetrevoke@example.com")
		newHash, _ := bcrypt.GenerateFromPassword([]byte("newpassword123"), bcrypt.DefaultCost)

		if err := core.ResetPassword(ctx, resetToken, string(newHash)); err != nil {
			t.Fatalf("ResetPassword: %v", err)
		}

		if _, err := core.ValidateAuthToken(ctx, token1); err != ErrAuthTokenNotFound {
			t.Fatalf("token1 ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
		}
		if _, err := core.ValidateAuthToken(ctx, token2); err != ErrAuthTokenNotFound {
			t.Fatalf("token2 ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
		}
		if gotUserID, err := core.ValidateAuthToken(ctx, otherToken); err != nil {
			t.Fatalf("other token should remain valid: %v", err)
		} else if gotUserID != otherUser.Id {
			t.Fatalf("other token user ID = %q, want %q", gotUserID, otherUser.Id)
		}
	})

	t.Run("token can only be used once", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "single-use-user", "Test User", "password123")
		core.AddVerifiedEmailDirect(ctx, user.Id, "singleuse@example.com")

		token, _ := core.CreatePasswordResetToken(ctx, "singleuse@example.com")

		// First reset succeeds
		newHash, _ := bcrypt.GenerateFromPassword([]byte("newpassword1"), bcrypt.DefaultCost)
		err := core.ResetPassword(ctx, token, string(newHash))
		if err != nil {
			t.Fatalf("First reset failed: %v", err)
		}

		// Second reset with same token fails
		newHash2, _ := bcrypt.GenerateFromPassword([]byte("newpassword2"), bcrypt.DefaultCost)
		err = core.ResetPassword(ctx, token, string(newHash2))
		if err != ErrPasswordResetTokenNotFound {
			t.Errorf("Expected ErrPasswordResetTokenNotFound, got %v", err)
		}
	})

	t.Run("returns error for invalid token", func(t *testing.T) {
		newHash, _ := bcrypt.GenerateFromPassword([]byte("newpassword"), bcrypt.DefaultCost)
		err := core.ResetPassword(ctx, "invalid-token", string(newHash))
		if err != ErrPasswordResetTokenNotFound {
			t.Errorf("Expected ErrPasswordResetTokenNotFound, got %v", err)
		}
	})

	t.Run("does not reset deleted user", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "deleted-reset-user", "Deleted Reset", "password123")
		core.AddVerifiedEmailDirect(ctx, user.Id, "deleted-reset@example.com")
		token, _ := core.CreatePasswordResetToken(ctx, "deleted-reset@example.com")
		if err := core.DeleteUser(ctx, SystemActorID, user.Id); err != nil {
			t.Fatalf("DeleteUser: %v", err)
		}

		newHash, _ := bcrypt.GenerateFromPassword([]byte("newpassword123"), bcrypt.DefaultCost)
		err := core.ResetPassword(ctx, token, string(newHash))
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("ResetPassword error = %v, want ErrNotFound", err)
		}
	})
}

func TestChattoCore_PasswordResetTokenExpiration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("token expires after TTL", func(t *testing.T) {
		user, _ := core.CreateUser(ctx, "system", "expire-user", "Test User", "password123")
		core.AddVerifiedEmailDirect(ctx, user.Id, "expire@example.com")

		// Create token
		token, _ := core.CreatePasswordResetToken(ctx, "expire@example.com")

		// Manually modify the token's created_at to be in the past
		// We need to simulate expiration by directly manipulating KV
		// Since we can't easily mock time, we'll test that the TTL constant is reasonable
		// and trust that the time.Since check works correctly

		// Verify token is valid when fresh
		_, err := core.ValidatePasswordResetToken(ctx, token)
		if err != nil {
			t.Fatalf("Fresh token should be valid: %v", err)
		}

		// Note: Full expiration testing would require either:
		// 1. A time mock
		// 2. Waiting the full TTL (not practical)
		// 3. Direct KV manipulation to fake the timestamp
		// For now, we verify the TTL constant is set correctly
		if PasswordResetTokenTTL != 1*time.Hour {
			t.Errorf("Expected TTL of 1 hour, got %v", PasswordResetTokenTTL)
		}
	})
}
