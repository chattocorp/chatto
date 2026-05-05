package graph

import (
	"context"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// resolvePrimarySpace returns the *corev1.Space for the configured primary
// space (issue #330 / ADR-027), or (nil, nil) on fresh installs. Used by the
// space-discovery resolvers (Query.spaces, Query.space, User.spaces) to
// collapse the API surface onto a single Server during the migration.
func (r *Resolver) resolvePrimarySpace(ctx context.Context) (*corev1.Space, error) {
	id, err := r.core.ResolvePrimarySpaceID(ctx, r.serverConfig.PrimarySpaceID)
	if err != nil {
		if r.serverConfig.PrimarySpaceID != "" {
			return nil, err
		}
		return nil, nil
	}
	if id == "" {
		return nil, nil
	}
	return r.core.GetSpace(ctx, id)
}

// isPrimarySpace reports whether spaceID matches this deployment's primary
// space. Returns false on fresh installs or when resolution errors transiently.
func (r *Resolver) isPrimarySpace(ctx context.Context, spaceID string) bool {
	id, err := r.core.ResolvePrimarySpaceID(ctx, r.serverConfig.PrimarySpaceID)
	if err != nil || id == "" {
		return false
	}
	return id == spaceID
}
