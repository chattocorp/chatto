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

	hubCtx, hubCancel := context.WithCancel(context.Background())
	go c.PresenceHub.Run(hubCtx)
	t.Cleanup(hubCancel)

	return c
}

func TestApplyBootstrap_CreatesUsersAndInstance(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{
				Login:        "alice",
				DisplayName:  "Alice",
				Email:        "alice@example.com",
				Password:     "devpassword",
				InstanceRole: "owner",
			},
			{
				Login:    "bob",
				Email:    "bob@example.com",
				Password: "devpassword",
			},
		},
		Instance: &config.BootstrapInstance{
			Name:        "Engineering",
			Description: "Where things happen",
			Rooms:       []string{"random", "qa"},
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

	if isOwner, err := c.IsInstanceOwner(ctx, alice.Id); err != nil || !isOwner {
		t.Errorf("expected alice to have instance-owner role (err=%v)", err)
	}

	// The instance config should carry the bootstrap name.
	cm := c.ConfigManager()
	if cm == nil {
		t.Fatal("expected ConfigManager to be available")
	}
	cfgInstance, _, err := cm.GetInstanceConfig(ctx)
	if err != nil {
		t.Fatalf("get instance config: %v", err)
	}
	if cfgInstance == nil || cfgInstance.InstanceName != "Engineering" {
		t.Errorf("expected instance name 'Engineering', got %+v", cfgInstance)
	}

	primaryID, err := c.FirstUserFacingSpaceID(ctx)
	if err != nil || primaryID == "" {
		t.Fatalf("expected a primary space to exist: id=%q err=%v", primaryID, err)
	}

	rooms, err := c.ListRoomsBySpace(ctx, primaryID)
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	gotRooms := map[string]bool{}
	for _, r := range rooms {
		gotRooms[r.Name] = true
	}
	for _, want := range []string{"random", "qa"} {
		if !gotRooms[want] {
			t.Errorf("expected room %q on the primary space, got rooms %v", want, gotRooms)
		}
	}
}

func TestApplyBootstrap_IsIdempotent(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "alice", Email: "alice@example.com", Password: "devpassword", InstanceRole: "owner"},
		},
		Instance: &config.BootstrapInstance{Name: "OnlyOne"},
	}

	applyBootstrap(ctx, c, cfg)
	applyBootstrap(ctx, c, cfg) // second run should be a no-op for the same entries

	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		t.Fatalf("list spaces: %v", err)
	}
	count := 0
	for _, sp := range spaces {
		if sp.Name == "OnlyOne" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 primary space, got %d", count)
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
// existing as orphan members of the instance.
func TestApplyBootstrap_AutoJoinsServer(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "devuser", Email: "dev@example.com", Password: "devpassword", InstanceRole: "owner"},
			{Login: "alice", Email: "alice@example.com", Password: "devpassword"},
			{Login: "bob", Email: "bob@example.com", Password: "devpassword"},
		},
		Instance: &config.BootstrapInstance{Name: "Engineering"},
	}
	applyBootstrap(ctx, c, cfg)

	primaryID, err := c.FirstUserFacingSpaceID(ctx)
	if err != nil || primaryID == "" {
		t.Fatalf("expected a primary space to exist: id=%q err=%v", primaryID, err)
	}

	for _, login := range []string{"alice", "bob"} {
		u, err := c.GetUserByLogin(ctx, login)
		if err != nil || u == nil {
			t.Fatalf("expected %s to exist: %v", login, err)
		}
		isMember, err := c.SpaceMembershipExists(ctx, u.Id, primaryID)
		if err != nil {
			t.Fatalf("SpaceMembershipExists(%s): %v", login, err)
		}
		if !isMember {
			t.Errorf("expected %s to be auto-joined to the server", login)
		}
	}
}

// When no user is marked as instance-role=owner, the bootstrap falls back to
// the first defined user as the underlying primary-space owner.
func TestApplyBootstrap_DerivesOwnerFromFirstUser(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "first", Email: "first@example.com", Password: "devpassword"},
			{Login: "second", Email: "second@example.com", Password: "devpassword"},
		},
		Instance: &config.BootstrapInstance{Name: "Fallback"},
	}
	applyBootstrap(ctx, c, cfg)

	primaryID, err := c.FirstUserFacingSpaceID(ctx)
	if err != nil || primaryID == "" {
		t.Fatalf("expected a primary space to exist: id=%q err=%v", primaryID, err)
	}
}
