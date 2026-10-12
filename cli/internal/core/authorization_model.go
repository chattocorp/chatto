package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// AuthorizationModel owns shared permission decisions, authenticated operation
// guards and stable-input checks. It uses the same content-view transaction as
// the domain models; it does not create another projection or authorization cache.
type AuthorizationModel struct {
	rooms       *RoomModel
	users       *UserModel
	rbac        *RBACModel
	resolver    *PermissionResolver
	memberships *RoomMembershipModel
	publisher   *evtstream.Publisher
	view        *ServerContentView
	messages    authorizationMessages
}

// resolveCallPermissions reads all media grants in the caller's content-view transaction.
func (a *AuthorizationModel) resolveCallPermissions(ctx context.Context, actorID string, kind RoomKind, roomID string) (CallPermissions, error) {
	var result CallPermissions
	for i, target := range []*bool{&result.Start, &result.Join, &result.Voice, &result.Camera, &result.ScreenShare} {
		allowed, err := a.resolver.HasRoomPermission(ctx, actorID, kind, roomID, callPermissionIDs()[i])
		if err != nil {
			return CallPermissions{}, err
		}
		*target = allowed
	}
	return result, nil
}

// requireBotManager admits the human owner or a caller with bot management authority.
func (a *AuthorizationModel) requireBotManager(ctx context.Context, actorID, botID string) (*evtv1.User, error) {
	if err := a.requireHumanUser(ctx, actorID); err != nil {
		return nil, err
	}
	bot, err := a.users.GetUser(ctx, botID)
	if err != nil {
		return nil, err
	}
	if !bot.GetIsBot() {
		return nil, ErrNotFound
	}
	if bot.GetBotOwnerUserId() == actorID {
		return bot, nil
	}
	allowed, err := a.CanManageBots(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPermissionDenied
	}
	return bot, nil
}

func (a *AuthorizationModel) requireHumanUser(ctx context.Context, userID string) error {
	if userID == "" {
		return ErrNotAuthenticated
	}
	user, err := a.users.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user.GetIsBot() {
		return ErrHumanAccountRequired
	}
	return nil
}
