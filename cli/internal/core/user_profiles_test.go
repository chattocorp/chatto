package core

import (
	"errors"
	"strings"
	"testing"
	"time"

	"hmans.de/chatto/internal/evtstream"
)

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

func TestChattoCore_UpdateUserLoginReleasesOldMentionHandle(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "oldhandle", "Old Handle", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.UpdateUserLogin(ctx, user.Id, "newhandle"); err != nil {
		t.Fatalf("UpdateUserLogin: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "oldhandle", "Old Handle", ""); err != nil {
		t.Fatalf("CreateServerRole with released login: %v", err)
	}
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

func TestChattoCore_AdminUpdateUserAuthorization(t *testing.T) {
	t.Run("unauthenticated actor is rejected", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		target, err := c.CreateUser(ctx, SystemActorID, "adminauth-target", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}
		login := "adminauth-renamed"
		_, err = c.AdminUpdateUser(ctx, "", target.Id, AdminUpdateUserInput{Login: &login})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Fatalf("AdminUpdateUser err = %v, want ErrNotAuthenticated", err)
		}
		if err := c.AdminSetUserPasswordAuthorized(ctx, "", target.Id, "newpassword456"); !errors.Is(err, ErrNotAuthenticated) {
			t.Fatalf("AdminSetUserPasswordAuthorized err = %v, want ErrNotAuthenticated", err)
		}
	})

	t.Run("regular user cannot update another user", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		regular, err := c.CreateUser(ctx, SystemActorID, "adminauth-regular", "Regular", "password123")
		if err != nil {
			t.Fatalf("CreateUser regular: %v", err)
		}
		target, err := c.CreateUser(ctx, SystemActorID, "adminauth-target2", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}
		login := "adminauth-denied"
		_, err = c.AdminUpdateUser(ctx, regular.Id, target.Id, AdminUpdateUserInput{Login: &login})
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminUpdateUser err = %v, want ErrPermissionDenied", err)
		}
		if err := c.AdminClearLoginChangeCooldown(ctx, regular.Id, target.Id); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminClearLoginChangeCooldown err = %v, want ErrPermissionDenied", err)
		}
		if err := c.AdminSetUserPasswordAuthorized(ctx, regular.Id, target.Id, "newpassword456"); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminSetUserPasswordAuthorized err = %v, want ErrPermissionDenied", err)
		}
	})

	t.Run("admin role holder can update another user", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		admin, err := c.CreateUser(ctx, SystemActorID, "adminauth-admin", "Admin", "password123")
		if err != nil {
			t.Fatalf("CreateUser admin: %v", err)
		}
		if err := c.AssignAdminRole(ctx, admin.Id); err != nil {
			t.Fatalf("AssignAdminRole: %v", err)
		}
		target, err := c.CreateUser(ctx, SystemActorID, "adminauth-target3", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}
		login := "adminauth-updated"
		displayName := "Admin Updated"
		updated, err := c.AdminUpdateUser(ctx, admin.Id, target.Id, AdminUpdateUserInput{
			Login:       &login,
			DisplayName: &displayName,
		})
		if err != nil {
			t.Fatalf("AdminUpdateUser: %v", err)
		}
		if updated.GetLogin() != login || updated.GetDisplayName() != displayName {
			t.Fatalf("updated user = %+v, want login %q display %q", updated, login, displayName)
		}
		if err := c.AdminClearLoginChangeCooldown(ctx, admin.Id, target.Id); err != nil {
			t.Fatalf("AdminClearLoginChangeCooldown: %v", err)
		}
		if err := c.AdminSetUserPasswordAuthorized(ctx, admin.Id, target.Id, "adminpassword456"); err != nil {
			t.Fatalf("AdminSetUserPasswordAuthorized: %v", err)
		}
		if _, err := c.VerifyPassword(ctx, updated.GetLogin(), "adminpassword456"); err != nil {
			t.Fatalf("admin-set password should verify: %v", err)
		}
		if err := c.GrantUserPermission(ctx, SystemActorID, admin.Id, PermAdminAuditView); err != nil {
			t.Fatalf("GrantUserPermission admin.view-audit: %v", err)
		}
		log, err := c.ListEventLog(ctx, admin.Id, EventLogQuery{
			Limit: 10,
			Filter: EventLogFilter{
				EventType: "UserPasswordHashChangedEvent",
				ActorID:   admin.Id,
			},
		})
		if err != nil {
			t.Fatalf("ListEventLog password reset: %v", err)
		}
		var found bool
		for _, entry := range log.Entries {
			if strings.Contains(entry.PayloadJSON, target.Id) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("admin password reset audit entry with actor %q for target %q not found: %+v", admin.Id, target.Id, log.Entries)
		}
	})

	t.Run("role assignment permission cannot reset another user password", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		roleAssigner, err := c.CreateUser(ctx, SystemActorID, "adminauth-role-assigner", "Role Assigner", "password123")
		if err != nil {
			t.Fatalf("CreateUser role assigner: %v", err)
		}
		if err := c.GrantUserPermission(ctx, SystemActorID, roleAssigner.Id, PermRoleAssign); err != nil {
			t.Fatalf("GrantUserPermission role.assign: %v", err)
		}
		target, err := c.CreateUser(ctx, SystemActorID, "adminauth-target-role-only", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}

		if err := c.AdminSetUserPasswordAuthorized(ctx, roleAssigner.Id, target.Id, "newpassword456"); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminSetUserPasswordAuthorized err = %v, want ErrPermissionDenied", err)
		}
		if _, err := c.VerifyPassword(ctx, target.Login, "password123"); err != nil {
			t.Fatalf("original password should still verify: %v", err)
		}
	})

	t.Run("account management permission can reset another user password", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		accountManager, err := c.CreateUser(ctx, SystemActorID, "adminauth-account-manager", "Account Manager", "password123")
		if err != nil {
			t.Fatalf("CreateUser account manager: %v", err)
		}
		if err := c.GrantUserPermission(ctx, SystemActorID, accountManager.Id, PermUserManageAccounts); err != nil {
			t.Fatalf("GrantUserPermission user.manage-accounts: %v", err)
		}
		target, err := c.CreateUser(ctx, SystemActorID, "adminauth-target-account-manager", "Target", "password123")
		if err != nil {
			t.Fatalf("CreateUser target: %v", err)
		}

		if err := c.AdminSetUserPasswordAuthorized(ctx, accountManager.Id, target.Id, "managedpassword456"); err != nil {
			t.Fatalf("AdminSetUserPasswordAuthorized: %v", err)
		}
		if _, err := c.VerifyPassword(ctx, target.Login, "managedpassword456"); err != nil {
			t.Fatalf("account-manager-set password should verify: %v", err)
		}
	})

	t.Run("self update uses account path not admin mutation path", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		user, err := c.CreateUser(ctx, SystemActorID, "adminauth-self", "Self", "password123")
		if err != nil {
			t.Fatalf("CreateUser self: %v", err)
		}
		login := "adminauth-self-updated"
		if _, err := c.AdminUpdateUser(ctx, user.Id, user.Id, AdminUpdateUserInput{Login: &login}); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminUpdateUser self err = %v, want ErrPermissionDenied", err)
		}
		if err := c.AdminClearLoginChangeCooldown(ctx, user.Id, user.Id); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("AdminClearLoginChangeCooldown self err = %v, want ErrPermissionDenied", err)
		}
		updated, err := c.UpdateUserLogin(ctx, user.Id, login)
		if err != nil {
			t.Fatalf("UpdateUserLogin self: %v", err)
		}
		if updated.GetLogin() != login {
			t.Fatalf("updated login = %q, want %q", updated.GetLogin(), login)
		}
	})

	t.Run("self password reset is rejected", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)
		user, err := c.CreateUser(ctx, SystemActorID, "adminauth-self-password", "Self Password", "password123")
		if err != nil {
			t.Fatalf("CreateUser self: %v", err)
		}
		if err := c.AdminSetUserPasswordAuthorized(ctx, user.Id, user.Id, "newpassword456"); !errors.Is(err, ErrAdminCannotSetOwnPassword) {
			t.Fatalf("AdminSetUserPasswordAuthorized self err = %v, want ErrAdminCannotSetOwnPassword", err)
		}
		if _, err := c.VerifyPassword(ctx, user.Login, "password123"); err != nil {
			t.Fatalf("original password should still verify: %v", err)
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

func TestChattoCore_SetAndClearUserCustomStatus(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "statususer", "Status User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC()

	updated, err := core.SetUserCustomStatus(ctx, user.Id, "🌿", "In focus mode", &expiresAt)
	if err != nil {
		t.Fatalf("SetUserCustomStatus failed: %v", err)
	}
	if got := updated.GetCustomStatus().GetEmoji(); got != "🌿" {
		t.Fatalf("custom status emoji = %q, want 🌿", got)
	}
	if got := updated.GetCustomStatus().GetText(); got != "In focus mode" {
		t.Fatalf("custom status text = %q, want In focus mode", got)
	}

	if _, err := core.SetUserCustomStatus(ctx, user.Id, "🌿", "   ", nil); !errors.Is(err, ErrCustomStatusTextRequired) {
		t.Fatalf("SetUserCustomStatus blank text error = %v, want ErrCustomStatusTextRequired", err)
	}
	if _, err := core.SetUserCustomStatus(ctx, user.Id, "e", "Invalid emoji", nil); !errors.Is(err, ErrCustomStatusEmojiInvalid) {
		t.Fatalf("SetUserCustomStatus invalid emoji error = %v, want ErrCustomStatusEmojiInvalid", err)
	}
	if _, err := core.SetUserCustomStatus(ctx, user.Id, "🌿🌿", "Too many emoji", nil); !errors.Is(err, ErrCustomStatusEmojiInvalid) {
		t.Fatalf("SetUserCustomStatus multiple emoji error = %v, want ErrCustomStatusEmojiInvalid", err)
	}

	statusEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserCustomStatusSet))
	if err != nil {
		t.Fatalf("SubjectEvents custom status set failed: %v", err)
	}
	if len(statusEvents) != 1 {
		t.Fatalf("custom status set events = %d, want 1", len(statusEvents))
	}

	cleared, err := core.ClearUserCustomStatus(ctx, user.Id)
	if err != nil {
		t.Fatalf("ClearUserCustomStatus failed: %v", err)
	}
	if cleared.GetCustomStatus() != nil {
		t.Fatalf("custom status after clear = %#v, want nil", cleared.GetCustomStatus())
	}

	clearEvents, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserCustomStatusCleared))
	if err != nil {
		t.Fatalf("SubjectEvents custom status cleared failed: %v", err)
	}
	if len(clearEvents) != 1 {
		t.Fatalf("custom status cleared events = %d, want 1", len(clearEvents))
	}
}

func TestChattoCore_UpdateUserBio(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "biouser", "Bio User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	bioEvents := func() int {
		events, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserBioChanged))
		if err != nil {
			t.Fatalf("SubjectEvents bio changed failed: %v", err)
		}
		return len(events)
	}

	updated, err := core.UpdateUserBio(ctx, user.Id, "  Hello, I write chat software.  ")
	if err != nil {
		t.Fatalf("UpdateUserBio failed: %v", err)
	}
	if got := updated.GetBio(); got != "Hello, I write chat software." {
		t.Fatalf("bio = %q, want trimmed text", got)
	}
	if got, err := core.GetUser(ctx, user.Id); err != nil || got.GetBio() != "Hello, I write chat software." {
		t.Fatalf("projected bio = %q, err = %v", got.GetBio(), err)
	}
	if n := bioEvents(); n != 1 {
		t.Fatalf("bio events after first set = %d, want 1", n)
	}

	// Unchanged bio must be a no-op: no EVT append.
	if _, err := core.UpdateUserBio(ctx, user.Id, "Hello, I write chat software."); err != nil {
		t.Fatalf("no-op UpdateUserBio failed: %v", err)
	}
	if n := bioEvents(); n != 1 {
		t.Fatalf("bio events after unchanged update = %d, want 1 (no new event)", n)
	}

	atLimit := strings.Repeat("x", MaxBioLength)
	if _, err := core.UpdateUserBio(ctx, user.Id, atLimit); err != nil {
		t.Fatalf("bio at length limit failed: %v", err)
	}

	tooLong := strings.Repeat("x", MaxBioLength+1)
	if _, err := core.UpdateUserBio(ctx, user.Id, tooLong); !errors.Is(err, ErrBioTooLong) {
		t.Fatalf("oversized bio error = %v, want ErrBioTooLong", err)
	}

	cleared, err := core.UpdateUserBio(ctx, user.Id, "   ")
	if err != nil {
		t.Fatalf("clear bio failed: %v", err)
	}
	if cleared.GetBio() != "" {
		t.Fatalf("bio after clear = %q, want empty", cleared.GetBio())
	}
	if n := bioEvents(); n != 3 {
		t.Fatalf("bio events after clear = %d, want 3", n)
	}
}
