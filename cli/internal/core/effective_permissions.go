package core

import (
	"context"
	"slices"
)

// EffectivePermission is allowed authority at one scope. CoversDescendants is
// evaluated across all applicable child scopes, including invisible rooms.
// Membership and operation-specific requirements remain separate.
type EffectivePermission struct {
	Permission        Permission
	Scope             PermissionMatrixScope
	CoversDescendants bool
}

// ListEffectivePermissions reads one coherent content view. Bot targets are
// visible to authenticated members; human targets require permission management.
// It returns no stored overrides or inactive grants and filters room identities
// only after evaluating child coverage, so hidden restrictions cannot cause a
// false claim of global authority.
func (c *ChattoCore) ListEffectivePermissions(ctx context.Context, actorID, userID string) ([]EffectivePermission, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	var result []EffectivePermission
	err := c.ReadServerContentView(ctx, func(ctx context.Context, _ uint64) error {
		result = nil
		_, _, actorExists := c.userModel.isBotAndOwner(actorID)
		if !actorExists {
			return ErrNotFound
		}
		isBot, _, exists := c.userModel.isBotAndOwner(userID)
		if !exists {
			return ErrNotFound
		}
		if !isBot {
			if err := c.requireCanManageUserPermissionTarget(ctx, actorID); err != nil {
				return err
			}
		}
		scopes, err := c.buildMatrixScopes(ctx, true)
		if err != nil {
			return err
		}
		slices.SortFunc(scopes, func(a, b PermissionMatrixScope) int {
			if a.ID < b.ID {
				return -1
			}
			if a.ID > b.ID {
				return 1
			}
			return 0
		})
		visible := make(map[string]bool, len(scopes))
		for _, scope := range scopes {
			visible[scope.ID] = true
			if scope.Kind == MatrixScopeRoom {
				visible[scope.ID], err = c.CanSeeRoom(ctx, actorID, KindChannel, scopeRefID(scope.ID, "room:"))
				if err != nil {
					return err
				}
			}
		}
		for _, meta := range AllPermissions() {
			if isBot && !botPermissionDelegable(meta.Permission) {
				continue
			}
			allowed := make(map[string]bool, len(scopes))
			for _, scope := range scopes {
				cell, applicable, err := c.buildUserPermissionMatrixCell(ctx, userID, meta.Permission, scope)
				if err != nil {
					return err
				}
				if applicable {
					allowed[scope.ID] = cell.Effective == MatrixDecisionAllow
				}
			}
			coverage := effectivePermissionCoverage(scopes, allowed)
			for _, scope := range scopes {
				if visible[scope.ID] && allowed[scope.ID] {
					result = append(result, EffectivePermission{Permission: meta.Permission, Scope: scope, CoversDescendants: coverage[scope.ID]})
				}
			}
		}
		return nil
	})
	return result, err
}

// Each applicable child can restrict at most two ancestors. DM authority is
// independent of channel defaults and never affects server/group coverage.
func effectivePermissionCoverage(scopes []PermissionMatrixScope, allowed map[string]bool) map[string]bool {
	coverage := make(map[string]bool, len(allowed))
	for id, grant := range allowed {
		coverage[id] = grant
	}
	for _, scope := range scopes {
		grant, applicable := allowed[scope.ID]
		if !applicable || grant {
			continue
		}
		if scope.Kind == MatrixScopeGroup || scope.Kind == MatrixScopeRoom {
			coverage["server"] = false
		}
		if scope.Kind == MatrixScopeRoom {
			coverage["group:"+scope.ParentGroupID] = false
		}
	}
	return coverage
}
