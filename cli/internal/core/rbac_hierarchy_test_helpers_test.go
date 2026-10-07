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
	order := slices.DeleteFunc(c.orderableRoleNames(), func(name string) bool { return name == roleName })
	adminIndex := slices.Index(order, RoleAdmin)
	order = slices.Insert(order, adminIndex, roleName)
	if _, err := c.ReorderServerRoles(ctx, SystemActorID, order); err != nil {
		t.Fatalf("ReorderServerRoles: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, userID, roleName); err != nil {
		t.Fatalf("AssignServerRole %s: %v", roleName, err)
	}
}
