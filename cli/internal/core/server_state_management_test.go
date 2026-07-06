package core

import (
	"bytes"
	"errors"
	"slices"
	"testing"
)

func TestChattoCoreServerStateManagementRequiresServerManage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	actor, err := core.CreateUser(ctx, SystemActorID, "server-state-manager", "Server State Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}

	if _, err := core.UpdateServerConfig(ctx, actor.Id, ServerConfigUpdateInput{ServerName: stringPtrForCoreTest("Nope")}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("UpdateServerConfig without server.manage error = %v, want ErrPermissionDenied", err)
	}
	if _, err := core.GetManagedServerConfig(ctx, actor.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetManagedServerConfig without server.manage error = %v, want ErrPermissionDenied", err)
	}
	if _, err := core.GetServerSecurityConfig(ctx, actor.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetServerSecurityConfig without server.manage error = %v, want ErrPermissionDenied", err)
	}
	if _, err := core.UpdateBlockedUsernames(ctx, actor.Id, []string{"root"}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("UpdateBlockedUsernames without server.manage error = %v, want ErrPermissionDenied", err)
	}
	if _, err := core.UploadManagedServerLogo(ctx, actor.Id, bytes.NewReader(createTestPNG(100, 100))); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("UploadManagedServerLogo without server.manage error = %v, want ErrPermissionDenied", err)
	}
	if err := core.DeleteManagedServerLogo(ctx, actor.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("DeleteManagedServerLogo without server.manage error = %v, want ErrPermissionDenied", err)
	}

	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermServerManage); err != nil {
		t.Fatalf("GrantServerPermission server.manage: %v", err)
	}

	cfg, err := core.UpdateServerConfig(ctx, actor.Id, ServerConfigUpdateInput{
		ServerName:     stringPtrForCoreTest("Core Managed Server"),
		Description:    stringPtrForCoreTest("Core-managed description"),
		MOTD:           stringPtrForCoreTest("Core-managed MOTD"),
		WelcomeMessage: stringPtrForCoreTest("Core-managed welcome"),
	})
	if err != nil {
		t.Fatalf("UpdateServerConfig with server.manage: %v", err)
	}
	if cfg.GetServerName() != "Core Managed Server" ||
		cfg.GetDescription() != "Core-managed description" ||
		cfg.GetMotd() != "Core-managed MOTD" ||
		cfg.GetWelcomeMessage() != "Core-managed welcome" {
		t.Fatalf("updated server config = %+v", cfg)
	}
	readCfg, err := core.GetManagedServerConfig(ctx, actor.Id)
	if err != nil {
		t.Fatalf("GetManagedServerConfig with server.manage: %v", err)
	}
	if readCfg.GetServerName() != "Core Managed Server" ||
		readCfg.GetDescription() != "Core-managed description" ||
		readCfg.GetMotd() != "Core-managed MOTD" ||
		readCfg.GetWelcomeMessage() != "Core-managed welcome" {
		t.Fatalf("managed server config = %+v", readCfg)
	}

	blocked, err := core.GetServerSecurityConfig(ctx, actor.Id)
	if err != nil {
		t.Fatalf("GetServerSecurityConfig with server.manage: %v", err)
	}
	defaultBlockedUsernames := []string{"root", "admin", "superuser", "op", "operator", "support"}
	if !slices.Equal(blocked, defaultBlockedUsernames) {
		t.Fatalf("default blocked usernames = %q, want %q", blocked, defaultBlockedUsernames)
	}
	blocked, err = core.UpdateBlockedUsernames(ctx, actor.Id, []string{"root", "Reserved", "admin"})
	if err != nil {
		t.Fatalf("UpdateBlockedUsernames with server.manage: %v", err)
	}
	if want := []string{"root", "reserved", "admin"}; !slices.Equal(blocked, want) {
		t.Fatalf("blocked usernames = %q, want %q", blocked, want)
	}
	blocked, err = core.UpdateBlockedUsernames(ctx, actor.Id, []string{"root\nreserved"})
	if err != nil {
		t.Fatalf("UpdateBlockedUsernames newline compatibility with server.manage: %v", err)
	}
	if want := []string{"root", "reserved"}; !slices.Equal(blocked, want) {
		t.Fatalf("compat blocked usernames = %q, want %q", blocked, want)
	}

	logo, err := core.UploadManagedServerLogo(ctx, actor.Id, bytes.NewReader(createTestPNG(100, 100)))
	if err != nil {
		t.Fatalf("UploadManagedServerLogo with server.manage: %v", err)
	}
	if logo.GetId() == "" {
		t.Fatal("managed logo asset id is empty")
	}
	storedLogo, err := core.GetServerLogo(ctx)
	if err != nil {
		t.Fatalf("GetServerLogo: %v", err)
	}
	if storedLogo.GetId() != logo.GetId() {
		t.Fatalf("stored logo id = %q, want %q", storedLogo.GetId(), logo.GetId())
	}
	if err := core.DeleteManagedServerLogo(ctx, actor.Id); err != nil {
		t.Fatalf("DeleteManagedServerLogo with server.manage: %v", err)
	}
	storedLogo, err = core.GetServerLogo(ctx)
	if err != nil {
		t.Fatalf("GetServerLogo after delete: %v", err)
	}
	if storedLogo != nil {
		t.Fatalf("stored logo after delete = %+v, want nil", storedLogo)
	}
}

func stringPtrForCoreTest(value string) *string {
	return &value
}
