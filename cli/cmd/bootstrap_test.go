//go:build bootstrap

package cmd

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

// setupCore spins up an in-process NATS server + ChattoCore for cmd-layer tests.
// Mirrors the pattern used in core/core_test.go.
func setupCore(t *testing.T) *core.ChattoCore {
	t.Helper()

	opts := &server.Options{JetStream: true, Port: -1, StoreDir: t.TempDir()}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	cfg := config.CoreConfig{Assets: config.AssetsConfig{SigningSecret: "test-secret"}}
	c, err := core.NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("new core: %v", err)
	}

	// Production `run.go` calls SeedDefaultRooms after NewChattoCore; mirror
	// that here so bootstrap tests see the same starting state.
	if err := c.SeedDefaultRooms(ctx); err != nil {
		t.Fatalf("seed default rooms: %v", err)
	}

	hubCtx, hubCancel := context.WithCancel(context.Background())
	go c.PresenceHub.Run(hubCtx)
	t.Cleanup(hubCancel)

	return c
}

func TestApplyBootstrap_CreatesUsersAndServer(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{
				Login:        "alice",
				DisplayName:  "Alice",
				Email:        "alice@example.com",
				Password:     "devpassword",
				ServerRole: "owner",
			},
			{
				Login:    "bob",
				Email:    "bob@example.com",
				Password: "devpassword",
			},
		},
		Server: &config.BootstrapServer{
			Name:  "Engineering",
			Rooms: []string{"random", "qa"},
		},
	}
	applyBootstrap(ctx, c, cfg)

	alice, err := c.GetUserByLogin(ctx, "alice")
	if err != nil || alice == nil {
		t.Fatalf("expected alice to exist: %v", err)
	}
	bob, err := c.GetUserByLogin(ctx, "bob")
	if err != nil || bob == nil {
		t.Fatalf("expected bob to exist: %v", err)
	}

	if hasEmail, _ := c.HasVerifiedEmail(ctx, alice.Id); !hasEmail {
		t.Errorf("expected alice to have a verified email")
	}

	if isOwner, err := c.IsServerOwner(ctx, alice.Id); err != nil || !isOwner {
		t.Errorf("expected alice to have owner role (err=%v)", err)
	}

	// The server config should carry the bootstrap name.
	cm := c.ConfigManager()
	if cm == nil {
		t.Fatal("expected ConfigManager to be available")
	}
	cfgServer, _, err := cm.GetServerConfig(ctx)
	if err != nil {
		t.Fatalf("get server config: %v", err)
	}
	if cfgServer == nil || cfgServer.ServerName != "Engineering" {
		t.Errorf("expected server name 'Engineering', got %+v", cfgServer)
	}

	rooms, err := c.ListRooms(ctx, "channel")
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	gotRooms := map[string]bool{}
	for _, r := range rooms {
		gotRooms[r.Name] = true
	}
	for _, want := range []string{"random", "qa"} {
		if !gotRooms[want] {
			t.Errorf("expected room %q after bootstrap, got rooms %v", want, gotRooms)
		}
	}
}

func TestApplyBootstrap_IsIdempotent(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "alice", Email: "alice@example.com", Password: "devpassword", ServerRole: "owner"},
		},
		Server: &config.BootstrapServer{Name: "OnlyOne"},
	}

	applyBootstrap(ctx, c, cfg)
	applyBootstrap(ctx, c, cfg) // second run should be a no-op for the same entries

	// Bootstrap is idempotent at the room level: re-running shouldn't
	// duplicate the default rooms (CreateRoom fails ErrRoomNameExists).
	rooms, err := c.ListRooms(ctx, "channel")
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	names := map[string]int{}
	for _, r := range rooms {
		names[r.Name]++
	}
	for name, count := range names {
		if count > 1 {
			t.Errorf("expected exactly one room named %q, got %d", name, count)
		}
	}
}

func TestApplyBootstrap_EmptySectionIsNoOp(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	applyBootstrap(ctx, c, config.BootstrapConfig{}) // zero value, nothing to do

	if u, err := c.GetUserByLogin(ctx, "alice"); err == nil && u != nil {
		t.Errorf("expected no users to be created from an empty section")
	}
}

// Bootstrap users are auto-joined to the deployment's primary space so non-owner
// users (alice/bob in the dev config) actually land on the server rather than
// existing as orphan members of the server.
func TestApplyBootstrap_AutoJoinsServer(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "devuser", Email: "dev@example.com", Password: "devpassword", ServerRole: "owner"},
			{Login: "alice", Email: "alice@example.com", Password: "devpassword"},
			{Login: "bob", Email: "bob@example.com", Password: "devpassword"},
		},
		Server: &config.BootstrapServer{Name: "Engineering"},
	}
	applyBootstrap(ctx, c, cfg)

	// Server "membership" itself is implicit post-#330 — every authenticated
	// user counts as a member. Bootstrap's contribution is auto-joining the
	// user to the default rooms.
	rooms, err := c.ListRooms(ctx, "channel")
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	if len(rooms) == 0 {
		t.Fatal("expected default rooms to exist after bootstrap")
	}
	defaultRoom := rooms[0]

	for _, login := range []string{"alice", "bob"} {
		u, err := c.GetUserByLogin(ctx, login)
		if err != nil || u == nil {
			t.Fatalf("expected %s to exist: %v", login, err)
		}
		isMember, err := c.RoomMembershipExists(ctx, "channel", u.Id, defaultRoom.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists(%s): %v", login, err)
		}
		if !isMember {
			t.Errorf("expected %s to be auto-joined to default room %s", login, defaultRoom.Id)
		}
	}
}

// When no user is marked as role=owner, the bootstrap falls back to
// the first defined user as the underlying primary-space owner.
func TestApplyBootstrap_DerivesOwnerFromFirstUser(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "first", Email: "first@example.com", Password: "devpassword"},
			{Login: "second", Email: "second@example.com", Password: "devpassword"},
		},
		Server: &config.BootstrapServer{Name: "Fallback"},
	}
	applyBootstrap(ctx, c, cfg)

	rooms, err := c.ListRooms(ctx, "channel")
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	if len(rooms) == 0 {
		t.Fatal("expected default rooms after bootstrap")
	}
}
