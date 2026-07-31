package core

import (
	"errors"
	"strings"
	"testing"
)

func TestChattoCore_VerifyPassword(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	login := "testuser"
	password := "secret123"

	// Create user with password
	_, err := core.CreateUser(ctx, "system", login, "Test User", password)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify correct password
	user, err := core.VerifyPassword(ctx, login, password)
	if err != nil {
		t.Fatalf("Failed to verify password: %v", err)
	}
	if user == nil {
		t.Fatal("Expected user to be returned")
	}
	if user.Login != login {
		t.Errorf("Expected login '%s', got '%s'", login, user.Login)
	}

	// Verify incorrect password
	_, err = core.VerifyPassword(ctx, login, "wrongpassword")
	if err == nil {
		t.Error("Expected error with incorrect password")
	}

	// Verify non-existent user
	_, err = core.VerifyPassword(ctx, "nonexistent", password)
	if err == nil {
		t.Error("Expected error with non-existent user")
	}
}

func TestChattoCore_VerifyPassword_WithEmail(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	login := "emailuser"
	email := "test@example.com"
	password := "secret123"

	// Create user with password
	user, err := core.CreateUser(ctx, "system", login, "Email Test User", password)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Add verified email to user
	err = core.AddVerifiedEmailDirect(ctx, user.Id, email)
	if err != nil {
		t.Fatalf("Failed to add verified email: %v", err)
	}

	// Verify login with username still works
	verified, err := core.VerifyPassword(ctx, login, password)
	if err != nil {
		t.Fatalf("Failed to verify password with login: %v", err)
	}
	if verified.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, verified.Id)
	}

	// Verify login with email works
	verified, err = core.VerifyPassword(ctx, email, password)
	if err != nil {
		t.Fatalf("Failed to verify password with email: %v", err)
	}
	if verified.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, verified.Id)
	}

	// Verify incorrect password with email fails
	_, err = core.VerifyPassword(ctx, email, "wrongpassword")
	if err == nil {
		t.Error("Expected error with incorrect password")
	}

	// Verify non-existent email fails
	_, err = core.VerifyPassword(ctx, "nonexistent@example.com", password)
	if err == nil {
		t.Error("Expected error with non-existent email")
	}

	// Verify email login is case-insensitive
	verified, err = core.VerifyPassword(ctx, "TEST@EXAMPLE.COM", password)
	if err != nil {
		t.Fatalf("Failed to verify password with uppercase email: %v", err)
	}
	if verified.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, verified.Id)
	}
}

func TestChattoCore_SetPasswordHash(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user with initial password
	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "initial123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Change password
	newPassword := "newpassword456"
	err = core.SetPasswordHash(ctx, user.Id, newPassword)
	if err != nil {
		t.Fatalf("Failed to set password: %v", err)
	}

	// Old password should not work
	_, err = core.VerifyPassword(ctx, user.Login, "initial123")
	if err == nil {
		t.Error("Expected old password to fail")
	}

	// New password should work
	verified, err := core.VerifyPassword(ctx, user.Login, newPassword)
	if err != nil {
		t.Fatalf("Failed to verify new password: %v", err)
	}
	if verified.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, verified.Id)
	}
}

func TestChattoCore_SetPasswordHash_RechecksCurrentPasswordAfterOCCConflict(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "stale-password-user", "Stale Password User", "initial123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	checks := 0
	err = core.setPasswordHash(ctx, user.Id, user.Id, "staleoverwrite789", true, func() error {
		checks++
		if err := core.verifyUserPasswordCurrent(ctx, user.Id, "initial123"); err != nil {
			return err
		}
		if checks == 1 {
			if err := core.SetPasswordHash(ctx, user.Id, "newerpassword456"); err != nil {
				return err
			}
		}
		return nil
	})
	if !errors.Is(err, ErrCurrentPasswordInvalid) {
		t.Fatalf("setPasswordHash stale proof error = %v, want ErrCurrentPasswordInvalid", err)
	}
	if checks < 2 {
		t.Fatalf("current password check ran %d time(s), want retry after conflict", checks)
	}
	if _, err := core.VerifyPassword(ctx, user.Login, "newerpassword456"); err != nil {
		t.Fatalf("newer password should remain valid: %v", err)
	}
	if _, err := core.VerifyPassword(ctx, user.Login, "staleoverwrite789"); err == nil {
		t.Fatal("stale password overwrite should not be valid")
	}
}

func TestChattoCore_SetPasswordHashRejectsMissingUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	err := core.SetPasswordHash(ctx, "UmissingPassword", "newpassword456")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetPasswordHash error = %v, want ErrNotFound", err)
	}
}

func TestChattoCore_SetPasswordHash_RevokesBearerTokens(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "password-revoke-user", "Password Revoke User", "initial123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	otherUser, err := core.CreateUser(ctx, "system", "password-revoke-other", "Password Revoke Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}

	token1, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken 1: %v", err)
	}
	token2, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken 2: %v", err)
	}
	otherToken, err := core.CreateAuthToken(ctx, otherUser.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken other: %v", err)
	}

	if err := core.SetPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetPasswordHash: %v", err)
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
}

func TestChattoCore_SetInitialPasswordHash(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "initial-password-user", "Initial Password User", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	token, err := core.CreateAuthToken(ctx, user.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken: %v", err)
	}

	if hasPassword, err := core.HasPassword(ctx, user.Id); err != nil {
		t.Fatalf("HasPassword before: %v", err)
	} else if hasPassword {
		t.Fatal("HasPassword before = true, want false")
	}

	if err := core.SetInitialPasswordHash(ctx, user.Id, "newpassword456"); err != nil {
		t.Fatalf("SetInitialPasswordHash: %v", err)
	}
	if hasPassword, err := core.HasPassword(ctx, user.Id); err != nil {
		t.Fatalf("HasPassword after: %v", err)
	} else if !hasPassword {
		t.Fatal("HasPassword after = false, want true")
	}
	if verified, err := core.VerifyPassword(ctx, user.Login, "newpassword456"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	} else if verified.Id != user.Id {
		t.Fatalf("verified user ID = %q, want %q", verified.Id, user.Id)
	}
	if gotUserID, err := core.ValidateAuthToken(ctx, token); err != nil {
		t.Fatalf("existing auth token should remain valid: %v", err)
	} else if gotUserID != user.Id {
		t.Fatalf("token user ID = %q, want %q", gotUserID, user.Id)
	}
	if err := core.SetInitialPasswordHash(ctx, user.Id, "anotherpassword456"); !errors.Is(err, ErrPasswordAlreadySet) {
		t.Fatalf("second SetInitialPasswordHash err = %v, want ErrPasswordAlreadySet", err)
	}
}

func TestChattoCore_SetOwnPassword(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	passwordless, err := core.CreateUser(ctx, SystemActorID, "own-passwordless", "Own Passwordless", "")
	if err != nil {
		t.Fatalf("CreateUser passwordless: %v", err)
	}
	token, err := core.CreateAuthToken(ctx, passwordless.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken passwordless: %v", err)
	}
	if err := core.SetOwnPassword(ctx, passwordless.Id, "", "newpassword456"); err != nil {
		t.Fatalf("SetOwnPassword passwordless: %v", err)
	}
	if _, err := core.VerifyPassword(ctx, passwordless.Login, "newpassword456"); err != nil {
		t.Fatalf("VerifyPassword passwordless: %v", err)
	}
	if gotUserID, err := core.ValidateAuthToken(ctx, token); err != nil {
		t.Fatalf("initial password should preserve existing token: %v", err)
	} else if gotUserID != passwordless.Id {
		t.Fatalf("token user ID = %q, want %q", gotUserID, passwordless.Id)
	}

	existing, err := core.CreateUser(ctx, SystemActorID, "own-existing", "Own Existing", "oldpassword123")
	if err != nil {
		t.Fatalf("CreateUser existing: %v", err)
	}
	existingToken, err := core.CreateAuthToken(ctx, existing.Id)
	if err != nil {
		t.Fatalf("CreateAuthToken existing: %v", err)
	}
	if err := core.SetOwnPassword(ctx, existing.Id, "", "newpassword456"); !errors.Is(err, ErrCurrentPasswordRequired) {
		t.Fatalf("SetOwnPassword missing current err = %v, want ErrCurrentPasswordRequired", err)
	}
	if err := core.SetOwnPassword(ctx, existing.Id, "wrongpassword", "newpassword456"); !errors.Is(err, ErrCurrentPasswordInvalid) {
		t.Fatalf("SetOwnPassword wrong current err = %v, want ErrCurrentPasswordInvalid", err)
	}
	if err := core.SetOwnPassword(ctx, existing.Id, "oldpassword123", "newpassword456"); err != nil {
		t.Fatalf("SetOwnPassword existing: %v", err)
	}
	if _, err := core.VerifyPassword(ctx, existing.Login, "oldpassword123"); err == nil {
		t.Fatal("old password should no longer verify")
	}
	if _, err := core.VerifyPassword(ctx, existing.Login, "newpassword456"); err != nil {
		t.Fatalf("new password should verify: %v", err)
	}
	if _, err := core.ValidateAuthToken(ctx, existingToken); err != ErrAuthTokenNotFound {
		t.Fatalf("existing password change token err = %v, want ErrAuthTokenNotFound", err)
	}
}

func TestChattoCore_FailedPasswordChangeKeepsOldPasswordUsable(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "password-failed-change-user", "Password Failed Change User", "oldpassword")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.VerifyPassword(ctx, user.Login, "oldpassword"); err != nil {
		t.Fatalf("old password should initially verify: %v", err)
	}
	authGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}

	if err := core.SetPasswordHash(ctx, user.Id, "short"); err == nil {
		t.Fatal("SetPasswordHash should reject too-short password")
	}

	afterGeneration, err := core.CurrentAuthGeneration(ctx, user.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration after failure: %v", err)
	}
	if afterGeneration != authGeneration {
		t.Fatalf("auth generation = %d, want unchanged %d", afterGeneration, authGeneration)
	}
	if verified, err := core.VerifyPassword(ctx, user.Login, "oldpassword"); err != nil {
		t.Fatalf("old password should remain usable after failed change: %v", err)
	} else if verified.Id != user.Id {
		t.Fatalf("verified user ID = %q, want %q", verified.Id, user.Id)
	}
}

func TestChattoCore_CreateUser_WithoutPassword(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create OAuth user without password
	user, err := core.CreateUser(ctx, "system", "oauthuser", "oauthuser", "")
	if err != nil {
		t.Fatalf("Failed to create OAuth user: %v", err)
	}

	if user == nil {
		t.Fatal("Expected user to be returned")
	}

	if user.Id == "" {
		t.Error("Expected user ID to be set")
	}

	// Verify we can retrieve the user
	retrieved, err := core.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if retrieved.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, retrieved.Id)
	}

	// Verify password authentication fails for OAuth-only user
	_, err = core.VerifyPassword(ctx, user.Login, "anypassword")
	if err == nil {
		t.Error("Expected error when verifying password for OAuth-only user")
	}
}

func TestChattoCore_AddPasswordToOAuthUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create OAuth user without password
	user, err := core.CreateUser(ctx, "system", "oauthuser", "oauthuser", "")
	if err != nil {
		t.Fatalf("Failed to create OAuth user: %v", err)
	}

	// Verify no password initially
	_, err = core.VerifyPassword(ctx, user.Login, "anypassword")
	if err == nil {
		t.Error("Expected error when verifying password for OAuth-only user")
	}

	// Add password to OAuth user
	newPassword := "newpassword789"
	err = core.SetPasswordHash(ctx, user.Id, newPassword)
	if err != nil {
		t.Fatalf("Failed to add password to OAuth user: %v", err)
	}

	// Now password should work
	verified, err := core.VerifyPassword(ctx, user.Login, newPassword)
	if err != nil {
		t.Fatalf("Failed to verify new password: %v", err)
	}
	if verified.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, verified.Id)
	}
}

func TestChattoCore_CreateUser_ShortPassword(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Try to create user with password that's too short
	_, err := core.CreateUser(ctx, "system", "testuser", "testuser", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("Expected ErrPasswordTooShort, got: %v", err)
	}
}

func TestChattoCore_CreateUser_TooLongPassword(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// bcrypt silently truncates above 72 bytes, so passwords over MaxPasswordLength
	// must be rejected outright to avoid surprising hash collisions on shared prefixes.
	tooLong := strings.Repeat("a", MaxPasswordLength+1)
	_, err := core.CreateUser(ctx, "system", "testuser", "testuser", tooLong)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("Expected ErrPasswordTooLong, got: %v", err)
	}
}

func TestChattoCore_SetPasswordHash_TooLongPassword(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "initial123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	tooLong := strings.Repeat("a", MaxPasswordLength+1)
	err = core.SetPasswordHash(ctx, user.Id, tooLong)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("Expected ErrPasswordTooLong, got: %v", err)
	}
}
