package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectivePermissionsAuthorizationAndScopeCoverage(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "effective-owner", "Owner", "password123")
	require.NoError(t, err)
	viewer, err := c.CreateUser(ctx, SystemActorID, "effective-viewer", "Viewer", "password123")
	require.NoError(t, err)
	bot, err := c.CreateBot(ctx, owner.Id, "effective_bot", "Bot")
	require.NoError(t, err)
	_, err = c.ListEffectivePermissions(ctx, "", bot.User.Id)
	require.ErrorIs(t, err, ErrNotAuthenticated)
	_, err = c.ListEffectivePermissions(ctx, viewer.Id, owner.Id)
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = c.ListEffectivePermissions(ctx, viewer.Id, viewer.Id)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.GrantUserPermission(ctx, owner.Id, bot.User.Id, PermMessageRead))
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "effective-secret", "")
	require.NoError(t, err)
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, owner.Id, PermMessageRead))
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, viewer.Id, PermRoomList))
	entries, err := c.ListEffectivePermissions(ctx, viewer.Id, bot.User.Id)
	require.NoError(t, err)
	var server, dm bool
	for _, entry := range entries {
		require.NotEqual(t, "room:"+room.Id, entry.Scope.ID)
		if entry.Permission == PermMessageRead && entry.Scope.Kind == MatrixScopeServer {
			server = true
			require.False(t, entry.CoversDescendants)
		}
		if entry.Permission == PermMessageRead && entry.Scope.Kind == MatrixScopeDM {
			dm = true
			require.True(t, entry.CoversDescendants)
		}
	}
	require.True(t, server)
	require.True(t, dm)
	// Bot callers can inspect public bot authority, without becoming managers.
	_, err = c.ListEffectivePermissions(ctx, bot.User.Id, bot.User.Id)
	require.NoError(t, err)
	_, err = c.ListEffectivePermissions(ctx, bot.User.Id, owner.Id)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, viewer.Id, PermUserManagePermissions))
	human, err := c.ListEffectivePermissions(ctx, viewer.Id, owner.Id)
	require.NoError(t, err)
	require.NotEmpty(t, human)
	// Restoring owner authority restores descendant coverage.
	require.NoError(t, c.ClearUserRoomPermissionState(ctx, SystemActorID, room.Id, owner.Id, PermMessageRead))
	entries, err = c.ListEffectivePermissions(ctx, viewer.Id, bot.User.Id)
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Permission == PermMessageRead && entry.Scope.Kind == MatrixScopeServer {
			require.True(t, entry.CoversDescendants)
		}
	}

	// A saved grant blocked by the owner is absent even for the bot manager.
	require.NoError(t, c.GrantUserPermission(ctx, owner.Id, bot.User.Id, PermMessageManage))
	require.NoError(t, c.DenyUserPermission(ctx, SystemActorID, owner.Id, PermMessageManage))
	entries, err = c.ListEffectivePermissions(ctx, owner.Id, bot.User.Id)
	require.NoError(t, err)
	for _, entry := range entries {
		require.NotEqual(t, PermMessageManage, entry.Permission)
	}
	// Revocation is enforced on the next read, including a caller's own account.
	require.NoError(t, c.ClearUserPermissionState(ctx, SystemActorID, viewer.Id, PermUserManagePermissions))
	_, err = c.ListEffectivePermissions(ctx, viewer.Id, owner.Id)
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = c.DeleteBot(ctx, owner.Id, bot.User.Id)
	require.NoError(t, err)
	_, err = c.ListEffectivePermissions(ctx, viewer.Id, bot.User.Id)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestEffectivePermissionCoverageKeepsGroupsAndDMIndependent(t *testing.T) {
	scopes := []PermissionMatrixScope{
		{ID: "server", Kind: MatrixScopeServer},
		{ID: "dm", Kind: MatrixScopeDM},
		{ID: "group:a", Kind: MatrixScopeGroup},
		{ID: "group:b", Kind: MatrixScopeGroup},
		{ID: "room:restricted", Kind: MatrixScopeRoom, ParentGroupID: "a"},
		{ID: "room:allowed", Kind: MatrixScopeRoom, ParentGroupID: "b"},
	}
	coverage := effectivePermissionCoverage(scopes, map[string]bool{
		"server": true, "dm": true, "group:a": true, "group:b": true,
		"room:restricted": false, "room:allowed": true,
	})
	require.False(t, coverage["server"])
	require.False(t, coverage["group:a"])
	require.True(t, coverage["group:b"])
	require.True(t, coverage["dm"])
	coverage = effectivePermissionCoverage(scopes, map[string]bool{"server": true, "dm": false})
	require.True(t, coverage["server"], "DM restrictions must not narrow channel defaults")
}
