package model

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ServerEvent is the GraphQL wrapper for any event delivered through the
// `myServerEvents` subscription. The `Id`, `ActorId`, and `CreatedAt` fields
// are denormalised from whichever proto envelope arrived so gqlgen can bind
// the top-level fields directly; field-level resolvers for `actor` and
// `event` dispatch on RoomProto / LiveProto (exactly one of which is set).
type ServerEvent struct {
	Id        string
	ActorId   string
	CreatedAt *timestamppb.Timestamp

	// RoomProto is set for room-scoped events sourced from
	// core.StreamMyServerEvents.
	RoomProto *corev1.ServerEvent
	// LiveProto is set for deployment-scoped events sourced from
	// core.StreamMyLiveEvents.
	LiveProto *corev1.LiveEvent
}

// Payload returns the underlying proto oneof for whichever wrapper carries
// the event (RoomProto.Event or LiveProto.Event). Useful for tests that
// inspect the concrete event type with a `%T` log line or type switch.
func (e *ServerEvent) Payload() any {
	if e.RoomProto != nil {
		return e.RoomProto.Event
	}
	if e.LiveProto != nil {
		return e.LiveProto.Event
	}
	return nil
}
