package core

import (
	"errors"
	"fmt"
	"testing"
)

func TestPermissionScopePagination(t *testing.T) {
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
			if tc.name == "default" && (got[0].ID != "server" || got[1].ID != "dm" || got[2].ID != "room:000") {
				t.Fatal("unstable scope ordering")
			}
		})
	}
	for _, q := range []PermissionScopeQuery{{Limit: -1}, {Offset: -1}, {Scope: &PermissionTargetScope{}}, {Scope: &PermissionTargetScope{Kind: MatrixScopeRoom}}, {Scope: &PermissionTargetScope{Kind: MatrixScopeServer, ID: "bad"}}} {
		if _, _, err := selectPermissionScopes(scopes, q); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
}
