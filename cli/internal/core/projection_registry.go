package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/events"
)

type projectionRegistration struct {
	name      string
	projector *events.Projector
	estimate  func() (entries int64, estimatedBytes int64, metrics []ProjectionAdminMetric)
}

type projectionWaitTarget struct {
	name      string
	projector *events.Projector
}

func waitForProjection(name string, projector *events.Projector) projectionWaitTarget {
	return projectionWaitTarget{name: name, projector: projector}
}

func waitForSeqAll(ctx context.Context, seq uint64, targets ...projectionWaitTarget) error {
	for _, target := range targets {
		if err := target.projector.WaitForSeq(ctx, seq); err != nil {
			return fmt.Errorf("wait for %s projection: %w", target.name, err)
		}
	}
	return nil
}

func (c *ChattoCore) waitForProjectionSubjectsCurrent(ctx context.Context, name string, projector *events.Projector, subjects ...string) error {
	var target uint64
	for _, subject := range subjects {
		seq, err := c.EventPublisher.LastSubjectSeq(ctx, subject)
		if err != nil {
			return fmt.Errorf("read %s projection target seq: %w", name, err)
		}
		if seq > target {
			target = seq
		}
	}
	if target == 0 {
		return nil
	}
	if err := projector.WaitForSeq(ctx, target); err != nil {
		return fmt.Errorf("wait for %s projection: %w", name, err)
	}
	return nil
}

func (c *ChattoCore) waitForUserContentKeysCurrent(ctx context.Context, userID string) error {
	agg := events.UserAggregate(userID)
	return c.waitForProjectionSubjectsCurrent(ctx, "content key", c.ContentKeysProjector,
		agg.Subject(events.EventUserDEKGenerated),
		agg.Subject(events.EventUserKeyShredded),
	)
}

func (c *ChattoCore) waitForRoomReactionsCurrent(ctx context.Context, roomID string) error {
	agg := events.RoomAggregate(roomID)
	return c.waitForProjectionSubjectsCurrent(ctx, "reactions", c.ReactionsProjector,
		agg.Subject(events.EventReactionAdded),
		agg.Subject(events.EventReactionRemoved),
	)
}
