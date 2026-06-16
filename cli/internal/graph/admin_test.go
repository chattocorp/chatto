package graph

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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

	t.Run("authenticated non-admin user gets AdminMutations namespace", func(t *testing.T) {
		regularUser := env.createVerifiedUser(t, "regular", "Regular User", "password123")

		result, err := mutation.Admin(env.authContextForUser(regularUser))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result == nil {
			t.Error("expected AdminMutations object for authenticated user")
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

	t.Run("user without verified owner email gets AdminMutations namespace", func(t *testing.T) {
		envWithAdminEmail := setupTestResolverWithAdmin(t, []string{"admin@example.com"})

		unverifiedUser, err := envWithAdminEmail.core.CreateUser(envWithAdminEmail.ctx, "system", "no-verified-email", "No Verified", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		mutation2 := envWithAdminEmail.resolver.Mutation()
		result, err := mutation2.Admin(envWithAdminEmail.authContextForUser(unverifiedUser))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result == nil {
			t.Error("expected AdminMutations object for authenticated user")
		}
	})

	t.Run("user with different verified email gets AdminMutations namespace", func(t *testing.T) {
		userWithDifferentEmail := env.createVerifiedUser(t, "diff-email", "Different Email", "password123")

		result, err := mutation.Admin(env.authContextForUser(userWithDifferentEmail))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result == nil {
			t.Error("expected AdminMutations object for authenticated user")
		}
	})
}

// ============================================================================
// UpdateServerConfig Admin-Focused Tests
// ============================================================================

func TestUpdateServerConfig_AdminUserCanUpdateWelcomeMessage(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})

	t.Run("admin can update server config", func(t *testing.T) {
		welcomeMsg := "Welcome to Chatto!"
		result, err := env.resolver.Mutation().UpdateServerConfig(env.authContext(), model.UpdateServerConfigInput{
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

		welcomeMsg := "Hacked!"
		_, err := env.resolver.Mutation().UpdateServerConfig(
			env.authContextForUser(regularUser),
			model.UpdateServerConfigInput{
				WelcomeMessage: &welcomeMsg,
			},
		)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("unauthenticated user calling UpdateServerConfig gets not authenticated", func(t *testing.T) {
		welcomeMsg := "Hacked!"
		_, err := env.resolver.Mutation().UpdateServerConfig(
			env.unauthContext(),
			model.UpdateServerConfigInput{
				WelcomeMessage: &welcomeMsg,
			},
		)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestUpdateServerConfig_PublishesSingleServerUpdatedLiveEvent(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	events, cleanup := subscribeServerUpdatedLiveEvents(t, env.nc)
	defer cleanup()

	serverName := "Live Config Server"
	motd := "Live config MOTD"
	_, err := env.resolver.Mutation().UpdateServerConfig(env.authContext(), model.UpdateServerConfigInput{
		ServerName: &serverName,
		Motd:       &motd,
	})
	if err != nil {
		t.Fatalf("UpdateServerConfig: %v", err)
	}

	event := expectServerUpdatedLiveEvent(t, events)
	if got := event.GetServerUpdated().GetName(); got != serverName {
		t.Fatalf("ServerUpdatedEvent.name = %q, want %q", got, serverName)
	}
	expectNoServerUpdatedLiveEvent(t, events)
}

func TestUpdateBlockedUsernames_Authorization(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	adminMutations := env.resolver.AdminMutations()

	t.Run("admin can update blocked usernames", func(t *testing.T) {
		blocked := "root\nadmin\nsupport"
		result, err := adminMutations.UpdateBlockedUsernames(env.authContext(), &model.AdminMutations{}, model.UpdateBlockedUsernamesInput{
			BlockedUsernames: blocked,
		})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if result != blocked {
			t.Errorf("expected blocked usernames %q, got %q", blocked, result)
		}
	})

	t.Run("non-admin gets permission denied", func(t *testing.T) {
		regularUser := env.createVerifiedUser(t, "regular-blocked-usernames", "Regular User", "password123")

		_, err := adminMutations.UpdateBlockedUsernames(
			env.authContextForUser(regularUser),
			&model.AdminMutations{},
			model.UpdateBlockedUsernamesInput{BlockedUsernames: "root"},
		)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	t.Run("unauthenticated user gets not authenticated", func(t *testing.T) {
		_, err := adminMutations.UpdateBlockedUsernames(
			env.unauthContext(),
			&model.AdminMutations{},
			model.UpdateBlockedUsernamesInput{BlockedUsernames: "root"},
		)
		if !errors.Is(err, core.ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestUpdateBlockedUsernames_DoesNotPublishMemberVisibleLiveEvent(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	adminMutations := env.resolver.AdminMutations()
	events, cleanup := subscribeServerUpdatedLiveEvents(t, env.nc)
	defer cleanup()

	blocked := "secret-admin-only\nreserved"
	result, err := adminMutations.UpdateBlockedUsernames(env.authContext(), &model.AdminMutations{}, model.UpdateBlockedUsernamesInput{
		BlockedUsernames: blocked,
	})
	if err != nil {
		t.Fatalf("UpdateBlockedUsernames: %v", err)
	}
	if result != blocked {
		t.Fatalf("blocked usernames result = %q, want %q", result, blocked)
	}
	expectNoServerUpdatedLiveEvent(t, events)
}

// ============================================================================
// AdminMutations.UpdateUser / ClearUsernameCooldown Tests
// ============================================================================

func subscribeServerUpdatedLiveEvents(t *testing.T, nc *nats.Conn) (<-chan *corev1.LiveEvent, func()) {
	t.Helper()

	events := make(chan *corev1.LiveEvent, 4)
	sub, err := nc.Subscribe(subjects.LiveSyncConfigEvent("server_updated"), func(msg *nats.Msg) {
		var event corev1.LiveEvent
		if err := proto.Unmarshal(msg.Data, &event); err != nil {
			t.Errorf("failed to unmarshal live event: %v", err)
			return
		}
		events <- &event
	})
	if err != nil {
		t.Fatalf("Subscribe(server_updated): %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	return events, func() {
		if err := sub.Unsubscribe(); err != nil {
			t.Errorf("Unsubscribe(server_updated): %v", err)
		}
	}
}

func expectServerUpdatedLiveEvent(t *testing.T, events <-chan *corev1.LiveEvent) *corev1.LiveEvent {
	t.Helper()

	select {
	case event := <-events:
		if event.GetServerUpdated() == nil {
			t.Fatalf("expected ServerUpdatedEvent, got %T", event.GetEvent())
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ServerUpdatedEvent")
		return nil
	}
}

func expectNoServerUpdatedLiveEvent(t *testing.T, events <-chan *corev1.LiveEvent) {
	t.Helper()

	select {
	case event := <-events:
		t.Fatalf("unexpected ServerUpdatedEvent: %+v", event.GetServerUpdated())
	case <-time.After(200 * time.Millisecond):
	}
}

// TestAdminUpdateUser_Authorization verifies authorization and owner-protection
// behavior for the admin user-management mutations.
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

	t.Run("rbac admin can edit owner", func(t *testing.T) {
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
		if err != nil {
			t.Errorf("expected success, got: %v", err)
		}

		_, err = amr.ClearUsernameCooldown(env.authContextForUser(admin2), &model.AdminMutations{}, model.ClearUsernameCooldownInput{UserID: env.testUser.Id})
		if err != nil {
			t.Errorf("ClearUsernameCooldown: expected success, got: %v", err)
		}
	})

	t.Run("rbac admin can edit another user", func(t *testing.T) {
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

	t.Run("peer owner can edit other owner", func(t *testing.T) {
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
		if err != nil {
			t.Fatalf("expected peer-owner edit to succeed, got: %v", err)
		}

		_, err = amr.ClearUsernameCooldown(env.authContextForUser(cfgAdmin), &model.AdminMutations{}, model.ClearUsernameCooldownInput{UserID: env.testUser.Id})
		if err != nil {
			t.Fatalf("expected peer-owner cooldown clear to succeed, got: %v", err)
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

	t.Run("authenticated non-admin user gets AdminQueries namespace", func(t *testing.T) {
		regularUser := env.createVerifiedUser(t, "regular-query", "Regular User", "password123")

		result, err := query.Admin(env.authContextForUser(regularUser))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if result == nil {
			t.Fatal("expected AdminQueries object for authenticated user")
		}
		_, err = env.resolver.AdminQueries().SystemInfo(env.authContextForUser(regularUser), result)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Fatalf("expected SystemInfo permission denial, got: %v", err)
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
		systemInfo, err := env.resolver.AdminQueries().SystemInfo(env.authContext(), result)
		if err != nil {
			t.Fatalf("expected SystemInfo resolver success, got error: %v", err)
		}
		if systemInfo == nil {
			t.Error("expected SystemInfo, got nil")
		}
	})
}
