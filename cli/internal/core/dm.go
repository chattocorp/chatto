package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MaxDMParticipants is the maximum number of participants allowed in a DM.
// Beyond this, users should create a proper space/room with moderation.
const MaxDMParticipants = 10

// RoomKind is the closed enum of room kinds carried in subjects and KV
// keys (`server.room.{kind}.>`, `room_membership.{kind}.{roomId}.{userId}`,
// etc.). The string form goes on the wire — don't rename the variants.
type RoomKind string

const (
	// KindChannel is a regular (non-DM) chat room.
	KindChannel RoomKind = "channel"
	// KindDM is a direct-message room.
	KindDM RoomKind = "dm"
)

// ProtoKindForRoomKind maps the Go-side RoomKind string to the proto
// enum stored on Room.kind.
func ProtoKindForRoomKind(kind RoomKind) corev1.RoomKind {
	if kind == KindDM {
		return corev1.RoomKind_ROOM_KIND_DM
	}
	return corev1.RoomKind_ROOM_KIND_CHANNEL
}

// KindOfRoom returns the canonical RoomKind for a Room. Reads
// Room.kind directly; legacy `space_id`-based fallback was removed
// when the field was retired from the proto.
func KindOfRoom(room *corev1.Room) RoomKind {
	switch room.Kind {
	case corev1.RoomKind_ROOM_KIND_DM:
		return KindDM
	default:
		return KindChannel
	}
}

// DMRoomID generates a deterministic room ID from participant IDs.
// The same set of participants always produces the same room ID,
// regardless of order. This enables find-or-create semantics without
// database queries.
func DMRoomID(participantIDs []string) string {
	if len(participantIDs) < 1 {
		return ""
	}

	// Sort to ensure consistent ordering
	sorted := make([]string, len(participantIDs))
	copy(sorted, participantIDs)
	sort.Strings(sorted)

	// Hash the sorted participant list
	h := sha256.New()
	for _, id := range sorted {
		h.Write([]byte(id))
		h.Write([]byte{0}) // separator to prevent collisions
	}

	// 14 hex chars (matches NanoID length used elsewhere)
	return hex.EncodeToString(h.Sum(nil))[:14]
}

// ============================================================================
// DM Room Management
// ============================================================================

// FindOrCreateDM finds an existing DM conversation or creates a new one.
// The caller (creatorID) is automatically included in the participant list.
// Returns the room and a boolean indicating whether it was newly created.
//
// For existing DMs, the caller must already be a participant.
// For new DMs, all participants are automatically joined to the room.
func (c *ChattoCore) FindOrCreateDM(ctx context.Context, creatorID string, participantIDs []string) (*corev1.Room, bool, error) {
	// Ensure creator is in participants
	allParticipants := ensureInList(participantIDs, creatorID)

	if len(allParticipants) < 1 {
		return nil, false, fmt.Errorf("DM requires at least 1 participant")
	}
	if len(allParticipants) > MaxDMParticipants {
		return nil, false, fmt.Errorf("DM conversations are limited to %d participants", MaxDMParticipants)
	}

	roomID := DMRoomID(allParticipants)
	if roomID == "" {
		return nil, false, fmt.Errorf("failed to generate DM room ID")
	}

	// Try to get existing room
	room, err := c.GetRoom(ctx, KindDM, roomID)
	if err == nil {
		// Room exists - verify caller is a participant
		isMember, err := c.RoomMembershipExists(ctx, KindDM, creatorID, roomID)
		if err != nil {
			return nil, false, fmt.Errorf("failed to check DM membership: %w", err)
		}
		if !isMember {
			return nil, false, fmt.Errorf("access denied: not a participant in this DM")
		}
		return room, false, nil
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, false, fmt.Errorf("failed to check existing DM: %w", err)
	}

	// Create new DM room
	room, err = c.createDMRoom(ctx, roomID, allParticipants)
	if err != nil {
		// Handle race condition: another request published the
		// RoomCreated first. JetStream's per-subject OCC (expected
		// seq 0) is what arbitrates — the loser sees ErrConflict
		// and looks up the now-existing room.
		if errors.Is(err, events.ErrConflict) {
			room, err = c.GetRoom(ctx, KindDM, roomID)
			if err != nil {
				return nil, false, fmt.Errorf("failed to get DM after race: %w", err)
			}
			return room, false, nil
		}
		return nil, false, fmt.Errorf("failed to create DM: %w", err)
	}

	c.logger.Info("Created DM conversation", "room_id", roomID, "participants", len(allParticipants))
	return room, true, nil
}

// createDMRoom creates a new DM room and joins all participants
// atomically via a single AppendBatch — RoomCreatedEvent followed
// by N×UserJoinedRoomEvent on the same per-room subject, committed
// as one unit. No rollback path is needed: either all events land
// or none do.
//
// Concurrency safety: the first batch entry carries HasOCC with
// ExpectedSeq=0, so a race where another replica already created
// this DM (same hashed roomID) is rejected with events.ErrConflict
// — the caller (FindOrCreateDM) re-fetches.
//
// Post-batch side effects (per-participant read markers, legacy
// live publishes) happen after the batch acks, since they're
// outside the durable event log.
func (c *ChattoCore) createDMRoom(ctx context.Context, roomID string, participantIDs []string) (*corev1.Room, error) {
	room := &corev1.Room{
		Id:   roomID,
		Kind: corev1.RoomKind_ROOM_KIND_DM,
		Name: "", // DMs don't have names - derived from participants in UI
	}

	agg := events.RoomAggregate(roomID)

	// "system" actor reflects that the conversation is created by
	// the platform on the first participant's behalf — DMs have no
	// operator-driven creation flow.
	createdEvent := newEvent("system", &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId:      roomID,
				Name:        "",
				Description: "",
				Kind:        corev1.RoomKind_ROOM_KIND_DM,
			},
		},
	})

	// First entry uses wildcard OCC against the aggregate's full
	// filter — "the entire room aggregate must be empty," not just
	// "no prior RoomCreated event." Preserves the per-aggregate
	// uniqueness guarantee under the per-(agg, event-type) subject
	// shape.
	entries := []events.BatchEntry{
		{
			Subject:       agg.SubjectFor(createdEvent),
			Event:         createdEvent,
			HasOCC:        true,
			FilterSubject: agg.AllEventsFilter(),
		},
	}
	joinEvents := make(map[string]*corev1.Event, len(participantIDs))
	for _, pid := range participantIDs {
		joinEvent := newEvent(pid, &corev1.Event{
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID},
			},
		})
		joinEvents[pid] = joinEvent
		entries = append(entries, events.BatchEntry{
			Subject: agg.SubjectFor(joinEvent),
			Event:   joinEvent,
		})
	}

	seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
	if err != nil {
		// Includes events.ErrConflict on race (per-subject OCC
		// rejected because another replica created this DM first).
		return nil, err
	}

	// Wait per-projector for the seq of the last event each
	// actually consumes. Projectors subscribe to narrow event-type
	// filters now — waiting on a seq that doesn't match the filter
	// blocks forever (the LastSeq only advances on filter-matching
	// events). seqs[0] is the RoomCreated (catalog); seqs[len-1] is
	// the last UserJoinedRoom (membership).
	if err := c.RoomCatalogProjector.WaitForSeq(ctx, seqs[0]); err != nil {
		c.logger.Warn("DM room catalog projection wait failed", "error", err, "room_id", roomID)
	}
	if err := c.RoomMembershipProjector.WaitForSeq(ctx, seqs[len(seqs)-1]); err != nil {
		c.logger.Warn("DM membership projection wait failed", "error", err, "room_id", roomID)
	}
	if err := c.RoomTimelineProjector.WaitForSeq(ctx, seqs[len(seqs)-1]); err != nil {
		c.logger.Warn("DM room timeline projection wait failed", "error", err, "room_id", roomID)
	}

	// Per-participant non-batched side effects: initialise the
	// read marker (so HasUnread distinguishes a fresh member from a
	// deploy-era user; see GetLastReadEventID), and write legacy
	// SERVER_EVENTS copies for migration/import tooling.
	legacySubject := subjects.RoomMeta(string(KindDM), roomID)
	for _, pid := range participantIDs {
		if err := c.SetLastReadEventID(ctx, KindDM, pid, roomID, ""); err != nil {
			c.logger.Warn("Failed to initialize DM read marker", "error", err, "user_id", pid, "room_id", roomID)
		}
		if err := c.publishServerEvent(ctx, legacySubject, joinEvents[pid]); err != nil {
			c.logger.Error("failed to publish UserJoinedRoomEvent for DM (legacy)", "error", err, "user_id", pid, "room_id", roomID)
		}
	}

	return room, nil
}

// ensureInList ensures the given ID is in the list, adding it if not present.
func ensureInList(list []string, id string) []string {
	for _, item := range list {
		if item == id {
			return list
		}
	}
	return append(list, id)
}

// notifyDMParticipants sends notifications to all DM participants except the sender.
// This creates persistent notifications (for bell icon) and publishes live events.
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifyDMParticipants(ctx context.Context, roomID, senderID, eventID string) {
	participants, err := c.GetRoomMembersList(ctx, KindDM, roomID)
	if err != nil {
		c.logger.Warn("Failed to get DM participants for notification",
			"room_id", roomID,
			"error", err)
		return
	}

	for _, participant := range participants {
		participantID := participant.UserId
		// Don't notify the sender
		if participantID == senderID {
			continue
		}

		// Skip if user has muted this DM room
		level, err := c.GetEffectiveNotificationLevel(ctx, participantID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for DM participant, continuing",
				"user_id", participantID, "error", err)
		} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}

		// Publish live DM notification event for unread indicator real-time update
		event := newLiveEvent(senderID, &corev1.LiveEvent{
			Event: &corev1.LiveEvent_NewDirectMessageNotification{
				NewDirectMessageNotification: &corev1.NewDirectMessageNotificationEvent{
					RoomId:   roomID,
					SenderId: senderID,
				},
			},
		})

		subject := subjects.LiveSyncUserEvent(participantID, "dm_message")
		if err := c.publishLiveEvent(ctx, subject, event); err != nil {
			c.logger.Warn("Failed to publish DM live event",
				"participant_id", participantID,
				"error", err)
		}

		// Create persistent notification (for bell icon and notification center)
		// This also publishes NotificationCreatedEvent for real-time updates
		_, createErr := c.CreateNotification(ctx, participantID, senderID, &corev1.Notification{
			Notification: &corev1.Notification_DmMessage{
				DmMessage: &corev1.DMMessageNotification{
					RoomId:  roomID,
					EventId: eventID,
				},
			},
		})
		if createErr != nil {
			c.logger.Warn("Failed to create DM notification",
				"participant_id", participantID,
				"sender_id", senderID,
				"room_id", roomID,
				"error", err)
		} else {
			c.logger.Debug("Created DM notification",
				"participant_id", participantID,
				"sender_id", senderID,
				"room_id", roomID)
		}
	}
}
