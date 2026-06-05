package core

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/internal/kms"
)

func TestChattoCore_CreateUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if user == nil {
		t.Fatal("Expected user to be returned")
	}

	if user.Id == "" {
		t.Error("Expected user ID to be set")
	}

	if user.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got '%s'", user.Login)
	}

	// Verify we can retrieve the user
	retrieved, err := core.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}

	if retrieved.Id != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, retrieved.Id)
	}

	// Verify password was stored separately
	_, err = core.VerifyPassword(ctx, user.Login, "password123")
	if err != nil {
		t.Errorf("Expected password to be verifiable: %v", err)
	}
}

type cancelAfterWrapKeyWrapper struct {
	kms.KeyWrapper
	cancel    context.CancelFunc
	wrapped   bool
	wrappedBy string
}

func (w *cancelAfterWrapKeyWrapper) WrapContentKey(ctx context.Context, keyRef string, contentKey, aad []byte) (*kms.WrappedContentKey, error) {
	wrapped, err := w.KeyWrapper.WrapContentKey(ctx, keyRef, contentKey, aad)
	if err == nil && !w.wrapped {
		w.wrapped = true
		w.wrappedBy = keyRef
		w.cancel()
	}
	return wrapped, err
}

func TestChattoCore_CreateUser_AppendFailureCleansUpEncryptionKey(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx, cancel := context.WithCancel(testContext(t))
	wrapper := &cancelAfterWrapKeyWrapper{
		KeyWrapper: core.encryption.keyWrapper,
		cancel:     cancel,
	}
	core.encryption.keyWrapper = wrapper

	_, err := core.CreateUser(ctx, "system", "cancelled-signup", "Cancelled Signup", "password123")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateUser error = %v, want context.Canceled", err)
	}
	if !wrapper.wrapped {
		t.Fatal("test did not reach content-key wrapping")
	}

	exists, err := wrapper.KeyWrapper.KeyExists(context.Background(), wrapper.wrappedBy)
	if err != nil {
		t.Fatalf("KeyExists: %v", err)
	}
	if exists {
		t.Fatalf("encryption key for failed signup key ref %q still exists", wrapper.wrappedBy)
	}
}

func TestChattoCore_GetUser_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetUser(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent user")
	}
}

// TestChattoCore_CreateUser_DisplayNameTooLong tests that oversized display names are rejected.
// This is a security test to prevent storage issues and UI problems.
func TestChattoCore_CreateUser_DisplayNameTooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("display name at max length succeeds", func(t *testing.T) {
		// Create a display name at exactly the max length
		maxName := make([]byte, MaxDisplayNameLength)
		for i := range maxName {
			maxName[i] = 'a'
		}

		_, err := core.CreateUser(ctx, "system", "maxlengthuser", string(maxName), "password123")
		if err != nil {
			t.Errorf("Expected success for display name at max length, got: %v", err)
		}
	})

	t.Run("display name over max length fails", func(t *testing.T) {
		// Create a display name over the max length
		oversizedName := make([]byte, MaxDisplayNameLength+1)
		for i := range oversizedName {
			oversizedName[i] = 'a'
		}

		_, err := core.CreateUser(ctx, "system", "oversizeduser", string(oversizedName), "password123")
		if err == nil {
			t.Error("Expected error for oversized display name")
		}
		if err != ErrDisplayNameTooLong {
			t.Errorf("Expected ErrDisplayNameTooLong, got: %v", err)
		}
	})
}

// TestChattoCore_UpdateUserDisplayName_TooLong tests that oversized display names are rejected on update.
func TestChattoCore_UpdateUserDisplayName_TooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "updateuser", "Original Name", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	t.Run("update to max length succeeds", func(t *testing.T) {
		maxName := make([]byte, MaxDisplayNameLength)
		for i := range maxName {
			maxName[i] = 'b'
		}

		_, err := core.UpdateUserDisplayName(ctx, user.Id, string(maxName))
		if err != nil {
			t.Errorf("Expected success for display name at max length, got: %v", err)
		}
	})

	t.Run("update to over max length fails", func(t *testing.T) {
		oversizedName := make([]byte, MaxDisplayNameLength+1)
		for i := range oversizedName {
			oversizedName[i] = 'c'
		}

		_, err := core.UpdateUserDisplayName(ctx, user.Id, string(oversizedName))
		if err == nil {
			t.Error("Expected error for oversized display name")
		}
		if err != ErrDisplayNameTooLong {
			t.Errorf("Expected ErrDisplayNameTooLong, got: %v", err)
		}
	})
}

// TestChattoCore_CreateUser_InvalidDisplayNameCharacters tests that invalid characters are rejected.
func TestChattoCore_CreateUser_InvalidDisplayNameCharacters(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	tests := []struct {
		name        string
		login       string
		displayName string
		wantErr     error
	}{
		// Valid names
		{"simple ASCII", "user1", "John Doe", nil},
		{"international", "user2", "田中太郎", nil},
		{"with emoji", "user3", "Alice 🚀", nil},
		{"with underscore", "user4", "Cool_User", nil},

		// Invalid - control characters
		{"with newline", "user5", "John\nDoe", ErrDisplayNameInvalidCharacter},
		{"with tab", "user6", "John\tDoe", ErrDisplayNameInvalidCharacter},

		// Invalid - zero-width characters
		{"with ZWSP", "user7", "John\u200BDoe", ErrDisplayNameInvalidCharacter},
		{"with ZWJ", "user8", "John\u200DDoe", ErrDisplayNameInvalidCharacter},

		// Invalid - consecutive spaces
		{"double space", "user9", "John  Doe", ErrDisplayNameInvalidCharacter},

		// Invalid - disallowed punctuation
		{"with semicolon", "user10", "John; DROP TABLE", ErrDisplayNameInvalidCharacter},
		{"with at sign", "user11", "user@domain", ErrDisplayNameInvalidCharacter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := core.CreateUser(ctx, "system", tt.login, tt.displayName, "password123")
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("CreateUser() with display name %q = %v, want nil", tt.displayName, err)
				}
			} else {
				if err != tt.wantErr {
					t.Errorf("CreateUser() with display name %q = %v, want %v", tt.displayName, err, tt.wantErr)
				}
			}
		})
	}
}

// TestChattoCore_UpdateUserDisplayName_InvalidCharacters tests that invalid characters are rejected on update.
func TestChattoCore_UpdateUserDisplayName_InvalidCharacters(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "charuser", "Original Name", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	tests := []struct {
		name        string
		displayName string
		wantErr     error
	}{
		// Valid updates
		{"simple update", "New Name", nil},
		{"with emoji", "Star 🌟", nil},
		{"international", "Müller", nil},

		// Invalid updates
		{"with newline", "Bad\nName", ErrDisplayNameInvalidCharacter},
		{"with ZWSP", "Bad\u200BName", ErrDisplayNameInvalidCharacter},
		{"double space", "Bad  Name", ErrDisplayNameInvalidCharacter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := core.UpdateUserDisplayName(ctx, user.Id, tt.displayName)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("UpdateUserDisplayName() with %q = %v, want nil", tt.displayName, err)
				}
			} else {
				if err != tt.wantErr {
					t.Errorf("UpdateUserDisplayName() with %q = %v, want %v", tt.displayName, err, tt.wantErr)
				}
			}
		})
	}
}

func TestChattoCore_GetUserByLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	created, err := core.CreateUser(ctx, "system", "mylogin", "mylogin", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Retrieve by login
	retrieved, err := core.GetUserByLogin(ctx, "mylogin")
	if err != nil {
		t.Fatalf("Failed to get user by login: %v", err)
	}

	if retrieved.Id != created.Id {
		t.Errorf("Expected user ID '%s', got '%s'", created.Id, retrieved.Id)
	}

	if retrieved.Login != "mylogin" {
		t.Errorf("Expected login 'mylogin', got '%s'", retrieved.Login)
	}
}

func TestChattoCore_GetUserByLogin_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetUserByLogin(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting user by nonexistent login")
	}
}

// TestChattoCore_LoginCaseSensitivity verifies that usernames preserve their
// original casing while remaining case-insensitive for lookup, auth, and uniqueness.
func TestChattoCore_LoginCaseSensitivity(t *testing.T) {
	t.Run("preserves casing on create and lookup", func(t *testing.T) {
		tests := []struct {
			name        string
			createLogin string
			lookupAs    string
		}{
			{"mixed case via lowercase", "AliceSmith", "alicesmith"},
			{"mixed case via uppercase", "AliceSmith", "ALICESMITH"},
			{"mixed case via original", "AliceSmith", "AliceSmith"},
			{"all caps via lowercase", "BOBSMITH", "bobsmith"},
			{"lowercase via uppercase", "charlie", "CHARLIE"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core, _ := setupTestCore(t)
				ctx := testContext(t)

				created, err := core.CreateUser(ctx, "system", tt.createLogin, "User", "password123")
				if err != nil {
					t.Fatalf("Failed to create user: %v", err)
				}

				// Created user should have original casing
				if created.Login != tt.createLogin {
					t.Errorf("Expected login %q, got %q", tt.createLogin, created.Login)
				}

				// Lookup should find by any casing
				found, err := core.GetUserByLogin(ctx, tt.lookupAs)
				if err != nil {
					t.Fatalf("GetUserByLogin(%q) failed: %v", tt.lookupAs, err)
				}
				if found.Id != created.Id {
					t.Errorf("Expected user ID %q, got %q", created.Id, found.Id)
				}

				// Found user should still have original casing
				if found.Login != tt.createLogin {
					t.Errorf("Expected preserved login %q, got %q", tt.createLogin, found.Login)
				}
			})
		}
	})

	t.Run("password auth is case-insensitive", func(t *testing.T) {
		tests := []struct {
			name        string
			createLogin string
			authAs      string
		}{
			{"lowercase login", "AliceSmith", "alicesmith"},
			{"uppercase login", "AliceSmith", "ALICESMITH"},
			{"original casing", "AliceSmith", "AliceSmith"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core, _ := setupTestCore(t)
				ctx := testContext(t)

				_, err := core.CreateUser(ctx, "system", tt.createLogin, "User", "password123")
				if err != nil {
					t.Fatalf("Failed to create user: %v", err)
				}

				verified, err := core.VerifyPassword(ctx, tt.authAs, "password123")
				if err != nil {
					t.Fatalf("VerifyPassword(%q) failed: %v", tt.authAs, err)
				}
				if verified.Login != tt.createLogin {
					t.Errorf("Expected login %q after auth, got %q", tt.createLogin, verified.Login)
				}
			})
		}
	})

	t.Run("uniqueness is case-insensitive", func(t *testing.T) {
		tests := []struct {
			name        string
			firstLogin  string
			secondLogin string
		}{
			{"exact duplicate", "samelogin", "samelogin"},
			{"different case", "uniquename", "UNIQUENAME"},
			{"mixed vs lower", "CamelCase", "camelcase"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				core, _ := setupTestCore(t)
				ctx := testContext(t)

				_, err := core.CreateUser(ctx, "system", tt.firstLogin, "user1", "password123")
				if err != nil {
					t.Fatalf("Failed to create first user: %v", err)
				}

				_, err = core.CreateUser(ctx, "system", tt.secondLogin, "user2", "password456")
				if err == nil {
					t.Errorf("Expected duplicate error creating %q after %q", tt.secondLogin, tt.firstLogin)
				}
			})
		}
	})
}

func TestChattoCore_ConcurrentUserCreation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Try to create two users with the same login concurrently
	// One should succeed, one should fail (atomic login claim)
	login := "concurrent"
	errChan := make(chan error, 2)
	userChan := make(chan string, 2)

	createUser := func(displayName string) {
		user, err := core.CreateUser(ctx, "system", login, displayName, "password123")
		if err != nil {
			errChan <- err
			userChan <- ""
		} else {
			errChan <- nil
			userChan <- user.Id
		}
	}

	go createUser("User 1")
	go createUser("User 2")

	// Collect results
	err1 := <-errChan
	err2 := <-errChan
	user1 := <-userChan
	user2 := <-userChan

	// Exactly one should succeed and one should fail
	successCount := 0
	if err1 == nil {
		successCount++
	}
	if err2 == nil {
		successCount++
	}

	if successCount != 1 {
		t.Errorf("Expected exactly 1 success, got %d (err1=%v, err2=%v)", successCount, err1, err2)
	}

	// Verify only one user exists with this login
	retrieved, err := core.GetUserByLogin(ctx, login)
	if err != nil {
		t.Fatalf("Failed to get user by login: %v", err)
	}

	// The retrieved user should match one of the attempted creations
	if user1 != "" && retrieved.Id != user1 {
		t.Errorf("Expected user ID %s, got %s", user1, retrieved.Id)
	}
	if user2 != "" && retrieved.Id != user2 {
		t.Errorf("Expected user ID %s, got %s", user2, retrieved.Id)
	}
}

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

func TestChattoCore_CreateUser_BlockedUsername(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Default blocked usernames include: root, admin, superuser, op, operator, support
	blockedNames := []string{"admin", "root", "superuser", "op", "operator", "support"}

	for _, name := range blockedNames {
		t.Run(name, func(t *testing.T) {
			_, err := core.CreateUser(ctx, "system", name, name, "password123")
			if err == nil {
				t.Errorf("Expected error when creating user with blocked username '%s'", name)
			}
			if err != ErrUsernameBlocked {
				t.Errorf("Expected ErrUsernameBlocked, got: %v", err)
			}
		})
	}

	// Also test case-insensitivity
	t.Run("ADMIN (uppercase)", func(t *testing.T) {
		_, err := core.CreateUser(ctx, "system", "ADMIN", "ADMIN", "password123")
		if err == nil {
			t.Error("Expected error when creating user with blocked username 'ADMIN'")
		}
		if err != ErrUsernameBlocked {
			t.Errorf("Expected ErrUsernameBlocked, got: %v", err)
		}
	})

	t.Run("Admin (mixed case)", func(t *testing.T) {
		_, err := core.CreateUser(ctx, "system", "Admin", "Admin", "password123")
		if err == nil {
			t.Error("Expected error when creating user with blocked username 'Admin'")
		}
		if err != ErrUsernameBlocked {
			t.Errorf("Expected ErrUsernameBlocked, got: %v", err)
		}
	})
}

func TestChattoCore_UpdateUserLogin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "oldlogin", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	t.Run("successful login change", func(t *testing.T) {
		updated, err := core.UpdateUserLogin(ctx, user.Id, "newlogin")
		if err != nil {
			t.Fatalf("UpdateUserLogin failed: %v", err)
		}
		if updated.Login != "newlogin" {
			t.Errorf("Expected login 'newlogin', got %q", updated.Login)
		}

		// Verify lookup by new login works
		found, err := core.GetUserByLogin(ctx, "newlogin")
		if err != nil {
			t.Fatalf("GetUserByLogin(newlogin) failed: %v", err)
		}
		if found.Id != user.Id {
			t.Errorf("Expected user ID %q, got %q", user.Id, found.Id)
		}

		// Verify old login no longer resolves
		_, err = core.GetUserByLogin(ctx, "oldlogin")
		if err == nil {
			t.Error("Expected error looking up old login, got nil")
		}
	})

	t.Run("preserves mixed case", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "caseytest", "Casey", "password123")

		updated, err := core2.UpdateUserLogin(ctx2, u.Id, "NewCasey")
		if err != nil {
			t.Fatalf("UpdateUserLogin failed: %v", err)
		}
		if updated.Login != "NewCasey" {
			t.Errorf("Expected login 'NewCasey' with preserved casing, got %q", updated.Login)
		}

		// Verify case-insensitive lookup still works
		found, err := core2.GetUserByLogin(ctx2, "newcasey")
		if err != nil {
			t.Fatalf("GetUserByLogin(newcasey) failed: %v", err)
		}
		if found.Login != "NewCasey" {
			t.Errorf("Expected login 'NewCasey', got %q", found.Login)
		}
	})

	t.Run("case-only change is allowed", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "alice", "Alice", "password123")

		updated, err := core2.UpdateUserLogin(ctx2, u.Id, "Alice")
		if err != nil {
			t.Fatalf("Case-only change should succeed, got: %v", err)
		}
		if updated.Login != "Alice" {
			t.Errorf("Expected login 'Alice', got %q", updated.Login)
		}

		// Verify the stored record has the new casing
		found, err := core2.GetUserByLogin(ctx2, "alice")
		if err != nil {
			t.Fatalf("GetUserByLogin(alice) failed: %v", err)
		}
		if found.Login != "Alice" {
			t.Errorf("Expected stored login 'Alice', got %q", found.Login)
		}
	})

	t.Run("case-only change skips cooldown", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "cooluser", "Cool", "password123")

		// First real change triggers cooldown
		_, err := core2.UpdateUserLogin(ctx2, u.Id, "newname")
		if err != nil {
			t.Fatalf("First login change failed: %v", err)
		}

		// A real second change should be blocked by cooldown
		_, err = core2.UpdateUserLogin(ctx2, u.Id, "anothername")
		if err != ErrLoginChangeCooldown {
			t.Errorf("Expected ErrLoginChangeCooldown, got: %v", err)
		}

		// But a case-only change should still work
		updated, err := core2.UpdateUserLogin(ctx2, u.Id, "NewName")
		if err != nil {
			t.Fatalf("Case-only change should bypass cooldown, got: %v", err)
		}
		if updated.Login != "NewName" {
			t.Errorf("Expected login 'NewName', got %q", updated.Login)
		}
	})

	t.Run("unchanged login is no-op", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "SameLogin", "Same", "password123")

		updated, err := core2.UpdateUserLogin(ctx2, u.Id, "SameLogin")
		if err != nil {
			t.Fatalf("Expected no error for unchanged login, got: %v", err)
		}
		if updated.Login != "SameLogin" {
			t.Errorf("Expected login 'SameLogin', got %q", updated.Login)
		}
	})

	t.Run("already taken login", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		core2.CreateUser(ctx2, "system", "taken", "User A", "password123")
		userB, _ := core2.CreateUser(ctx2, "system", "available", "User B", "password123")

		_, err := core2.UpdateUserLogin(ctx2, userB.Id, "taken")
		if err != ErrLoginAlreadyTaken {
			t.Errorf("Expected ErrLoginAlreadyTaken, got: %v", err)
		}
	})

	t.Run("blocked username", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "normaluser", "Normal", "password123")

		_, err := core2.UpdateUserLogin(ctx2, u.Id, "admin")
		if err != ErrUsernameBlocked {
			t.Errorf("Expected ErrUsernameBlocked, got: %v", err)
		}
	})

	t.Run("invalid login characters", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "validuser", "Valid", "password123")

		_, err := core2.UpdateUserLogin(ctx2, u.Id, "invalid user!")
		if err != ErrLoginInvalidCharacter {
			t.Errorf("Expected ErrLoginInvalidCharacter, got: %v", err)
		}
	})

	t.Run("login too short", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "shorttest", "Short", "password123")

		_, err := core2.UpdateUserLogin(ctx2, u.Id, "a")
		if err != ErrLoginTooShort {
			t.Errorf("Expected ErrLoginTooShort, got: %v", err)
		}
	})

	t.Run("cooldown enforcement", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "cooldownuser", "Cool", "password123")

		// First change should succeed
		_, err := core2.UpdateUserLogin(ctx2, u.Id, "changed1")
		if err != nil {
			t.Fatalf("First login change failed: %v", err)
		}

		// Second change should fail with cooldown
		_, err = core2.UpdateUserLogin(ctx2, u.Id, "changed2")
		if err != ErrLoginChangeCooldown {
			t.Errorf("Expected ErrLoginChangeCooldown, got: %v", err)
		}
	})

	t.Run("admin update bypasses cooldown and does not advance the user clock", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "adminuser", "User", "password123")

		// User-driven change starts the cooldown
		if _, err := core2.UpdateUserLogin(ctx2, u.Id, "userchose"); err != nil {
			t.Fatalf("User login change failed: %v", err)
		}
		userTimestamp, err := core2.GetLastLoginChange(ctx2, u.Id)
		if err != nil {
			t.Fatalf("GetLastLoginChange failed: %v", err)
		}
		if userTimestamp.IsZero() {
			t.Fatal("Expected user-driven change to record a timestamp")
		}

		// Admin override succeeds despite the cooldown
		if _, err := core2.AdminUpdateUserLogin(ctx2, u.Id, "adminchose"); err != nil {
			t.Fatalf("Admin login change failed: %v", err)
		}

		// And does not advance the cooldown timestamp — the user retains their
		// original allowance.
		laterTimestamp, err := core2.GetLastLoginChange(ctx2, u.Id)
		if err != nil {
			t.Fatalf("GetLastLoginChange failed: %v", err)
		}
		if !laterTimestamp.Equal(userTimestamp) {
			t.Errorf("Admin edit advanced cooldown clock: was %v, now %v", userTimestamp, laterTimestamp)
		}

		// User attempting another change is still gated by their original cooldown.
		if _, err := core2.UpdateUserLogin(ctx2, u.Id, "userretry"); err != ErrLoginChangeCooldown {
			t.Errorf("Expected ErrLoginChangeCooldown after admin override, got: %v", err)
		}
	})

	t.Run("admin update still rejects blocked usernames", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "blockedtest", "User", "password123")

		_, err := core2.AdminUpdateUserLogin(ctx2, u.Id, "admin")
		if err != ErrUsernameBlocked {
			t.Errorf("Expected ErrUsernameBlocked from admin path, got: %v", err)
		}
	})

	t.Run("admin update still rejects invalid logins", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "invalidtest", "User", "password123")

		_, err := core2.AdminUpdateUserLogin(ctx2, u.Id, "a")
		if err != ErrLoginTooShort {
			t.Errorf("Expected ErrLoginTooShort from admin path, got: %v", err)
		}
	})

	t.Run("clear cooldown unblocks the user", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "clearuser", "User", "password123")

		if _, err := core2.UpdateUserLogin(ctx2, u.Id, "first"); err != nil {
			t.Fatalf("First login change failed: %v", err)
		}
		if _, err := core2.UpdateUserLogin(ctx2, u.Id, "second"); err != ErrLoginChangeCooldown {
			t.Fatalf("Expected cooldown, got: %v", err)
		}

		if err := core2.ClearLoginChangeCooldown(ctx2, u.Id); err != nil {
			t.Fatalf("ClearLoginChangeCooldown failed: %v", err)
		}

		// User can now rename again immediately.
		if _, err := core2.UpdateUserLogin(ctx2, u.Id, "second"); err != nil {
			t.Errorf("Expected rename to succeed after clearing cooldown, got: %v", err)
		}
	})

	t.Run("clear cooldown is idempotent", func(t *testing.T) {
		core2, _ := setupTestCore(t)
		ctx2 := testContext(t)
		u, _ := core2.CreateUser(ctx2, "system", "idempuser", "User", "password123")

		// Never changed login — clearing should still succeed.
		if err := core2.ClearLoginChangeCooldown(ctx2, u.Id); err != nil {
			t.Errorf("ClearLoginChangeCooldown should be idempotent, got: %v", err)
		}
		// Calling again is also fine.
		if err := core2.ClearLoginChangeCooldown(ctx2, u.Id); err != nil {
			t.Errorf("ClearLoginChangeCooldown second call failed: %v", err)
		}
	})
}

func TestChattoCore_GetLastLoginChange(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "lcuser", "LC User", "password123")

	t.Run("returns zero time when never changed", func(t *testing.T) {
		lastChange, err := core.GetLastLoginChange(ctx, user.Id)
		if err != nil {
			t.Fatalf("GetLastLoginChange failed: %v", err)
		}
		if !lastChange.IsZero() {
			t.Errorf("Expected zero time, got %v", lastChange)
		}
	})

	t.Run("returns timestamp after login change", func(t *testing.T) {
		before := time.Now().Add(-time.Second)
		_, err := core.UpdateUserLogin(ctx, user.Id, "newlcuser")
		if err != nil {
			t.Fatalf("UpdateUserLogin failed: %v", err)
		}
		after := time.Now().Add(time.Second)

		lastChange, err := core.GetLastLoginChange(ctx, user.Id)
		if err != nil {
			t.Fatalf("GetLastLoginChange failed: %v", err)
		}
		if lastChange.Before(before) || lastChange.After(after) {
			t.Errorf("Expected timestamp between %v and %v, got %v", before, after, lastChange)
		}
	})
}

// createTestImage creates a test PNG image with the specified dimensions.
func createTestImage(width, height int) io.Reader {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return bytes.NewReader(buf.Bytes())
}

func TestChattoCore_UploadUserAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload avatar
	testImage := createTestImage(100, 100)
	asset, err := core.UploadUserAvatar(ctx, user.Id, testImage)
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}

	if asset == nil {
		t.Fatal("Expected asset to be returned")
	}

	// Verify it's a NATS asset
	natsAsset := asset.GetNats()
	if natsAsset == nil {
		t.Fatal("Expected NATS asset")
	}

	if natsAsset.Key == "" {
		t.Error("Expected asset key to be set")
	}
}

func TestChattoCore_SetUserAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Initially no avatar
	avatar, err := core.GetUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if avatar != nil {
		t.Error("Expected no avatar initially")
	}

	// Upload and set avatar
	testImage := createTestImage(100, 100)
	asset, err := core.UploadUserAvatar(ctx, user.Id, testImage)
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}

	err = core.SetUserAvatar(ctx, user.Id, asset)
	if err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	// Verify avatar is set (stored separately from user record)
	avatar, err = core.GetUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if avatar == nil {
		t.Fatal("Expected avatar to be set")
	}

	if avatar.GetNats().Key != asset.GetNats().Key {
		t.Error("Avatar key mismatch")
	}
}

func TestChattoCore_SetUserAvatar_DoesNotModifyUserProfile(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload and set avatar
	testImage := createTestImage(100, 100)
	asset, _ := core.UploadUserAvatar(ctx, user.Id, testImage)
	err = core.SetUserAvatar(ctx, user.Id, asset)
	if err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	updated, err := core.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if updated.Login != user.Login || updated.DisplayName != user.DisplayName {
		t.Error("User profile fields were modified when avatar changed")
	}

	avatar, err := core.GetUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get avatar: %v", err)
	}
	if avatar == nil || avatar.GetNats().GetKey() != asset.GetNats().GetKey() {
		t.Error("Expected avatar projection to contain the uploaded avatar")
	}
}

func TestChattoCore_GetUserAvatarURL(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "avataruser", "Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// No avatar initially - should return empty string
	url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url != "" {
		t.Errorf("Expected empty URL for user without avatar, got '%s'", url)
	}

	// Upload and set avatar
	testImage := createTestImage(100, 100)
	asset, _ := core.UploadUserAvatar(ctx, user.Id, testImage)
	core.SetUserAvatar(ctx, user.Id, asset)

	// Now should return URL
	url, err = core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url == "" {
		t.Error("Expected non-empty URL after setting avatar")
	}

	// URL should contain the asset key
	if !bytes.Contains([]byte(url), []byte(asset.GetNats().Key)) {
		t.Errorf("URL should contain asset key, got '%s'", url)
	}
}

func TestChattoCore_GetUserAvatarURL_AbsoluteURL(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user with an avatar
	user, err := core.CreateUser(ctx, "system", "absurl-user", "Abs URL User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	testImage := createTestImage(100, 100)
	asset, _ := core.UploadUserAvatar(ctx, user.Id, testImage)
	core.SetUserAvatar(ctx, user.Id, asset)

	t.Run("returns relative URL when AssetBaseURL is empty", func(t *testing.T) {
		core.AssetBaseURL = ""
		url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
		if err != nil {
			t.Fatalf("Failed to get avatar URL: %v", err)
		}
		if !bytes.HasPrefix([]byte(url), []byte("/assets/server/")) {
			t.Errorf("Expected relative URL starting with /assets/server/, got '%s'", url)
		}
	})

	t.Run("returns absolute URL when AssetBaseURL is set", func(t *testing.T) {
		core.AssetBaseURL = "https://chat.example.com"
		defer func() { core.AssetBaseURL = "" }()

		url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
		if err != nil {
			t.Fatalf("Failed to get avatar URL: %v", err)
		}
		if !bytes.HasPrefix([]byte(url), []byte("https://chat.example.com/assets/server/")) {
			t.Errorf("Expected absolute URL, got '%s'", url)
		}
	})

	t.Run("returns absolute transformed URL when AssetBaseURL is set", func(t *testing.T) {
		core.AssetBaseURL = "https://chat.example.com"
		defer func() { core.AssetBaseURL = "" }()

		w, h := 64, 64
		url, err := core.GetUserAvatarURL(ctx, user.Id, &w, &h, "cover")
		if err != nil {
			t.Fatalf("Failed to get avatar URL: %v", err)
		}
		if !bytes.HasPrefix([]byte(url), []byte("https://chat.example.com/assets/server/")) {
			t.Errorf("Expected absolute transformed URL, got '%s'", url)
		}
	})
}

func TestChattoCore_UploadUserAvatar_ReplacesOld(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "replaceuser", "Replace User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload first avatar
	testImage1 := createTestImage(50, 50)
	asset1, _ := core.UploadUserAvatar(ctx, user.Id, testImage1)
	core.SetUserAvatar(ctx, user.Id, asset1)
	oldKey := asset1.GetNats().Key

	// Upload second avatar (should delete old one)
	testImage2 := createTestImage(75, 75)
	asset2, err := core.UploadUserAvatar(ctx, user.Id, testImage2)
	if err != nil {
		t.Fatalf("Failed to upload second avatar: %v", err)
	}

	// Keys should be different
	if asset2.GetNats().Key == oldKey {
		t.Error("Expected different asset keys for old and new avatars")
	}

	// Old asset should be deleted from object store
	_, err = core.ServerStore().Get(ctx, oldKey)
	if err == nil {
		t.Error("Expected old avatar to be deleted from object store")
	}
}

func TestChattoCore_UploadUserAvatar_InvalidUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	testImage := createTestImage(100, 100)
	_, err := core.UploadUserAvatar(ctx, "nonexistent", testImage)
	if err == nil {
		t.Error("Expected error when uploading avatar for non-existent user")
	}
}

func TestChattoCore_DeleteUserAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "deleteavataruser", "Delete Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Upload and set an avatar
	testImage := createTestImage(100, 100)
	asset, err := core.UploadUserAvatar(ctx, user.Id, testImage)
	if err != nil {
		t.Fatalf("Failed to upload avatar: %v", err)
	}
	err = core.SetUserAvatar(ctx, user.Id, asset)
	if err != nil {
		t.Fatalf("Failed to set avatar: %v", err)
	}

	// Verify avatar is set
	url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url == "" {
		t.Fatal("Expected avatar URL to be set before deletion")
	}

	// Delete the avatar
	err = core.DeleteUserAvatar(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to delete avatar: %v", err)
	}

	// Verify avatar is gone
	url, err = core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL after deletion: %v", err)
	}
	if url != "" {
		t.Errorf("Expected empty avatar URL after deletion, got '%s'", url)
	}

	// Verify asset was removed from object store
	_, err = core.ServerStore().Get(ctx, asset.GetNats().Key)
	if err == nil {
		t.Error("Expected asset to be deleted from object store")
	}
}

func TestChattoCore_DeleteUserAvatar_NoAvatar(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user without an avatar
	user, err := core.CreateUser(ctx, "system", "noavataruser", "No Avatar User", "")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Delete should be a no-op (not an error)
	err = core.DeleteUserAvatar(ctx, user.Id)
	if err != nil {
		t.Errorf("DeleteUserAvatar on user without avatar should not error, got: %v", err)
	}

	// Verify still no avatar
	url, err := core.GetUserAvatarURL(ctx, user.Id, nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to get avatar URL: %v", err)
	}
	if url != "" {
		t.Errorf("Expected empty avatar URL, got '%s'", url)
	}
}

func TestChattoCore_DeleteUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "deletetest", "Delete Test", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify user exists
	_, err = core.GetUser(ctx, user.Id)
	if err != nil {
		t.Fatalf("Failed to get user after creation: %v", err)
	}

	// Delete the user (self-deletion)
	err = core.DeleteUser(ctx, user.Id, user.Id)
	if err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	// Verify user no longer exists
	_, err = core.GetUser(ctx, user.Id)
	if err == nil {
		t.Error("Expected error when getting deleted user")
	}

	// Verify login index is removed (can't retrieve by login)
	_, err = core.GetUserByLogin(ctx, "deletetest")
	if err == nil {
		t.Error("Expected error when getting deleted user by login")
	}

	// Verify password no longer works
	_, err = core.VerifyPassword(ctx, "deletetest", "password123")
	if err == nil {
		t.Error("Expected error when verifying password for deleted user")
	}
}

// TestChattoCore_CanDeleteUser tests the authorization check function.
// Note: Core.DeleteUser no longer checks authorization - that's the API layer's responsibility.
// This test verifies the CanDeleteUser helper that the API layer uses.
func TestChattoCore_CanDeleteUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "user1", "User One", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}

	user2, err := core.CreateUser(ctx, "system", "user2", "User Two", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// user1 can delete themselves
	canDelete, err := core.CanDeleteUser(ctx, user1.Id, user1.Id)
	if err != nil {
		t.Fatalf("CanDeleteUser failed: %v", err)
	}
	if !canDelete {
		t.Error("user1 should be able to delete themselves")
	}

	// user1 cannot delete user2 (no admin permission)
	canDelete, err = core.CanDeleteUser(ctx, user1.Id, user2.Id)
	if err != nil {
		t.Fatalf("CanDeleteUser failed: %v", err)
	}
	if canDelete {
		t.Error("user1 should NOT be able to delete user2 without permission")
	}

	// user2 can still be retrieved (we only tested authorization, not deletion)
	_, err = core.GetUser(ctx, user2.Id)
	if err != nil {
		t.Fatalf("user2 should still exist: %v", err)
	}
}

func TestChattoCore_DeleteUser_PreservesSpaceAndPurgesUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "spacemember", "Space Member", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	if err := core.DeleteUser(ctx, user.Id, user.Id); err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	if _, err := core.GetUser(ctx, user.Id); err == nil {
		t.Error("Expected user record to be gone after deletion")
	}
}

func TestChattoCore_DeleteUser_WithVerifiedEmail(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "emailtest", "Email Test", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Add a verified email directly
	err = core.AddVerifiedEmailDirect(ctx, user.Id, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to add verified email: %v", err)
	}

	// Verify email is claimed
	isClaimed, err := core.IsEmailClaimed(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to check email claim: %v", err)
	}
	if !isClaimed {
		t.Error("Expected email to be claimed")
	}

	// Delete the user
	err = core.DeleteUser(ctx, user.Id, user.Id)
	if err != nil {
		t.Fatalf("Failed to delete user: %v", err)
	}

	// Verify email is no longer claimed (index entry deleted)
	isClaimed, err = core.IsEmailClaimed(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("Failed to check email claim after deletion: %v", err)
	}
	if isClaimed {
		t.Error("Expected email to no longer be claimed after user deletion")
	}
}

func TestChattoCore_DeleteUser_WithMessageBodies(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "msgauthor", "Msg Author", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "otheruser", "Other User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a space with both users

	// User 2 joins the space

	// Create a room
	room, err := core.CreateRoom(ctx, user1.Id, KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Both users join the room
	_, err = core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user1): %v", err)
	}
	_, err = core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user2): %v", err)
	}

	// User 1 posts two messages
	event1, err := core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 1 from user1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 1: %v", err)
	}
	msg1ID := event1.Id

	event2, err := core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 2 from user1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 2: %v", err)
	}
	msg2ID := event2.Id

	// User 2 posts one message
	event3, err := core.PostMessage(ctx, KindChannel, room.Id, user2.Id, "Message from user2", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 3: %v", err)
	}
	msg3ID := event3.Id

	// Verify all message bodies exist
	_, err = core.GetMessageBody(ctx, KindChannel, msg1ID)
	if err != nil {
		t.Fatalf("Expected message 1 to exist: %v", err)
	}
	_, err = core.GetMessageBody(ctx, KindChannel, msg2ID)
	if err != nil {
		t.Fatalf("Expected message 2 to exist: %v", err)
	}
	_, err = core.GetMessageBody(ctx, KindChannel, msg3ID)
	if err != nil {
		t.Fatalf("Expected message 3 to exist: %v", err)
	}

	// Delete user 1
	err = core.DeleteUser(ctx, user1.Id, user1.Id)
	if err != nil {
		t.Fatalf("Failed to delete user1: %v", err)
	}

	// Verify user 1's message bodies are deleted (GetMessageBody returns empty string for missing bodies)
	body1, err := core.GetMessageBody(ctx, KindChannel, msg1ID)
	if err != nil {
		t.Fatalf("Unexpected error getting message 1: %v", err)
	}
	if body1 != "" {
		t.Errorf("Expected message 1 body to be empty after user deletion, got: %s", body1)
	}

	body2, err := core.GetMessageBody(ctx, KindChannel, msg2ID)
	if err != nil {
		t.Fatalf("Unexpected error getting message 2: %v", err)
	}
	if body2 != "" {
		t.Errorf("Expected message 2 body to be empty after user deletion, got: %s", body2)
	}

	// Verify user 2's message body still exists
	body3, err := core.GetMessageBody(ctx, KindChannel, msg3ID)
	if err != nil {
		t.Fatalf("Failed to get message 3: %v", err)
	}
	if body3 == "" {
		t.Error("Expected message 3 body to still exist after user1 deletion")
	}
}

func TestChattoCore_DeleteUser_RoomMembershipIntegrity(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "deleteuser1", "Delete User 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "remaininguser", "Remaining User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a space

	// User 2 joins the space

	// Create a room
	room, err := core.CreateRoom(ctx, user1.Id, KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Both users join the room
	_, err = core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user1): %v", err)
	}
	_, err = core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user2): %v", err)
	}

	// Verify both users are room members
	members, err := core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room members before deletion: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("Expected 2 room members before deletion, got %d", len(members))
	}

	// Delete user 1
	err = core.DeleteUser(ctx, user1.Id, user1.Id)
	if err != nil {
		t.Fatalf("Failed to delete user1: %v", err)
	}

	// CRITICAL: Verify user 2 is still a room member
	members, err = core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room members after deletion: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("Expected 1 room member after deletion, got %d", len(members))
	}

	// Verify the remaining member is user 2
	if len(members) > 0 && members[0].UserId != user2.Id {
		t.Errorf("Expected remaining member to be user2 (%s), got %s", user2.Id, members[0].UserId)
	}

	// Verify user 2 can still check their own membership
	isMember, err := core.RoomMembershipExists(ctx, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check room membership for user2: %v", err)
	}
	if !isMember {
		t.Error("Expected user2 to still be a room member")
	}

	// Verify a new user can join and be listed
	user3, err := core.CreateUser(ctx, "system", "newuser", "New User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user3: %v", err)
	}
	_, err = core.JoinRoom(ctx, user3.Id, KindChannel, user3.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room (user3): %v", err)
	}

	// Verify all expected members are listed
	members, err = core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room members after new user joins: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("Expected 2 room members after new user joins, got %d", len(members))
	}
}
