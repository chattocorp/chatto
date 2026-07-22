package core

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
		if err := c.rooms().waitForGroupLayout(ctx, groupPosition); err != nil {
			return err
		}
	}
	if scope == ScopeRoom {
		roomPosition, err := c.EventPublisher.LastSubjectPosition(ctx, events.RoomSubjectFilter())
		if err != nil {
			return err
		}
		if err := c.rooms().waitForDirectory(ctx, roomPosition); err != nil {
			return err
		}
	}
	return nil
}

func (c *ChattoCore) hasPermissionAtScope(ctx context.Context, userID string, scope PermissionScope, scopeID string, perm Permission) (bool, error) {
	switch scope {
	case ScopeServer:
		return c.HasServerPermission(ctx, userID, perm)
	case ScopeGroup:
		return c.hasGroupPermission(ctx, KindChannel, scopeID, userID, perm)
	case ScopeRoom:
		return c.hasRoomPermission(ctx, KindChannel, scopeID, userID, perm)
	default:
		return false, ErrInvalidArgument
	}
}
