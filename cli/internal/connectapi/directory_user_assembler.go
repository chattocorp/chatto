package connectapi

import (
	"context"
	"errors"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/parallel"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

// directoryUserAssembler resolves a bounded batch in request order. Encryption
// keys are shared only within this request; failed reads fail the whole batch.
type directoryUserAssembler struct{ api *API }

func (a *directoryUserAssembler) assemble(ctx context.Context, ids []string) ([]*apiv1.DirectoryMember, error) {
	ctx = core.WithDEKRequestCache(ctx)
	presences, err := a.api.core.GetUserPresences(ctx, ids)
	if err != nil {
		return nil, err
	}
	return parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, ids, func(ctx context.Context, _ int, id string) (*apiv1.DirectoryMember, error) {
		user, err := a.api.core.GetUser(ctx, id)
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		roles, err := a.api.core.GetUserRoles(ctx, id)
		if err != nil {
			return nil, err
		}
		if user.GetIsBot() {
			roles = nil
		} else {
			roles = append([]string{core.RoleEveryone}, roles...)
		}
		return directoryMemberWithPresence(ctx, a.api, user, roles, presences[id])
	})
}
