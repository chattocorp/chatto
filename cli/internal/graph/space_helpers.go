package graph

import (
	"context"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// resolveServerSpace returns the *corev1.Space for the deployment's
// user-facing space, or (nil, nil) on fresh installs. Used by the space-
// discovery resolvers (Query.spaces, Query.space, User.spaces) to collapse
// the API surface onto a single Server.
func (r *Resolver) resolveServerSpace(ctx context.Context) (*corev1.Space, error) {
	id := r.core.ServerSpaceID()
	if id == "" {
		return nil, nil
	}
	return r.core.GetSpace(ctx, id)
}

// isServerSpace reports whether spaceID matches this deployment's server
// space. Returns false on fresh installs.
func (r *Resolver) isServerSpace(spaceID string) bool {
	id := r.core.ServerSpaceID()
	return id != "" && id == spaceID
}

// appendDMRoomsForServer appends the user's DM conversations to a server-space
// rooms list (issue #330 / ADR-027 phase 3). Storage stays in the hidden DM
// space (ADR-015); only the API surface merges. The caller's dm.view permission
// is checked — without it the original list is returned unchanged.
//
// No-op for non-server spaces, so resolvers can call it unconditionally.
func (r *Resolver) appendDMRoomsForServer(ctx context.Context, spaceID, userID string, rooms []*corev1.Room) ([]*corev1.Room, error) {
	if !r.isServerSpace(spaceID) {
		return rooms, nil
	}
	canDM, err := r.core.CanDMView(ctx, userID)
	if err != nil || !canDM {
		return rooms, nil
	}
	dms, err := r.core.ListDMConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	return append(rooms, dms...), nil
}
