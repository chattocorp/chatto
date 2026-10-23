package core

import (
	"context"
)

func (a *AuthorizationModel) requireCanManageAnyRoom(ctx context.Context, actorID string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	ok, err := a.CanManageAnyRoom(ctx, actorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}

func (a *AuthorizationModel) requireCanManageRoomGroup(ctx context.Context, actorID, groupID string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	ok, err := a.CanManageRoomGroup(ctx, actorID, groupID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}
