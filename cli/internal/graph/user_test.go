package graph

import (
	"errors"
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
		url, err := resolver.AvatarURL(env.ctx, env.testUser, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if url != nil {
			t.Errorf("expected nil URL for user without avatar, got %s", *url)
		}
	})

	t.Run("works without auth context (public field)", func(t *testing.T) {
		url, err := resolver.AvatarURL(env.unauthContext(), env.testUser, nil, nil, nil)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if url != nil {
			t.Errorf("expected nil URL, got %s", *url)
		}
	})

	t.Run("accepts width and height parameters", func(t *testing.T) {
		w, h := int32(100), int32(100)
		url, err := resolver.AvatarURL(env.ctx, env.testUser, &w, &h, nil)
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
// resolvers (Server.roleUsers, Server.userEffectivePermissions,
// Server.userEffectiveDenials, Server.roles, Server.role) require the
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

	t.Run("userEffectivePermissions denied to regular user", func(t *testing.T) {
		_, err := env.resolver.Server().UserEffectivePermissions(regularCtx, server, env.testUser.Id)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("userEffectiveDenials denied to regular user", func(t *testing.T) {
		_, err := env.resolver.Server().UserEffectiveDenials(regularCtx, server, env.testUser.Id)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("Server.roles is available to regular user (role catalog is public)", func(t *testing.T) {
		// Server.roles returns role definitions (name, displayName, permissions
		// granted to each role). That's the operational role catalog — not
		// per-user roster info — and every authenticated user can see it.
		// The sensitive thing (who has the admin role) is roleUsers, gated above.
		if _, err := env.resolver.Server().Roles(regularCtx, server); err != nil {
			t.Errorf("Server.roles should be readable by any authenticated user, got %v", err)
		}
	})

	t.Run("admin can read the gated roster", func(t *testing.T) {
		admin := env.createVerifiedUser(t, "roster-admin", "Roster Admin", "password123")
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.Id, core.RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		adminCtx := env.authContextForUser(admin)
		if _, err := env.resolver.Server().RoleUsers(adminCtx, server, core.RoleAdmin); err != nil {
			t.Errorf("admin RoleUsers: %v", err)
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
// Email Exposure Authorization
// ============================================================================

// TestUserResolver_VerifiedEmails locks down the access-control contract for
// User.verifiedEmails: self always sees their own, holders of
// `admin.view-users` see anyone's, and everyone else gets an empty list
// (not an error — we don't want the field's existence to leak whether the
// caller is authorized).
func TestUserResolver_VerifiedEmails(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	resolver := env.resolver.User()

	t.Run("self sees own verified emails", func(t *testing.T) {
		emails, err := resolver.VerifiedEmails(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("VerifiedEmails: %v", err)
		}
		if len(emails) == 0 || emails[0] != "testuser@example.com" {
			t.Errorf("expected to see own email, got %v", emails)
		}
	})

	t.Run("admin (admin.view-users) sees other user's emails", func(t *testing.T) {
		other := env.createVerifiedUser(t, "ve-other", "Other", "password123")
		emails, err := resolver.VerifiedEmails(env.authContext(), other)
		if err != nil {
			t.Fatalf("VerifiedEmails: %v", err)
		}
		if len(emails) == 0 || emails[0] != "ve-other@example.com" {
			t.Errorf("admin should see other user's emails, got %v", emails)
		}
	})

	t.Run("regular user gets empty list for other user", func(t *testing.T) {
		// Plain user — no admin.view-users permission.
		regular := env.createVerifiedUser(t, "ve-regular", "Regular", "password123")
		other := env.createVerifiedUser(t, "ve-target", "Target", "password123")

		emails, err := resolver.VerifiedEmails(env.authContextForUser(regular), other)
		if err != nil {
			t.Fatalf("VerifiedEmails: %v", err)
		}
		if len(emails) != 0 {
			t.Errorf("regular user should get empty list, got %v", emails)
		}
	})

	t.Run("unauthenticated gets empty list", func(t *testing.T) {
		emails, err := resolver.VerifiedEmails(env.unauthContext(), env.testUser)
		if err != nil {
			t.Fatalf("VerifiedEmails: %v", err)
		}
		if len(emails) != 0 {
			t.Errorf("unauthenticated should get empty list, got %v", emails)
		}
	})

	t.Run("hasVerifiedEmail is gated the same way", func(t *testing.T) {
		// A regular user querying another user's hasVerifiedEmail must NOT
		// learn whether the target has a verified address — that's a weak
		// confirmation oracle for email-fishing.
		regular := env.createVerifiedUser(t, "hve-regular", "Regular", "password123")
		other := env.createVerifiedUser(t, "hve-target", "Target", "password123")

		has, err := resolver.HasVerifiedEmail(env.authContextForUser(regular), other)
		if err != nil {
			t.Fatalf("HasVerifiedEmail: %v", err)
		}
		if has {
			t.Errorf("regular user should NOT see other user's hasVerifiedEmail")
		}

		// Self always works.
		has, err = resolver.HasVerifiedEmail(env.authContextForUser(regular), regular)
		if err != nil {
			t.Fatalf("HasVerifiedEmail (self): %v", err)
		}
		if !has {
			t.Errorf("self should see own hasVerifiedEmail")
		}
	})
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
		result, err := mutation.UpdateMyPresence(env.authContext(), model.UpdateMyPresenceInput{Status: model.PresenceStatusInputOnline})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !result {
			t.Error("expected true result")
		}
	})

	t.Run("authenticated user can set away", func(t *testing.T) {
		result, err := mutation.UpdateMyPresence(env.authContext(), model.UpdateMyPresenceInput{Status: model.PresenceStatusInputAway})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !result {
			t.Error("expected true result")
		}
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		_, err := mutation.UpdateMyPresence(env.unauthContext(), model.UpdateMyPresenceInput{Status: model.PresenceStatusInputOnline})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})
}
