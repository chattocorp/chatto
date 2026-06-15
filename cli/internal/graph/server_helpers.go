package graph

import (
	"context"
	"net/url"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// resolveRoomKind returns the room kind ("channel" or "dm") for a given
// room ID. Use this in any resolver that operates on an existing room — its
// room ID alone does not tell you which kind's CONFIG bucket holds the
// membership/permission state.
func (r *Resolver) resolveRoomKind(ctx context.Context, roomID string) (core.RoomKind, error) {
	return r.core.FindRoomKind(ctx, roomID)
}

// serverModel constructs the singleton Instance value used as the receiver
// for server-scoped mutation results.
func (r *mutationResolver) serverModel() *model.Server {
	return &model.Server{
		Version:       r.version,
		AuthProviders: authProviderModels(r.authConfig.PublicProviders()),
	}
}

func authProviderModels(providers []config.AuthProviderConfig) []*model.AuthProvider {
	result := make([]*model.AuthProvider, 0, len(providers))
	for _, provider := range providers {
		result = append(result, &model.AuthProvider{
			ID:       provider.ID,
			Type:     provider.Type,
			Label:    provider.LabelOrDefault(),
			LoginURL: "/auth/providers/" + url.PathEscape(provider.ID),
		})
	}
	return result
}

// requireServerManager is the common gate for server-admin mutations:
// requires authentication and admin.instance.manage permission. Returns the
// authenticated user on success.
func (r *mutationResolver) requireServerManager(ctx context.Context) (*corev1.User, error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	can, err := r.core.CanManageServer(ctx, user.Id)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, core.ErrPermissionDenied
	}
	return user, nil
}
