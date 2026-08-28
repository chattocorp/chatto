package core

import (
	"context"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/jetstreamutil"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

const notificationUnreadMarkerKeyPrefix = "notification_unread_marker."

type notificationUnreadMarkerWrite struct {
	changed  bool
	key      string
	revision uint64
}

func notificationUnreadMarkerKey(userID, roomID, threadRootEventID string) string {
	key := notificationUnreadMarkerKeyPrefix + userID + "." + roomID
	if threadRootEventID != "" {
		key += "." + threadRootEventID
	}
	return key
}

func notificationUnreadMarkerFilter(userID string) string {
	return notificationUnreadMarkerKeyPrefix + userID + ".>"
}

// recordNotificationUnreadMarker stores one monotonic latest-value Badge
// decision. Redelivery and older replicas cannot replace a newer source in the
// same room or thread scope.
func (m *NotificationOccurrenceModel) recordNotificationUnreadMarker(ctx context.Context, input CreateNotificationOccurrenceInput) (bool, error) {
	result, err := m.writeNotificationUnreadMarker(ctx, input)
	if err != nil {
		return false, err
	}
	if result.revision != 0 {
		if err := m.core.notificationBoundaries.waitForRevision(ctx, result.key, result.revision); err != nil {
			return false, err
		}
	}
	return result.changed, nil
}

// writeNotificationUnreadMarker commits one marker without waiting for the
// local boundary index. The materializer uses this to pipeline distinct marker
// keys and then waits for one collective applied-revision barrier.
func (m *NotificationOccurrenceModel) writeNotificationUnreadMarker(ctx context.Context, input CreateNotificationOccurrenceInput) (notificationUnreadMarkerWrite, error) {
	if input.Mode != evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE {
		return notificationUnreadMarkerWrite{}, nil
	}
	message := notificationSignalMessage(input.Signal)
	if message == nil || input.SourceStreamSequence == 0 || input.SourceCreated.IsZero() {
		return notificationUnreadMarkerWrite{}, invalidArgument("a Badge delivery requires an exact source time, sequence, and message")
	}
	expiresAt := input.SourceCreated.UTC().Add(notificationTTL)
	now := m.now().UTC()
	if !now.Before(expiresAt) {
		return notificationUnreadMarkerWrite{}, nil
	}
	marker := &runtimestatev1.NotificationUnreadMarker{
		SourceEventId:        input.SourceEventID,
		ActorId:              input.ActorID,
		Signal:               proto.Clone(input.Signal).(*notificationv1.NotificationSignal),
		SourceStreamSequence: input.SourceStreamSequence,
	}
	value, err := proto.Marshal(marker)
	if err != nil {
		return notificationUnreadMarkerWrite{}, fmt.Errorf("encode notification unread marker: %w", err)
	}
	key := notificationUnreadMarkerKey(input.RecipientID, message.GetRoomId(), message.GetThreadRootEventId())
	for attempt := 0; attempt < maxNotificationStateWriteRetries; attempt++ {
		current, err := m.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			revision, createErr := m.kv.Create(ctx, key, value, jetstream.KeyTTL(expiresAt.Sub(now)))
			if createErr == nil {
				return notificationUnreadMarkerWrite{changed: true, key: key, revision: revision}, nil
			}
			if !jetstreamutil.IsSequenceConflict(createErr) {
				return notificationUnreadMarkerWrite{}, fmt.Errorf("create notification unread marker: %w", createErr)
			}
			continue
		}
		if err != nil {
			return notificationUnreadMarkerWrite{}, fmt.Errorf("read notification unread marker: %w", err)
		}
		var previous runtimestatev1.NotificationUnreadMarker
		if err := proto.Unmarshal(current.Value(), &previous); err != nil {
			return notificationUnreadMarkerWrite{}, fmt.Errorf("decode notification unread marker: %w", err)
		}
		if previous.GetSourceStreamSequence() >= marker.GetSourceStreamSequence() {
			return notificationUnreadMarkerWrite{key: key, revision: current.Revision()}, nil
		}
		revision, updateErr := m.core.updateRuntimeStateUntil(ctx, key, value, current.Revision(), expiresAt, now)
		if updateErr == nil {
			return notificationUnreadMarkerWrite{changed: true, key: key, revision: revision}, nil
		}
		if !jetstreamutil.IsSequenceConflict(updateErr) {
			return notificationUnreadMarkerWrite{}, fmt.Errorf("update notification unread marker: %w", updateErr)
		}
	}
	return notificationUnreadMarkerWrite{}, fmt.Errorf("write notification unread marker after %d attempts", maxNotificationStateWriteRetries)
}

// HasNotificationUnread reports whether a room or exact thread has uncovered
// Badge attention. An empty thread root includes nested thread markers so the
// parent room indicator can roll them up.
func (m *NotificationOccurrenceModel) HasNotificationUnread(ctx context.Context, userID, roomID, threadRootEventID string) (bool, error) {
	markers, err := m.core.notificationBoundaries.unreadMarkers(ctx, userID, roomID, threadRootEventID)
	if err != nil {
		return false, err
	}
	for _, marker := range markers {
		active, err := m.notificationUnreadMarkerActive(ctx, userID, marker)
		if err != nil {
			return false, err
		}
		if active {
			return true, nil
		}
	}
	return false, nil
}

func (m *NotificationOccurrenceModel) notificationUnreadMarkerActive(ctx context.Context, userID string, marker *runtimestatev1.NotificationUnreadMarker) (bool, error) {
	if marker == nil || marker.GetSourceStreamSequence() == 0 {
		return false, nil
	}
	message := notificationSignalMessage(marker.GetSignal())
	if message == nil {
		return false, nil
	}
	afterBoundary, err := m.core.notificationMaterializer.sourceAfterVisibilityBoundary(ctx, userID, message.GetRoomId(), marker.GetSourceStreamSequence())
	if err != nil || !afterBoundary {
		return false, err
	}
	if boundary, exists, err := m.notificationReadBoundary(ctx, userID, message.GetRoomId(), message.GetThreadRootEventId()); err != nil {
		return false, err
	} else if exists && m.notificationSignalCoveredByBoundary(marker.GetSignal(), marker.GetSourceStreamSequence(), boundary) {
		return false, nil
	}

	entry, exists := m.core.roomModel.timelineEntry(message.GetEventId())
	if !exists || entry.Event == nil || roomIDOfEvent(entry.Event) != message.GetRoomId() {
		return false, nil
	}
	if _, retracted, known := m.core.roomModel.latestBody(message.GetEventId()); known && retracted {
		return false, nil
	}
	if reaction := marker.GetSignal().GetReactionReceived(); reaction != nil {
		current := m.core.roomModel.reactionMutationSnapshot(message.GetRoomId(), message.GetEventId(), reaction.GetEmoji(), marker.GetActorId())
		return current.Exists && current.SourceEventID == marker.GetSourceEventId(), nil
	}
	return marker.GetSourceEventId() == message.GetEventId(), nil
}

func (m *NotificationOccurrenceModel) notificationSignalCoveredByBoundary(signal *notificationv1.NotificationSignal, sourceSequence uint64, boundary notificationReadBoundary) bool {
	if signal == nil || sourceSequence == 0 {
		return false
	}
	message := notificationSignalMessage(signal)
	if message == nil {
		return false
	}
	if signal.GetReactionReceived() != nil {
		targetEntry, ok := m.core.roomModel.timelineEntry(message.GetEventId())
		return ok && targetEntry.StreamSeq <= boundary.targetSequence && sourceSequence <= boundary.observedSequence
	}
	return sourceSequence <= boundary.targetSequence
}

// deleteNotificationUnreadMarkerBefore removes an inaccessible Badge marker
// without deleting a newer source that another replica has already committed.
func (m *NotificationOccurrenceModel) deleteNotificationUnreadMarkerBefore(ctx context.Context, scope notificationReadBoundaryScope, sequence uint64) (bool, error) {
	key := notificationUnreadMarkerKey(scope.userID, scope.roomID, scope.threadRootEventID)
	for attempt := 0; attempt < maxNotificationStateWriteRetries; attempt++ {
		entry, err := m.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("read notification unread marker for deletion: %w", err)
		}
		var marker runtimestatev1.NotificationUnreadMarker
		if err := proto.Unmarshal(entry.Value(), &marker); err != nil {
			return false, fmt.Errorf("decode notification unread marker for deletion: %w", err)
		}
		if marker.GetSourceStreamSequence() >= sequence {
			return false, nil
		}
		if err := m.kv.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err == nil {
			if err := m.core.notificationBoundaries.waitForRevisionAfter(ctx, key, entry.Revision()); err != nil {
				return false, err
			}
			return true, nil
		} else if !jetstreamutil.IsSequenceConflict(err) {
			return false, fmt.Errorf("delete notification unread marker: %w", err)
		}
		if err := m.core.notificationBoundaries.waitForRevisionAfter(ctx, key, entry.Revision()); err != nil {
			return false, err
		}
	}
	return false, fmt.Errorf("delete notification unread marker after %d attempts", maxNotificationStateWriteRetries)
}

func (m *NotificationOccurrenceModel) purgeNotificationUnreadMarkers(ctx context.Context, userID string) error {
	lister, err := m.kv.ListKeysFiltered(ctx, notificationUnreadMarkerFilter(userID))
	if errors.Is(err, jetstream.ErrNoKeysFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list notification unread markers: %w", err)
	}
	for key := range lister.Keys() {
		if err := m.core.notificationMaterializer.deleteRuntimeStateKey(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// NotifyNotificationUnreadChanged publishes a content-free, user-scoped
// invalidation after authoritative Badge state changes.
func (c *ChattoCore) NotifyNotificationUnreadChanged(ctx context.Context, userID, actorID, roomID, threadRootEventID string) {
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
// hints; the unread markers remain authoritative across a lost publication.
func (c *ChattoCore) publishNotificationUnreadInvalidations(ctx context.Context, invalidations []notificationUnreadInvalidation) {
	publications := make([]liveEventPublication, 0, len(invalidations))
	for _, invalidation := range invalidations {
		publications = append(publications, liveEventPublication{
			subject: subjects.LiveSyncUserEvent(invalidation.userID, "notification_unread"),
			event: newLiveEvent(invalidation.actorID, &livev1.LiveEvent{
				Event: &livev1.LiveEvent_NotificationUnreadChanged{
					NotificationUnreadChanged: &livev1.NotificationUnreadChangedEvent{
						RoomId: invalidation.roomID, ThreadRootEventId: invalidation.threadRootEventID,
					},
				},
			}),
		})
	}
	if err := c.publishLiveEvents(ctx, publications); err != nil {
		c.logger.Warn("Failed to publish notification unread invalidations", "count", len(publications), "error", err)
	}
}

func (m *NotificationOccurrenceModel) publishUnreadMarkerTargetInvalidations(ctx context.Context, roomID, eventID, actorID, emoji string) {
	for _, scope := range m.core.notificationBoundaries.unreadMarkerScopes("", roomID, 0) {
		marker, _, exists, err := m.core.notificationBoundaries.unreadMarker(ctx, scope)
		if err != nil || !exists {
			continue
		}
		message := notificationSignalMessage(marker.GetSignal())
		if message == nil || message.GetEventId() != eventID {
			continue
		}
		if emoji != "" {
			reaction := marker.GetSignal().GetReactionReceived()
			if reaction == nil || reaction.GetEmoji() != emoji || marker.GetActorId() != actorID {
				continue
			}
		}
		m.core.NotifyNotificationUnreadChanged(ctx, scope.userID, actorID, scope.roomID, scope.threadRootEventID)
	}
}
