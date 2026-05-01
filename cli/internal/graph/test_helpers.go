package graph

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/auth"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// testEnv holds all test dependencies for GraphQL resolver tests
type testEnv struct {
	ctx      context.Context
	core     *core.ChattoCore
	nc       *nats.Conn
	resolver *Resolver
	// Common test data
	testUser  *corev1.User
	testSpace *corev1.Space
	testRoom  *corev1.Room
}

// setupTestResolver creates a complete test environment with resolver and test data
func setupTestResolver(t *testing.T) *testEnv {
	t.Helper()

	// Start embedded NATS server
	opts := &server.Options{
		JetStream: true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create NATS server: %v", err)
	}

	go ns.Start()
	if !ns.ReadyForConnections(5 * 1e9) {
		t.Fatal("NATS server not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Use a context with timeout for setup
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer setupCancel()

	// Create ChattoCore
	cfg := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	chattoCore, err := core.NewChattoCore(setupCtx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	// Start PresenceHub in background (needed by StreamMySpaceEvents)
	hubCtx, hubCancel := context.WithCancel(context.Background())
	go chattoCore.PresenceHub.Run(hubCtx)

	t.Cleanup(func() {
		hubCancel()
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

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

	// Create test user with verified email and assign the instance-owner role.
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
	if err := e.core.AssignInstanceOwnerRole(e.ctx, user.Id); err != nil {
		t.Fatalf("Failed to assign owner role to test user: %v", err)
	}
	e.testUser = user

	// Create test space
	space, err := e.core.CreateSpace(e.ctx, user.Id, "Test Space", "A space for testing")
	if err != nil {
		t.Fatalf("Failed to create test space: %v", err)
	}
	e.testSpace = space

	// Join the space (required for accessing rooms)
	_, err = e.core.JoinSpace(e.ctx, user.Id, space.Id)
	if err != nil {
		t.Fatalf("Failed to join test space: %v", err)
	}

	// Create test room
	room, err := e.core.CreateRoom(e.ctx, user.Id, space.Id, "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create test room: %v", err)
	}
	e.testRoom = room

	// Join the room (required for posting messages)
	_, err = e.core.JoinRoom(e.ctx, user.Id, space.Id, user.Id, room.Id)
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
// users with matching verified emails are treated as instance owners.
func setupTestResolverWithAdmin(t *testing.T, ownerEmails []string) *testEnv {
	t.Helper()

	// Start embedded NATS server
	opts := &server.Options{
		JetStream: true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create NATS server: %v", err)
	}

	go ns.Start()
	if !ns.ReadyForConnections(5 * 1e9) {
		t.Fatal("NATS server not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Use a context with timeout for setup
	setupCtx, setupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer setupCancel()

	// Create ChattoCore
	cfg := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	chattoCore, err := core.NewChattoCore(setupCtx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	// Start PresenceHub in background (needed by StreamMySpaceEvents)
	hubCtx, hubCancel := context.WithCancel(context.Background())
	go chattoCore.PresenceHub.Run(hubCtx)

	// Create owners config
	ownersConfig := config.OwnersConfig{Emails: ownerEmails}

	t.Cleanup(func() {
		hubCancel()
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

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
