package connectapi

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/parallel"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

// roleMemberAssembler hydrates only the selected page, with bounded concurrency.
// Failed reads fail the page instead of silently changing its membership.
type roleMemberAssembler struct{ api *API }

func (a *roleMemberAssembler) assemble(ctx context.Context, ids []string) ([]*apiv1.User, error) {
	return parallel.Map(ctx, maxConnectAPIHydrationConcurrency, ids, func(ctx context.Context, _ int, id string) (*apiv1.User, error) {
		user, err := a.api.core.GetUser(ctx, id)
		if err != nil {
			return nil, err
		}
		return requiredUserSummary(ctx, a.api, user)
	})
}
