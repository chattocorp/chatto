package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNewRBACServiceWiresDependencies(t *testing.T) {
	projection := NewRBACProjection()
	projector := testEventProjector(t)

	service := newRBACService(projection, projector)

	if service.projection != projection {
		t.Fatal("RBAC projection was not wired")
	}
	if service.projector != projector {
		t.Fatal("RBAC projector was not wired")
	}
}

func TestRBACServiceWaitForRejectsUnconsumedSubject(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewRBACProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := newRBACService(projection, projector)
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

func TestRBACServiceWaitForProjectsRoleCreation(t *testing.T) {
	harness := newTestEventHarness(t)
	projection := NewRBACProjection()
	projector := harness.projector(projection)
	startTestProjector(t, projector)
	service := newRBACService(projection, projector)
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

	role, ok := projection.GetRole("moderator")
	if !ok {
		t.Fatal("RBAC projection did not contain appended role")
	}
	if role.GetDisplayName() != "Moderator" {
		t.Fatalf("role display name = %q, want %q", role.GetDisplayName(), "Moderator")
	}
}
