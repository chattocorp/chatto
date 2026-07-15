package core

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

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
			Pingable:    true,
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
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacRolePingableChanged{
		RbacRolePingableChanged: &corev1.RbacRolePingableChangedEvent{
			RoleName: "alpha",
			Pingable: false,
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
	if alpha.GetPingable() {
		t.Fatal("alpha pingable = true, want false")
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
		RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", "editor", PermMessagePost),
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
		RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", "admin", PermMessagePost),
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacUserPermissionDeniedEvent(ScopeRoom, "Rabc123", "U123", PermMessagePost),
	}})
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeGroup, "Gabc123", "moderator", PermRoomJoin),
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
		RbacPermissionCleared: rbacUserPermissionClearedEvent(ScopeRoom, "Rabc123", "U123", PermMessagePost),
	}})
	if got := p.GetDecision(ScopeRoom, "Rabc123", "U123", PermMessagePost); got != DecisionNone {
		t.Fatalf("cleared room decision = %v, want DecisionNone", got)
	}
}

func TestRBACProjection_LegacyPermissionDecisionUnknownFields(t *testing.T) {
	p := NewRBACProjection()

	granted := &corev1.RbacPermissionGrantedEvent{Permission: string(PermMessagePost)}
	granted.ProtoReflect().SetUnknown(legacyRBACPermissionUnknown("server", "admin"))
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: granted,
	}})
	if got := p.GetDecision(ScopeServer, "", "admin", PermMessagePost); got != DecisionAllow {
		t.Fatalf("legacy server decision = %v, want DecisionAllow", got)
	}

	denied := &corev1.RbacPermissionDeniedEvent{Permission: string(PermRoomJoin)}
	denied.ProtoReflect().SetUnknown(legacyRBACPermissionUnknown("Gabc123", "moderator"))
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: denied,
	}})
	if got := p.GetDecision(ScopeGroup, "Gabc123", "moderator", PermRoomJoin); got != DecisionDeny {
		t.Fatalf("legacy group decision = %v, want DecisionDeny", got)
	}

	cleared := &corev1.RbacPermissionClearedEvent{Permission: string(PermRoomJoin)}
	cleared.ProtoReflect().SetUnknown(legacyRBACPermissionUnknown("Gabc123", "moderator"))
	applyRBACProjectionEvent(t, p, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: cleared,
	}})
	if got := p.GetDecision(ScopeGroup, "Gabc123", "moderator", PermRoomJoin); got != DecisionNone {
		t.Fatalf("legacy cleared decision = %v, want DecisionNone", got)
	}
}

func TestRBACProjection_LegacyPermissionDecisionWireBytes(t *testing.T) {
	p := NewRBACProjection()

	granted := unmarshalLegacyRBACPermissionEvent(t, 810, "server", "admin", string(PermMessagePost))
	applyRBACProjectionEvent(t, p, granted)
	if got := p.GetDecision(ScopeServer, "", "admin", PermMessagePost); got != DecisionAllow {
		t.Fatalf("legacy wire server decision = %v, want DecisionAllow", got)
	}

	denied := unmarshalLegacyRBACPermissionEvent(t, 811, "Gabc123", "moderator", string(PermRoomJoin))
	applyRBACProjectionEvent(t, p, denied)
	if got := p.GetDecision(ScopeGroup, "Gabc123", "moderator", PermRoomJoin); got != DecisionDeny {
		t.Fatalf("legacy wire group decision = %v, want DecisionDeny", got)
	}

	cleared := unmarshalLegacyRBACPermissionEvent(t, 812, "Gabc123", "moderator", string(PermRoomJoin))
	applyRBACProjectionEvent(t, p, cleared)
	if got := p.GetDecision(ScopeGroup, "Gabc123", "moderator", PermRoomJoin); got != DecisionNone {
		t.Fatalf("legacy wire cleared decision = %v, want DecisionNone", got)
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

func legacyRBACPermissionUnknown(location, subject string) []byte {
	var unknown []byte
	unknown = protowire.AppendTag(unknown, 1, protowire.BytesType)
	unknown = protowire.AppendString(unknown, location)
	unknown = protowire.AppendTag(unknown, 2, protowire.BytesType)
	unknown = protowire.AppendString(unknown, subject)
	return unknown
}

func unmarshalLegacyRBACPermissionEvent(t *testing.T, eventField protowire.Number, location, subject, permission string) *corev1.Event {
	t.Helper()
	var payload []byte
	payload = append(payload, legacyRBACPermissionUnknown(location, subject)...)
	payload = protowire.AppendTag(payload, 3, protowire.BytesType)
	payload = protowire.AppendString(payload, permission)

	var encoded []byte
	encoded = protowire.AppendTag(encoded, eventField, protowire.BytesType)
	encoded = protowire.AppendBytes(encoded, payload)

	var event corev1.Event
	if err := proto.Unmarshal(encoded, &event); err != nil {
		t.Fatalf("unmarshal legacy RBAC permission event: %v", err)
	}
	return &event
}

func applyRBACProjectionEvent(t *testing.T, p *RBACProjection, event *corev1.Event) {
	t.Helper()
	if err := p.Apply(event, 0); err != nil {
		t.Fatalf("apply event: %v", err)
	}
}
