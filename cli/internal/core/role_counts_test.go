package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/testutil"
)

func TestAdministrativeRoleCounts(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "counts-owner", "Counts Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	member, err := c.CreateUser(ctx, SystemActorID, "counts-member", "Counts Member", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, member.Id, RoleModerator); err != nil {
		t.Fatal(err)
	}
	if err := c.GrantGroupPermission(ctx, SystemActorID, "G-counts", RoleModerator, PermMessagePost); err != nil {
		t.Fatal(err)
	}
	if err := c.DenyRoomPermission(ctx, SystemActorID, "R-counts", RoleModerator, PermMessageRead); err != nil {
		t.Fatal(err)
	}
	read := func(actorID, roleName string) RoleWithPermissions {
		t.Helper()
		catalog, err := c.AdminListServerRoles(ctx, actorID)
		if err != nil {
			t.Fatal(err)
		}
		for _, role := range catalog.Roles {
			if role.Name == roleName {
				return role
			}
		}
		t.Fatalf("role %s missing from catalog", roleName)
		return RoleWithPermissions{}
	}
	moderator := read(owner.Id, RoleModerator)
	want := RoleCounts{MemberCount: 1, PermissionGrantCount: 4, PermissionDenialCount: 1}
	if moderator.Counts == nil || *moderator.Counts != want {
		t.Fatalf("moderator counts = %+v, want %+v", moderator.Counts, want)
	}
	if !moderator.IsSystem {
		t.Fatal("moderator must retain its system-role classification")
	}
	if got := read(owner.Id, RoleEveryone).Counts; got == nil || got.MemberCount != 2 {
		t.Fatalf("everyone counts = %+v, want 2 human members", got)
	}
	if got := read(member.Id, RoleModerator).Counts; got != nil {
		t.Fatalf("ordinary member received administrative counts: %+v", got)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, member.Id, PermRoleAssign); err != nil {
		t.Fatal(err)
	}
	if got := read(member.Id, RoleModerator).Counts; got == nil || *got != want {
		t.Fatalf("assigner counts = %+v, want %+v", got, want)
	}
	if err := c.DenyGroupPermission(ctx, SystemActorID, "G-counts", RoleModerator, PermMessagePost); err != nil {
		t.Fatal(err)
	}
	if err := c.ClearRoomPermissionState(ctx, SystemActorID, "R-counts", RoleModerator, PermMessageRead); err != nil {
		t.Fatal(err)
	}
	if err := c.RevokeServerRole(ctx, SystemActorID, member.Id, RoleModerator); err != nil {
		t.Fatal(err)
	}
	want = RoleCounts{PermissionGrantCount: 3, PermissionDenialCount: 1}
	if got := read(owner.Id, RoleModerator).Counts; got == nil || *got != want {
		t.Fatalf("updated counts = %+v, want %+v", got, want)
	}
	bot, err := c.CreateBot(ctx, owner.Id, "counts_bot", "Counts Bot")
	if err != nil {
		t.Fatal(err)
	}
	if got := read(owner.Id, RoleEveryone).Counts; got == nil || got.MemberCount != 2 {
		t.Fatalf("everyone counts with bot = %+v, want 2 human members", got)
	}
	if err := c.DeleteUser(ctx, owner.Id, bot.User.Id); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteUser(ctx, owner.Id, member.Id); err != nil {
		t.Fatal(err)
	}
	if got := read(owner.Id, RoleEveryone).Counts; got == nil || got.MemberCount != 1 {
		t.Fatalf("everyone counts after deletion = %+v, want 1 human member", got)
	}
}

// An account can be created without a new RBAC fact. The count read must wait
// for that account boundary too, even when this replica has current roles.
func TestAdministrativeRoleCountsWaitForAccountChanges(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	primary := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	owner, err := primary.CreateUser(ctx, SystemActorID, "count-replica-owner", "Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := primary.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatal(err)
	}
	replica, err := NewChattoCore(ctx, newReplicaConn(t, ns), testCoreConfig())
	if err != nil {
		t.Fatal(err)
	}
	stop := testutil.StartService(t, "count replica", replica.Run, replica.WaitForBoot)
	if _, err := replica.AdminListServerRoles(ctx, owner.Id); err != nil {
		t.Fatal(err)
	}
	stop()
	if _, err := primary.CreateUser(ctx, SystemActorID, "count-replica-member", "Member", "password123"); err != nil {
		t.Fatal(err)
	}
	// Nested reads retain the captured generation and must not wait while the
	// content-view barrier prevents this replica from advancing.
	if err := replica.ReadServerContentView(ctx, func(readCtx context.Context, _ uint64) error {
		bounded, cancel := context.WithTimeout(readCtx, 50*time.Millisecond)
		defer cancel()
		catalog, err := replica.AdminListServerRoles(bounded, owner.Id)
		if err != nil {
			return err
		}
		for _, role := range catalog.Roles {
			if role.Name == RoleEveryone && (role.Counts == nil || role.Counts.MemberCount != 1) {
				t.Fatalf("nested count read did not retain its captured generation: %+v", role.Counts)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	bounded, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := replica.AdminListServerRoles(bounded, owner.Id); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("count read returned before the account projection caught up: %v", err)
	}
}
