package http_server

import (
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"testing"
)

func TestRealtimeRoleEventsDoNotExposePrivatePermissionDecisions(t *testing.T) {
	source := &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{RbacPermissionDenied: &evtv1.RbacPermissionDeniedEvent{
		Permission: "message.read",
		Scope:      &evtv1.RbacPermissionScope{Kind: evtv1.RbacPermissionScopeKind_RBAC_PERMISSION_SCOPE_KIND_ROOM, Id: "private-room"},
		Subject:    &evtv1.RbacPermissionSubject{Kind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, Id: "alice"},
	}}}
	original := proto.Clone(source)
	if got := projectRealtimeEvent("bob", source); got != nil {
		t.Fatalf("another user's direct decision was disclosed: %v", got)
	}
	if got := projectRealtimeEvent("alice", source); got.GetViewerPermissionsChanged() == nil {
		t.Fatalf("missing own permission event: %v", got)
	}
	if !proto.Equal(source, original) {
		t.Fatal("mapping changed stored fact")
	}
	source.GetRbacPermissionDenied().Subject = &evtv1.RbacPermissionSubject{Kind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE, Id: "helper"}
	got := projectRealtimeEvent("bob", source)
	if got.GetRolePermissionsChanged().GetRoleName() != "helper" {
		t.Fatalf("missing role subject: %v", got)
	}
	if got.GetRolePermissionsChanged().ProtoReflect().Descriptor().Fields().Len() != 1 {
		t.Fatal("role event exposes private decision fields")
	}
}

func TestRealtimeAssignmentEventsRemainVisibleToOtherViewers(t *testing.T) {
	source := &evtv1.Event{Event: &evtv1.Event_RbacRoleAssigned{RbacRoleAssigned: &evtv1.RbacRoleAssignedEvent{UserId: "alice", RoleName: "helper"}}}
	for _, viewer := range []string{"alice", "bob"} {
		got := projectRealtimeEvent(viewer, source)
		if got.GetRoleAssigned().GetUserId() != "alice" || got.GetRoleAssigned().GetRoleName() != "helper" {
			t.Fatalf("missing member-list update for %s: %v", viewer, got)
		}
	}
}

func TestRealtimeBotReceivesOwnerPermissionBoundaryWithoutPrivateDetails(t *testing.T) {
	env := setupWebSocketTestServer(t)
	owner, err := env.core.CreateUser(env.ctx, core.SystemActorID, "bot-owner-events", "Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	bot, err := env.core.CreateBot(env.ctx, owner.Id, "event_bot", "Event Bot")
	if err != nil {
		t.Fatal(err)
	}
	source := &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{RbacPermissionDenied: &evtv1.RbacPermissionDeniedEvent{
		Subject: &evtv1.RbacPermissionSubject{Kind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER, Id: owner.Id},
	}}}
	got, err := env.httpServer.projectViewerRealtimeEvent(env.ctx, bot.User.Id, source)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetViewerPermissionsChanged() == nil {
		t.Fatalf("missing bot owner boundary: %v", got)
	}
	source = &evtv1.Event{Event: &evtv1.Event_RbacRoleDisplayNameChanged{RbacRoleDisplayNameChanged: &evtv1.RbacRoleDisplayNameChangedEvent{RoleName: "helper", DisplayName: "New"}}}
	got, err = env.httpServer.projectViewerRealtimeEvent(env.ctx, bot.User.Id, source)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetRoleUpdated() == nil {
		t.Fatalf("cosmetic update unnecessarily resets bot: %v", got)
	}
}
