package core

import (
	"context"
	"fmt"
	"time"

	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// badgeQueryFor prepares a Badge query that reads the user's read boundaries
// from the process-local boundary index. It waits for the index's initial sync
// here, because the query itself runs under the decision projection lock and
// must not block.
func (m *NotificationOccurrenceModel) badgeQueryFor(ctx context.Context, userID, roomID, threadRootEventID string) (badgeQuery, error) {
	boundaries := m.core.notificationBoundaries
	if err := boundaries.waitReady(ctx); err != nil {
		return badgeQuery{}, fmt.Errorf("wait for notification boundaries: %w", err)
	}
	return badgeQuery{
		userID: userID, roomID: roomID, threadRootEventID: threadRootEventID,
		now: m.now().UTC(),
		readBoundary: func(threadRootEventID string) (notificationReadBoundary, bool) {
			return boundaries.readBoundaryNow(userID, roomID, threadRootEventID)
		},
	}, nil
}

// HasNotificationUnread reports whether a room or exact thread has Badge
// attention. It is computed from current projected state and the user's read
// boundaries; no per-user Badge state is stored. An empty
// thread root includes every thread of the room.
func (m *NotificationOccurrenceModel) HasNotificationUnread(ctx context.Context, userID, roomID, threadRootEventID string) (bool, error) {
	query, err := m.badgeQueryFor(ctx, userID, roomID, threadRootEventID)
	if err != nil {
		return false, err
	}
	var unread bool
	if err := m.core.notificationMaterializer.decisions.Projection().withCurrent(query.now, func(snapshot *notificationDecisionSnapshot) error {
		unread = snapshot.hasBadgeAttention(query)
		return nil
	}); err != nil {
		return false, err
	}
	return unread, nil
}

// NotifyNotificationUnreadStateChanged publishes a content-free, user-scoped
// invalidation after Badge state may have changed.
func (c *ChattoCore) NotifyNotificationUnreadStateChanged(ctx context.Context, userID, actorID, roomID, threadRootEventID string) {
	c.publishNotificationUnreadInvalidations(ctx, []notificationUnreadInvalidation{{
		userID: userID, actorID: actorID, roomID: roomID, threadRootEventID: threadRootEventID,
	}})
}

type notificationUnreadInvalidation struct {
	userID            string
	actorID           string
	roomID            string
	threadRootEventID string
}

// publishNotificationUnreadInvalidations publishes one related Badge fanout
// with one final NATS flush. The invalidations are best-effort convergence
// hints; clients re-read the computed Badge state, and reconnect catch-up
// repairs a lost publication.
func (c *ChattoCore) publishNotificationUnreadInvalidations(ctx context.Context, invalidations []notificationUnreadInvalidation) {
	if len(invalidations) == 0 {
		return
	}
	publications := make([]pubsubEventPublication, 0, len(invalidations))
	for _, invalidation := range invalidations {
		publications = append(publications, userPubSubEventPublication(
			invalidation.userID,
			newPubSubEvent(invalidation.actorID, &pubsubv1.PubSubEvent{
				Event: &pubsubv1.PubSubEvent_NotificationUnreadStateChanged{
					NotificationUnreadStateChanged: &realtimev1.NotificationUnreadStateChangedEvent{
						RoomId: invalidation.roomID, ThreadRootEventId: invalidation.threadRootEventID,
					},
				},
			}),
		))
	}
	if err := c.publishPubSubEvents(ctx, publications); err != nil {
		c.logger.Warn("Failed to publish notification unread invalidations", "count", len(publications), "error", err)
	}
}

// badgeRoomStates evaluates the user's room-level Badge attention in every
// room of the notification policy scope where the user is an explicit or
// universal member.
func (c *ChattoCore) badgeRoomStates(ctx context.Context, userID string, scope NotificationPolicyScope) (map[string]bool, error) {
	var roomIDs []string
	decisions := c.notificationMaterializer.decisions.Projection()
	if err := decisions.withCurrent(time.Now().UTC(), func(snapshot *notificationDecisionSnapshot) error {
		for _, roomID := range snapshot.badgeRoomsForUser(userID) {
			switch scope.Kind {
			case NotificationPolicyScopeRoom:
				if roomID != scope.ID {
					continue
				}
			case NotificationPolicyScopeRoomGroup:
				if snapshot.groups.Groups.GroupForRoom(roomID) != scope.ID {
					continue
				}
			}
			roomIDs = append(roomIDs, roomID)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	queries := make([]badgeQuery, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		query, err := c.notificationOccurrences.badgeQueryFor(ctx, userID, roomID, "")
		if err != nil {
			return nil, err
		}
		queries = append(queries, query)
	}
	states := make(map[string]bool, len(queries))
	if err := decisions.withCurrent(time.Now().UTC(), func(snapshot *notificationDecisionSnapshot) error {
		for _, query := range queries {
			states[query.roomID] = snapshot.hasBadgeAttention(query)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return states, nil
}

// withBadgeRoomHints runs change, which alters the user's notification policy
// in scope, and then hints every room of the scope whose Badge attention
// changed. Hints are best effort: a failure to compare states is logged,
// because clients also re-read state on reconnect.
func (c *ChattoCore) withBadgeRoomHints(ctx context.Context, userID string, scope NotificationPolicyScope, change func() error) error {
	before, beforeErr := c.badgeRoomStates(ctx, userID, scope)
	if err := change(); err != nil {
		return err
	}
	if beforeErr != nil {
		c.logger.Warn("Failed to read Badge state before a change", "error", beforeErr)
		return nil
	}
	if err := c.notificationMaterializer.decisions.Projector().WaitForCurrent(ctx); err != nil {
		c.logger.Warn("Failed to wait for notification decisions after a change", "error", err)
		return nil
	}
	after, err := c.badgeRoomStates(ctx, userID, scope)
	if err != nil {
		c.logger.Warn("Failed to read Badge state after a change", "error", err)
		return nil
	}
	var invalidations []notificationUnreadInvalidation
	for _, roomID := range sortedMapKeys(after) {
		if after[roomID] != before[roomID] {
			invalidations = append(invalidations, notificationUnreadInvalidation{userID: userID, actorID: userID, roomID: roomID})
		}
	}
	c.publishNotificationUnreadInvalidations(ctx, invalidations)
	return nil
}
