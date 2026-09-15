package core

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBotPermissionsVisibilityCeilingAndInclusion(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "summary-owner", "Owner", "password123")
	require.NoError(t, err)
	viewer, err := c.CreateUser(ctx, SystemActorID, "summary-viewer", "Viewer", "password123")
	require.NoError(t, err)
	bot, err := c.CreateBot(ctx, owner.Id, "summary_bot", "Summary Bot")
	require.NoError(t, err)
	botID := bot.User.Id
	entries, err := c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	require.Empty(t, entries)
	require.NoError(t, c.GrantUserPermission(ctx, owner.Id, botID, PermMessageRead))
	entries, err = c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	require.Equal(t, []BotPermission{
		{Permission: PermMessageRead, Scope: PermissionMatrixScope{ID: "dm", Label: "Direct messages", Kind: MatrixScopeDM}, Active: true},
		{Permission: PermMessageRead, Scope: PermissionMatrixScope{ID: "server", Label: "Server", Kind: MatrixScopeServer}, Active: true},
	}, entries)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "summary-secret", "")
	require.NoError(t, err)
	// A hidden room's owner denial must invalidate a global claim without
	// revealing its ID or name to an ordinary viewer.
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, owner.Id, PermMessageRead))
	require.NoError(t, c.DenyUserRoomPermission(ctx, SystemActorID, room.Id, viewer.Id, PermRoomList))
	require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, owner.Id, PermMessageReadInteractions))
	entries, err = c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	for _, entry := range entries {
		require.True(t, entry.Active)
		require.NotEqual(t, "server", entry.Scope.ID)
		require.NotEqual(t, "room:"+room.Id, entry.Scope.ID)
	}
	managerEntries, err := c.ListBotPermissions(ctx, owner.Id, botID)
	require.NoError(t, err)
	scope := PermissionMatrixScope{ID: "room:" + room.Id, Label: room.Name, Kind: MatrixScopeRoom, ParentGroupID: room.GroupId}
	require.Contains(t, managerEntries, BotPermission{Permission: PermMessageRead, Scope: scope, Active: false})
	require.Contains(t, managerEntries, BotPermission{Permission: PermMessageReadInteractions, Scope: scope, Active: true})
	// Account administration alone must not expose unavailable grants.
	require.NoError(t, c.ClearUserRoomPermissionState(ctx, SystemActorID, room.Id, viewer.Id, PermRoomList))
	require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, viewer.Id, PermUserManageAccounts))
	entries, err = c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	for _, entry := range entries {
		require.True(t, entry.Active)
	}
	require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, viewer.Id, PermBotManage))
	entries, err = c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	require.Contains(t, entries, BotPermission{Permission: PermMessageRead, Scope: scope, Active: false})
	// A bot can inspect public access, but cannot acquire manager-only details.
	entries, err = c.ListBotPermissions(ctx, botID, botID)
	require.NoError(t, err)
	for _, entry := range entries {
		require.True(t, entry.Active)
	}
	require.NoError(t, c.ClearUserRoomPermissionState(ctx, SystemActorID, room.Id, owner.Id, PermMessageRead))
	entries, err = c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, MatrixScopeServer, entries[1].Scope.Kind)
	require.NoError(t, c.SetUserPermissionState(ctx, owner.Id, botID, PermissionTargetScope{Kind: MatrixScopeDM}, PermMessageRead, PermissionStateAllow))
	entries, err = c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, MatrixScopeDM, entries[0].Scope.Kind)
	require.Equal(t, MatrixScopeServer, entries[1].Scope.Kind)
	require.NoError(t, c.ClearUserPermissionState(ctx, owner.Id, botID, PermMessageRead))
	entries, err = c.ListBotPermissions(ctx, viewer.Id, botID)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, MatrixScopeDM, entries[0].Scope.Kind)
	_, err = c.ListBotPermissions(ctx, viewer.Id, owner.Id)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.ListBotPermissions(ctx, "", botID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCompactBotPermission(t *testing.T) {
	scopes := []PermissionMatrixScope{
		{ID: "server", Kind: MatrixScopeServer}, {ID: "dm", Kind: MatrixScopeDM},
		{ID: "group:g", Kind: MatrixScopeGroup},
		{ID: "room:a", Kind: MatrixScopeRoom, ParentGroupID: "g"},
		{ID: "room:b", Kind: MatrixScopeRoom, ParentGroupID: "g"},
	}
	tests := []struct {
		name   string
		states map[string]int
		ids    []string
	}{
		{"global and DM", map[string]int{"server": 1, "group:g": 1, "room:a": 1, "room:b": 1, "dm": 1}, []string{"server", "dm"}},
		{"group grant", map[string]int{"server": 0, "group:g": 1, "room:a": 1, "room:b": 1}, []string{"group:g"}},
		{"room restriction", map[string]int{"server": 1, "group:g": 1, "room:a": 1, "room:b": 2}, []string{"room:a", "room:b"}},
		{"inactive global", map[string]int{"server": 2, "group:g": 2, "room:a": 2, "room:b": 2}, []string{"server"}},
		{"group-only permission", map[string]int{"server": 1, "group:g": 1}, []string{"server"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := compactBotPermission(PermMessageRead, scopes, tt.states)
			var ids []string
			for _, entry := range entries {
				ids = append(ids, entry.Scope.ID)
			}
			require.Equal(t, tt.ids, ids)
		})
	}
}
