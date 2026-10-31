package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
)

// AuthorizationModel owns shared permission decisions, effective room
// membership, authenticated operation guards and stable-input checks. It uses
// the same content-view transaction as the domain models; it does not create
// another projection or authorization cache.
//
// Effective membership belongs here, not in RoomMembershipModel, because a
// universal channel derives it from join authority. Domain models depend on
// this model; this model depends on no domain operation model.
type AuthorizationModel struct {
	rooms     *RoomModel
	users     *UserModel
	rbac      *RBACModel
	resolver  *PermissionResolver
	publisher *evtstream.Publisher
	view      *ServerContentView
	messages  authorizationMessages
}

// newAuthorizationModel returns an AuthorizationModel with all collaborators.
// view may be nil in isolated tests; decisions then read without a
// content-view transaction.
func newAuthorizationModel(
	rooms *RoomModel,
	users *UserModel,
	rbac *RBACModel,
	resolver *PermissionResolver,
	publisher *evtstream.Publisher,
	view *ServerContentView,
	messages authorizationMessages,
) *AuthorizationModel {
	return &AuthorizationModel{
		rooms: rooms, users: users, rbac: rbac, resolver: resolver,
		publisher: publisher, view: view, messages: messages,
	}
}

// RoomMembershipExists reports whether userID is an effective member of
// roomID. Explicit memberships are the durable state. A universal channel also
// grants effective membership to every server member who can currently join
// it; this membership is derived at read time.
func (a *AuthorizationModel) RoomMembershipExists(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error) {
	return a.readContentDecision(ctx, func(readCtx context.Context) (bool, error) {
		return a.roomMembershipExists(readCtx, kind, userID, roomID)
	})
}

// roomMembershipExists is RoomMembershipExists inside the caller's
// content-view transaction.
func (a *AuthorizationModel) roomMembershipExists(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error) {
	if a.rooms.hasExplicitRoomMembership(roomID, userID) {
		return true, nil
	}
	if kind != KindChannel {
		return false, nil
	}
	room, err := a.rooms.GetRoom(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if !room.GetUniversal() {
		return false, nil
	}
	return a.canJoinRoomAt(ctx, userID, kind, roomID)
}

// resolveCallPermissions reads all media grants in the caller's content-view transaction.
func (a *AuthorizationModel) resolveCallPermissions(ctx context.Context, actorID string, kind RoomKind, roomID string) (CallPermissions, error) {
	var result CallPermissions
	ids := callPermissionIDs()
	for i, target := range []*bool{&result.Start, &result.Join, &result.Voice, &result.Camera, &result.ScreenShare} {
		allowed, err := a.resolver.HasRoomPermission(ctx, actorID, kind, roomID, ids[i])
		if err != nil {
			return CallPermissions{}, err
		}
		*target = allowed
	}
	return result, nil
}

// requireBotManager admits the human owner or any caller with bot management authority.
func (a *AuthorizationModel) requireBotManager(ctx context.Context, actorID, botID string) (*readmodel.User, error) {
	if err := a.requireUser(ctx, actorID); err != nil {
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

// requireUser checks that an authenticated actor has an existing user record,
// without restricting the actor's account kind.
func (a *AuthorizationModel) requireUser(ctx context.Context, userID string) error {
	if err := requireAuthenticatedActor(userID); err != nil {
		return err
	}
	_, err := a.users.GetUser(ctx, userID)
	return err
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
