package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// User Field Resolver Tests
// ============================================================================

func TestUserResolver_Spaces(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("get own spaces", func(t *testing.T) {
		// Must be authenticated as the user whose spaces we're querying
		spaces, err := env.resolver.User().Spaces(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(spaces) == 0 {
			t.Fatal("Expected at least one space")
		}

		// Verify test space exists
		found := false
		for _, space := range spaces {
			if space.Id == env.testSpace.Id {
				found = true
				break
			}
		}

		if !found {
			t.Error("Test space not found")
		}
	})

	t.Run("cannot view other user's spaces", func(t *testing.T) {
		// Create another user
		otherUser, err := env.core.CreateUser(env.ctx, "system", "otheruser", "Other User", "password123")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		// Try to view otherUser's spaces while authenticated as testUser
		_, err = env.resolver.User().Spaces(env.authContext(), otherUser)
		if !errors.Is(err, ErrNotSelf) {
			t.Errorf("Expected ErrNotSelf, got %v", err)
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		_, err := env.resolver.User().Spaces(env.unauthContext(), env.testUser)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestUserResolver_Rooms(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("get own rooms", func(t *testing.T) {
		// Must be authenticated as the user whose rooms we're querying
		rooms, err := env.resolver.User().Rooms(env.authContext(), env.testUser, env.testSpace.Id, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(rooms) == 0 {
			t.Fatal("Expected at least one room")
		}

		// Verify test room exists
		found := false
		for _, room := range rooms {
			if room.Id == env.testRoom.Id {
				found = true
				break
			}
		}

		if !found {
			t.Error("Test room not found")
		}
	})

	t.Run("cannot view other user's rooms", func(t *testing.T) {
		// Create another user
		otherUser, err := env.core.CreateUser(env.ctx, "system", "otheruser-room", "Other User", "password123")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		// Try to view otherUser's rooms while authenticated as testUser
		_, err = env.resolver.User().Rooms(env.authContext(), otherUser, env.testSpace.Id, nil)
		if !errors.Is(err, ErrNotSelf) {
			t.Errorf("Expected ErrNotSelf, got %v", err)
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		_, err := env.resolver.User().Rooms(env.unauthContext(), env.testUser, env.testSpace.Id, nil)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

// ============================================================================
// User.AvatarURL Field Resolver Tests
// ============================================================================

func TestUserResolver_AvatarURL(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.User()

	t.Run("returns nil when no avatar set", func(t *testing.T) {
		url, err := resolver.AvatarURL(env.ctx, env.testUser, nil, nil)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if url != nil {
			t.Errorf("expected nil URL for user without avatar, got %s", *url)
		}
	})

	t.Run("works without auth context (public field)", func(t *testing.T) {
		url, err := resolver.AvatarURL(env.unauthContext(), env.testUser, nil, nil)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if url != nil {
			t.Errorf("expected nil URL, got %s", *url)
		}
	})

	t.Run("accepts width and height parameters", func(t *testing.T) {
		w, h := int32(100), int32(100)
		url, err := resolver.AvatarURL(env.ctx, env.testUser, &w, &h)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		// No avatar set, so still nil
		if url != nil {
			t.Errorf("expected nil URL, got %s", *url)
		}
	})
}

// ============================================================================
// User.HasVerifiedEmail Field Resolver Tests
// ============================================================================

func TestUserResolver_HasVerifiedEmail(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	resolver := env.resolver.User()

	t.Run("authenticated user can check own verified email status", func(t *testing.T) {
		// testUser has a verified email
		hasVerified, err := resolver.HasVerifiedEmail(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !hasVerified {
			t.Error("expected true for user with verified email")
		}
	})

	t.Run("unauthenticated returns false", func(t *testing.T) {
		hasVerified, err := resolver.HasVerifiedEmail(env.unauthContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if hasVerified {
			t.Error("expected false for unauthenticated request")
		}
	})

	t.Run("non-admin cannot check other user's status", func(t *testing.T) {
		otherUser := env.createVerifiedUser(t, "other-verified", "Other User", "password123")

		// otherUser checking testUser's status - not admin, not self
		hasVerified, err := resolver.HasVerifiedEmail(env.authContextForUser(otherUser), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if hasVerified {
			t.Error("expected false when non-admin checks other user's status")
		}
	})

	t.Run("admin can check other user's status", func(t *testing.T) {
		otherUser := env.createVerifiedUser(t, "other-for-admin-check", "Other For Admin", "password123")

		// testUser is admin (email in admin config) checking otherUser
		hasVerified, err := resolver.HasVerifiedEmail(env.authContext(), otherUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !hasVerified {
			t.Error("expected true - admin should see other user's verified email status")
		}
	})
}

// ============================================================================
// User.VerifiedEmails Field Resolver Tests
// ============================================================================

func TestUserResolver_VerifiedEmails(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	resolver := env.resolver.User()

	t.Run("can view own verified emails", func(t *testing.T) {
		emails, err := resolver.VerifiedEmails(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(emails) == 0 {
			t.Fatal("expected at least one verified email")
		}
		if emails[0] != "testuser@example.com" {
			t.Errorf("expected 'testuser@example.com', got %s", emails[0])
		}
	})

	t.Run("non-admin gets empty list for other user", func(t *testing.T) {
		otherUser := env.createVerifiedUser(t, "other-ve", "Other VE", "password123")

		emails, err := resolver.VerifiedEmails(env.authContextForUser(otherUser), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(emails) != 0 {
			t.Errorf("expected empty list for non-admin, got %v", emails)
		}
	})

	t.Run("unauthenticated gets empty list", func(t *testing.T) {
		emails, err := resolver.VerifiedEmails(env.unauthContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(emails) != 0 {
			t.Errorf("expected empty list for unauthenticated, got %v", emails)
		}
	})

	t.Run("admin can view other user's verified emails", func(t *testing.T) {
		otherUser := env.createVerifiedUser(t, "other-ve-admin", "Other VE Admin", "password123")

		// testUser is admin
		emails, err := resolver.VerifiedEmails(env.authContext(), otherUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(emails) == 0 {
			t.Fatal("expected admin to see other user's emails")
		}
		if emails[0] != "other-ve-admin@example.com" {
			t.Errorf("expected 'other-ve-admin@example.com', got %s", emails[0])
		}
	})
}

// ============================================================================
// User.InstanceRoles Field Resolver Tests
// ============================================================================

func TestUserResolver_InstanceRoles(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.User()

	t.Run("authenticated user gets instance roles", func(t *testing.T) {
		roles, err := resolver.InstanceRoles(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		// Every user should have at least the member role
		if roles == nil {
			t.Fatal("expected roles slice, got nil")
		}
	})

	t.Run("unauthenticated gets empty list", func(t *testing.T) {
		roles, err := resolver.InstanceRoles(env.unauthContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(roles) != 0 {
			t.Errorf("expected empty list for unauthenticated, got %v", roles)
		}
	})

	t.Run("can view other user's roles when authenticated", func(t *testing.T) {
		otherUser, err := env.core.CreateUser(env.ctx, "system", "other-roles", "Other Roles", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		roles, err := resolver.InstanceRoles(env.authContext(), otherUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if roles == nil {
			t.Fatal("expected roles slice, got nil")
		}
	})
}

// ============================================================================
// User.SpaceRoles Field Resolver Tests
// ============================================================================

func TestUserResolver_SpaceRoles(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.User()

	t.Run("space member can view own roles", func(t *testing.T) {
		roles, err := resolver.SpaceRoles(env.authContext(), env.testUser, env.testSpace.Id)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		// Space creator should have roles
		if roles == nil {
			t.Fatal("expected roles slice, got nil")
		}
	})

	t.Run("space member can view other member's roles", func(t *testing.T) {
		otherUser, err := env.core.CreateUser(env.ctx, "system", "other-space-roles", "Other Space Roles", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, otherUser.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("failed to join space: %v", err)
		}

		// testUser viewing otherUser's roles in the same space
		roles, err := resolver.SpaceRoles(env.authContext(), otherUser, env.testSpace.Id)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if roles == nil {
			t.Fatal("expected roles slice, got nil")
		}
	})

	t.Run("non-member gets empty list", func(t *testing.T) {
		nonMember, err := env.core.CreateUser(env.ctx, "system", "non-member-roles", "Non Member", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// nonMember is not in the space, tries to view testUser's roles
		roles, err := resolver.SpaceRoles(env.authContextForUser(nonMember), env.testUser, env.testSpace.Id)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(roles) != 0 {
			t.Errorf("expected empty list for non-member, got %v", roles)
		}
	})

	t.Run("unauthenticated gets empty list", func(t *testing.T) {
		roles, err := resolver.SpaceRoles(env.unauthContext(), env.testUser, env.testSpace.Id)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if len(roles) != 0 {
			t.Errorf("expected empty list for unauthenticated, got %v", roles)
		}
	})
}

// ============================================================================
// User.ViewerCanDeleteAccount Field Resolver Tests
// ============================================================================

func TestUserResolver_ViewerCanDeleteAccount(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.User()

	t.Run("unauthenticated returns false", func(t *testing.T) {
		canDelete, err := resolver.ViewerCanDeleteAccount(env.unauthContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if canDelete {
			t.Error("expected false for unauthenticated viewer")
		}
	})

	t.Run("authenticated user gets a result for own account", func(t *testing.T) {
		// Just verify it doesn't error - the actual logic is in Core
		_, err := resolver.ViewerCanDeleteAccount(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
	})
}

// ============================================================================
// User.LastLoginChange Field Resolver Tests
// ============================================================================

func TestUserResolver_LastLoginChange(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.User()

	t.Run("self can view last login change", func(t *testing.T) {
		// May return nil if no login change recorded, but shouldn't error
		_, err := resolver.LastLoginChange(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
	})

	t.Run("other user gets nil", func(t *testing.T) {
		otherUser, err := env.core.CreateUser(env.ctx, "system", "other-login", "Other Login", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// testUser trying to view otherUser's last login change
		result, err := resolver.LastLoginChange(env.authContext(), otherUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != nil {
			t.Error("expected nil for other user's last login change")
		}
	})

	t.Run("unauthenticated gets nil", func(t *testing.T) {
		result, err := resolver.LastLoginChange(env.unauthContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != nil {
			t.Error("expected nil for unauthenticated request")
		}
	})
}

// ============================================================================
// UpdateMyPresence Mutation Tests
// ============================================================================

func TestMutation_UpdateMyPresence(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	t.Run("authenticated user can set online", func(t *testing.T) {
		result, err := mutation.UpdateMyPresence(env.authContext(), model.UpdateMyPresenceInput{Status: model.PresenceStatusOnline})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !result {
			t.Error("expected true result")
		}
	})

	t.Run("authenticated user can set away", func(t *testing.T) {
		result, err := mutation.UpdateMyPresence(env.authContext(), model.UpdateMyPresenceInput{Status: model.PresenceStatusAway})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !result {
			t.Error("expected true result")
		}
	})

	t.Run("cannot set offline status", func(t *testing.T) {
		_, err := mutation.UpdateMyPresence(env.authContext(), model.UpdateMyPresenceInput{Status: model.PresenceStatusOffline})
		if err == nil {
			t.Error("expected error when setting OFFLINE status")
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		_, err := mutation.UpdateMyPresence(env.unauthContext(), model.UpdateMyPresenceInput{Status: model.PresenceStatusOnline})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})
}
