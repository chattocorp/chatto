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

func TestApplyBootstrap_CreatesUsersAndSpaces(t *testing.T) {
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
		Spaces: []config.BootstrapSpace{
			{
				Name:        "Engineering",
				Description: "Where things happen",
				OwnerLogin:  "alice",
				Rooms:       []string{"random", "qa"},
			},
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

	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		t.Fatalf("list spaces: %v", err)
	}
	var engID string
	for _, sp := range spaces {
		if sp.Name == "Engineering" {
			engID = sp.Id
			break
		}
	}
	if engID == "" {
		t.Fatal("expected Engineering space to exist")
	}

	rooms, err := c.ListRoomsBySpace(ctx, engID)
	if err != nil {
		t.Fatalf("list rooms: %v", err)
	}
	gotRooms := map[string]bool{}
	for _, r := range rooms {
		gotRooms[r.Name] = true
	}
	for _, want := range []string{"random", "qa"} {
		if !gotRooms[want] {
			t.Errorf("expected room %q in Engineering, got rooms %v", want, gotRooms)
		}
	}
}

func TestApplyBootstrap_IsIdempotent(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "alice", Email: "alice@example.com", Password: "devpassword"},
		},
		Spaces: []config.BootstrapSpace{
			{Name: "OnlyOne", OwnerLogin: "alice"},
		},
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
		t.Errorf("expected exactly 1 OnlyOne space, got %d", count)
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

func TestApplyBootstrap_BadOwnerLoginSkipsSpace(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	cfg := config.BootstrapConfig{
		Users: []config.BootstrapUser{
			{Login: "alice", Email: "alice@example.com", Password: "devpassword"},
		},
		Spaces: []config.BootstrapSpace{
			{Name: "Orphan", OwnerLogin: "ghost"},
		},
	}
	applyBootstrap(ctx, c, cfg)

	spaces, _ := c.ListSpaces(ctx)
	for _, sp := range spaces {
		if sp.Name == "Orphan" {
			t.Errorf("space with bad owner_login should not be created")
		}
	}
}
