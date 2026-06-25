package core

import (
	"context"

	"hmans.de/chatto/internal/events"
)

// RBACModel owns the RBAC projection and its readiness barrier.
type RBACModel struct {
	projection *RBACProjection
	projector  *events.Projector
}

func newRBACModel(projection *RBACProjection, projector *events.Projector) *RBACModel {
	return &RBACModel{projection: projection, projector: projector}
}

func (m *RBACModel) waitFor(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("RBAC", m.projector))
}
