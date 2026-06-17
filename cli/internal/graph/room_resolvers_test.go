package graph

import (
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomMembersSkipsMissingUsers(t *testing.T) {
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
	if members.TotalCount != 1 {
		t.Fatalf("members.TotalCount = %d, want 1", members.TotalCount)
	}
	if len(members.Users) != 1 || members.Users[0].Id != env.testUser.Id {
		t.Fatalf("members.Users = %#v, want only %s", members.Users, env.testUser.Id)
	}
}
