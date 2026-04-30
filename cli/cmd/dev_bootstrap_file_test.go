//go:build dev

package cmd

import (
	"context"
	"os"
	"path/filepath"
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

func writeBootstrapFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bootstrap.toml")
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func TestDevBootstrapFromFile_CreatesUsersAndSpaces(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	path := writeBootstrapFile(t, `
[[users]]
login = "alice"
display_name = "Alice"
email = "alice@example.com"
password = "devpassword"
instance_role = "owner"

[[users]]
login = "bob"
email = "bob@example.com"
password = "devpassword"

[[spaces]]
name = "Engineering"
description = "Where things happen"
owner_login = "alice"
rooms = ["random", "qa"]
`)
	t.Setenv("CHATTO_BOOTSTRAP_FILE", path)

	devBootstrapFromFile(ctx, c)

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
	var eng *string
	for _, sp := range spaces {
		if sp.Name == "Engineering" {
			id := sp.Id
			eng = &id
			break
		}
	}
	if eng == nil {
		t.Fatal("expected Engineering space to exist")
	}

	rooms, err := c.ListRoomsBySpace(ctx, *eng)
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

func TestDevBootstrapFromFile_IsIdempotent(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	path := writeBootstrapFile(t, `
[[users]]
login = "alice"
email = "alice@example.com"
password = "devpassword"

[[spaces]]
name = "OnlyOne"
owner_login = "alice"
`)
	t.Setenv("CHATTO_BOOTSTRAP_FILE", path)

	devBootstrapFromFile(ctx, c)
	devBootstrapFromFile(ctx, c) // second run should be a no-op for the same entries

	// Still exactly one space named "OnlyOne".
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

func TestDevBootstrapFromFile_NoFileEnvVar_NoOp(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	t.Setenv("CHATTO_BOOTSTRAP_FILE", "")
	devBootstrapFromFile(ctx, c) // should silently return; no users created

	// Sanity check: no user named "alice" because we never said anything about her.
	if u, err := c.GetUserByLogin(ctx, "alice"); err == nil && u != nil {
		t.Errorf("expected no users to be created when CHATTO_BOOTSTRAP_FILE is unset")
	}
}

func TestDevBootstrapFromFile_BadOwnerLoginSkipsSpace(t *testing.T) {
	c := setupCore(t)
	ctx := context.Background()

	path := writeBootstrapFile(t, `
[[users]]
login = "alice"
email = "alice@example.com"
password = "devpassword"

[[spaces]]
name = "Orphan"
owner_login = "ghost"
`)
	t.Setenv("CHATTO_BOOTSTRAP_FILE", path)

	devBootstrapFromFile(ctx, c)

	spaces, _ := c.ListSpaces(ctx)
	for _, sp := range spaces {
		if sp.Name == "Orphan" {
			t.Errorf("space with bad owner_login should not be created")
		}
	}
}
