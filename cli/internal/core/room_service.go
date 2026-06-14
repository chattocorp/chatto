package core

import (
	"context"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomService owns the room-derived projections and their projectors.
//
// ChattoCore is still the compatibility facade for most room APIs, but room
// write paths should use this type for projection readiness instead of naming
// individual projector fields. That keeps the "which projections must catch
// up?" knowledge with the room read models.
type RoomService struct {
	directory          *RoomDirectoryProjection
	directoryProjector *events.Projector

	groupLayout          *RoomGroupLayoutProjection
	groupLayoutProjector *events.Projector

	timeline          *RoomTimelineProjection
	timelineProjector *events.Projector

	threads          *ThreadProjection
	threadsProjector *events.Projector

	reactions          *ReactionProjection
	reactionsProjector *events.Projector
}

func newRoomService(
	directory *RoomDirectoryProjection,
	directoryProjector *events.Projector,
	groupLayout *RoomGroupLayoutProjection,
	groupLayoutProjector *events.Projector,
	timeline *RoomTimelineProjection,
	timelineProjector *events.Projector,
	threads *ThreadProjection,
	threadsProjector *events.Projector,
	reactions *ReactionProjection,
	reactionsProjector *events.Projector,
) *RoomService {
	return &RoomService{
		directory:            directory,
		directoryProjector:   directoryProjector,
		groupLayout:          groupLayout,
		groupLayoutProjector: groupLayoutProjector,
		timeline:             timeline,
		timelineProjector:    timelineProjector,
		threads:              threads,
		threadsProjector:     threadsProjector,
		reactions:            reactions,
		reactionsProjector:   reactionsProjector,
	}
}

func (m *RoomService) waitForDirectory(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room directory", m.directoryProjector))
}

func (m *RoomService) waitForGroupLayout(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room group layout", m.groupLayoutProjector))
}

func (m *RoomService) waitForTimeline(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("room timeline", m.timelineProjector))
}

func (m *RoomService) waitForThreads(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("threads", m.threadsProjector))
}

func (m *RoomService) waitForReactions(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("reactions", m.reactionsProjector))
}

func (m *RoomService) waitForReactionsCurrent(ctx context.Context, publisher *events.Publisher, roomID string) error {
	agg := events.RoomAggregate(roomID)
	return waitForProjectionSubjectsCurrent(ctx, publisher, "reactions", m.reactionsProjector,
		agg.Subject(events.EventReactionAdded),
		agg.Subject(events.EventReactionRemoved),
	)
}

func (m *RoomService) waitForDirectoryAndTimeline(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos,
		waitForProjection("room directory", m.directoryProjector),
		waitForProjection("room timeline", m.timelineProjector),
	)
}

func (m *RoomService) waitForTimelineAndThreads(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos,
		waitForProjection("room timeline", m.timelineProjector),
		waitForProjection("threads", m.threadsProjector),
	)
}

func (m *RoomService) waitForMyEventsReplayTail(ctx context.Context, pub *events.Publisher, pos events.StreamPosition) error {
	if err := waitForPositionAll(ctx, pos,
		waitForProjection("room timeline", m.timelineProjector),
		waitForProjection("threads", m.threadsProjector),
		waitForProjection("room directory", m.directoryProjector),
	); err != nil {
		return err
	}
	return waitForProjectionSubjectsCurrent(ctx, pub, "reactions", m.reactionsProjector,
		events.RoomEventTypeFilter(events.EventReactionAdded),
		events.RoomEventTypeFilter(events.EventReactionRemoved),
	)
}

func (m *RoomService) waitForMyEventsReplayCurrent(ctx context.Context) error {
	return waitForCurrentAll(ctx,
		waitForProjection("room timeline", m.timelineProjector),
		waitForProjection("threads", m.threadsProjector),
		waitForProjection("room directory", m.directoryProjector),
		waitForProjection("reactions", m.reactionsProjector),
	)
}

func (m *RoomService) appendDirectoryEventually(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.AppendEventually(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForDirectory(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (m *RoomService) appendGroupLayout(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.Append(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForGroupLayout(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (m *RoomService) appendGroupLayoutEventually(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.AppendEventually(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForGroupLayout(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}

func (m *RoomService) appendTimelineEventually(ctx context.Context, pub *events.Publisher, agg events.Aggregate, event *corev1.Event) (events.StreamPosition, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.AppendEventually(ctx, subject, event)
	if err != nil {
		return events.StreamPosition{}, err
	}
	pos := events.SubjectPosition(subject, seq)
	if err := m.waitForTimeline(ctx, pos); err != nil {
		return pos, err
	}
	return pos, nil
}
