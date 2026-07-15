package core

import (
	"errors"
	"testing"
)

func TestGetAdminDiagnosticsRequiresOwner(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	member, err := core.CreateUser(ctx, SystemActorID, "core-diagnostics-member", "Core Diagnostics Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := core.GetAdminDiagnostics(ctx, member.Id); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetAdminDiagnostics regular member error = %v, want ErrPermissionDenied", err)
	}

	owner, err := core.CreateUser(ctx, SystemActorID, "core-diagnostics-owner", "Core Diagnostics Owner", "password")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("AssignOwnerRole: %v", err)
	}
	diagnostics, err := core.GetAdminDiagnostics(ctx, owner.Id)
	if err != nil {
		t.Fatalf("GetAdminDiagnostics owner: %v", err)
	}
	if diagnostics.Connection == nil {
		t.Fatal("Connection = nil")
	}
	if diagnostics.Account == nil {
		t.Fatal("Account = nil")
	}
	if diagnostics.Stats == nil {
		t.Fatal("Stats = nil")
	}
	if diagnostics.JetStream == nil {
		t.Fatal("JetStream = nil")
	}
	if len(diagnostics.Projections) == 0 {
		t.Fatal("Projections len = 0, want projection diagnostics")
	}

	if _, err := core.storage.memoryCacheKV.Put(ctx, assetCleanupStatusKey, []byte("not-json")); err != nil {
		t.Fatalf("write malformed cleanup status: %v", err)
	}
	diagnostics, err = core.GetAdminDiagnostics(ctx, owner.Id)
	if err != nil {
		t.Fatalf("GetAdminDiagnostics with malformed cleanup status: %v", err)
	}
	if diagnostics.AssetCleanup.Health != AssetCleanupHealthUnavailable {
		t.Fatalf("AssetCleanup health = %v, want unavailable", diagnostics.AssetCleanup.Health)
	}
}
