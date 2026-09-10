package core

import (
	"fmt"
	"sort"
)

// PermissionScopeQuery selects a live page of scopes before permission evaluation.
// Scope is an optional exact filter. Limit and Offset count scopes, not cells.
type PermissionScopeQuery struct {
	Scope  *PermissionTargetScope
	Limit  int
	Offset int
}

// PermissionScopePage describes the matching visible scope collection.
type PermissionScopePage struct {
	TotalCount int
	HasMore    bool
}

func firstPermissionScopeQuery(queries []PermissionScopeQuery) PermissionScopeQuery {
	if len(queries) == 0 {
		return PermissionScopeQuery{}
	}
	return queries[0]
}

func (q PermissionScopeQuery) includesDM() bool {
	return q.Scope != nil && q.Scope.Kind == MatrixScopeDM
}

// selectPermissionScopes uses stable kind/ID ordering. Enumeration can still
// scale with the directory; permission evaluation and result size are bounded.
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
	rank := func(kind MatrixScopeKind) int {
		switch kind {
		case MatrixScopeServer:
			return 0
		case MatrixScopeDM:
			return 1
		case MatrixScopeGroup:
			return 2
		default:
			return 3
		}
	}
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].Kind != scopes[j].Kind {
			return rank(scopes[i].Kind) < rank(scopes[j].Kind)
		}
		return scopes[i].ID < scopes[j].ID
	})
	page := PermissionScopePage{TotalCount: len(scopes)}
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
