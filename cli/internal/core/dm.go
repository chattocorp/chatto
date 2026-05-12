package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// DMSpaceID is the kind discriminator for direct-message rooms. Stored on
// Room.SpaceId so room-kind routing (room.dm.* vs room.channel.*) survives
// the retirement of the Space tier (ADR-030).
const DMSpaceID = "DM"

// DMSpaceName is the display name for the DM space.
const DMSpaceName = "Direct Messages"

// ServerSpaceID is the kind discriminator for channel (non-DM) rooms.
// Post-ADR-030 there's no longer a per-deployment Space record; this constant
// is what every channel-scoped core call feeds into `KindForSpace` (which
// returns "channel" for any non-DM value).
const ServerSpaceID = "server"

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

// IsDMSpace returns true if the given space ID is the DM system space.
func IsDMSpace(spaceID string) bool {
	return spaceID == DMSpaceID
}

// KindForSpace returns the room kind for a persisted SpaceId value:
// KindDM for the DM system space, KindChannel for everything else.
func KindForSpace(spaceID string) RoomKind {
	if IsDMSpace(spaceID) {
		return KindDM
	}
	return KindChannel
}

// SpaceIDForKind is the inverse of KindForSpace: returns DMSpaceID for
// KindDM, ServerSpaceID otherwise. Used only at the proto boundary where
// wire-format-frozen `space_id` fields still need a value written.
func SpaceIDForKind(kind RoomKind) string {
	if kind == KindDM {
		return DMSpaceID
	}
	return ServerSpaceID
}

// isDMPermissionAllowed returns whether a permission is allowed in the DM space.
// The DM space has no roles - permissions are granted implicitly based on room membership.
// Room membership is verified separately by the GraphQL resolver.
//
// Allowed permissions (granted to all DM room members):
//   - PermRoomJoin: join DM rooms (needed for FindOrCreateDM)
//   - PermMessageReply: use reply attribution (inReplyTo) on messages
//
// Denied permissions (no one can do these in DMs):
//   - PermServerManage: server settings are server-scope, not DM-scope
//   - PermRoleManage, PermRoleAssign: no roles in DM space
//   - PermRoomList: DM room listing uses separate API (ListDMConversations)
//   - PermRoomCreate, PermRoomManage: DM rooms managed via FindOrCreateDM
//   - PermMemberInvite, PermMemberRemove: handled via DM-specific APIs
func isDMPermissionAllowed(perm Permission) bool {
	switch perm {
	case PermRoomJoin, PermMessageReply:
		return true
	default:
		return false
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
		// Handle race condition - another request may have created it
		if errors.Is(err, jetstream.ErrKeyExists) {
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

// createDMRoom creates a new DM room and joins all participants atomically.
// If any participant fails to join, the room is deleted and an error is returned.
// This is an internal function - use FindOrCreateDM for the public API.
func (c *ChattoCore) createDMRoom(ctx context.Context, roomID string, participantIDs []string) (*corev1.Room, error) {
	room := &corev1.Room{
		Id:      roomID,
		SpaceId: DMSpaceID,
		Name:    "", // DMs don't have names - derived from participants in UI
	}

	// Get config bucket for room storage
	bucket := c.storage.serverConfigKV

	// Store room (atomic create to handle race conditions)
	roomData, err := proto.Marshal(room)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal DM room: %w", err)
	}

	_, err = bucket.Create(ctx, roomKey(KindDM, roomID), roomData)
	if err != nil {
		return nil, err // Let caller handle ErrKeyExists for race condition
	}

	// Join all participants - rollback room on failure
	var joinedParticipants []string
	for _, participantID := range participantIDs {
		if err := c.joinDMRoom(ctx, bucket, participantID, roomID); err != nil {
			c.logger.Error("Failed to join participant to DM, rolling back", "participant", participantID, "room_id", roomID, "error", err)

			// Rollback: delete memberships we already created
			for _, joinedID := range joinedParticipants {
				if delErr := bucket.Delete(ctx, roomMembershipKey(KindDM, roomID, joinedID)); delErr != nil {
					c.logger.Error("Failed to rollback DM membership", "participant", joinedID, "room_id", roomID, "error", delErr)
				}
			}

			// Rollback: delete the room
			if delErr := bucket.Delete(ctx, roomKey(KindDM, roomID)); delErr != nil {
				c.logger.Error("Failed to rollback DM room", "room_id", roomID, "error", delErr)
			}

			return nil, fmt.Errorf("failed to add participant %s to DM: %w", participantID, err)
		}
		joinedParticipants = append(joinedParticipants, participantID)
	}

	return room, nil
}

// joinDMRoom adds a user to a DM room (internal, no authorization check).
// Publishes a UserJoinedRoomEvent to initialize the room's event stream — this is
// required for JetStream consumers to work properly. The frontend filters out these
// join events in DM rooms since they're not useful for 1:1 conversations (see
// RoomEvent.svelte).
func (c *ChattoCore) joinDMRoom(ctx context.Context, bucket jetstream.KeyValue, userID, roomID string) error {
	membership := &corev1.RoomMembership{
		UserId: userID,
		RoomId: roomID,
	}

	data, err := proto.Marshal(membership)
	if err != nil {
		return fmt.Errorf("failed to marshal DM membership: %w", err)
	}

	_, err = bucket.Put(ctx, roomMembershipKey(KindDM, roomID, userID), data)
	if err != nil {
		return fmt.Errorf("failed to create DM membership: %w", err)
	}

	// Initialize an empty read marker so HasUnread distinguishes a fresh DM
	// member from a deploy-era user without any marker (see GetLastReadEventID).
	if err := c.SetLastReadEventID(ctx, KindDM, userID, roomID, ""); err != nil {
		c.logger.Warn("Failed to initialize DM read marker", "error", err, "user_id", userID, "room_id", roomID)
	}

	// Publish UserJoinedRoomEvent to seed the room's event stream.
	// This event is filtered out in the frontend for DM rooms.
	event := newEvent(userID, &corev1.Event{
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{
				SpaceId: DMSpaceID,
				RoomId:  roomID,
			},
		},
	})
	subject := subjects.RoomMeta(string(KindDM), roomID)
	if err := c.publishServerEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish UserJoinedRoomEvent for DM", "error", err, "user_id", userID, "room_id", roomID)
	}

	return nil
}

// ListDMConversations returns DM rooms the user is a member of that have at least
// one message. Empty DM rooms (created but never messaged) are excluded.
// Rooms are sorted by last message time, newest first.
func (c *ChattoCore) ListDMConversations(ctx context.Context, userID string) ([]*corev1.Room, error) {
	// Get user's room memberships in DM space
	memberships, err := c.GetUserRoomMemberships(ctx, KindDM, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DM memberships: %w", err)
	}

	// Collect rooms with their last message timestamps
	type roomWithTime struct {
		room      *corev1.Room
		lastMsgAt time.Time
	}
	roomsWithTime := make([]roomWithTime, 0, len(memberships))

	for _, membership := range memberships {
		room, err := c.GetRoom(ctx, KindDM, membership.RoomId)
		if err != nil {
			// Skip rooms that no longer exist (eventual consistency)
			c.logger.Warn("DM room not found for membership", "room_id", membership.RoomId, "user_id", userID)
			continue
		}

		lastMsgAt, err := c.GetRoomLastMessageAt(ctx, KindDM, room.Id)
		if err != nil {
			c.logger.Debug("No messages in DM room, skipping", "room_id", room.Id)
			continue
		}

		// Skip empty conversations (no messages ever posted)
		if lastMsgAt.IsZero() {
			continue
		}

		roomsWithTime = append(roomsWithTime, roomWithTime{room: room, lastMsgAt: lastMsgAt})
	}

	// Sort by last message time, newest first
	sort.Slice(roomsWithTime, func(i, j int) bool {
		return roomsWithTime[i].lastMsgAt.After(roomsWithTime[j].lastMsgAt)
	})

	// Extract sorted rooms
	rooms := make([]*corev1.Room, len(roomsWithTime))
	for i, rwt := range roomsWithTime {
		rooms[i] = rwt.room
	}

	return rooms, nil
}

// GetDMParticipants returns all participant user IDs for a DM room.
func (c *ChattoCore) GetDMParticipants(ctx context.Context, roomID string) ([]string, error) {
	members, err := c.GetRoomMembersList(ctx, KindDM, roomID)
	if err != nil {
		return nil, fmt.Errorf("failed to get DM participants: %w", err)
	}

	participantIDs := make([]string, len(members))
	for i, member := range members {
		participantIDs[i] = member.UserId
	}

	return participantIDs, nil
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
	participants, err := c.GetDMParticipants(ctx, roomID)
	if err != nil {
		c.logger.Warn("Failed to get DM participants for notification",
			"room_id", roomID,
			"error", err)
		return
	}

	for _, participantID := range participants {
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
		event := &corev1.Event{
			Id:        NewEventID(),
			ActorId:   senderID,
			CreatedAt: timestamppb.Now(),
			Event: &corev1.Event_NewDirectMessageNotification{
				NewDirectMessageNotification: &corev1.NewDirectMessageNotificationEvent{
					RoomId:   roomID,
					SenderId: senderID,
				},
			},
		}

		subject := subjects.LiveUserEvent(participantID, "dm_message")
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
