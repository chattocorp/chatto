package core

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPermissionScopePagination(t *testing.T) {
	t.Parallel()

	scopes := []PermissionMatrixScope{{ID: "server", Kind: MatrixScopeServer}, {ID: "dm", Kind: MatrixScopeDM}}
	for i := 109; i >= 0; i-- {
		scopes = append(scopes, PermissionMatrixScope{ID: fmt.Sprintf("room:%03d", i), Kind: MatrixScopeRoom})
	}
	for _, tc := range []struct {
		name         string
		q            PermissionScopeQuery
		count, total int
		more         bool
	}{
		{"default", PermissionScopeQuery{}, 20, 112, true},
		{"cap", PermissionScopeQuery{Limit: 500}, 100, 112, true},
		{"last", PermissionScopeQuery{Offset: 100}, 12, 112, false},
		{"past end", PermissionScopeQuery{Offset: int(^uint(0) >> 1)}, 0, 112, false},
		{"room", PermissionScopeQuery{Scope: &PermissionTargetScope{Kind: MatrixScopeRoom, ID: "009"}}, 1, 1, false},
		{"missing", PermissionScopeQuery{Scope: &PermissionTargetScope{Kind: MatrixScopeRoom, ID: "missing"}}, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, page, err := selectPermissionScopes(append([]PermissionMatrixScope(nil), scopes...), tc.q)
			if err != nil || len(got) != tc.count || page.TotalCount != tc.total || page.HasMore != tc.more {
				t.Fatalf("len=%d page=%+v err=%v", len(got), page, err)
			}
			if tc.name == "default" && (got[0].ID != "server" || got[1].ID != "dm" || got[2].ID != "room:109") {
				t.Fatal("scope order changed")
			}
		})
	}
	for _, q := range []PermissionScopeQuery{{Limit: -1}, {Offset: -1}, {Scope: &PermissionTargetScope{}}, {Scope: &PermissionTargetScope{Kind: MatrixScopeRoom}}, {Scope: &PermissionTargetScope{Kind: MatrixScopeServer, ID: "bad"}}} {
		if _, _, err := selectPermissionScopes(scopes, q); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
}

// Exercise the stored sidebar layout and page boundaries together. IDs must not
// replace the configured group and room order.
func TestPermissionScopePagesFollowSidebarOrder(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	group, err := c.CreateRoomGroup(ctx, SystemActorID, "Matrix group", "")
	require.NoError(t, err)
	first, err := c.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "first", "")
	require.NoError(t, err)
	second, err := c.CreateRoom(ctx, SystemActorID, KindChannel, group.Id, "second", "")
	require.NoError(t, err)
	require.NoError(t, c.ReorderRoomsInGroup(ctx, SystemActorID, group.Id, []string{second.Id, first.Id}))
	groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
	require.NoError(t, err)
	groupIDs := []string{group.Id}
	for _, other := range groups {
		if other.Id != group.Id {
			groupIDs = append(groupIDs, other.Id)
		}
	}
	require.NoError(t, c.ReorderRoomGroups(ctx, SystemActorID, groupIDs))
	scopes, err := c.buildMatrixScopes(ctx, true)
	require.NoError(t, err)
	groups, err = c.ListRoomGroupsOrdered(ctx, KindChannel)
	require.NoError(t, err)
	expected := []string{"server", "dm"}
	for _, group := range groups {
		expected = append(expected, "group:"+group.Id)
		for _, id := range group.RoomIds {
			expected = append(expected, "room:"+id)
		}
	}
	var actual []string
	for offset := 0; ; offset += 2 {
		pageScopes, page, err := selectPermissionScopes(scopes, PermissionScopeQuery{Limit: 2, Offset: offset})
		require.NoError(t, err)
		require.Equal(t, len(expected), page.TotalCount)
		for _, scope := range pageScopes {
			actual = append(actual, scope.ID)
		}
		if !page.HasMore {
			break
		}
	}
	require.Equal(t, expected, actual)
	require.Equal(t, []string{"server", "dm", "group:" + group.Id, "room:" + second.Id, "room:" + first.Id}, actual[:5])
}
