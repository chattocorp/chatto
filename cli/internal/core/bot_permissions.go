package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// BotPermissionMatrixCell adds the bot owner's current delegation ceiling to
// the ordinary direct/effective user permission cell.
type BotPermissionMatrixCell struct {
	Permission   string
	ScopeID      string
	Direct       MatrixDecision
	Effective    MatrixDecision
	OwnerAllowed bool
}

// BotPermissionMatrix describes one manageable bot across every permission
// scope visible to the server.
type BotPermissionMatrix struct {
	BotID                 string
	ApplicablePermissions []string
	Scopes                []PermissionMatrixScope
	Cells                 []BotPermissionMatrixCell
}

// GetBotPermissionMatrix returns the bot's direct/effective permission state
// together with the owner's current delegation ceiling.
func (c *ChattoCore) GetBotPermissionMatrix(ctx context.Context, actorID, botID string) (*BotPermissionMatrix, error) {
	bot, err := c.GetUser(ctx, botID)
	if err != nil {
		return nil, err
	}
	if bot.GetBot() == nil {
		return nil, ErrNotFound
	}
	allowed, err := c.CanManageBot(ctx, actorID, botID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPermissionDenied
	}

	matrix, err := c.buildUserPermissionMatrix(ctx, botID)
	if err != nil {
		return nil, err
	}
	ownerID := bot.GetBot().GetOwnerId()
	scopes := make(map[string]PermissionMatrixScope, len(matrix.Scopes))
	for _, scope := range matrix.Scopes {
		scopes[scope.ID] = scope
	}
	cells := make([]BotPermissionMatrixCell, 0, len(matrix.Cells))
	for _, cell := range matrix.Cells {
		scope, ok := scopes[cell.ScopeID]
		if !ok {
			continue
		}
		permissionScope, scopeID, ok := botPermissionCoreScope(scope)
		if !ok {
			continue
		}
		ownerAllowed, err := c.hasPermissionAtScope(ctx, ownerID, permissionScope, scopeID, Permission(cell.Permission))
		if err != nil {
			return nil, err
		}
		cells = append(cells, BotPermissionMatrixCell{
			Permission:   cell.Permission,
			ScopeID:      cell.ScopeID,
			Direct:       cell.Override,
			Effective:    cell.Effective,
			OwnerAllowed: ownerAllowed,
		})
	}
	return &BotPermissionMatrix{
		BotID:                 botID,
		ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...),
		Scopes:                append([]PermissionMatrixScope(nil), matrix.Scopes...),
		Cells:                 cells,
	}, nil
}

func botPermissionCoreScope(scope PermissionMatrixScope) (PermissionScope, string, bool) {
	switch scope.Kind {
	case MatrixScopeServer:
		return ScopeServer, "", true
	case MatrixScopeGroup:
		return ScopeGroup, scopeRefID(scope.ID, "group:"), true
	case MatrixScopeRoom:
		return ScopeRoom, scopeRefID(scope.ID, "room:"), true
	default:
		return "", "", false
	}
}

// SetBotPermission writes one direct permission decision for a bot. Bot owners
// require bot.create; other human actors require bot.manage. Allows are
// accepted only while the bot's owner currently has the same scoped
// permission. The runtime owner intersection remains the ultimate ceiling.
func (c *ChattoCore) SetBotPermission(ctx context.Context, actorID, botID string, scope PermissionScope, scopeID string, perm Permission, decision DecisionKind) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	if !PermissionAppliesAtScope(perm, scope) {
		return fmt.Errorf("permission %s does not apply at %s scope", perm, scope)
	}
	if scope == ScopeServer {
		scopeID = ""
	} else if scopeID == "" {
		return ErrInvalidArgument
	}

	var event *corev1.Event
	switch decision {
	case DecisionAllow:
		event = newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
			RbacPermissionGranted: rbacUserPermissionGrantedEvent(scope, scopeID, botID, perm),
		}})
	case DecisionDeny:
		event = newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
			RbacPermissionDenied: rbacUserPermissionDeniedEvent(scope, scopeID, botID, perm),
		}})
	case DecisionNone:
		event = newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
			RbacPermissionCleared: rbacUserPermissionClearedEvent(scope, scopeID, botID, perm),
		}})
	default:
		return ErrInvalidArgument
	}

	_, err := c.appendRBACEvent(ctx, event, func() error {
		if err := c.waitForBotPermissionInputs(ctx, actorID, botID, scope); err != nil {
			return err
		}
		allowed, err := c.CanManageBot(ctx, actorID, botID)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrPermissionDenied
		}
		if decision != DecisionAllow {
			return nil
		}
		ownerID, _, _, _ := c.Users.AuthorizationIdentity(botID)
		ownerAllowed, err := c.hasPermissionAtScope(ctx, ownerID, scope, scopeID, perm)
		if err != nil {
			return err
		}
		if !ownerAllowed {
			return ErrPermissionDenied
		}
		return nil
	})
	return err
}

func (c *ChattoCore) waitForBotPermissionInputs(ctx context.Context, actorID, botID string, scope PermissionScope) error {
	for _, userID := range []string{actorID, botID} {
		if userID == "" || userID == SystemActorID {
			continue
		}
		if err := c.userModel.waitForUsersCurrent(ctx, "bot permission account", events.UserAggregate(userID).AllEventsFilter()); err != nil {
			return err
		}
	}
	ownerID, _, _, exists := c.Users.AuthorizationIdentity(botID)
	if exists && ownerID != "" {
		if err := c.userModel.waitForUsersCurrent(ctx, "bot owner permission", events.UserAggregate(ownerID).AllEventsFilter()); err != nil {
			return err
		}
	}
	if scope == ScopeGroup || scope == ScopeRoom {
		groupPosition, err := c.EventPublisher.LastSubjectPosition(ctx, events.GroupSubjectFilter())
		if err != nil {
			return err
		}
		if err := c.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
			return err
		}
	}
	if scope == ScopeRoom {
		roomPosition, err := c.EventPublisher.LastSubjectPosition(ctx, events.RoomSubjectFilter())
		if err != nil {
			return err
		}
		if err := c.roomModel.waitForDirectory(ctx, roomPosition); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChattoCore) hasPermissionAtScope(ctx context.Context, userID string, scope PermissionScope, scopeID string, perm Permission) (bool, error) {
	var (
		decision DecisionKind
		err      error
	)
	switch scope {
	case ScopeServer:
		decision, err = c.PermResolver().resolveBotOwnerCeilingWithGroup(ctx, userID, KindChannel, "", "", perm)
	case ScopeGroup:
		decision, err = c.PermResolver().resolveBotOwnerCeilingWithGroup(ctx, userID, KindChannel, "", scopeID, perm)
	case ScopeRoom:
		decision, err = c.PermResolver().resolveBotOwnerCeilingWithGroup(ctx, userID, KindChannel, scopeID, "", perm)
	default:
		return false, ErrInvalidArgument
	}
	return decision == DecisionAllow, err
}
