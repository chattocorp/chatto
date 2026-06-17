package graph

import (
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomMembersReturnsTombstonesForMissingUsers(t *testing.T) {
	env := setupTestResolver(t)

	missingUserID := "UmissingMember"
	event := &corev1.Event{
		Id:        core.NewEventID(),
		CreatedAt: timestamppb.Now(),
		ActorId:   missingUserID,
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{
				RoomId: env.testRoom.Id,
			},
		},
	}
	subject := events.RoomAggregate(env.testRoom.Id).Subject(events.EventUserJoinedRoom)
	seq, err := env.core.EventPublisher.AppendEventually(env.ctx, subject, event)
	if err != nil {
		t.Fatalf("append stale membership event: %v", err)
	}
	if err := env.core.RoomDirectoryProjector.WaitFor(env.ctx, events.SubjectPosition(subject, seq)); err != nil {
		t.Fatalf("wait for room directory projection: %v", err)
	}

	members, err := env.resolver.Room().Members(env.authContext(), env.testRoom, nil, nil)
	if err != nil {
		t.Fatalf("Room.members returned error for stale member: %v", err)
	}
	if members.TotalCount != 2 {
		t.Fatalf("members.TotalCount = %d, want 2", members.TotalCount)
	}
	if len(members.Users) != 2 {
		t.Fatalf("len(members.Users) = %d, want 2: %#v", len(members.Users), members.Users)
	}

	var tombstone *corev1.User
	for _, user := range members.Users {
		if user.Id == missingUserID {
			tombstone = user
		}
	}
	if tombstone == nil {
		t.Fatalf("members.Users = %#v, want tombstone for %s", members.Users, missingUserID)
	}
	if !tombstone.Deleted {
		t.Fatalf("tombstone.Deleted = false, want true")
	}
	if tombstone.DisplayName != core.DeletedUserDisplayName {
		t.Fatalf("tombstone.DisplayName = %q, want %q", tombstone.DisplayName, core.DeletedUserDisplayName)
	}
	if tombstone.Login != "" {
		t.Fatalf("tombstone.Login = %q, want empty", tombstone.Login)
	}
}
