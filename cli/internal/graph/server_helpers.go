package graph

import (
	"context"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// requireServerSpaceID returns the deployment-scoped channel space ID.
//
// Post-ADR-030 the Space tier is retired and there is no longer a per-
// deployment Space record to look up — every channel room lives under a
// single implicit deployment scope. The constant is what core methods feed
// into `KindForSpace`, which returns "channel" for any non-DM value.
func (r *Resolver) requireServerSpaceID(_ context.Context) (string, error) {
	return core.ServerSpaceID, nil
}

// resolveRoomSpaceID is the room-aware variant: given only a room ID, return
// the underlying space ID (channel rooms use ServerSpaceID, DM rooms use
// DMSpaceID). Use this in any resolver that operates on an existing room —
// its room ID alone does not tell you which kind's CONFIG bucket holds the
// membership/permission state.
func (r *Resolver) resolveRoomSpaceID(ctx context.Context, roomID string) (string, error) {
	return r.core.FindRoomSpaceID(ctx, roomID)
}

// serverModel constructs the singleton Instance value used as the receiver
// for instance-scoped mutation results.
func (r *mutationResolver) serverModel() *model.Server {
	return &model.Server{
		Version:              r.version,
		EnabledAuthProviders: r.authConfig.EnabledProviders(),
	}
}

// requireInstanceManager is the common gate for server-admin mutations:
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
	can, err := r.core.CanAdminSpaceManage(ctx, user.Id, core.KindForSpace(spaceID))
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, core.ErrPermissionDenied
	}
	return user, nil
}
