package graph

import (
	"context"

	"hmans.de/chatto/internal/graph/model"
)

// serverSpaceID returns the deployment's server space ID, or an empty string
// if the instance hasn't been bootstrapped with a user-facing space yet.
func (r *instanceResolver) serverSpaceID(ctx context.Context) (string, error) {
	return r.core.FirstUserFacingSpaceID(ctx)
}

// instanceModel constructs the singleton Instance value used as the receiver
// for instance-scoped mutation results.
func (r *mutationResolver) instanceModel() *model.Instance {
	return &model.Instance{
		Version:              r.version,
		EnabledAuthProviders: r.authConfig.EnabledProviders(),
	}
}
