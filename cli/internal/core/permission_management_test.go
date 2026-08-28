package core

import "testing"

func TestRolePermissionMatrixAppliesMessageReadInclusion(t *testing.T) {
	scope := PermissionMatrixScope{ID: "server", Kind: MatrixScopeServer}

	t.Run("parent allow wins over child deny", func(t *testing.T) {
		cell, ok := buildRolePermissionCell(
			PermMessageReadInteractions,
			scope,
			[]Permission{PermMessageRead},
			[]Permission{PermMessageReadInteractions},
			nil, nil, nil, nil, nil,
		)
		if !ok {
			t.Fatal("child permission did not apply at server scope")
		}
		if cell.Override != MatrixDecisionDeny || cell.Effective != MatrixDecisionAllow {
			t.Fatalf("cell = %+v, want deny override and allow effective decision", cell)
		}
	})

	t.Run("child allow is independent of parent deny", func(t *testing.T) {
		cell, ok := buildRolePermissionCell(
			PermMessageReadInteractions,
			scope,
			[]Permission{PermMessageReadInteractions},
			[]Permission{PermMessageRead},
			nil, nil, nil, nil, nil,
		)
		if !ok {
			t.Fatal("child permission did not apply at server scope")
		}
		if cell.Override != MatrixDecisionAllow || cell.Effective != MatrixDecisionAllow {
			t.Fatalf("cell = %+v, want allow override and effective decision", cell)
		}
	})
}

func TestRolePermissionMatrixAppliesTransitiveInclusion(t *testing.T) {
	broad, _, narrow := installTestPermissionChain(t)
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
		t.Fatalf("cell = %+v, want deny override and transitive allow", cell)
	}
}
