package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestChattoCore_SyncOIDCRoleClaimsPreservesIndependentSources(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-role-user", "OIDC Role User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	providerA := config.AuthProviderConfig{
		ID: "oidc-a", Type: config.AuthProviderTypeOpenIDConnect,
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleAdmin, RoleModerator}, RoleClaimMode: config.OIDCRoleClaimModeReconcile,
	}
	providerB := config.AuthProviderConfig{
		ID: "oidc-b", Type: config.AuthProviderTypeOpenIDConnect,
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{RoleModerator},
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerA, true, []string{RoleAdmin, "unknown"}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims provider A: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("OIDC provider A should grant admin")
	}
	if err := chatto.AssignServerRoleToExistingUser(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatalf("AssignServerRoleToExistingUser: %v", err)
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerB, true, []string{RoleModerator}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims provider B: %v", err)
	}

	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerA, true, nil); err != nil {
		t.Fatalf("reconcile provider A empty roles: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleAdmin) {
		t.Fatal("reconcile should revoke provider A's admin grant")
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("manual and provider B moderator grants must survive provider A reconciliation")
	}

	if err := chatto.RevokeServerRoleFromExistingUser(ctx, SystemActorID, user.Id, RoleModerator); err != nil {
		t.Fatalf("manual moderator revoke: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("provider B's moderator grant must survive manual revoke")
	}
	if err := chatto.RevokeServerRoleFromExistingUser(ctx, SystemActorID, user.Id, RoleModerator); !errors.Is(err, ErrRoleManagedByOIDC) {
		t.Fatalf("revoke OIDC-only role error = %v, want ErrRoleManagedByOIDC", err)
	}

	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerB, false, nil); err != nil {
		t.Fatalf("missing configured claim should preserve grants: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("missing claim must preserve OIDC grant")
	}
	providerB.RoleClaim = ""
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, providerB, false, nil); err != nil {
		t.Fatalf("disabled role claim cleanup: %v", err)
	}
	if chatto.RBAC.HasRole(user.Id, RoleModerator) {
		t.Fatal("disabled role claim must remove provider-managed grants")
	}
}

func TestChattoCore_SyncOIDCRoleClaimsWildcardIncludesOwner(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "oidc-owner", "OIDC Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	provider := config.AuthProviderConfig{
		ID: "oidc-owner", Type: config.AuthProviderTypeOpenIDConnect,
		RoleClaim: "roles", RoleClaimAllowedRoles: []string{"*"},
	}
	if err := chatto.SyncOIDCRoleClaims(ctx, user.Id, provider, true, []string{RoleOwner, RoleEveryone, "does-not-exist"}); err != nil {
		t.Fatalf("SyncOIDCRoleClaims: %v", err)
	}
	if !chatto.RBAC.HasRole(user.Id, RoleOwner) {
		t.Fatal("wildcard should allow owner")
	}
	if chatto.RBAC.HasRole(user.Id, RoleEveryone) {
		t.Fatal("implicit everyone must never become an explicit OIDC role")
	}
}
