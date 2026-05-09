package graph

import (
	"context"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// serverSpaceID returns the deployment's server space ID, or an empty string
// if the instance hasn't been bootstrapped with a user-facing space yet.
//
// Lives on *Resolver so every resolver type (mutation, query, subscription,
// instance, ...) can call it without re-implementing the lookup. PR(b)
// dropped `spaceId` from the API surface, so most call sites used to take
// the spaceId from inputs and now derive it via this helper.
func (r *Resolver) serverSpaceID(ctx context.Context) (string, error) {
	return r.core.FirstUserFacingSpaceID(ctx)
}

// requireServerSpaceID is the common form: `r.serverSpaceID(ctx)` plus an
// error if the instance hasn't been bootstrapped.
func (r *Resolver) requireServerSpaceID(ctx context.Context) (string, error) {
	id, err := r.serverSpaceID(ctx)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", core.ErrInstanceNotBootstrapped
	}
	return id, nil
}

// resolveRoomSpaceID is the room-aware variant: given only a room ID, return
// the underlying space ID (channel rooms live in the primary server space,
// DM rooms in the system DM space). Use this in any resolver that operates
// on an existing room — its room ID alone does not tell you which space's
// CONFIG bucket holds the membership/permission state.
func (r *Resolver) resolveRoomSpaceID(ctx context.Context, roomID string) (string, error) {
	return r.core.FindRoomSpaceID(ctx, roomID)
}

// instanceModel constructs the singleton Instance value used as the receiver
// for instance-scoped mutation results.
func (r *mutationResolver) instanceModel() *model.Instance {
	return &model.Instance{
		Version:              r.version,
		EnabledAuthProviders: r.authConfig.EnabledProviders(),
	}
}

// requireInstanceManager is the common gate for instance-admin mutations:
// requires authentication and admin.instance.manage permission. Returns the
// authenticated user on success.
func (r *mutationResolver) requireInstanceManager(ctx context.Context) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	spaceID, err := r.requireServerSpaceID(ctx)
	if err != nil {
		return nil, err
	}
	can, err := r.core.CanAdminSpaceManage(ctx, user.Id, spaceID)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, core.ErrPermissionDenied
	}
	return user, nil
}
