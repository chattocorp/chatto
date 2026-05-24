package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// Admin Mutations Authorization Tests
// ============================================================================

func TestAdminMutations_Authorization(t *testing.T) {
	// Set up environment with admin config - testuser@example.com is the admin
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	mutation := env.resolver.Mutation()

	t.Run("unauthenticated user gets nil", func(t *testing.T) {
		result, err := mutation.Admin(env.unauthContext())
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for unauthenticated user")
		}
	})

	t.Run("non-admin user gets nil", func(t *testing.T) {
		// Create a regular user (not in admin emails list)
		regularUser := env.createVerifiedUser(t, "regular", "Regular User", "password123")

		result, err := mutation.Admin(env.authContextForUser(regularUser))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for non-admin user")
		}
	})

	t.Run("admin user (by verified email) gets AdminMutations", func(t *testing.T) {
		// testUser has verified email testuser@example.com which is in admin list
		result, err := mutation.Admin(env.authContext())
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Error("expected AdminMutations object, got nil")
		}
	})

	t.Run("user without verified admin email gets nil", func(t *testing.T) {
		// Set up environment where admin@example.com is in the admin list
		envWithAdminEmail := setupTestResolverWithAdmin(t, []string{"admin@example.com"})

		// Create a user without any verified email (or with a different one)
		unverifiedUser, err := envWithAdminEmail.core.CreateUser(envWithAdminEmail.ctx, "system", "no-verified-email", "No Verified", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		// This user has no verified emails at all, so shouldn't get admin access
		// even though admin@example.com is in the admin list

		mutation2 := envWithAdminEmail.resolver.Mutation()
		result, err := mutation2.Admin(envWithAdminEmail.authContextForUser(unverifiedUser))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for user without verified admin email")
		}
	})

	t.Run("user with different verified email gets nil", func(t *testing.T) {
		// User has a verified email but it's not in the admin list
		userWithDifferentEmail := env.createVerifiedUser(t, "diff-email", "Different Email", "password123")
		// This creates a user with verified email diff-email@example.com
		// But the admin list only has testuser@example.com

		result, err := mutation.Admin(env.authContextForUser(userWithDifferentEmail))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for user with different verified email")
		}
	})
}

// ============================================================================
// UpdateServerConfig Defense-in-Depth Tests
// ============================================================================

func TestUpdateServerConfig_Authorization(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})

	t.Run("admin can update server config", func(t *testing.T) {
		// First get the admin mutations object
		adminMutations, err := env.resolver.Mutation().Admin(env.authContext())
		if err != nil {
			t.Fatalf("failed to get admin mutations: %v", err)
		}
		if adminMutations == nil {
			t.Fatal("expected admin mutations, got nil")
		}

		// Now call UpdateServerConfig
		welcomeMsg := "Welcome to Chatto!"
		adminMutResolver := env.resolver.AdminMutations()
		result, err := adminMutResolver.UpdateServerConfig(env.authContext(), adminMutations, model.UpdateServerConfigInput{
			WelcomeMessage: &welcomeMsg,
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.WelcomeMessage == nil || *result.WelcomeMessage != welcomeMsg {
			t.Errorf("expected welcome message %q, got %v", welcomeMsg, result.WelcomeMessage)
		}
	})

	t.Run("non-admin calling UpdateServerConfig directly gets permission denied", func(t *testing.T) {
		// Create a non-admin user
		regularUser := env.createVerifiedUser(t, "regular-config", "Regular User", "password123")

		// Try to call UpdateServerConfig directly (bypassing parent resolver)
		adminMutResolver := env.resolver.AdminMutations()
		welcomeMsg := "Hacked!"
		_, err := adminMutResolver.UpdateServerConfig(
			env.authContextForUser(regularUser),
			&model.AdminMutations{},
			model.UpdateServerConfigInput{
				WelcomeMessage: &welcomeMsg,
			},
		)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("unauthenticated user calling UpdateServerConfig gets not authenticated", func(t *testing.T) {
		adminMutResolver := env.resolver.AdminMutations()
		welcomeMsg := "Hacked!"
		_, err := adminMutResolver.UpdateServerConfig(
			env.unauthContext(),
			&model.AdminMutations{},
			model.UpdateServerConfigInput{
				WelcomeMessage: &welcomeMsg,
			},
		)
		if !errors.Is(err, core.ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})
}

// ============================================================================
// AdminMutations.UpdateUser / ClearUsernameCooldown Tests
// ============================================================================

// TestAdminUpdateUser_Authorization verifies authorization, role-hierarchy
// enforcement, and config-admin bypass for the admin user-management
// mutations. The hierarchy check is the privilege-escalation guard — a
// regression here lets a moderator-rank admin rename an instance-owner.
func TestAdminUpdateUser_Authorization(t *testing.T) {
	t.Run("unauthenticated caller gets not authenticated", func(t *testing.T) {
		env := setupTestResolver(t)
		amr := env.resolver.AdminMutations()
		newName := "newname"
		_, err := amr.UpdateUser(env.unauthContext(), &model.AdminMutations{}, model.AdminUpdateUserInput{
			UserID: env.testUser.Id,
			Login:  &newName,
		})
		if !errors.Is(err, core.ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got: %v", err)
		}

		_, err = amr.ClearUsernameCooldown(env.unauthContext(), &model.AdminMutations{}, model.ClearUsernameCooldownInput{UserID: env.testUser.Id})
		if !errors.Is(err, core.ErrNotAuthenticated) {
			t.Errorf("ClearUsernameCooldown: expected ErrNotAuthenticated, got: %v", err)
		}
	})

	t.Run("non-admin caller gets permission denied", func(t *testing.T) {
		env := setupTestResolver(t)
		regular := env.createVerifiedUser(t, "regular-noperms", "Regular", "password123")

		amr := env.resolver.AdminMutations()
		newName := "newname"
		_, err := amr.UpdateUser(env.authContextForUser(regular), &model.AdminMutations{}, model.AdminUpdateUserInput{
			UserID: env.testUser.Id,
			Login:  &newName,
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got: %v", err)
		}

		_, err = amr.ClearUsernameCooldown(env.authContextForUser(regular), &model.AdminMutations{}, model.ClearUsernameCooldownInput{UserID: env.testUser.Id})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("ClearUsernameCooldown: expected ErrPermissionDenied, got: %v", err)
		}
	})

	t.Run("rbac admin cannot edit owner (hierarchy enforcement)", func(t *testing.T) {
		// testUser was created first → auto-promoted to server owner.
		// admin2 has the instance-admin role (rank 1) — outranked by owner (rank 0).
		env := setupTestResolver(t)
		admin2 := env.createVerifiedUser(t, "rbac-admin", "RBAC Admin", "password123")
		if err := env.core.AssignAdminRole(env.ctx, admin2.Id); err != nil {
			t.Fatalf("failed to assign admin role: %v", err)
		}

		amr := env.resolver.AdminMutations()
		newName := "ownerhacked"
		_, err := amr.UpdateUser(env.authContextForUser(admin2), &model.AdminMutations{}, model.AdminUpdateUserInput{
			UserID: env.testUser.Id, // the owner
			Login:  &newName,
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied (admin cannot edit owner), got: %v", err)
		}

		_, err = amr.ClearUsernameCooldown(env.authContextForUser(admin2), &model.AdminMutations{}, model.ClearUsernameCooldownInput{UserID: env.testUser.Id})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("ClearUsernameCooldown: expected ErrPermissionDenied (admin cannot manage owner), got: %v", err)
		}
	})

	t.Run("rbac admin can edit lower-ranked user", func(t *testing.T) {
		env := setupTestResolver(t)
		admin2 := env.createVerifiedUser(t, "rbac-admin-ok", "RBAC Admin", "password123")
		if err := env.core.AssignAdminRole(env.ctx, admin2.Id); err != nil {
			t.Fatalf("failed to assign admin role: %v", err)
		}
		target := env.createVerifiedUser(t, "regular-target", "Regular", "password123")

		amr := env.resolver.AdminMutations()
		newName := "renamed"
		updated, err := amr.UpdateUser(env.authContextForUser(admin2), &model.AdminMutations{}, model.AdminUpdateUserInput{
			UserID: target.Id,
			Login:  &newName,
		})
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if updated == nil || updated.Login != newName {
			t.Errorf("expected login %q, got %v", newName, updated)
		}
	})

	t.Run("peer owner cannot edit other owner (strict rank)", func(t *testing.T) {
		// requireUserAdminTarget requires the caller to *strictly* outrank
		// the target. Two owners are peers (both position 0), so neither
		// can administer the other's identity. To do so, one must be
		// demoted first. This matches RevokeServerRole's symmetric
		// peer-deny.
		env := setupTestResolverWithAdmin(t, []string{"cfg-admin@example.com"})
		cfgAdmin, err := env.core.CreateUser(env.ctx, "system", "cfg-admin", "Config Admin", "password123")
		if err != nil {
			t.Fatalf("failed to create config admin user: %v", err)
		}
		if err := env.core.AddVerifiedEmailDirect(env.ctx, cfgAdmin.Id, "cfg-admin@example.com"); err != nil {
			t.Fatalf("failed to verify config admin email: %v", err)
		}

		amr := env.resolver.AdminMutations()
		newName := "ownerrenamed"
		_, err = amr.UpdateUser(env.authContextForUser(cfgAdmin), &model.AdminMutations{}, model.AdminUpdateUserInput{
			UserID: env.testUser.Id, // the owner
			Login:  &newName,
		})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Fatalf("expected peer-owner edit to be denied, got: %v", err)
		}

		_, err = amr.ClearUsernameCooldown(env.authContextForUser(cfgAdmin), &model.AdminMutations{}, model.ClearUsernameCooldownInput{UserID: env.testUser.Id})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Fatalf("expected peer-owner cooldown clear to be denied, got: %v", err)
		}
	})

	t.Run("empty input is rejected", func(t *testing.T) {
		env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
		amr := env.resolver.AdminMutations()
		_, err := amr.UpdateUser(env.authContext(), &model.AdminMutations{}, model.AdminUpdateUserInput{
			UserID: env.testUser.Id,
		})
		if err == nil {
			t.Error("expected error for empty input, got nil")
		}
	})
}

// ============================================================================
// Admin Query Authorization Tests
// ============================================================================

func TestAdminQuery_Authorization(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	query := env.resolver.Query()

	t.Run("unauthenticated user gets nil", func(t *testing.T) {
		result, err := query.Admin(env.unauthContext())
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for unauthenticated user")
		}
	})

	t.Run("non-admin user gets nil", func(t *testing.T) {
		regularUser := env.createVerifiedUser(t, "regular-query", "Regular User", "password123")

		result, err := query.Admin(env.authContextForUser(regularUser))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result != nil {
			t.Error("expected nil result for non-admin user")
		}
	})

	t.Run("admin user gets AdminQueries", func(t *testing.T) {
		result, err := query.Admin(env.authContext())
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result == nil {
			t.Error("expected AdminQueries object, got nil")
		}
		if result.SystemInfo == nil {
			t.Error("expected SystemInfo, got nil")
		}
	})
}
