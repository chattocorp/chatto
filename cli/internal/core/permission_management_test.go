package core

import "testing"

func TestRolePermissionMatrixAppliesMessageReadInclusion(t *testing.T) {
	scope := PermissionMatrixScope{ID: "server", Kind: MatrixScopeServer}

	t.Run("broad allow wins over narrow deny", func(t *testing.T) {
		cell, ok := buildRolePermissionCell(
			PermMessageReadInteractions,
			scope,
			[]Permission{PermMessageRead},
			[]Permission{PermMessageReadInteractions},
			nil, nil, nil, nil, nil,
		)
		if !ok {
			t.Fatal("narrow permission did not apply at server scope")
		}
		if cell.Override != MatrixDecisionDeny || cell.Effective != MatrixDecisionAllow {
			t.Fatalf("cell = %+v, want deny override and allow effective decision", cell)
		}
	})

	t.Run("narrow allow is independent of broad deny", func(t *testing.T) {
		cell, ok := buildRolePermissionCell(
			PermMessageReadInteractions,
			scope,
			[]Permission{PermMessageReadInteractions},
			[]Permission{PermMessageRead},
			nil, nil, nil, nil, nil,
		)
		if !ok {
			t.Fatal("narrow permission did not apply at server scope")
		}
		if cell.Override != MatrixDecisionAllow || cell.Effective != MatrixDecisionAllow {
			t.Fatalf("cell = %+v, want allow override and effective decision", cell)
		}
	})
}

func TestRolePermissionMatrixAppliesExplicitInclusion(t *testing.T) {
	broad, narrow := installTestPermissionInclusion(t)
	cell, ok := buildRolePermissionCell(
		narrow,
		PermissionMatrixScope{ID: "server", Kind: MatrixScopeServer},
		[]Permission{broad},
		[]Permission{narrow},
		nil, nil, nil, nil, nil,
	)
	if !ok {
		t.Fatal("narrow permission did not apply at server scope")
	}
	if cell.Override != MatrixDecisionDeny || cell.Effective != MatrixDecisionAllow {
		t.Fatalf("cell = %+v, want deny override and included allow", cell)
	}
}
