package core

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// grantTestRank assigns userID a new role without permissions and places it
// directly below admin. The user then outranks moderators, custom roles, and
// accounts without roles, but not admins or owners.
func grantTestRank(t *testing.T, c *ChattoCore, ctx context.Context, userID string) {
	t.Helper()
	roleName := "rank-" + strings.ToLower(userID)
	if _, err := c.CreateServerRole(ctx, SystemActorID, roleName, "Test rank", ""); err != nil {
		t.Fatalf("CreateServerRole %s: %v", roleName, err)
	}
	moveTestRoleBelow(t, c, ctx, roleName, RoleAdmin)
	if err := c.AssignServerRole(ctx, SystemActorID, userID, roleName); err != nil {
		t.Fatalf("AssignServerRole %s: %v", roleName, err)
	}
}

// moveTestRoleBelow places roleName directly below upperRoleName.
func moveTestRoleBelow(t *testing.T, c *ChattoCore, ctx context.Context, roleName, upperRoleName string) {
	t.Helper()
	order := slices.DeleteFunc(c.orderableRoleNames(), func(name string) bool { return name == roleName })
	before := ""
	if upper := slices.Index(order, upperRoleName); upper > 0 {
		before = order[upper-1]
	}
	if _, err := c.MoveServerRole(ctx, SystemActorID, roleName, before); err != nil {
		t.Fatalf("MoveServerRole %s below %s: %v", roleName, upperRoleName, err)
	}
}
