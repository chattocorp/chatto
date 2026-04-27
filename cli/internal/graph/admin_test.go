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
// UpdateInstanceConfig Defense-in-Depth Tests
// ============================================================================

func TestUpdateInstanceConfig_Authorization(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})

	t.Run("admin can update instance config", func(t *testing.T) {
		// First get the admin mutations object
		adminMutations, err := env.resolver.Mutation().Admin(env.authContext())
		if err != nil {
			t.Fatalf("failed to get admin mutations: %v", err)
		}
		if adminMutations == nil {
			t.Fatal("expected admin mutations, got nil")
		}

		// Now call UpdateInstanceConfig
		welcomeMsg := "Welcome to Chatto!"
		adminMutResolver := env.resolver.AdminMutations()
		result, err := adminMutResolver.UpdateInstanceConfig(env.authContext(), adminMutations, model.UpdateInstanceConfigInput{
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

	t.Run("non-admin calling UpdateInstanceConfig directly gets permission denied", func(t *testing.T) {
		// Create a non-admin user
		regularUser := env.createVerifiedUser(t, "regular-config", "Regular User", "password123")

		// Try to call UpdateInstanceConfig directly (bypassing parent resolver)
		adminMutResolver := env.resolver.AdminMutations()
		welcomeMsg := "Hacked!"
		_, err := adminMutResolver.UpdateInstanceConfig(
			env.authContextForUser(regularUser),
			&model.AdminMutations{},
			model.UpdateInstanceConfigInput{
				WelcomeMessage: &welcomeMsg,
			},
		)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("unauthenticated user calling UpdateInstanceConfig gets not authenticated", func(t *testing.T) {
		adminMutResolver := env.resolver.AdminMutations()
		welcomeMsg := "Hacked!"
		_, err := adminMutResolver.UpdateInstanceConfig(
			env.unauthContext(),
			&model.AdminMutations{},
			model.UpdateInstanceConfigInput{
				WelcomeMessage: &welcomeMsg,
			},
		)
		if !errors.Is(err, core.ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})
}

// ============================================================================
// ResetInstanceConfig Defense-in-Depth Tests
// ============================================================================

func TestResetInstanceConfig_Authorization(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})

	t.Run("admin can reset instance config", func(t *testing.T) {
		// First set some config
		adminMutations, err := env.resolver.Mutation().Admin(env.authContext())
		if err != nil {
			t.Fatalf("failed to get admin mutations: %v", err)
		}
		adminMutResolver := env.resolver.AdminMutations()
		welcomeMsg := "Custom welcome"
		_, err = adminMutResolver.UpdateInstanceConfig(env.authContext(), adminMutations, model.UpdateInstanceConfigInput{
			WelcomeMessage: &welcomeMsg,
		})
		if err != nil {
			t.Fatalf("failed to set config: %v", err)
		}

		// Now reset it
		success, err := adminMutResolver.ResetInstanceConfig(env.authContext(), adminMutations)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if !success {
			t.Error("expected success=true")
		}
	})

	t.Run("non-admin calling ResetInstanceConfig directly gets permission denied", func(t *testing.T) {
		// Create a non-admin user
		regularUser := env.createVerifiedUser(t, "regular-reset", "Regular User", "password123")

		// Try to call ResetInstanceConfig directly (bypassing parent resolver)
		adminMutResolver := env.resolver.AdminMutations()
		_, err := adminMutResolver.ResetInstanceConfig(
			env.authContextForUser(regularUser),
			&model.AdminMutations{},
		)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("unauthenticated user calling ResetInstanceConfig gets not authenticated", func(t *testing.T) {
		adminMutResolver := env.resolver.AdminMutations()
		_, err := adminMutResolver.ResetInstanceConfig(
			env.unauthContext(),
			&model.AdminMutations{},
		)
		if !errors.Is(err, core.ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
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
