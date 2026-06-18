package graph

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

// testEnv holds all test dependencies for GraphQL resolver tests
type testEnv struct {
	ctx      context.Context
	core     *core.ChattoCore
	nc       *nats.Conn
	resolver *Resolver
	// Common test data
	testUser *corev1.User
	testRoom *corev1.Room
}

// setupTestResolver creates a complete test environment with resolver and test data
func setupTestResolver(t *testing.T) *testEnv {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)

	// Use a context with timeout for setup
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer setupCancel()

	// Create ChattoCore
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	chattoCore, err := core.NewChattoCore(setupCtx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	// Run core's background services (PresenceHub + projectors) for the
	// lifetime of the test. StreamMyEvents needs PresenceHub; membership
	// mutations need the projector loops to advance so WaitForSeq returns.
	servicesCtx, servicesCancel := context.WithCancel(context.Background())
	servicesDone := make(chan error, 1)
	go func() { servicesDone <- chattoCore.Run(servicesCtx) }()

	t.Cleanup(func() {
		servicesCancel()
		select {
		case <-servicesDone:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})

	// Wait for Run's boot phase before letting the test issue reads
	// against the projections.
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := chattoCore.WaitForBoot(bootCtx); err != nil {
		bootCancel()
		t.Fatalf("WaitForBoot: %v", err)
	}
	bootCancel()

	// Create resolver with empty owners/auth/push config for tests
	resolver := NewResolver(chattoCore, config.OwnersConfig{}, config.AuthConfig{}, config.PushConfig{}, config.VideoConfig{}, config.LiveKitConfig{}, "test")

	env := &testEnv{
		ctx:      context.Background(),
		core:     chattoCore,
		nc:       nc,
		resolver: resolver,
	}

	// Create common test data
	env.createTestData(t)

	return env
}

// createTestData creates common test fixtures (user, space, room)
func (e *testEnv) createTestData(t *testing.T) {
	t.Helper()

	// Create test user with verified email and assign the owner role.
	// This mirrors the pre-existing test convention (when CreateUser auto-promoted
	// the first user) so existing tests that assume `e.testUser` is owner keep
	// working without per-test role-assignment boilerplate.
	user, err := e.core.CreateUser(e.ctx, "system", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	if err := e.core.AddVerifiedEmailDirect(e.ctx, user.Id, "testuser@example.com"); err != nil {
		t.Fatalf("Failed to verify test user: %v", err)
	}
	if err := e.core.AssignOwnerRole(e.ctx, user.Id); err != nil {
		t.Fatalf("Failed to assign owner role to test user: %v", err)
	}
	e.testUser = user

	// RBAC defaults (system roles, owner role for verified-email owners) are
	// seeded at boot via initServerRBAC; no per-test setup needed.
	// Server membership is implicit post-#330; no explicit join step either.

	// Create test room
	room, err := e.core.CreateRoom(e.ctx, user.Id, core.KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create test room: %v", err)
	}
	e.testRoom = room

	// Join the room (required for posting messages)
	_, err = e.core.JoinRoom(e.ctx, user.Id, core.KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join test room: %v", err)
	}
}

// authContext returns a new context with the test user authenticated
func (e *testEnv) authContext() context.Context {
	return auth.WithUser(e.ctx, e.testUser)
}

// authContextForUser returns a new context with a specific user authenticated
func (e *testEnv) authContextForUser(user *corev1.User) context.Context {
	return auth.WithUser(e.ctx, user)
}

// unauthContext returns a context without any authenticated user
func (e *testEnv) unauthContext() context.Context {
	return e.ctx
}

// createVerifiedUser creates a new user with a verified email address
func (e *testEnv) createVerifiedUser(t *testing.T, login, displayName, password string) *corev1.User {
	t.Helper()
	user, err := e.core.CreateUser(e.ctx, "system", login, displayName, password)
	if err != nil {
		t.Fatalf("Failed to create user %s: %v", login, err)
	}
	if err := e.core.AddVerifiedEmailDirect(e.ctx, user.Id, login+"@example.com"); err != nil {
		t.Fatalf("Failed to verify user %s: %v", login, err)
	}
	return user
}

// setupTestResolverWithAdmin creates a test environment with owners config so
// users with matching verified emails are treated as server owners.
func setupTestResolverWithAdmin(t *testing.T, ownerEmails []string) *testEnv {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)

	// Use a context with timeout for setup
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer setupCancel()

	// Create owners config first so it can be threaded into the core for the
	// auto-promotion-on-email-verification path.
	ownersConfig := config.OwnersConfig{Emails: ownerEmails}

	// Create ChattoCore
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
		Owners: ownersConfig,
	}
	chattoCore, err := core.NewChattoCore(setupCtx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	// Run core's background services (PresenceHub + projectors) for the
	// lifetime of the test. StreamMyEvents needs PresenceHub; membership
	// mutations need the projector loops to advance so WaitForSeq returns.
	servicesCtx, servicesCancel := context.WithCancel(context.Background())
	servicesDone := make(chan error, 1)
	go func() { servicesDone <- chattoCore.Run(servicesCtx) }()

	t.Cleanup(func() {
		servicesCancel()
		select {
		case <-servicesDone:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})

	// Wait for Run's boot phase before letting the test issue reads
	// against the projections.
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := chattoCore.WaitForBoot(bootCtx); err != nil {
		bootCancel()
		t.Fatalf("WaitForBoot: %v", err)
	}
	bootCancel()

	// Create resolver with provided owners config
	resolver := NewResolver(chattoCore, ownersConfig, config.AuthConfig{}, config.PushConfig{}, config.VideoConfig{}, config.LiveKitConfig{}, "test")

	env := &testEnv{
		ctx:      context.Background(),
		core:     chattoCore,
		nc:       nc,
		resolver: resolver,
	}

	// Create common test data
	env.createTestData(t)

	return env
}
