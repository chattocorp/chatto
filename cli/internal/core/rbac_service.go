package core

import (
	"context"

	"hmans.de/chatto/internal/events"
)

// RBACService owns the RBAC projection and its readiness barrier.
type RBACService struct {
	projection *RBACProjection
	projector  *events.Projector
}

func newRBACService(projection *RBACProjection, projector *events.Projector) *RBACService {
	return &RBACService{projection: projection, projector: projector}
}

func (m *RBACService) waitFor(ctx context.Context, pos events.StreamPosition) error {
	return waitForPositionAll(ctx, pos, waitForProjection("RBAC", m.projector))
}
