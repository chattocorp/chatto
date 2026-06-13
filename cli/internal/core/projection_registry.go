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

func waitForPositionAll(ctx context.Context, pos events.StreamPosition, targets ...projectionWaitTarget) error {
	for _, target := range targets {
		if err := target.projector.WaitFor(ctx, pos); err != nil {
			return fmt.Errorf("wait for %s projection: %w", target.name, err)
		}
	}
	return nil
}

func waitForCurrentAll(ctx context.Context, targets ...projectionWaitTarget) error {
	for _, target := range targets {
		if err := target.projector.WaitForCurrent(ctx); err != nil {
			return fmt.Errorf("wait for %s projection: %w", target.name, err)
		}
	}
	return nil
}

func waitForProjectionSubjectsCurrent(ctx context.Context, publisher *events.Publisher, name string, projector *events.Projector, subjects ...string) error {
	var target events.StreamPosition
	for _, subject := range subjects {
		pos, err := publisher.LastSubjectPosition(ctx, subject)
		if err != nil {
			return fmt.Errorf("read %s projection target seq: %w", name, err)
		}
		if pos.Seq > target.Seq {
			target = pos
		}
	}
	if target.IsZero() {
		return nil
	}
	if err := projector.WaitFor(ctx, target); err != nil {
		return fmt.Errorf("wait for %s projection: %w", name, err)
	}
	return nil
}
