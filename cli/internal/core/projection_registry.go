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
