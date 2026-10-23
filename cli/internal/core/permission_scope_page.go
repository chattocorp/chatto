package core

import (
	"fmt"
)

// PermissionScopeQuery selects matching scopes before permission evaluation.
// All selects a complete matrix; otherwise Limit and Offset count scopes, not cells.
// Scope is an optional exact filter in both modes.
type PermissionScopeQuery struct {
	Scope  *PermissionTargetScope
	Limit  int
	Offset int
	// All returns every matching scope for a complete matrix. Decision lists use pages.
	All bool
}

// PermissionScopePage describes the matching visible scope collection.
type PermissionScopePage struct {
	TotalCount int
	HasMore    bool
}

func (q PermissionScopeQuery) includesDM() bool {
	return q.Scope != nil && q.Scope.Kind == MatrixScopeDM
}

// selectPermissionScopes preserves the sidebar order from buildMatrixScopesVisibleTo.
// Enumeration scales with the directory. Decision lists bound evaluation and
// result size; complete matrices evaluate every matching scope.
func selectPermissionScopes(scopes []PermissionMatrixScope, q PermissionScopeQuery) ([]PermissionMatrixScope, PermissionScopePage, error) {
	if q.Limit < 0 || q.Offset < 0 {
		return nil, PermissionScopePage{}, fmt.Errorf("%w: negative scope page", ErrInvalidArgument)
	}
	if q.Scope != nil {
		target := *q.Scope
		switch target.Kind {
		case MatrixScopeServer, MatrixScopeDM:
			if target.ID != "" {
				return nil, PermissionScopePage{}, fmt.Errorf("%w: scope id must be empty", ErrInvalidArgument)
			}
		case MatrixScopeGroup, MatrixScopeRoom:
			if target.ID == "" {
				return nil, PermissionScopePage{}, fmt.Errorf("%w: scope id is required", ErrInvalidArgument)
			}
		default:
			return nil, PermissionScopePage{}, fmt.Errorf("%w: explicit scope kind is required", ErrInvalidArgument)
		}
		filtered := make([]PermissionMatrixScope, 0, 1)
		for _, scope := range scopes {
			id := scope.ID
			if scope.Kind == MatrixScopeGroup {
				id = scopeRefID(id, "group:")
			}
			if scope.Kind == MatrixScopeRoom {
				id = scopeRefID(id, "room:")
			}
			if scope.Kind == MatrixScopeServer || scope.Kind == MatrixScopeDM {
				id = ""
			}
			if scope.Kind == target.Kind && id == target.ID {
				filtered = append(filtered, scope)
			}
		}
		scopes = filtered
	}
	page := PermissionScopePage{TotalCount: len(scopes)}
	if q.All {
		return scopes, page, nil
	}
	if q.Offset >= len(scopes) {
		return []PermissionMatrixScope{}, page, nil
	}
	limit := q.Limit
	if limit == 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	end := q.Offset + min(limit, len(scopes)-q.Offset)
	page.HasMore = end < len(scopes)
	return scopes[q.Offset:end], page, nil
}
