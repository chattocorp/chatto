package core

import (
	"context"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/parallel"
	"github.com/chattocorp/chatto/pkg/events"
)

type projectionRegistration struct {
	key             string
	name            string
	projector       *events.Projector
	subjects        []string
	snapshotPolicy  projectionSnapshotPolicy
	snapshotEnabled bool
	snapshotOutcome *snapshotRestoreObservation
	// componentSnapshots is true when the projection snapshots as several
	// components. Its snapshots then use cohort storage.
	componentSnapshots bool
	streamName         string
	identityResolver   events.StreamIdentityResolver
	estimate           func() (entries int64, estimatedBytes int64, metrics []ProjectionAdminMetric)
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

func waitForProjectionSubjectsCurrent(ctx context.Context, publisher *evtstream.Publisher, name string, projector *events.Projector, subjects ...string) error {
	target, err := lastSubjectsPosition(ctx, publisher, subjects)
	if err != nil {
		return fmt.Errorf("read %s projection target seq: %w", name, err)
	}
	if target.IsZero() {
		return nil
	}
	if err := projector.WaitFor(ctx, target); err != nil {
		return fmt.Errorf("wait for %s projection: %w", name, err)
	}
	return nil
}

// lastSubjectsPosition returns the newest position among subjects, or the
// zero position when no subject has a message. Each subject is one request to
// the stream leader. The requests run concurrently, so the wait is one round
// trip and not one for each subject.
func lastSubjectsPosition(ctx context.Context, publisher *evtstream.Publisher, subjects []string) (events.StreamPosition, error) {
	positions, err := parallel.Map(ctx, 0, subjects, func(ctx context.Context, _ int, subject string) (events.StreamPosition, error) {
		return publisher.LastSubjectPosition(ctx, subject)
	})
	if err != nil {
		return events.StreamPosition{}, err
	}
	return newestStreamPosition(positions), nil
}

// newestStreamPosition returns the position with the highest sequence, or the
// zero position when positions has none.
func newestStreamPosition(positions []events.StreamPosition) events.StreamPosition {
	var newest events.StreamPosition
	for _, pos := range positions {
		if pos.Seq > newest.Seq {
			newest = pos
		}
	}
	return newest
}
