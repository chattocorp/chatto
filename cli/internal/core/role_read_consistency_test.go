package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
)

func TestRoleReadsWaitForAnotherReplica(t *testing.T) {
	primary, nc := setupTestCore(t)
	ctx := testContext(t)
	if _, err := primary.CreateServerRole(ctx, SystemActorID, "helper", "Fresh name", ""); err != nil {
		t.Fatal(err)
	}
	viewer, err := primary.CreateUser(ctx, SystemActorID, "role-read", "Role Reader", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := primary.AssignServerRole(ctx, SystemActorID, viewer.Id, "helper"); err != nil {
		t.Fatal(err)
	}
	replica, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	reads := map[string]func(context.Context) error{
		"role":      func(ctx context.Context) error { _, err := replica.GetServerRole(ctx, "helper"); return err },
		"catalogue": func(ctx context.Context) error { _, err := replica.ListServerRoles(ctx); return err },
		"members":   func(ctx context.Context) error { _, err := replica.GetRoleUsers(ctx, "helper"); return err },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			bounded, cancel := context.WithTimeout(ctx, 25*time.Millisecond)
			defer cancel()
			if err := read(bounded); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("read returned before the replica started: %v", err)
			}
		})
	}
	startCoreServices(t, replica)
	role, err := replica.GetServerRole(ctx, "helper")
	if err != nil {
		t.Fatal(err)
	}
	if role.DisplayName != "Fresh name" {
		t.Fatalf("stale role: %+v", role)
	}
	roles, err := replica.ListServerRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, role := range roles {
		if role.Name == "helper" && role.DisplayName == "Fresh name" {
			found = true
		}
	}
	if !found {
		t.Fatal("role missing from catalogue")
	}
	members, err := replica.GetRoleUsers(ctx, "helper")
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 1 || members[0] != viewer.Id {
		t.Fatalf("stale member list: %v", members)
	}
	// A nested role read must retain the caller's exact content generation,
	// even if another replica commits a newer fact during that read.
	err = replica.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		if _, err := primary.CreateServerRole(ctx, SystemActorID, "later", "Later", ""); err != nil {
			return err
		}
		bounded, cancel := context.WithTimeout(readCtx, 50*time.Millisecond)
		defer cancel()
		if _, err := replica.GetServerRole(bounded, "later"); !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("nested read did not retain its snapshot: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replica.GetServerRole(ctx, "later"); err != nil {
		t.Fatalf("next read did not reach the new role: %v", err)
	}
}
