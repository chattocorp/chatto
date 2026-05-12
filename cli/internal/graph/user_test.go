package graph

import (
	"errors"
	"os"
	"strings"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// User Field Resolver Tests
// ============================================================================

// User.Spaces was retired in PR(a); user-facing membership is now reflected by
// User.rooms (and the implicit instance membership). No separate test.

func TestUserResolver_Rooms(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("get own rooms", func(t *testing.T) {
		rooms, err := env.resolver.User().Rooms(env.authContext(), env.testUser, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(rooms) == 0 {
			t.Fatal("Expected at least one room")
		}

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
		otherUser, err := env.core.CreateUser(env.ctx, "system", "otheruser-room", "Other User", "password123")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		_, err = env.resolver.User().Rooms(env.authContext(), otherUser, nil)
		if !errors.Is(err, ErrNotSelf) {
			t.Errorf("Expected ErrNotSelf, got %v", err)
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		_, err := env.resolver.User().Rooms(env.unauthContext(), env.testUser, nil)
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
// Role-roster Authorization
// ============================================================================

// TestServer_RoleRosterRequiresPermission asserts that the role-roster
// resolvers (Server.roleUsers, Server.userRoleBasedPermissions,
// Server.userRoleBasedDenials, Server.roles, Server.role) require the
// `role.assign` permission. Without this gate, any authenticated user
// could enumerate "who's an admin" and "which permissions does this user
// hold" — operationally sensitive information.
func TestServer_RoleRosterRequiresPermission(t *testing.T) {
	env := setupTestResolver(t)
	server := &model.Server{}

	regular := env.createVerifiedUser(t, "regular-roster", "Regular", "password123")
	regularCtx := env.authContextForUser(regular)

	t.Run("roleUsers denied to regular user", func(t *testing.T) {
		_, err := env.resolver.Server().RoleUsers(regularCtx, server, core.RoleAdmin)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("userRoleBasedPermissions denied to regular user", func(t *testing.T) {
		_, err := env.resolver.Server().UserRoleBasedPermissions(regularCtx, server, env.testUser.Id)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("userRoleBasedDenials denied to regular user", func(t *testing.T) {
		_, err := env.resolver.Server().UserRoleBasedDenials(regularCtx, server, env.testUser.Id)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("roles denied to regular user", func(t *testing.T) {
		_, err := env.resolver.Server().Roles(regularCtx, server)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("role denied to regular user", func(t *testing.T) {
		_, err := env.resolver.Server().Role(regularCtx, server, core.RoleAdmin)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("admin can read the roster", func(t *testing.T) {
		admin := env.createVerifiedUser(t, "roster-admin", "Roster Admin", "password123")
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		adminCtx := env.authContextForUser(admin)
		if _, err := env.resolver.Server().RoleUsers(adminCtx, server, core.RoleAdmin); err != nil {
			t.Errorf("admin RoleUsers: %v", err)
		}
		if _, err := env.resolver.Server().Roles(adminCtx, server); err != nil {
			t.Errorf("admin Roles: %v", err)
		}
	})

	t.Run("unauthenticated denied", func(t *testing.T) {
		_, err := env.resolver.Server().RoleUsers(env.unauthContext(), server, core.RoleAdmin)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})
}

// ============================================================================
// Email Exposure Contract
// ============================================================================

// TestUser_NoEmailFieldsInSchema asserts that the User GraphQL type does
// not expose any email addresses. This is a hard contract: emails MUST
// NEVER be served via the GraphQL API, not even to admins or the user
// themselves. The only email-related field on User is `hasVerifiedEmail`,
// which is a boolean.
//
// If this test starts failing it means someone added a field exposing
// email content to the schema. Revert that change unless the rule in
// .claude/rules/authorization.md has explicitly changed.
func TestUser_NoEmailFieldsInSchema(t *testing.T) {
	schema := readUserSchemaFile(t)

	// Allowed: anything containing "email" that is clearly a boolean
	// indicator. Forbidden: any field that returns the address itself.
	forbidden := []string{
		"verifiedEmails",
		"emails:",
		"email:",
		"primaryEmail",
		"emailAddress",
	}
	for _, needle := range forbidden {
		if containsCaseSensitive(schema, needle) {
			t.Errorf("User schema unexpectedly mentions %q — emails must not be exposed via GraphQL", needle)
		}
	}
}

func readUserSchemaFile(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("user.graphqls")
	if err != nil {
		t.Fatalf("read user.graphqls: %v", err)
	}
	return string(b)
}

func containsCaseSensitive(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// ============================================================================
// User.Roles Field Resolver Tests
// ============================================================================

func TestUserResolver_Roles(t *testing.T) {
	env := setupTestResolver(t)
	resolver := env.resolver.User()

	t.Run("authenticated user gets roles", func(t *testing.T) {
		roles, err := resolver.Roles(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if roles == nil {
			t.Fatal("expected roles slice, got nil")
		}
	})

	t.Run("unauthenticated gets empty list", func(t *testing.T) {
		roles, err := resolver.Roles(env.unauthContext(), env.testUser)
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

		roles, err := resolver.Roles(env.authContext(), otherUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if roles == nil {
			t.Fatal("expected roles slice, got nil")
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
