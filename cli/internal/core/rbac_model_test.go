package core

import (
	"errors"
	"reflect"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNewRBACModelWiresDependencies(t *testing.T) {
	projection := NewRBACProjection()
	rbac := detachedTestProjectionHandle(projection)

	service := newRBACModel(rbac)

	if service.rbac.Projection() != projection {
		t.Fatal("RBAC projection was not wired")
	}
	if service.rbac.Projector() != rbac.Projector() {
		t.Fatal("RBAC projector was not wired")
	}
}

func TestRBACModelWaitForRejectsUnconsumedSubject(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewRBACProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := newTestRBACModel(t, projection, projector)
	ctx := testContext(t)

	event := newEvent(SystemActorID, roomCreatedEvent("R-not-rbac", "not-rbac", "", corev1.RoomKind_ROOM_KIND_CHANNEL))
	subject := events.RoomAggregate("R-not-rbac").SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}

	err = service.waitFor(ctx, events.SubjectPosition(subject, seq))
	if !errors.Is(err, events.ErrProjectionSubjectNotConsumed) {
		t.Fatalf("waitFor error = %v, want ErrProjectionSubjectNotConsumed", err)
	}
}

func TestRBACModelWaitForProjectsRoleCreation(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewRBACProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := newTestRBACModel(t, projection, projector)
	ctx := testContext(t)

	event := newEvent(SystemActorID, &corev1.Event{
		Event: &corev1.Event_RbacRoleCreated{
			RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
				RoleName:    "moderator",
				DisplayName: "Moderator",
				Description: "Keeps rooms tidy",
				Rank:        PositionCustomFirst,
			},
		},
	})
	subject := events.RBACAggregate().SubjectFor(event)
	seq, err := harness.publisher.AppendEventually(ctx, subject, event)
	if err != nil {
		t.Fatalf("AppendEventually returned error: %v", err)
	}
	if err := service.waitFor(ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("waitFor returned error: %v", err)
	}

	role, ok := service.role("moderator")
	if !ok {
		t.Fatal("RBAC model did not contain appended role")
	}
	if role.GetDisplayName() != "Moderator" {
		t.Fatalf("role display name = %q, want %q", role.GetDisplayName(), "Moderator")
	}
}

func TestRBACModelOwnsProjectionReads(t *testing.T) {
	projection := NewRBACProjection()
	model := newTestRBACModel(t, projection, nil)

	for _, event := range []*corev1.Event{
		{Event: &corev1.Event_RbacRoleCreated{RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
			RoleName: "beta", DisplayName: "Beta", Rank: PositionCustomFirst + 1,
		}}},
		{Event: &corev1.Event_RbacRoleCreated{RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
			RoleName: "alpha", DisplayName: "Alpha", Rank: PositionCustomFirst,
		}}},
		{Event: &corev1.Event_RbacRoleAssigned{RbacRoleAssigned: &corev1.RbacRoleAssignedEvent{
			UserId: "U2", RoleName: "alpha",
		}}},
		{Event: &corev1.Event_RbacRoleAssigned{RbacRoleAssigned: &corev1.RbacRoleAssignedEvent{
			UserId: "U1", RoleName: "alpha",
		}}},
		{Event: &corev1.Event_RbacPermissionGranted{RbacPermissionGranted: rbacRolePermissionGrantedEvent(
			ScopeServer, "", "alpha", PermMessagePost,
		)}},
		{Event: &corev1.Event_RbacPermissionDenied{RbacPermissionDenied: rbacRolePermissionDeniedEvent(
			ScopeRoom, "R1", "alpha", PermRoomJoin,
		)}},
	} {
		applyRBACProjectionEvent(t, projection, event)
	}

	role, ok := model.role("alpha")
	if !ok || role.GetDisplayName() != "Alpha" {
		t.Fatalf("role(alpha) = %+v, %v; want Alpha, true", role, ok)
	}
	role.DisplayName = "mutated"
	roleAgain, _ := model.role("alpha")
	if roleAgain.GetDisplayName() != "Alpha" {
		t.Fatal("role returned projection-owned mutable state")
	}
	if !model.roleExists("beta") || model.roleExists("missing") {
		t.Fatal("roleExists did not preserve projection existence semantics")
	}

	roles := model.roles()
	if got := []string{roles[0].GetName(), roles[1].GetName()}; !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("roles = %v, want [alpha beta]", got)
	}
	if got := model.userRoles("U1"); !reflect.DeepEqual(got, []string{"alpha"}) {
		t.Fatalf("userRoles(U1) = %v, want [alpha]", got)
	}
	if !model.hasRole("U1", "alpha") || model.hasRole("U1", "beta") {
		t.Fatal("hasRole did not preserve assignment semantics")
	}
	if got := model.roleUsers("alpha"); !reflect.DeepEqual(got, []string{"U1", "U2"}) {
		t.Fatalf("roleUsers(alpha) = %v, want [U1 U2]", got)
	}

	wantDecisions := []ScopedRolePermissionDecision{
		{Scope: ScopeRoom, ScopeID: "R1", Permission: PermRoomJoin, Decision: DecisionDeny},
		{Scope: ScopeServer, Permission: PermMessagePost, Decision: DecisionAllow},
	}
	if got := model.rolePermissionDecisions("alpha"); !reflect.DeepEqual(got, wantDecisions) {
		t.Fatalf("rolePermissionDecisions(alpha) = %#v, want %#v", got, wantDecisions)
	}
	if got := model.decision(ScopeRoom, "R1", "alpha", PermRoomJoin); got != DecisionDeny {
		t.Fatalf("room decision = %s, want deny", got)
	}
	if got := model.decision(ScopeRoom, "R1", "alpha", PermMessagePost); got != DecisionNone {
		t.Fatalf("missing room decision = %s, want none", got)
	}
	if grants, denials := model.decisionsFor(ScopeRoom, "R1", "alpha"); !reflect.DeepEqual(grants, []Permission(nil)) || !reflect.DeepEqual(denials, []Permission{PermRoomJoin}) {
		t.Fatalf("room decisions = grants %v, denials %v", grants, denials)
	}
	if grants, denials := model.decisionsForRoleServer("alpha"); !reflect.DeepEqual(grants, []Permission{PermMessagePost}) || !reflect.DeepEqual(denials, []Permission(nil)) {
		t.Fatalf("server decisions = grants %v, denials %v", grants, denials)
	}
	if got, want := model.nextAvailablePosition(), PositionCustomFirst+2; got != want {
		t.Fatalf("nextAvailablePosition = %d, want %d", got, want)
	}
}
