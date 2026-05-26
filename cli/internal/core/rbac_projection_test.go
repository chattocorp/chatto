package core

import (
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRBACProjection_RoleMetadataAndReorder(t *testing.T) {
	p := NewRBACProjection()

	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleCreated{
		RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
			RoleName:    "alpha",
			DisplayName: "Alpha",
			Description: "First",
			Rank:        10,
		},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleCreated{
		RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
			RoleName:    "beta",
			DisplayName: "Beta",
			Description: "Second",
			Rank:        20,
		},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleDisplayNameChanged{
		RbacRoleDisplayNameChanged: &corev1.RbacRoleDisplayNameChangedEvent{
			RoleName:    "alpha",
			DisplayName: "Alpha Prime",
		},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleDescriptionChanged{
		RbacRoleDescriptionChanged: &corev1.RbacRoleDescriptionChangedEvent{
			RoleName:    "alpha",
			Description: "Renamed first",
		},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRolesReordered{
		RbacRolesReordered: &corev1.RbacRolesReorderedEvent{
			RoleNames: []string{"beta", "alpha"},
		},
	}})

	alpha, ok := p.GetRole("alpha")
	if !ok {
		t.Fatal("alpha role missing")
	}
	if alpha.GetDisplayName() != "Alpha Prime" {
		t.Fatalf("alpha display name = %q, want Alpha Prime", alpha.GetDisplayName())
	}
	if alpha.GetDescription() != "Renamed first" {
		t.Fatalf("alpha description = %q, want Renamed first", alpha.GetDescription())
	}
	if alpha.GetPosition() != PositionCustomFirst+1 {
		t.Fatalf("alpha position = %d, want %d", alpha.GetPosition(), PositionCustomFirst+1)
	}

	beta, ok := p.GetRole("beta")
	if !ok {
		t.Fatal("beta role missing")
	}
	if beta.GetPosition() != PositionCustomFirst {
		t.Fatalf("beta position = %d, want %d", beta.GetPosition(), PositionCustomFirst)
	}
}

func TestRBACProjection_AssignRevokeAndDeleteRole(t *testing.T) {
	p := NewRBACProjection()

	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleCreated{
		RbacRoleCreated: &corev1.RbacRoleCreatedEvent{RoleName: "editor", Rank: PositionCustomFirst},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleAssigned{
		RbacRoleAssigned: &corev1.RbacRoleAssignedEvent{UserId: "U123", RoleName: "editor"},
	}})

	if !p.HasRole("U123", "editor") {
		t.Fatal("expected assigned role")
	}

	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleRevoked{
		RbacRoleRevoked: &corev1.RbacRoleRevokedEvent{UserId: "U123", RoleName: "editor"},
	}})
	if p.HasRole("U123", "editor") {
		t.Fatal("expected revoked role")
	}

	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleAssigned{
		RbacRoleAssigned: &corev1.RbacRoleAssignedEvent{UserId: "U123", RoleName: "editor"},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: &corev1.RbacPermissionGrantedEvent{
			Location:   string(ScopeServer),
			Subject:    "editor",
			Permission: string(PermMessagePost),
		},
	}})

	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRoleDeleted{
		RbacRoleDeleted: &corev1.RbacRoleDeletedEvent{RoleName: "editor"},
	}})
	if p.RoleExists("editor") {
		t.Fatal("expected deleted role")
	}
	if p.HasRole("U123", "editor") {
		t.Fatal("expected role assignment removed after role delete")
	}
	if got := p.GetDecision(ScopeServer, "", "editor", PermMessagePost); got != DecisionNone {
		t.Fatalf("deleted role decision = %v, want DecisionNone", got)
	}
}

func TestRBACProjection_PermissionLocations(t *testing.T) {
	p := NewRBACProjection()

	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: &corev1.RbacPermissionGrantedEvent{
			Location:   string(ScopeServer),
			Subject:    "admin",
			Permission: string(PermMessagePost),
		},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: &corev1.RbacPermissionDeniedEvent{
			Location:   "Rabc123",
			Subject:    "U123",
			Permission: string(PermMessagePost),
		},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: &corev1.RbacPermissionGrantedEvent{
			Location:   "Gabc123",
			Subject:    "moderator",
			Permission: string(PermRoomJoin),
		},
	}})

	if got := p.GetDecision(ScopeServer, "", "admin", PermMessagePost); got != DecisionAllow {
		t.Fatalf("server decision = %v, want DecisionAllow", got)
	}
	if got := p.GetDecision(ScopeRoom, "Rabc123", "U123", PermMessagePost); got != DecisionDeny {
		t.Fatalf("room decision = %v, want DecisionDeny", got)
	}
	if got := p.GetDecision(ScopeGroup, "Gabc123", "moderator", PermRoomJoin); got != DecisionAllow {
		t.Fatalf("group decision = %v, want DecisionAllow", got)
	}

	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: &corev1.RbacPermissionClearedEvent{
			Location:   "Rabc123",
			Subject:    "U123",
			Permission: string(PermMessagePost),
		},
	}})
	if got := p.GetDecision(ScopeRoom, "Rabc123", "U123", PermMessagePost); got != DecisionNone {
		t.Fatalf("cleared room decision = %v, want DecisionNone", got)
	}
}

func TestRBACProjection_IgnoresDuplicateEventID(t *testing.T) {
	p := NewRBACProjection()

	applyRBACProjectionEvent(t, p, &corev1.Event{Id: "evt-1", Event: &corev1.Event_RbacRoleCreated{
		RbacRoleCreated: &corev1.RbacRoleCreatedEvent{RoleName: "alpha", DisplayName: "Alpha", Rank: 1},
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Id: "evt-1", Event: &corev1.Event_RbacRoleDisplayNameChanged{
		RbacRoleDisplayNameChanged: &corev1.RbacRoleDisplayNameChangedEvent{RoleName: "alpha", DisplayName: "Changed"},
	}})

	role, ok := p.GetRole("alpha")
	if !ok {
		t.Fatal("alpha role missing")
	}
	if role.GetDisplayName() != "Alpha" {
		t.Fatalf("display name after duplicate event = %q, want Alpha", role.GetDisplayName())
	}
}

func applyRBACProjectionEvent(t *testing.T, p *RBACProjection, event *corev1.Event) {
	t.Helper()
	if err := p.Apply(event, 0); err != nil {
		t.Fatalf("apply event: %v", err)
	}
}
