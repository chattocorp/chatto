package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Room Operations
// ============================================================================

// getRoomLastMessage fetches the last message in a room directly from JetStream.
// Returns nil if no messages exist for this room yet.
func (c *ChattoCore) getRoomLastMessage(ctx context.Context, spaceID, roomID string) (*jetstream.RawStreamMsg, error) {
	stream := c.storage.serverEventsStream

	msg, err := stream.GetLastMsgForSubject(ctx, subjects.RoomAllMessages(kindForSpace(spaceID), roomID))
	if err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return nil, nil // No messages yet
		}
		return nil, fmt.Errorf("failed to get last message for room: %w", err)
	}
	return msg, nil
}

// getRoomLastRootMessage fetches the last root message (excluding thread replies) in a room.
// Returns nil if no root messages exist for this room yet.
// Used for unread tracking where thread replies should not affect room-level unread state.
func (c *ChattoCore) getRoomLastRootMessage(ctx context.Context, spaceID, roomID string) (*jetstream.RawStreamMsg, error) {
	stream := c.storage.serverEventsStream

	msg, err := stream.GetLastMsgForSubject(ctx, subjects.RoomRootMessages(kindForSpace(spaceID), roomID))
	if err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return nil, nil // No root messages yet
		}
		return nil, fmt.Errorf("failed to get last root message for room: %w", err)
	}
	return msg, nil
}

// GetRoomLastMessageAt returns the timestamp of the last message in a room.
// Derived directly from JetStream — no KV cache needed.
// Returns zero time if no messages exist for this room yet.
//
// The timestamp comes from the proto's `created_at` field rather than
// JetStream's stored time. The two are nearly identical for messages
// published after #354 phase 4d, but messages migrated by phase 4d have
// a fresh JetStream stamp; the proto time stays correct in both cases.
func (c *ChattoCore) GetRoomLastMessageAt(ctx context.Context, spaceID, roomID string) (time.Time, error) {
	msg, err := c.getRoomLastMessage(ctx, spaceID, roomID)
	if err != nil {
		return time.Time{}, err
	}
	if msg == nil {
		return time.Time{}, nil
	}
	return rawMsgEventCreatedAt(msg)
}

// rawMsgEventCreatedAt unmarshals a JetStream message as a SpaceEvent and
// returns its `created_at` time. Returns zero time + nil error if the
// message has no proto-level timestamp (defensive — every event we
// publish carries one).
func rawMsgEventCreatedAt(msg *jetstream.RawStreamMsg) (time.Time, error) {
	var event corev1.SpaceEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		return time.Time{}, fmt.Errorf("unmarshal event for timestamp: %w", err)
	}
	if event.CreatedAt == nil {
		return time.Time{}, nil
	}
	return event.CreatedAt.AsTime(), nil
}

// Room name validation constants
const (
	RoomNameMinLength        = 1
	RoomNameMaxLength        = 30
	RoomDescriptionMaxLength = 500
)

// ErrRoomNameExists is returned when a room with the same name (case-insensitive) already exists.
var ErrRoomNameExists = errors.New("a room with this name already exists in this space")

// ValidateRoomName validates a room name and returns an error if invalid.
// Room names must be URL-safe: only alphanumeric characters, hyphens, and underscores.
func ValidateRoomName(name string) error {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) < RoomNameMinLength {
		return fmt.Errorf("room name is required")
	}
	if len(trimmed) > RoomNameMaxLength {
		return fmt.Errorf("room name must be %d characters or less", RoomNameMaxLength)
	}

	// Check for URL-safe characters only (alphanumeric, hyphens, underscores)
	for _, ch := range trimmed {
		if !isURLSafeChar(ch) {
			return fmt.Errorf("room name must contain only alphanumeric characters, hyphens, and underscores (no spaces or special characters)")
		}
	}

	return nil
}

// urlSafeCharRegex matches URL-safe characters for room names.
// Allows: a-z, A-Z, 0-9, hyphen (-), and underscore (_)
var urlSafeCharRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]$`)

// isURLSafeChar returns true if the character is URL-safe for room names.
func isURLSafeChar(ch rune) bool {
	return urlSafeCharRegex.MatchString(string(ch))
}

// ValidateRoomDescription validates a room description and returns an error if invalid.
func ValidateRoomDescription(description string) error {
	if len(description) > RoomDescriptionMaxLength {
		return fmt.Errorf("room description must be %d characters or less", RoomDescriptionMaxLength)
	}
	return nil
}

// CreateRoom creates a new room in a space.
// KV store is written first, then an event is published for audit trail (best-effort).
// Authorization: Caller must verify CanCreateRoom before calling.
func (c *ChattoCore) CreateRoom(ctx context.Context, actorID string, space_id, name, description string) (*corev1.Room, error) {
	// Validate room name
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}

	// Validate room description
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}

	// Trim whitespace from name
	name = strings.TrimSpace(name)

	bucket := c.storage.serverConfigKV

	// Backfill name index for any pre-existing rooms (no-op after first call per process).
	if err := c.ensureRoomNameIndex(ctx, space_id, bucket); err != nil {
		return nil, fmt.Errorf("failed to ensure room name index: %w", err)
	}

	room_id := NewRoomID()

	// Atomically claim the name. kv.Create fails with ErrKeyExists if the name is taken,
	// which removes the read-then-write race that the previous list-and-scan check had.
	indexKey := roomNameIndexKey(name)
	if _, err := bucket.Create(ctx, indexKey, []byte(room_id)); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return nil, ErrRoomNameExists
		}
		return nil, fmt.Errorf("failed to claim room name: %w", err)
	}

	// Create room entity
	room := &corev1.Room{
		Id:          room_id,
		SpaceId:     space_id,
		Name:        name,
		Description: description,
	}

	roomData, err := proto.Marshal(room)
	if err != nil {
		// Roll back the name claim so the name doesn't end up reserved with no room behind it.
		c.bestEffortReleaseRoomNameClaim(ctx, bucket, indexKey, room_id)
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	if _, err := bucket.Put(ctx, roomKey(roomKindKeyFromSpaceID(room.SpaceId), room.Id), roomData); err != nil {
		c.bestEffortReleaseRoomNameClaim(ctx, bucket, indexKey, room_id)
		return nil, fmt.Errorf("failed to store room: %w", err)
	}

	// Create and publish audit event to space stream
	// Room events are stored in the unified space stream
	event := newSpaceEvent(actorID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId:      room_id,
				Name:        name,
				Description: description,
				SpaceId:     space_id,
			},
		},
	})
	subject := subjects.RoomMeta(kindForSpace(space_id), room_id)
	_, err = c.publishSpaceEventWithAck(ctx, subject, event)
	if err != nil {
		// Room was created in KV but event failed - log but don't fail
		c.logger.Error("failed to publish room created event", "error", err, "room_id", room_id)
	}

	// Set up special permissions for announcements rooms
	if strings.EqualFold(name, AnnouncementsRoomName) {
		if err := c.SetupAnnouncementsRoomPermissions(ctx, space_id, room_id); err != nil {
			c.logger.Warn("Failed to set up announcements room permissions", "error", err, "room_id", room_id)
			// Don't fail room creation if permission setup fails
		}
	}

	c.logger.Info("Room created", "space_id", space_id, "room_id", room_id, "name", name)

	return room, nil
}

// UpdateRoom updates an existing room.
// KV store is updated first, then an event is published for audit trail (best-effort).
// Authorization: Caller must verify CanAdminRoomsManage before calling.
func (c *ChattoCore) UpdateRoom(ctx context.Context, actorID string, space_id, room_id, name, description string) (*corev1.Room, error) {
	// Validate room name
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}

	// Validate room description
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}

	// Trim whitespace from name
	name = strings.TrimSpace(name)

	// Fetch existing room (preserves all fields like Archived, AutoJoin)
	room, err := c.GetRoom(ctx, space_id, room_id)
	if err != nil {
		return nil, err
	}

	bucket := c.storage.serverConfigKV

	// Backfill name index for pre-existing rooms (no-op after first call per process).
	if err := c.ensureRoomNameIndex(ctx, space_id, bucket); err != nil {
		return nil, fmt.Errorf("failed to ensure room name index: %w", err)
	}

	// Detect rename. Case-changes-only count as a rename for index purposes only when the
	// lowercased form actually changes (e.g. "general" → "General" keeps the same index key).
	oldIndexKey := roomNameIndexKey(room.Name)
	newIndexKey := roomNameIndexKey(name)
	renamed := oldIndexKey != newIndexKey

	if renamed {
		// Atomically claim the new name before writing the room. ErrKeyExists means the
		// name is taken by another room — fail without touching the room record.
		if _, err := bucket.Create(ctx, newIndexKey, []byte(room_id)); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				return nil, ErrRoomNameExists
			}
			return nil, fmt.Errorf("failed to claim room name: %w", err)
		}
	}

	// Update only the mutable fields
	room.Name = name
	room.Description = description

	// Write to KV store (source of truth)
	roomData, err := proto.Marshal(room)
	if err != nil {
		if renamed {
			c.bestEffortReleaseRoomNameClaim(ctx, bucket, newIndexKey, room_id)
		}
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	if _, err := bucket.Put(ctx, roomKey(roomKindKeyFromSpaceID(room.SpaceId), room.Id), roomData); err != nil {
		if renamed {
			c.bestEffortReleaseRoomNameClaim(ctx, bucket, newIndexKey, room_id)
		}
		return nil, fmt.Errorf("failed to update room: %w", err)
	}

	// Release the old name now that the room record points at the new one. Best-effort:
	// a leftover index entry is harmless (it would be reclaimed by the same room ID on a
	// retry, and a fresh CreateRoom for that name would still see ErrKeyExists, which is
	// the conservative choice).
	if renamed {
		c.bestEffortReleaseRoomNameClaim(ctx, bucket, oldIndexKey, room_id)
	}

	// Create and publish audit event to space stream (best-effort)
	event := newSpaceEvent(actorID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{
				RoomId:      room_id,
				Name:        name,
				Description: description,
				SpaceId:     space_id,
			},
		},
	})
	subject := subjects.RoomMeta(kindForSpace(space_id), room_id)
	if err := c.publishSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room updated event", "error", err, "room_id", room_id)
	}

	c.logger.Info("Room updated", "space_id", space_id, "room_id", room_id, "name", name)

	return room, nil
}

// DeleteRoom deletes a room.
// Publishes event first, then deletes from KV store, then deletes the stream.
// Authorization: Caller must verify CanAdminRoomsManage before calling.
func (c *ChattoCore) DeleteRoom(ctx context.Context, actorID string, space_id, room_id string) error {
	// Verify room exists
	room, err := c.GetRoom(ctx, space_id, room_id)
	if err != nil {
		return err
	}

	// Create and publish audit event to space stream BEFORE deletion (best-effort)
	event := newSpaceEvent(actorID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_RoomDeleted{
			RoomDeleted: &corev1.RoomDeletedEvent{
				SpaceId: space_id,
				RoomId:  room_id,
			},
		},
	})
	subject := subjects.RoomMeta(kindForSpace(space_id), room_id)
	if err := c.publishSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room deleted event", "error", err, "room_id", room_id)
	}

	// Delete from KV store (source of truth)
	bucket := c.storage.serverConfigKV
	err = bucket.Delete(ctx, roomKey(roomKindKeyFromSpaceID(space_id), room_id))
	if err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}

	// Release the room name so a new room can claim it. Best-effort: a stale entry only
	// blocks reuse of that exact name, and the next CreateRoom for it would log a clear
	// ErrRoomNameExists rather than silently wedging anything.
	c.bestEffortReleaseRoomNameClaim(ctx, bucket, roomNameIndexKey(room.Name), room_id)

	// Purge room events from the space stream
	if err := c.purgeRoomEvents(ctx, space_id, room_id); err != nil {
		c.logger.Error("failed to purge room events", "error", err, "space_id", space_id, "room_id", room_id)
		// Continue anyway - orphaned events can be cleaned up manually if needed
	}

	// Best-effort: remove room from layout if present
	c.removeRoomFromLayout(ctx, space_id, room_id)

	c.logger.Info("Room deleted", "space_id", space_id, "room_id", room_id)

	return nil
}

// ArchiveRoom sets a room's archived flag to true.
// Archived rooms are hidden from sidebars and Browse Rooms. Existing memberships are preserved.
// Authorization: Caller must verify CanAdminRoomsManage before calling.
func (c *ChattoCore) ArchiveRoom(ctx context.Context, actorID, spaceID, roomID string) (*corev1.Room, error) {
	room, err := c.GetRoom(ctx, spaceID, roomID)
	if err != nil {
		return nil, err
	}

	room.Archived = true

	bucket := c.storage.serverConfigKV
	roomData, err := proto.Marshal(room)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	_, err = bucket.Put(ctx, roomKey(roomKindKeyFromSpaceID(room.SpaceId), room.Id), roomData)
	if err != nil {
		return nil, fmt.Errorf("failed to archive room: %w", err)
	}

	// Remove from layout (archived rooms should not appear in layout sections)
	c.removeRoomFromLayout(ctx, spaceID, roomID)

	// Publish persisted event to space stream (best-effort)
	event := newSpaceEvent(actorID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_RoomArchived{
			RoomArchived: &corev1.RoomArchivedEvent{
				SpaceId: spaceID,
				RoomId:  roomID,
			},
		},
	})
	subject := subjects.RoomMeta(kindForSpace(spaceID), roomID)
	if err := c.publishSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room archived event", "error", err, "room_id", roomID)
	}

	// Publish live event for real-time sync (sidebar/layout updates)
	if err := c.PublishRoomLayoutUpdated(ctx, actorID, spaceID); err != nil {
		c.logger.Error("failed to publish room layout updated event after archive", "error", err)
	}

	c.logger.Info("Room archived", "space_id", spaceID, "room_id", roomID)
	return room, nil
}

// UnarchiveRoom sets a room's archived flag to false.
// The room will reappear in sidebars and Browse Rooms as an unsorted room.
// Authorization: Caller must verify CanAdminRoomsManage before calling.
func (c *ChattoCore) UnarchiveRoom(ctx context.Context, actorID, spaceID, roomID string) (*corev1.Room, error) {
	room, err := c.GetRoom(ctx, spaceID, roomID)
	if err != nil {
		return nil, err
	}

	room.Archived = false

	bucket := c.storage.serverConfigKV
	roomData, err := proto.Marshal(room)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	_, err = bucket.Put(ctx, roomKey(roomKindKeyFromSpaceID(room.SpaceId), room.Id), roomData)
	if err != nil {
		return nil, fmt.Errorf("failed to unarchive room: %w", err)
	}

	// Publish persisted event to space stream (best-effort)
	event := newSpaceEvent(actorID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_RoomUnarchived{
			RoomUnarchived: &corev1.RoomUnarchivedEvent{
				SpaceId: spaceID,
				RoomId:  roomID,
			},
		},
	})
	subject := subjects.RoomMeta(kindForSpace(spaceID), roomID)
	if err := c.publishSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room unarchived event", "error", err, "room_id", roomID)
	}

	// Publish live event for real-time sync (sidebar/layout updates)
	if err := c.PublishRoomLayoutUpdated(ctx, actorID, spaceID); err != nil {
		c.logger.Error("failed to publish room layout updated event after unarchive", "error", err)
	}

	c.logger.Info("Room unarchived", "space_id", spaceID, "room_id", roomID)
	return room, nil
}

// SetRoomAutoJoin sets the auto_join flag on a room.
// When auto_join is true, new space members automatically join this room.
// Authorization: Caller must verify CanAdminRoomsManage before calling.
func (c *ChattoCore) SetRoomAutoJoin(ctx context.Context, actorID, spaceID, roomID string, autoJoin bool) (*corev1.Room, error) {
	room, err := c.GetRoom(ctx, spaceID, roomID)
	if err != nil {
		return nil, err
	}

	room.AutoJoin = autoJoin

	bucket := c.storage.serverConfigKV
	roomData, err := proto.Marshal(room)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	_, err = bucket.Put(ctx, roomKey(roomKindKeyFromSpaceID(room.SpaceId), room.Id), roomData)
	if err != nil {
		return nil, fmt.Errorf("failed to update room auto_join: %w", err)
	}

	c.logger.Info("Room auto_join updated", "space_id", spaceID, "room_id", roomID, "auto_join", autoJoin)
	return room, nil
}

// GetRoom retrieves a room from the space-specific CONFIG bucket.
func (c *ChattoCore) GetRoom(ctx context.Context, space_id, room_id string) (*corev1.Room, error) {
	bucket := c.storage.serverConfigKV

	entry, err := bucket.Get(ctx, roomKey(roomKindKeyFromSpaceID(space_id), room_id))
	if err != nil {
		return nil, fmt.Errorf("room not found: %w", err)
	}

	room := &corev1.Room{}
	if err := proto.Unmarshal(entry.Value(), room); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room: %w", err)
	}

	return room, nil
}

// FindRoomSpaceID resolves the space ID for a room given only its ID.
// Tries the deployment's primary user-facing space first (channel rooms),
// then the DM system space. Returns ErrNotFound if neither has the room.
//
// Post-PR(b) the GraphQL surface no longer carries `spaceId`, so resolvers
// that take just a room ID use this to recover the space context the core
// API still needs for KV partitioning.
func (c *ChattoCore) FindRoomSpaceID(ctx context.Context, room_id string) (string, error) {
	primary, err := c.FirstUserFacingSpaceID(ctx)
	if err == nil && primary != "" {
		if _, err := c.GetRoom(ctx, primary, room_id); err == nil {
			return primary, nil
		}
	}
	if _, err := c.GetRoom(ctx, DMSpaceID, room_id); err == nil {
		return DMSpaceID, nil
	}
	return "", ErrNotFound
}

// ListRoomsBySpace retrieves all rooms in a space from the CONFIG bucket.
//
// Post-#330 phase 4b: the primary space and the DM system space share
// SERVER_CONFIG, with kind encoded in the key prefix (`room.channel.{X}`
// vs `room.dm.{X}`). The prefix scan returns only the matching kind, so
// no in-memory filter is needed.
func (c *ChattoCore) ListRoomsBySpace(ctx context.Context, space_id string) ([]*corev1.Room, error) {
	bucket := c.storage.serverConfigKV

	prefix := roomKeyPrefix(roomKindKeyFromSpaceID(space_id))
	keyLister, err := bucket.ListKeysFiltered(ctx, prefix)
	if err != nil {
		if err == jetstream.ErrNoKeysFound {
			return []*corev1.Room{}, nil
		}
		return nil, fmt.Errorf("failed to list room keys: %w", err)
	}

	var rooms []*corev1.Room
	for key := range keyLister.Keys() {
		entry, err := bucket.Get(ctx, key)
		if err != nil {
			c.logger.Warn("Failed to get room", "key", key, "error", err)
			continue
		}

		room := &corev1.Room{}
		if err := proto.Unmarshal(entry.Value(), room); err != nil {
			c.logger.Warn("Failed to unmarshal room", "key", key, "error", err)
			continue
		}

		rooms = append(rooms, room)
	}

	return rooms, nil
}

// RoomNameExists checks if a room with the given name already exists in the space.
// It performs a case-insensitive comparison after trimming whitespace.
func (c *ChattoCore) RoomNameExists(ctx context.Context, space_id, name string) (bool, error) {
	return c.RoomNameExistsExcluding(ctx, space_id, name, "")
}

// RoomNameExistsExcluding checks if a room with the given name exists, excluding a specific room.
// This is used by UpdateRoom to allow a room to keep its own name (with different casing).
// It performs a case-insensitive comparison after trimming whitespace.
//
// Backed by the room_name_index.* keys, so it's O(1) per call after the per-space backfill
// has run once. CreateRoom and UpdateRoom enforce uniqueness via atomic kv.Create rather
// than calling this — this method exists for callers that want to query without mutating.
func (c *ChattoCore) RoomNameExistsExcluding(ctx context.Context, space_id, name, excludeRoomID string) (bool, error) {
	bucket := c.storage.serverConfigKV

	if err := c.ensureRoomNameIndex(ctx, space_id, bucket); err != nil {
		return false, fmt.Errorf("failed to ensure room name index: %w", err)
	}

	entry, err := bucket.Get(ctx, roomNameIndexKey(name))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to look up room name index: %w", err)
	}

	if string(entry.Value()) == excludeRoomID {
		return false, nil
	}
	return true, nil
}

// ensureRoomNameIndex backfills room_name_index.<name> entries for any pre-existing rooms
// that were created before atomic name claiming was introduced. Idempotent and cached
// per-space-per-process so the cost is paid at most once. After that, every CreateRoom /
// UpdateRoom / DeleteRoom keeps the index in sync directly.
func (c *ChattoCore) ensureRoomNameIndex(ctx context.Context, spaceID string, bucket jetstream.KeyValue) error {
	if _, ok := c.roomNameIndexBackfilled.Load(spaceID); ok {
		return nil
	}

	// Channels only — DM rooms have empty names so there's nothing to index.
	keyLister, err := bucket.ListKeysFiltered(ctx, roomKeyPrefix(roomKindKeyFromSpaceID(spaceID)))
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			c.roomNameIndexBackfilled.Store(spaceID, struct{}{})
			return nil
		}
		return fmt.Errorf("failed to list room keys for backfill: %w", err)
	}

	for key := range keyLister.Keys() {
		entry, err := bucket.Get(ctx, key)
		if err != nil {
			c.logger.Warn("Skipping room during name-index backfill: get failed", "key", key, "error", err)
			continue
		}

		room := &corev1.Room{}
		if err := proto.Unmarshal(entry.Value(), room); err != nil {
			c.logger.Warn("Skipping room during name-index backfill: unmarshal failed", "key", key, "error", err)
			continue
		}

		indexKey := roomNameIndexKey(room.Name)
		if _, err := bucket.Create(ctx, indexKey, []byte(room.Id)); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) {
				continue // already indexed (idempotent retry)
			}
			c.logger.Warn("Failed to backfill room name index entry", "space_id", spaceID, "room_id", room.Id, "error", err)
		}
	}

	c.roomNameIndexBackfilled.Store(spaceID, struct{}{})
	return nil
}

// bestEffortReleaseRoomNameClaim removes a room_name_index entry but only if it still
// points at the expected room ID. This protects against accidentally deleting an index
// entry that another room has already claimed (e.g. after a partial failure that left
// the value rewritten by a retry).
func (c *ChattoCore) bestEffortReleaseRoomNameClaim(ctx context.Context, bucket jetstream.KeyValue, indexKey, expectedRoomID string) {
	entry, err := bucket.Get(ctx, indexKey)
	if err != nil {
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			c.logger.Warn("Failed to read room name index for release", "key", indexKey, "error", err)
		}
		return
	}
	if string(entry.Value()) != expectedRoomID {
		// Some other room owns this name now — leave it alone.
		return
	}
	if err := bucket.Delete(ctx, indexKey, jetstream.LastRevision(entry.Revision())); err != nil {
		// LastRevision guards against racing with a concurrent update; if that race
		// happened, the new owner still has the claim, which is exactly what we want.
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			c.logger.Warn("Failed to release room name index", "key", indexKey, "error", err)
		}
	}
}

// ============================================================================
// Room Membership Operations
// ============================================================================

// roomMembershipKey returns the KV key for a room membership.
// Pattern: `room_membership.{kind}.{roomID}.{userID}` where kind is
// "channel" or "dm". Same outer-to-inner scope ordering as roomKey
// (`room.{kind}.{roomID}`): kind, then room, then per-room detail.
func roomMembershipKey(kind, room_id, user_id string) string {
	return fmt.Sprintf("room_membership.%s.%s.%s", kind, room_id, user_id)
}

// roomMembershipKeyPrefixForRoom returns the key prefix for listing all
// memberships of a given room. Pattern: `room_membership.{kind}.{roomID}.*`.
// Pure prefix scan — used by room-deletion cleanup and member-list reads.
func roomMembershipKeyPrefixForRoom(kind, room_id string) string {
	return fmt.Sprintf("room_membership.%s.%s.*", kind, room_id)
}

// roomMembershipKeyMatchForUser returns the subject filter that matches
// a user's memberships of a given kind. The userID is in the trailing
// position of the key (`room_membership.{kind}.{roomID}.{userID}`), so
// this is an internal-wildcard filter rather than a pure prefix:
// `room_membership.{kind}.*.{userID}`. Server-side filtered by NATS.
func roomMembershipKeyMatchForUser(kind, user_id string) string {
	return fmt.Sprintf("room_membership.%s.*.%s", kind, user_id)
}

// roomMembershipKeyMatchForUserAnyKind returns the subject filter that matches
// a user's memberships across all kinds (channel + dm).
// Pattern: `room_membership.*.*.{userID}`.
func roomMembershipKeyMatchForUserAnyKind(user_id string) string {
	return fmt.Sprintf("room_membership.*.*.%s", user_id)
}

// GetRoomMembership retrieves a room membership for a user in a specific room.
func (c *ChattoCore) GetRoomMembership(ctx context.Context, space_id, user_id, room_id string) (*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	key := roomMembershipKey(roomKindKeyFromSpaceID(space_id), room_id, user_id)
	data, err := kv.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	var membership corev1.RoomMembership
	if err := proto.Unmarshal(data.Value(), &membership); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room membership data for user %s in room %s: %w", user_id, room_id, err)
	}

	return &membership, nil
}

// RoomMembershipExists checks if a user is a member of a room.
func (c *ChattoCore) RoomMembershipExists(ctx context.Context, space_id, user_id, room_id string) (bool, error) {
	_, err := c.GetRoomMembership(ctx, space_id, user_id, room_id)

	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to check membership for user %s in room %s: %w", user_id, room_id, err)
	}

	return true, nil
}

// JoinRoom creates or updates a room membership for a user.
// This operation is idempotent - calling it multiple times with the same parameters
// will succeed without error, making it safe for distributed systems where the same
// operation might be retried or executed concurrently.
// It validates that the user is a member of the space before allowing the room membership.
// Authorization: Caller must verify CanJoinRoom before calling.
func (c *ChattoCore) JoinRoom(ctx context.Context, actorID, space_id, user_id, room_id string) (*corev1.RoomMembership, error) {
	// Verify that the user is a member of the space
	isSpaceMember, err := c.SpaceMembershipExists(ctx, user_id, space_id)
	if err != nil {
		return nil, fmt.Errorf("failed to verify space membership for user %s in space %s: %w", user_id, space_id, err)
	}

	if !isSpaceMember {
		return nil, fmt.Errorf("user %s is not a member of space %s", user_id, space_id)
	}

	// Verify room exists and is not archived
	room, err := c.GetRoom(ctx, space_id, room_id)
	if err != nil {
		return nil, err
	}
	if room.Archived {
		return nil, fmt.Errorf("cannot join archived room")
	}

	// Check if this is a new membership (for event publishing)
	exists, err := c.RoomMembershipExists(ctx, space_id, user_id, room_id)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing membership: %w", err)
	}
	isNew := !exists

	kv := c.storage.serverConfigKV

	membership := &corev1.RoomMembership{
		UserId: user_id,
		RoomId: room_id,
	}

	data, err := proto.Marshal(membership)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room membership data: %w", err)
	}

	_, err = kv.Put(ctx, roomMembershipKey(roomKindKeyFromSpaceID(space_id), room_id, user_id), data)
	if err != nil {
		return nil, fmt.Errorf("failed to create room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	c.logger.Info("Created room membership", "user_id", user_id, "space_id", space_id, "room_id", room_id)

	// Initialize the read marker for new members. For non-empty rooms, mark
	// them caught up to the current last event so existing messages don't
	// surface as unread. For empty rooms, write an empty-string sentinel so
	// the key's presence still distinguishes "member with nothing to read
	// yet" from "no marker at all" (which the lazy-init path treats as a
	// deploy-era upgrade — see GetLastReadEventID).
	if isNew {
		var initEventID string
		if lastID, _, exists, err := c.GetRoomLastEvent(ctx, space_id, room_id); err != nil {
			c.logger.Warn("Failed to get room last event during join", "error", err, "room_id", room_id)
		} else if exists {
			initEventID = lastID
		}
		if err := c.SetLastReadEventID(ctx, space_id, user_id, room_id, initEventID); err != nil {
			c.logger.Warn("Failed to initialize read marker during join", "error", err, "room_id", room_id)
		}
	}

	// Publish UserJoinedRoomEvent if this is a new membership
	if isNew {
		event := newSpaceEvent(actorID, &corev1.SpaceEvent{
			Event: &corev1.SpaceEvent_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{
					SpaceId: space_id,
					RoomId:  room_id,
				},
			},
		})

		subject := subjects.RoomMeta(kindForSpace(space_id), room_id)
		if err := c.publishSpaceEvent(ctx, subject, event); err != nil {
			c.logger.Error("failed to publish UserJoinedRoomEvent", "error", err, "user_id", user_id, "room_id", room_id)
		}
	}

	return membership, nil
}

// LeaveRoom removes a room membership for a user.
// This operation is idempotent - it will succeed even if the membership doesn't exist.
//
// Business rule: DM conversations are permanent and cannot be left.
func (c *ChattoCore) LeaveRoom(ctx context.Context, actorID, space_id, user_id, room_id string) error {
	// DM conversations are permanent - users cannot leave them
	if IsDMSpace(space_id) {
		return ErrCannotLeaveDMConversation
	}

	// Check if the membership exists before deletion (for event publishing)
	exists, err := c.RoomMembershipExists(ctx, space_id, user_id, room_id)
	if err != nil {
		return fmt.Errorf("failed to check existing membership: %w", err)
	}

	kv := c.storage.serverConfigKV

	err = kv.Delete(ctx, roomMembershipKey(roomKindKeyFromSpaceID(space_id), room_id, user_id))
	if err != nil {
		return fmt.Errorf("failed to delete room membership for user %s in room %s: %w", user_id, room_id, err)
	}

	c.logger.Info("Deleted room membership", "user_id", user_id, "space_id", space_id, "room_id", room_id)

	// Publish UserLeftRoomEvent if the membership existed
	if exists {
		event := newSpaceEvent(actorID, &corev1.SpaceEvent{
			Event: &corev1.SpaceEvent_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{
					SpaceId: space_id,
					RoomId:  room_id,
				},
			},
		})

		subject := subjects.RoomMeta(kindForSpace(space_id), room_id)
		if err := c.publishSpaceEvent(ctx, subject, event); err != nil {
			c.logger.Error("failed to publish UserLeftRoomEvent", "error", err, "user_id", user_id, "room_id", room_id)
		}
	}

	return nil
}

// GetUserRoomMemberships retrieves all room memberships for a given user in a specific space.
func (c *ChattoCore) GetUserRoomMemberships(ctx context.Context, space_id, user_id string) ([]*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	kl, err := kv.ListKeysFiltered(ctx, roomMembershipKeyMatchForUser(roomKindKeyFromSpaceID(space_id), user_id))
	if err != nil {
		return nil, fmt.Errorf("failed to list room memberships for user %s in space %s: %w", user_id, space_id, err)
	}

	return readMembershipsFromKeys(ctx, kv, kl)
}

// GetAllUserRoomMemberships retrieves all of a user's room memberships across
// every kind (channel + dm). The post-pivot data layer is a single
// SERVER_CONFIG bucket, so the kind segment is the only thing that scoped a
// listing by space; callers that don't care about that distinction (e.g. the
// unified live-event subscription) use this.
func (c *ChattoCore) GetAllUserRoomMemberships(ctx context.Context, user_id string) ([]*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	kl, err := kv.ListKeysFiltered(ctx, roomMembershipKeyMatchForUserAnyKind(user_id))
	if err != nil {
		return nil, fmt.Errorf("failed to list room memberships for user %s: %w", user_id, err)
	}

	return readMembershipsFromKeys(ctx, kv, kl)
}

func readMembershipsFromKeys(ctx context.Context, kv jetstream.KeyValue, kl jetstream.KeyLister) ([]*corev1.RoomMembership, error) {
	var memberships []*corev1.RoomMembership
	for key := range kl.Keys() {
		data, err := kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get room membership data for key %s: %w", key, err)
		}

		var membership corev1.RoomMembership
		if err := proto.Unmarshal(data.Value(), &membership); err != nil {
			return nil, fmt.Errorf("failed to unmarshal room membership data for key %s: %w", key, err)
		}

		memberships = append(memberships, &membership)
	}
	return memberships, nil
}

// deleteUserRoomMembershipsInSpace deletes all room memberships for a user in a specific space.
// This is called when a user leaves a space (or their account is deleted) to clean up room memberships.
// It also publishes UserLeftRoomEvent for each room so clients can update their member lists.
func (c *ChattoCore) deleteUserRoomMembershipsInSpace(ctx context.Context, user_id, space_id string) error {
	kv := c.storage.serverConfigKV

	// List the user's memberships in this space's kind. Key format
	// post-#330 phase 4b: `room_membership.{kind}.{room_id}.{user_id}`.
	// userID is the trailing segment, so this is an internal-wildcard
	// filter rather than a pure prefix.
	kl, err := kv.ListKeysFiltered(ctx, roomMembershipKeyMatchForUser(roomKindKeyFromSpaceID(space_id), user_id))
	if err != nil {
		// No keys found is fine - user may not be in any rooms
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("failed to list room memberships for user %s in space %s: %w", user_id, space_id, err)
	}

	// Collect keys and extract room IDs
	type keyAndRoom struct {
		key    string
		roomID string
	}
	var entries []keyAndRoom
	for key := range kl.Keys() {
		// Extract room ID from key: room_membership.{kind}.{room_id}.{user_id}
		parts := strings.Split(key, ".")
		if len(parts) == 4 {
			entries = append(entries, keyAndRoom{key: key, roomID: parts[2]})
		}
	}

	// Delete each room membership and publish events
	for _, entry := range entries {
		if err := kv.Delete(ctx, entry.key); err != nil {
			c.logger.Warn("Failed to delete room membership", "key", entry.key, "error", err)
			continue
		}

		// Publish UserLeftRoomEvent so clients can update their member lists
		event := newSpaceEvent(user_id, &corev1.SpaceEvent{
			Event: &corev1.SpaceEvent_UserLeftRoom{
				UserLeftRoom: &corev1.UserLeftRoomEvent{
					SpaceId: space_id,
					RoomId:  entry.roomID,
				},
			},
		})
		subject := subjects.RoomMeta(kindForSpace(space_id), entry.roomID)
		if err := c.publishSpaceEvent(ctx, subject, event); err != nil {
			c.logger.Warn("Failed to publish UserLeftRoomEvent", "room_id", entry.roomID, "error", err)
		}
	}

	if len(entries) > 0 {
		c.logger.Info("Deleted user room memberships", "user_id", user_id, "space_id", space_id, "count", len(entries))
	}

	return nil
}

// GetRoomMembersList retrieves all user memberships for a given room.
func (c *ChattoCore) GetRoomMembersList(ctx context.Context, space_id, room_id string) ([]*corev1.RoomMembership, error) {
	kv := c.storage.serverConfigKV

	// List room memberships of the kind that lives in this space's bucket.
	// Key format: `room_membership.{kind}.{userID}.{roomID}`.
	kl, err := kv.ListKeysFiltered(ctx, fmt.Sprintf("room_membership.%s.>", roomKindKeyFromSpaceID(space_id)))
	if err != nil {
		if err == jetstream.ErrNoKeysFound {
			return []*corev1.RoomMembership{}, nil
		}
		return nil, fmt.Errorf("failed to list room membership keys in space %s: %w", space_id, err)
	}

	var memberships []*corev1.RoomMembership

	for key := range kl.Keys() {
		data, err := kv.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("failed to get room membership data for key %s: %w", key, err)
		}

		var membership corev1.RoomMembership
		if err := proto.Unmarshal(data.Value(), &membership); err != nil {
			return nil, fmt.Errorf("failed to unmarshal room membership data for key %s: %w", key, err)
		}

		// Filter by room_id
		if membership.RoomId == room_id {
			memberships = append(memberships, &membership)
		}
	}

	return memberships, nil
}

// ============================================================================
// Message Operations
// ============================================================================

const defaultHistoricalMessageLimit = 50

// DecryptedMessageBody represents a message body with decrypted content.
// Used as the return type for GetFullMessageBody since the proto no longer has a plaintext field.
type DecryptedMessageBody struct {
	AuthorId    string
	Body        string // Decrypted message text
	Attachments []*corev1.Attachment
	LinkPreview *corev1.LinkPreview
	CreatedAt   time.Time
	UpdatedAt   *time.Time // nil if never edited
}

// GetFullMessageBody retrieves the complete message body from the BODIES bucket.
// Used by GraphQL resolvers for lazy-loading message content and attachments.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
// Returns nil if the body doesn't exist (e.g., deleted for GDPR).
// If the encryption key is missing (crypto-shredded), returns nil (same as deleted)
// which triggers "[Message unavailable]" display in UI.
func (c *ChattoCore) GetFullMessageBody(ctx context.Context, space_id string, messageBodyKey string) (*DecryptedMessageBody, error) {
	bucket := c.storage.serverBodiesKV

	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil // Return nil for missing bodies (deleted for GDPR)
		}
		return nil, fmt.Errorf("failed to fetch message body: %w", err)
	}

	var messageBody corev1.MessageBody
	if err := proto.Unmarshal(entry.Value(), &messageBody); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Decrypt the message body
	decrypted, err := c.decryptMessageBody(ctx, &messageBody)
	if err != nil {
		// Key not found = crypto-shredded, treat as unavailable (same as deleted)
		if errors.Is(err, encryption.ErrKeyNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decrypt message body: %w", err)
	}

	result := &DecryptedMessageBody{
		AuthorId:    messageBody.AuthorId,
		Body:        string(decrypted),
		Attachments: messageBody.Attachments,
		LinkPreview: messageBody.LinkPreview,
		CreatedAt:   messageBody.CreatedAt.AsTime(),
	}
	if messageBody.UpdatedAt != nil {
		t := messageBody.UpdatedAt.AsTime()
		result.UpdatedAt = &t
	}
	return result, nil
}

// decryptMessageBody decrypts an encrypted message body using the author's key.
func (c *ChattoCore) decryptMessageBody(ctx context.Context, msg *corev1.MessageBody) ([]byte, error) {
	key, err := c.encryption.keyManager.GetUserKey(ctx, msg.AuthorId)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return nil, encryption.ErrKeyNotFound
	}

	return encryption.Decrypt(key, msg.EncryptedBody, msg.EncryptionNonce)
}

// GetMessageBody retrieves a message body text from the bodies KV bucket.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
// Returns empty string if the body has been deleted (GDPR), doesn't exist,
// or if the encryption key has been deleted (crypto-shredded).
// Prefer GetFullMessageBody when you need attachments or other metadata.
func (c *ChattoCore) GetMessageBody(ctx context.Context, space_id string, messageBodyKey string) (string, error) {
	body, err := c.GetFullMessageBody(ctx, space_id, messageBodyKey)
	if err != nil {
		return "", err
	}
	if body == nil {
		return "", nil
	}
	return body.Body, nil
}

// PostMessage posts a message to a room.
// The flow is: store body in BODIES bucket first (using NanoID), then publish event.
// This eliminates race conditions where subscribers receive the event before body is stored.
// Attachments should already be uploaded to ObjectStore; pass their metadata here.
// inThread is the event ID of the thread root message for thread replies, or empty string for top-level messages.
// If inThread is empty but inReplyTo points at a message that is itself in a thread, inThread is
// derived from the target's own inThread so the new message correctly joins that thread.
// inReplyTo is the event ID of the message this responds to (attribution only), or empty string.
// alsoSendToChannel publishes a MessagePostedEvent echo to the root subject for channel visibility.
// Authorization: Caller must verify room membership and CanPostMessage/CanPostInThread before calling, and CanEchoMessage (if alsoSendToChannel).
func (c *ChattoCore) PostMessage(ctx context.Context, space_id, room_id, user_id, body string, attachments []*corev1.Attachment, inThread, inReplyTo string, linkPreview *corev1.LinkPreview, alsoSendToChannel bool) (*corev1.SpaceEvent, error) {
	// Validate message body length to prevent DoS via oversized messages
	if len(body) > MaxMessageBodyLength {
		return nil, ErrMessageTooLong
	}

	// Validate that message has either body or attachments.
	// HasVisibleContent rejects messages with only invisible Unicode characters.
	hasBody := HasVisibleContent(body)
	hasAttachments := len(attachments) > 0
	if !hasBody && !hasAttachments {
		return nil, fmt.Errorf("message must have either body or attachments")
	}

	// Verify room exists
	_, err := c.GetRoom(ctx, space_id, room_id)
	if err != nil {
		return nil, err
	}

	// If replying to a message inside a thread, inherit its thread root.
	// This keeps the data invariant intact even when callers (bots, older clients,
	// extensions) only set inReplyTo. inReplyTo is attribution-only, so a lookup
	// failure here is not fatal — fall through and let the message post as a root.
	if inReplyTo != "" && inThread == "" {
		target, err := c.GetRoomEventByEventID(ctx, space_id, room_id, inReplyTo)
		if err == nil && target != nil {
			if msg := target.GetMessagePosted(); msg != nil && msg.InThread != "" {
				inThread = msg.InThread
			}
		}
	}

	// Validate thread root exists if posting to a thread.
	if inThread != "" {
		rootEvent, err := c.GetRoomEventByEventID(ctx, space_id, room_id, inThread)
		if err != nil {
			return nil, fmt.Errorf("failed to get thread root message: %w", err)
		}
		if rootEvent == nil {
			return nil, fmt.Errorf("thread root message not found: event ID %s", inThread)
		}
		rootMsg := rootEvent.GetMessagePosted()
		if rootMsg == nil {
			return nil, fmt.Errorf("thread root is not a message event: event ID %s", inThread)
		}
		// Verify it's actually a root message (not itself a thread reply)
		if rootMsg.InThread != "" {
			return nil, fmt.Errorf("thread root must be a root message, not a thread reply: event ID %s", inThread)
		}
	}

	now := time.Now()

	// Extract and resolve @mentions from message body
	var mentionedUserIDs []string
	if hasBody {
		usernames := ExtractMentionUsernames(body)
		if len(usernames) > 0 {
			resolved, err := c.ResolveMentions(ctx, space_id, usernames)
			if err != nil {
				c.logger.Warn("Failed to resolve mentions", "error", err)
				// Continue without mentions - don't fail the message
			} else {
				mentionedUserIDs = resolved
			}
		}
	}

	// STEP 1: Create event first to get the event ID for body storage
	// The compound key format is {userId}.{eventId} to enable efficient user-based filtering
	event := newSpaceEvent(user_id, &corev1.SpaceEvent{
		CreatedAt: timestamppb.New(now),
		Event: &corev1.SpaceEvent_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{
				SpaceId:          space_id,
				RoomId:           room_id,
				InReplyTo:        inReplyTo,
				InThread:         inThread,
				MentionedUserIds: mentionedUserIDs,
			},
		},
	})

	// Use event ID for body storage key
	messageBodyKey := messageBodyKey(user_id, event.Id)
	event.GetMessagePosted().MessageBodyId = messageBodyKey

	// STEP 2: Store message body in BODIES bucket BEFORE publishing event
	// This eliminates the race condition where subscribers receive event before body exists
	// Note: UpdatedAt is intentionally nil for new messages - only set when message is edited
	messageBody := &corev1.MessageBody{
		CreatedAt:   timestamppb.New(now),
		Attachments: attachments,
		AuthorId:    user_id,
		LinkPreview: linkPreview,
	}

	// Encrypt message body
	key, err := c.encryption.keyManager.GetUserKey(ctx, user_id)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return nil, fmt.Errorf("encryption key not found for user %s", user_id)
	}

	encrypted, err := encryption.Encrypt(key, []byte(body))
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message body: %w", err)
	}

	messageBody.EncryptedBody = encrypted.Ciphertext
	messageBody.EncryptionNonce = encrypted.Nonce

	bucket := c.storage.serverBodiesKV

	bodyData, err := proto.Marshal(messageBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Put(ctx, messageBodyKey, bodyData)
	if err != nil {
		return nil, fmt.Errorf("failed to store message body: %w", err)
	}

	// STEP 3: Publish event
	// Choose subject based on whether this is a root message or thread reply
	// Event ID is included in the subject for O(1) lookup via GetLastMsgForSubject
	var subject string
	if inThread == "" {
		subject = subjects.RoomMessage(kindForSpace(space_id), room_id, event.Id)
	} else {
		subject = subjects.RoomThread(kindForSpace(space_id), room_id, inThread, event.Id)
	}

	// Publish with OCC for reliable delivery with retry on concurrent publishes
	sequenceID, err := c.publishSpaceEventWithOCC(ctx, space_id, subject, event)
	if err != nil {
		// Body was stored but event failed to publish - clean up body
		_ = bucket.Delete(ctx, messageBodyKey)
		return nil, fmt.Errorf("failed to publish message event: %w", err)
	}

	c.logger.Info("Message posted", "space_id", space_id, "room_id", room_id, "message_body_key", messageBodyKey, "sequence_id", sequenceID, "user_id", user_id)

	// Mark the room as read for the poster. For root posts, the just-
	// published event is the new last root. For thread replies, we look up
	// the room's current last root so the read marker tracks a real root
	// event ID (HasUnread expects root events).
	var posterReadEventID string
	if inThread == "" {
		posterReadEventID = event.Id
	} else if lastRootID, _, exists, err := c.GetRoomLastEvent(ctx, space_id, room_id); err == nil && exists {
		posterReadEventID = lastRootID
	}
	if posterReadEventID != "" {
		if err := c.SetLastReadEventID(ctx, space_id, user_id, room_id, posterReadEventID); err != nil {
			c.logger.Warn("Failed to set last read event for poster", "error", err)
		}
	}

	// Update thread metadata if this is a thread reply
	if inThread != "" {
		// Get the thread root event to find the original author
		rootEvent, err := c.GetRoomEventByEventID(ctx, space_id, room_id, inThread)
		if err != nil {
			c.logger.Warn("Failed to get thread root event",
				"thread_root_id", inThread,
				"error", err)
		}

		var rootAuthorID string
		if rootEvent != nil {
			rootAuthorID = rootEvent.ActorId
		}

		if err := c.updateThreadMetadata(ctx, space_id, room_id, inThread, rootAuthorID, user_id, now); err != nil {
			c.logger.Warn("Failed to update thread metadata", "error", err, "thread_root_event_id", inThread)
			// Continue anyway - thread metadata is best-effort
		}

		// Update the poster's "last opened" timestamp for this thread.
		// This ensures that on page reload, their own message won't show as "unread".
		if _, err := c.SetThreadLastOpened(ctx, space_id, user_id, room_id, inThread); err != nil {
			c.logger.Warn("Failed to update thread last opened for poster", "error", err, "thread_root_event_id", inThread)
			// Continue anyway - this is best-effort
		}

		// Auto-follow the thread for the poster (best-effort).
		// Always follows, even if previously unfollowed — posting implies interest.
		if err := c.FollowThread(ctx, space_id, user_id, room_id, inThread); err != nil {
			c.logger.Warn("Failed to auto-follow thread for poster", "error", err, "thread_root_event_id", inThread)
		}

		// Auto-follow the root author only on the first reply to their message.
		// We check the reply count (already updated above): if 1, this is the first reply.
		// On subsequent replies, we don't re-add the root author — they can unfollow freely.
		if rootAuthorID != "" && rootAuthorID != user_id {
			threadMeta, err := c.GetThreadMetadata(ctx, space_id, room_id, inThread)
			if err != nil {
				c.logger.Warn("Failed to get thread metadata for root author auto-follow", "error", err, "thread_root_event_id", inThread)
			} else if threadMeta.ReplyCount == 1 {
				if err := c.FollowThread(ctx, space_id, rootAuthorID, room_id, inThread); err != nil {
					c.logger.Warn("Failed to auto-follow thread for root author", "error", err, "thread_root_event_id", inThread)
				}
			}
		}
	}

	// Notify mentioned users (best-effort, don't fail the message if this fails)
	if len(mentionedUserIDs) > 0 {
		c.notifyMentionedUsers(ctx, space_id, room_id, user_id, event.Id, inThread, mentionedUserIDs)
	}

	// Notify the author of the message being replied to (best-effort).
	// Fires for both room-level replies and in-thread replies with inReplyTo set.
	// Runs before notifyThreadFollowers so the more specific inReplyTo notification
	// takes priority (thread participants dedup against this).
	var replyNotifiedUserID string
	if inReplyTo != "" {
		replyNotifiedUserID = c.notifyInReplyToAuthor(ctx, space_id, room_id, user_id, event.Id, inReplyTo, inThread, mentionedUserIDs)
	}

	// Notify all thread participants (best-effort).
	// Skip users already notified by inReplyTo (they get the more specific notification).
	if inThread != "" {
		var skipIDs []string
		if replyNotifiedUserID != "" {
			skipIDs = []string{replyNotifiedUserID}
		}
		c.notifyThreadFollowers(ctx, space_id, room_id, user_id, event.Id, inThread, skipIDs)
	}

	// Notify DM participants for every new message (best-effort)
	if space_id == DMSpaceID {
		c.notifyDMParticipants(ctx, room_id, user_id, event.Id)
	}

	// Notify space members of new message for real-time unread indicators (best-effort).
	// Only for root messages — thread replies have their own notification via notifyThreadFollowers.
	// This works for both regular spaces and DMs (DM participants are "space members" of the DM space).
	if inThread == "" {
		c.notifySpaceMembersOfNewMessage(ctx, space_id, room_id, user_id)
	}

	// Notify room members who have ALL_MESSAGES notification level (root messages only).
	// Build a set of already-notified users to avoid duplicate notifications.
	if inThread == "" {
		alreadyNotified := make(map[string]bool)
		alreadyNotified[user_id] = true // Author
		for _, uid := range mentionedUserIDs {
			alreadyNotified[uid] = true
		}
		// Include in-reply-to author to avoid duplicate notification
		if replyNotifiedUserID != "" {
			alreadyNotified[replyNotifiedUserID] = true
		}
		// Include DM participants to avoid duplicate notifications
		// (they were already notified by notifyDMParticipants above)
		if space_id == DMSpaceID {
			if participants, err := c.GetDMParticipants(ctx, room_id); err == nil {
				for _, pid := range participants {
					alreadyNotified[pid] = true
				}
			}
		}
		c.notifyAllMessageSubscribers(ctx, space_id, room_id, user_id, event.Id, alreadyNotified)
	}

	// Publish echo event to root subject if "also send to channel" was requested.
	// This creates a separate event visible in GetRoomEvents (main channel timeline).
	// The echo shares the same messageBodyId, so edits/deletes propagate to both.
	if inThread != "" && alsoSendToChannel {
		echoEvent := newSpaceEvent(user_id, &corev1.SpaceEvent{
			CreatedAt: event.CreatedAt,
			Event: &corev1.SpaceEvent_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{
					SpaceId:           space_id,
					RoomId:            room_id,
					MessageBodyId:     messageBodyKey,
					InReplyTo:         inReplyTo,
					MentionedUserIds:  mentionedUserIDs,
					EchoOfEventId:     event.Id,
					EchoFromThreadRootEventId: inThread,
				},
			},
		})

		echoSubject := subjects.RoomMessage(kindForSpace(space_id), room_id, echoEvent.Id)
		echoSequenceID, err := c.publishSpaceEventWithOCC(ctx, space_id, echoSubject, echoEvent)
		if err != nil {
			c.logger.Warn("Failed to publish thread reply echo", "error", err, "thread_reply_event_id", event.Id)
		} else {
			c.logger.Info("Thread reply echo posted",
				"space_id", space_id, "room_id", room_id,
				"echo_event_id", echoEvent.Id, "original_event_id", event.Id,
				"echo_sequence_id", echoSequenceID)

			// Notify space members for real-time unread indicators (best-effort).
			// The echo is a root-level event, so it should trigger unread dots just like a root message.
			c.notifySpaceMembersOfNewMessage(ctx, space_id, room_id, user_id)

			// Notify room members with ALL_MESSAGES notification level (best-effort).
			// Build already-notified set: author + mentioned users (already notified above for original reply).
			echoAlreadyNotified := make(map[string]bool)
			echoAlreadyNotified[user_id] = true
			for _, uid := range mentionedUserIDs {
				echoAlreadyNotified[uid] = true
			}
			c.notifyAllMessageSubscribers(ctx, space_id, room_id, user_id, echoEvent.Id, echoAlreadyNotified)
		}
	}

	return event, nil
}

// notifySpaceMembersOfNewMessage publishes a live event to notify space members
// that a new message was posted. This enables real-time unread indicators on space icons.
// Uses a single publish to a space-scoped subject with server-side filtering to deliver
// to all space members (including the author - frontend can filter if needed).
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifySpaceMembersOfNewMessage(ctx context.Context, spaceID, roomID, authorID string) {
	event := &corev1.InstanceEvent{
		Id:        NewEventID(),
		ActorId:   authorID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.InstanceEvent_NewMessageInSpace{
			NewMessageInSpace: &corev1.NewMessageInSpaceEvent{
				SpaceId: spaceID,
				RoomId:  roomID,
			},
		},
	}

	subject := subjects.LiveInstanceSpaceEvent(spaceID, "new_message")
	if err := c.publishInstanceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish new message in space event",
			"space_id", spaceID,
			"room_id", roomID,
			"error", err)
	}
}

// notifyAllMessageSubscribers creates notifications for room members who have the
// ALL_MESSAGES notification level. Only called for root messages (not thread replies).
// Skips users who were already notified (mentions, thread replies, DM notifications).
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifyAllMessageSubscribers(ctx context.Context, spaceID, roomID, authorID, eventID string, alreadyNotified map[string]bool) {
	members, err := c.GetRoomMembersList(ctx, spaceID, roomID)
	if err != nil {
		c.logger.Warn("Failed to get room members for all-message notifications",
			"space_id", spaceID, "room_id", roomID, "error", err)
		return
	}

	notifiedCount := 0
	for _, member := range members {
		memberID := member.UserId
		if alreadyNotified[memberID] {
			continue
		}

		level, err := c.GetEffectiveNotificationLevel(ctx, spaceID, memberID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for all-message check",
				"user_id", memberID, "error", err)
			continue
		}
		if level != corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES {
			continue
		}

		_, err = c.CreateNotification(ctx, memberID, authorID, &corev1.Notification{
			Notification: &corev1.Notification_RoomMessage{
				RoomMessage: &corev1.RoomMessageNotification{
					SpaceId: spaceID,
					RoomId:  roomID,
					EventId: eventID,
				},
			},
		})
		if err != nil {
			c.logger.Warn("Failed to create all-message notification",
				"recipient_id", memberID, "author_id", authorID,
				"space_id", spaceID, "room_id", roomID, "error", err)
		} else {
			notifiedCount++
		}
	}

	if notifiedCount > 0 {
		c.logger.Debug("Created all-message notifications",
			"space_id", spaceID, "room_id", roomID, "count", notifiedCount)
	}
}

// NotifyRoomMarkedAsRead publishes a live event to notify the user that they marked
// a room as read. This enables real-time updates to space unread indicators.
// This is best-effort - failures are logged but don't affect the mark-as-read operation.
func (c *ChattoCore) NotifyRoomMarkedAsRead(ctx context.Context, userID, spaceID, roomID string) {
	event := &corev1.InstanceEvent{
		Id:        NewEventID(),
		ActorId:   userID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.InstanceEvent_RoomMarkedAsRead{
			RoomMarkedAsRead: &corev1.RoomMarkedAsReadEvent{
				SpaceId: spaceID,
				RoomId:  roomID,
			},
		},
	}

	// Publish to user's instance event stream (only they need to know)
	subject := subjects.LiveInstanceUserEvent(userID, "room_read")
	if err := c.publishInstanceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish room marked as read event",
			"user_id", userID,
			"space_id", spaceID,
			"room_id", roomID,
			"error", err)
	}
}

// GetMessageAuthorID retrieves the author ID for a message body.
// Returns empty string if the message doesn't exist (already deleted).
// Used by GraphQL layer to check ownership before calling DeleteMessage.
func (c *ChattoCore) GetMessageAuthorID(ctx context.Context, spaceID, messageBodyID string) (string, error) {
	messageBody, err := c.GetFullMessageBody(ctx, spaceID, messageBodyID)
	if err != nil {
		return "", err
	}
	if messageBody == nil {
		return "", nil // Message already deleted
	}
	return messageBody.AuthorId, nil
}

// DeleteMessage deletes a message body and its attachments for GDPR compliance.
// This removes the message content from the BODIES bucket and any attachments from the ASSETS
// ObjectStore, while preserving the event in the stream for audit trail purposes.
// Subsequent lazy-loading will result in an empty body field.
// Publishes a MessageDeletedEvent to notify connected clients in real-time.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
// Authorization: Caller must verify CanDeleteAnyMessage OR (CanDeleteOwnMessage AND ownership) before calling.
func (c *ChattoCore) DeleteMessage(ctx context.Context, actorID, spaceID, roomID, messageBodyKey string) error {
	// Get the full message body first to find any attachments
	messageBody, err := c.GetFullMessageBody(ctx, spaceID, messageBodyKey)
	if err != nil {
		return fmt.Errorf("failed to get message body: %w", err)
	}
	if messageBody == nil {
		// Already deleted, nothing to do
		c.logger.Debug("Message body already deleted", "message_body_key", messageBodyKey)
		return nil
	}

	// Delete all attachments from the ObjectStore (supports both NATS and S3)
	for _, attachment := range messageBody.Attachments {
		if err := c.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
			c.logger.Warn("Failed to delete attachment during message deletion",
				"attachment_id", attachment.Id,
				"message_body_key", messageBodyKey,
				"error", err)
			// Continue deleting other attachments even if one fails
		}
	}

	// Delete the message body from KV
	bucket := c.storage.serverBodiesKV

	err = bucket.Delete(ctx, messageBodyKey)
	if err != nil {
		return fmt.Errorf("failed to delete message body: %w", err)
	}

	c.logger.Info("Message body deleted", "space_id", spaceID, "room_id", roomID, "message_body_key", messageBodyKey, "actor_id", actorID, "attachments_deleted", len(messageBody.Attachments))

	// Publish live event to notify connected clients
	c.publishMessageDeletedEvent(ctx, spaceID, roomID, messageBodyKey, actorID)

	return nil
}

// publishMessageDeletedEvent publishes a MessageDeletedEvent directly to the live subject space.
// This notifies connected clients that a message has been deleted so they can update their UI.
func (c *ChattoCore) publishMessageDeletedEvent(ctx context.Context, spaceID, roomID, messageBodyID, userID string) {
	messageEventID := eventIDFromBodyKey(messageBodyID)
	event := newSpaceEvent(userID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_MessageDeleted{
			MessageDeleted: &corev1.MessageDeletedEvent{
				SpaceId:        spaceID,
				RoomId:         roomID,
				MessageBodyId:  messageBodyID,
				MessageEventId: messageEventID,
			},
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "message_deleted")
	if err := c.publishLiveSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish message deleted event", "error", err)
	}
}

// EditMessage edits a message body. Updates the body content and sets updated_at.
// Publishes a MessageUpdatedEvent to notify connected clients in real-time.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
//
// Business rule: Authors can only edit their own messages within MessageEditWindow (3 hours).
// Non-authors (moderators with message.edit.any) can edit at any time.
//
// Authorization: Caller must verify CanEditOwnMessage or CanEditAnyMessage before calling.
func (c *ChattoCore) EditMessage(ctx context.Context, actorID, spaceID, roomID, messageBodyKey, newBody string) error {
	bucket := c.storage.serverBodiesKV

	// Get message with revision for optimistic locking
	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrMessageNotFound
		}
		return fmt.Errorf("failed to get message body: %w", err)
	}

	// Unmarshal the message body
	messageBody := &corev1.MessageBody{}
	if err := proto.Unmarshal(entry.Value(), messageBody); err != nil {
		return fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Business rule: authors can only edit within the edit window
	// Non-authors (moderators) can edit at any time
	isAuthorEdit := messageBody.AuthorId == actorID
	if isAuthorEdit && time.Since(messageBody.CreatedAt.AsTime()) > MessageEditWindow {
		return ErrEditWindowExpired
	}

	// Update the message body with new encrypted content
	messageBody.UpdatedAt = timestamppb.Now()

	// Encrypt with the author's key and a new nonce
	key, err := c.encryption.keyManager.GetUserKey(ctx, messageBody.AuthorId)
	if err != nil {
		return fmt.Errorf("failed to get encryption key: %w", err)
	}
	if key == nil {
		return fmt.Errorf("cannot edit: encryption key not found (message was crypto-shredded)")
	}

	encrypted, err := encryption.Encrypt(key, []byte(newBody))
	if err != nil {
		return fmt.Errorf("failed to encrypt message body: %w", err)
	}

	messageBody.EncryptedBody = encrypted.Ciphertext
	messageBody.EncryptionNonce = encrypted.Nonce

	// Marshal and store with optimistic locking
	data, err := proto.Marshal(messageBody)
	if err != nil {
		return fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Update(ctx, messageBodyKey, data, entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Concurrent modification - could retry, but for now just fail
			return fmt.Errorf("message was modified concurrently")
		}
		return fmt.Errorf("failed to update message body: %w", err)
	}

	c.logger.Info("Message body edited", "space_id", spaceID, "room_id", roomID, "message_body_key", messageBodyKey, "actor_id", actorID)

	// Publish live event to notify connected clients
	c.publishMessageUpdatedEvent(ctx, spaceID, roomID, messageBodyKey, actorID)

	return nil
}

// DeleteAttachmentFromMessage deletes a single attachment from a message.
// Only the message author can delete their attachments.
// Removes the attachment from the MessageBody and deletes the file from ObjectStore.
// Publishes a MessageUpdatedEvent to notify connected clients in real-time.
// The messageBodyKey parameter is the full compound key ({userId}.{bodyId}) stored in the event.
func (c *ChattoCore) DeleteAttachmentFromMessage(ctx context.Context, actorID, spaceID, roomID, messageBodyKey, attachmentID string) error {
	bucket := c.storage.serverBodiesKV

	// Get message with revision for optimistic locking
	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrMessageNotFound
		}
		return fmt.Errorf("failed to get message body: %w", err)
	}

	// Unmarshal the message body
	messageBody := &corev1.MessageBody{}
	if err := proto.Unmarshal(entry.Value(), messageBody); err != nil {
		return fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Check ownership - only the author can delete their attachments
	if messageBody.AuthorId != actorID {
		return ErrNotMessageAuthor
	}

	// Find and remove the attachment from the slice
	attachmentIndex := -1
	for i, att := range messageBody.Attachments {
		if att.Id == attachmentID {
			attachmentIndex = i
			break
		}
	}
	if attachmentIndex == -1 {
		return fmt.Errorf("attachment not found in message")
	}

	// Save reference before removing from slice (needed for storage-aware deletion)
	removedAttachment := messageBody.Attachments[attachmentIndex]

	// Remove the attachment from the slice
	messageBody.Attachments = append(messageBody.Attachments[:attachmentIndex], messageBody.Attachments[attachmentIndex+1:]...)
	messageBody.UpdatedAt = timestamppb.Now()

	// Marshal and store with optimistic locking
	data, err := proto.Marshal(messageBody)
	if err != nil {
		return fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Update(ctx, messageBodyKey, data, entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("message was modified concurrently")
		}
		return fmt.Errorf("failed to update message body: %w", err)
	}

	// Delete the attachment file from storage (supports both NATS and S3)
	if err := c.DeleteAttachmentFromStorage(ctx, removedAttachment); err != nil {
		c.logger.Warn("Failed to delete attachment file after removing from message",
			"attachment_id", attachmentID,
			"message_body_key", messageBodyKey,
			"error", err)
		// Don't fail the operation - the attachment reference is already removed
	}

	c.logger.Info("Attachment deleted from message",
		"space_id", spaceID,
		"room_id", roomID,
		"message_body_key", messageBodyKey,
		"attachment_id", attachmentID,
		"actor_id", actorID)

	// Publish live event to notify connected clients
	c.publishMessageUpdatedEvent(ctx, spaceID, roomID, messageBodyKey, actorID)

	return nil
}

// DeleteLinkPreviewFromMessage removes a link preview from a message.
// Only the message author can delete link previews from their messages.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) DeleteLinkPreviewFromMessage(ctx context.Context, actorID, spaceID, roomID, messageBodyKey, previewURL string) error {
	bucket := c.storage.serverBodiesKV

	// Get message with revision for optimistic locking
	entry, err := bucket.Get(ctx, messageBodyKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrMessageNotFound
		}
		return fmt.Errorf("failed to get message body: %w", err)
	}

	// Unmarshal the message body
	messageBody := &corev1.MessageBody{}
	if err := proto.Unmarshal(entry.Value(), messageBody); err != nil {
		return fmt.Errorf("failed to unmarshal message body: %w", err)
	}

	// Check ownership - only the author can delete their link preview
	if messageBody.AuthorId != actorID {
		return ErrNotMessageAuthor
	}

	// Verify the preview exists and matches the requested URL
	if messageBody.LinkPreview == nil || messageBody.LinkPreview.Url != previewURL {
		return fmt.Errorf("link preview not found in message")
	}

	// Remove the link preview
	messageBody.LinkPreview = nil
	messageBody.UpdatedAt = timestamppb.Now()

	// Marshal and store with optimistic locking
	data, err := proto.Marshal(messageBody)
	if err != nil {
		return fmt.Errorf("failed to marshal message body: %w", err)
	}

	_, err = bucket.Update(ctx, messageBodyKey, data, entry.Revision())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return fmt.Errorf("message was modified concurrently, please retry")
		}
		return fmt.Errorf("failed to update message body: %w", err)
	}

	c.logger.Info("Link preview deleted from message",
		"space_id", spaceID,
		"room_id", roomID,
		"message_body_key", messageBodyKey,
		"preview_url", previewURL,
		"actor_id", actorID)

	// Publish live event to notify connected clients
	c.publishMessageUpdatedEvent(ctx, spaceID, roomID, messageBodyKey, actorID)

	return nil
}

// publishMessageUpdatedEvent publishes a MessageUpdatedEvent directly to the live subject space.
// This notifies connected clients that a message has been edited so they can update their UI.
func (c *ChattoCore) publishMessageUpdatedEvent(ctx context.Context, spaceID, roomID, messageBodyID, userID string) {
	messageEventID := eventIDFromBodyKey(messageBodyID)
	event := newSpaceEvent(userID, &corev1.SpaceEvent{
		Event: &corev1.SpaceEvent_MessageUpdated{
			MessageUpdated: &corev1.MessageUpdatedEvent{
				SpaceId:        spaceID,
				RoomId:         roomID,
				MessageBodyId:  messageBodyID,
				MessageEventId: messageEventID,
			},
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "message_updated")
	if err := c.publishLiveSpaceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish message updated event", "error", err)
	}
}

// deleteUserMessageBodiesInSpace deletes all message bodies authored by a user in a specific space.
// This is used during account deletion to remove the user's message content entirely.
// Returns the number of message bodies deleted.
// Note: This only removes bodies from spaces the user was a member of. Bodies in spaces they
// left before deletion will still be crypto-shredded when the encryption key is deleted.
//
// The key format is {userId}.{bodyId}, so we can efficiently filter by userId prefix
// to find only this user's message bodies without scanning the entire bucket.
func (c *ChattoCore) deleteUserMessageBodiesInSpace(ctx context.Context, userID, spaceID string) (int, error) {
	bucket := c.storage.serverBodiesKV

	// Use prefix filter to find only this user's message bodies
	// Key format: {userId}.{bodyId} - filter by userID prefix
	lister, err := bucket.ListKeysFiltered(ctx, userID+".")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil // No bodies for this user in this space
		}
		return 0, fmt.Errorf("failed to list message body keys: %w", err)
	}

	// Collect all keys first (iterator becomes invalid after first pass)
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	deleted := 0
	for _, key := range keys {
		// Get the message body to find attachments to delete
		entry, err := bucket.Get(ctx, key)
		if err != nil {
			c.logger.Debug("Failed to get message body during deletion", "key", key, "error", err)
			continue
		}

		var messageBody corev1.MessageBody
		if err := proto.Unmarshal(entry.Value(), &messageBody); err != nil {
			c.logger.Debug("Failed to unmarshal message body during deletion", "key", key, "error", err)
			continue
		}

		// Delete all attachments from storage (supports both NATS and S3)
		for _, attachment := range messageBody.Attachments {
			if err := c.DeleteAttachmentFromStorage(ctx, attachment); err != nil {
				c.logger.Warn("Failed to delete attachment during user deletion",
					"attachment_id", attachment.Id,
					"message_body_key", key,
					"error", err)
				// Continue deleting other attachments
			}
		}

		// Delete the message body
		if err := bucket.Delete(ctx, key); err != nil {
			c.logger.Warn("Failed to delete message body during user deletion", "key", key, "error", err)
			continue
		}

		deleted++
	}

	return deleted, nil
}

// GetRoomEvents fetches historical events for a specific room from the SPACE stream.
// Returns up to 'limit' most recent events. If 'beforeSeq' is provided, fetches events
// strictly older than that JetStream sequence. Uses sequence-based lookups for both
// initial load and pagination, with a small-room fast path when the total event count
// fits in one fetch. Message bodies are lazy-loaded via GraphQL resolvers.
func (c *ChattoCore) GetRoomEvents(ctx context.Context, space_id, room_id string, limit int, beforeSeq *uint64) (*RoomEventsResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	stream := c.storage.serverEventsStream

	// Filter for root messages and meta events only (excludes thread replies).
	// "msg.*" matches root messages; "meta" matches room lifecycle events (joins, leaves, etc.)
	filterSubjects := subjects.RoomRootEventsFilters(kindForSpace(space_id), room_id)

	// --- Small room fast path ---
	// Check total room event count (uses "room.>" which includes thread replies,
	// so the count may slightly overestimate — that's fine for this decision).
	roomAllSubject := subjects.RoomAllEvents(kindForSpace(space_id), room_id)
	streamInfo, err := stream.Info(ctx, jetstream.WithSubjectFilter(roomAllSubject))
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	// Sum the per-subject counts to get the room-specific event count.
	// State.Msgs is the total stream count (all rooms); State.Subjects
	// contains only subjects matching our WithSubjectFilter.
	var roomMsgCount uint64
	for _, count := range streamInfo.State.Subjects {
		roomMsgCount += count
	}

	if roomMsgCount == 0 {
		return &RoomEventsResult{}, nil
	}

	if int(roomMsgCount) <= limit {
		// Room has very few events — fetch everything in one shot
		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverAllPolicy,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, beforeSeq)
		if err != nil {
			return nil, err
		}
		hasOlder := len(events) > limit
		if hasOlder {
			events = events[len(events)-limit:]
		}
		c.logger.Debug("Fetched room events (small room fast path)", "space_id", space_id, "room_id", room_id, "count", len(events))
		return roomEventsResult(events, hasOlder, beforeSeq != nil), nil
	}

	// --- Large room paths ---
	if beforeSeq == nil {
		return c.getRoomEventsInitialLoad(ctx, stream, space_id, room_id, limit, filterSubjects, streamInfo)
	}
	return c.getRoomEventsPagination(ctx, stream, space_id, room_id, limit, *beforeSeq, filterSubjects, streamInfo)
}

// getRoomEventsInitialLoad fetches the most recent events using sequence-based start.
// Uses GetLastMsgForSubject to find the room's last event, then starts a consumer
// close to the end to avoid scanning the entire stream.
func (c *ChattoCore) getRoomEventsInitialLoad(
	ctx context.Context,
	stream jetstream.Stream,
	space_id, room_id string,
	limit int,
	filterSubjects []string,
	streamInfo *jetstream.StreamInfo,
) (*RoomEventsResult, error) {
	// Find the last sequence for this room's root messages and meta events.
	// Both are O(1) lookups in JetStream's subject index.
	msgSubject := subjects.RoomRootMessages(kindForSpace(space_id), room_id)
	metaSubject := subjects.RoomMeta(kindForSpace(space_id), room_id)

	var lastSeq uint64

	lastMsg, err := stream.GetLastMsgForSubject(ctx, msgSubject)
	if err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		return nil, fmt.Errorf("failed to get last message: %w", err)
	}
	if lastMsg != nil {
		lastSeq = lastMsg.Sequence
	}

	lastMeta, err := stream.GetLastMsgForSubject(ctx, metaSubject)
	if err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		return nil, fmt.Errorf("failed to get last meta event: %w", err)
	}
	if lastMeta != nil && lastMeta.Sequence > lastSeq {
		lastSeq = lastMeta.Sequence
	}

	if lastSeq == 0 {
		return &RoomEventsResult{}, nil
	}

	firstSeq := streamInfo.State.FirstSeq

	// Start the consumer at an estimated position near the end.
	// The stream contains events from ALL rooms in the space, so there may be
	// non-matching events between our target events. The multiplier accounts for this.
	multipliers := []uint64{3, 10, 50}
	for _, mult := range multipliers {
		startSeq := lastSeq - uint64(limit)*mult + 1
		if startSeq < firstSeq || startSeq > lastSeq {
			startSeq = firstSeq
		}

		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:       startSeq,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, nil)
		if err != nil {
			return nil, err
		}

		if len(events) >= limit || startSeq == firstSeq {
			// Got enough events, or we've already scanned from the beginning
			hasOlder := len(events) > limit || startSeq > firstSeq
			if len(events) > limit {
				events = events[len(events)-limit:]
			}
			c.logger.Debug("Fetched room events (initial load)", "space_id", space_id, "room_id", room_id, "count", len(events), "multiplier", mult)
			return roomEventsResult(events, hasOlder, false), nil
		}
		// Not enough events — widen the range and retry
	}

	// Shouldn't reach here, but fall back to DeliverAllPolicy
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, nil)
	if err != nil {
		return nil, err
	}
	hasOlder := len(events) > limit
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	c.logger.Debug("Fetched room events (initial load fallback)", "space_id", space_id, "room_id", room_id, "count", len(events))
	return roomEventsResult(events, hasOlder, false), nil
}

// getRoomEventsPagination fetches older events before a sequence cursor.
// Uses the same multiplier-based seeking as the initial-load path: start the
// consumer at `beforeSeq - limit*mult` and widen if we don't get enough
// matching events. The post-filter inside fetchRoomEventsWithConsumer ensures
// only events with sequence < beforeSeq are returned.
func (c *ChattoCore) getRoomEventsPagination(
	ctx context.Context,
	stream jetstream.Stream,
	space_id, room_id string,
	limit int,
	beforeSeq uint64,
	filterSubjects []string,
	streamInfo *jetstream.StreamInfo,
) (*RoomEventsResult, error) {
	firstSeq := streamInfo.State.FirstSeq

	if beforeSeq <= firstSeq {
		// Cursor points to or past the start of the stream — nothing older.
		return &RoomEventsResult{HasNewer: true}, nil
	}

	multipliers := []uint64{3, 10, 50}
	for _, mult := range multipliers {
		startSeq := beforeSeq - uint64(limit)*mult
		if startSeq < firstSeq || startSeq >= beforeSeq {
			startSeq = firstSeq
		}

		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:       startSeq,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, &beforeSeq)
		if err != nil {
			return nil, err
		}

		if len(events) >= limit || startSeq == firstSeq {
			hasOlder := len(events) > limit || startSeq > firstSeq
			if len(events) > limit {
				events = events[len(events)-limit:]
			}
			c.logger.Debug("Fetched room events (pagination)", "space_id", space_id, "room_id", room_id, "count", len(events), "multiplier", mult)
			result := roomEventsResult(events, hasOlder, true)
			return result, nil
		}
	}

	// Fallback: full scan with cursor filter
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, &beforeSeq)
	if err != nil {
		return nil, err
	}
	hasOlder := len(events) > limit
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	c.logger.Debug("Fetched room events (pagination fallback)", "space_id", space_id, "room_id", room_id, "count", len(events))
	return roomEventsResult(events, hasOlder, true), nil
}

// roomEventsResult assembles a RoomEventsResult and computes the start/end
// cursor sequences from the events slice. Returns a result with zero
// cursors if events is empty.
func roomEventsResult(events []*RoomEvent, hasOlder, hasNewer bool) *RoomEventsResult {
	r := &RoomEventsResult{
		Events:   events,
		HasOlder: hasOlder,
		HasNewer: hasNewer,
	}
	if len(events) > 0 {
		r.StartCursorSeq = events[0].Sequence
		r.EndCursorSeq = events[len(events)-1].Sequence
	}
	return r
}

// fetchRoomEventsWithConsumer creates an ephemeral consumer, fetches all matching events,
// filters them by sequence cursor, and cleans up the consumer. This is the shared fetch
// logic used by all GetRoomEvents code paths. If beforeSeq is non-nil, only events with
// JetStream stream sequence strictly less than *beforeSeq are returned.
func (c *ChattoCore) fetchRoomEventsWithConsumer(
	ctx context.Context,
	stream jetstream.Stream,
	filterSubjects []string,
	config jetstream.ConsumerConfig,
	beforeSeq *uint64,
) ([]*RoomEvent, error) {
	consumer, err := stream.CreateConsumer(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}
	defer stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer info: %w", err)
	}

	numPending := info.NumPending
	if numPending == 0 {
		return nil, nil
	}

	msgs, err := consumer.Fetch(int(numPending), jetstream.FetchMaxWait(5*time.Second))
	if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	var events []*RoomEvent
	if msgs != nil {
		for msg := range msgs.Messages() {
			meta, err := msg.Metadata()
			if err != nil {
				continue
			}
			seq := meta.Sequence.Stream

			// Filter: only include events strictly before the cursor sequence.
			if beforeSeq != nil && seq >= *beforeSeq {
				continue
			}

			var event corev1.SpaceEvent
			if err := proto.Unmarshal(msg.Data(), &event); err != nil {
				continue
			}

			// Skip events with unknown/removed inner types (e.g., old ThreadReplyEchoEvent)
			if event.Event == nil {
				continue
			}

			events = append(events, &RoomEvent{SpaceEvent: &event, Sequence: seq})
		}
	}

	return events, nil
}

// RoomEvent pairs a SpaceEvent with its JetStream stream sequence so the
// pagination layer can build opaque cursors without re-deriving the
// sequence per event. SpaceEvent is embedded so callers can access event
// fields directly (`event.Id`, `event.GetMessagePosted()`, etc.).
type RoomEvent struct {
	*corev1.SpaceEvent
	Sequence uint64
}

// RoomEventsResult is the return type for paginated room event queries.
// HasOlder/HasNewer indicate whether more events exist beyond the
// returned page. StartCursorSeq/EndCursorSeq are the JetStream sequences
// of the first and last event in the page; the GraphQL layer renders
// them as opaque cursor strings. Both are zero when Events is empty.
type RoomEventsResult struct {
	Events         []*RoomEvent
	HasOlder       bool
	HasNewer       bool
	StartCursorSeq uint64
	EndCursorSeq   uint64
}

// RoomEventsAroundResult contains the result of fetching events around a target event.
type RoomEventsAroundResult struct {
	Events      []*RoomEvent
	TargetIndex int
	HasOlder    bool
	HasNewer    bool
}

// GetRoomEventsAround fetches room events centered around a specific event.
// Returns a window of events with the target event roughly in the middle.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventsAround(ctx context.Context, spaceID, roomID, eventID string, limit int) (*RoomEventsAroundResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	// 1. Look up the target event's JetStream sequence (O(1) subject lookup)
	targetSeq, err := c.GetEventSequence(ctx, spaceID, roomID, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target event sequence: %w", err)
	}
	if targetSeq == 0 {
		return nil, fmt.Errorf("event not found: %s", eventID)
	}

	// 2. Get the stream and filter subjects
	stream := c.storage.serverEventsStream

	filterSubjects := subjects.RoomRootEventsFilters(kindForSpace(spaceID), roomID)

	streamInfo, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}
	firstSeq := streamInfo.State.FirstSeq

	// 3. Use progressive multiplier pattern to fetch events around the target.
	// We want limit/2 events before the target and limit/2 after.
	// The stream is shared across all rooms, so we need to over-fetch.
	halfLimit := limit / 2

	multipliers := []uint64{3, 10, 50}
	for _, mult := range multipliers {
		// Start well before the target to ensure we get enough events before it
		startSeq := targetSeq - uint64(halfLimit)*mult
		if startSeq < firstSeq || startSeq > targetSeq {
			startSeq = firstSeq
		}

		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:       startSeq,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, nil) // No end time filter — we want events after the target too
		if err != nil {
			return nil, err
		}

		// Find the target event in the fetched results
		targetIdx := -1
		for i, e := range events {
			if e.Id == eventID {
				targetIdx = i
				break
			}
		}

		if targetIdx == -1 {
			// Target not found in this fetch window — widen and retry
			if startSeq == firstSeq {
				// Already scanning from the beginning — target must not match filters
				return nil, fmt.Errorf("event %s not found in room root events", eventID)
			}
			continue
		}

		// We have enough events before the target (or started from the beginning)
		beforeCount := targetIdx
		if beforeCount >= halfLimit || startSeq == firstSeq {
			// Slice the window: halfLimit before + target + halfLimit after
			windowStart := targetIdx - halfLimit
			if windowStart < 0 {
				windowStart = 0
			}
			windowEnd := targetIdx + halfLimit + 1
			if windowEnd > len(events) {
				windowEnd = len(events)
			}

			windowEvents := events[windowStart:windowEnd]
			newTargetIdx := targetIdx - windowStart

			return &RoomEventsAroundResult{
				Events:      windowEvents,
				TargetIndex: newTargetIdx,
				HasOlder:    windowStart > 0 || startSeq > firstSeq,
				HasNewer:    windowEnd < len(events),
			}, nil
		}
		// Not enough events before target — widen and retry
	}

	// Fallback: scan from beginning
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, nil)
	if err != nil {
		return nil, err
	}

	targetIdx := -1
	for i, e := range events {
		if e.Id == eventID {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return nil, fmt.Errorf("event %s not found in room root events", eventID)
	}

	windowStart := targetIdx - halfLimit
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := targetIdx + halfLimit + 1
	if windowEnd > len(events) {
		windowEnd = len(events)
	}

	windowEvents := events[windowStart:windowEnd]
	newTargetIdx := targetIdx - windowStart

	c.logger.Debug("Fetched room events around target (fallback)", "space_id", spaceID, "room_id", roomID, "count", len(windowEvents))
	return &RoomEventsAroundResult{
		Events:      windowEvents,
		TargetIndex: newTargetIdx,
		HasOlder:    windowStart > 0,
		HasNewer:    windowEnd < len(events),
	}, nil
}

// GetRoomEventsAfter fetches room events after a given sequence cursor.
// Used for forward pagination in "jump to message" mode.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventsAfter(ctx context.Context, spaceID, roomID string, afterSeq uint64, limit int) (*RoomEventsResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	stream := c.storage.serverEventsStream

	filterSubjects := subjects.RoomRootEventsFilters(kindForSpace(spaceID), roomID)

	// Start the consumer at the sequence immediately after the cursor.
	// JetStream returns messages with stream sequence >= OptStartSeq, so
	// `afterSeq + 1` excludes the cursor event itself.
	startSeq := afterSeq + 1
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:       startSeq,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, nil)
	if err != nil {
		return nil, err
	}

	// Take the first `limit` events (forward pagination)
	hasNewer := len(events) > limit
	if hasNewer {
		events = events[:limit]
	}

	c.logger.Debug("Fetched room events after cursor", "space_id", spaceID, "room_id", roomID, "count", len(events))
	r := &RoomEventsResult{
		Events:   events,
		HasOlder: true, // Forward pagination always has older events (those before the cursor)
		HasNewer: hasNewer,
	}
	if len(events) > 0 {
		r.StartCursorSeq = events[0].Sequence
		r.EndCursorSeq = events[len(events)-1].Sequence
	}
	return r, nil
}



// getRoomEventMsg fetches the raw JetStream message for an event by its event ID.
// Supports both root messages and thread replies via O(1) subject lookup.
// Returns nil if the event doesn't exist.
func (c *ChattoCore) getRoomEventMsg(ctx context.Context, spaceID, roomID, eventID string) (*jetstream.RawStreamMsg, error) {
	stream := c.storage.serverEventsStream

	// First, try root message subject pattern: space.{s}.room.{r}.msg.{eventId}
	subject := subjects.RoomMessage(kindForSpace(spaceID), roomID, eventID)
	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	if err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		return nil, fmt.Errorf("failed to get message by subject: %w", err)
	}

	// If not found as root message, try thread reply pattern: space.{s}.room.{r}.thread.*.{eventId}
	if msg == nil {
		threadSubject := subjects.RoomThreadLookup(kindForSpace(spaceID), roomID, eventID)
		msg, err = stream.GetLastMsgForSubject(ctx, threadSubject)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get thread message by subject: %w", err)
		}
	}

	return msg, nil
}

// GetRoomEventByEventID fetches a room event by its event ID using O(1) subject lookup.
// Supports both root messages and thread replies.
// Returns nil if the event doesn't exist.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventByEventID(ctx context.Context, spaceID, roomID, eventID string) (*corev1.SpaceEvent, error) {
	msg, err := c.getRoomEventMsg(ctx, spaceID, roomID, eventID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}

	var event corev1.SpaceEvent
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Return nil for events with unknown/removed inner types (e.g., old ThreadReplyEchoEvent)
	if event.Event == nil {
		return nil, nil
	}

	return &event, nil
}

// GetEventSequence returns the JetStream stream sequence number for an event by its event ID.
// Returns 0 if the event doesn't exist.
func (c *ChattoCore) GetEventSequence(ctx context.Context, spaceID, roomID, eventID string) (uint64, error) {
	msg, err := c.getRoomEventMsg(ctx, spaceID, roomID, eventID)
	if err != nil {
		return 0, err
	}
	if msg == nil {
		return 0, nil
	}
	return msg.Sequence, nil
}

// GetThreadEvents fetches all events for a specific thread.
// Returns the root message followed by all replies in chronological order.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetThreadEvents(ctx context.Context, space_id, room_id string, threadRootEventId string) ([]*corev1.SpaceEvent, error) {
	stream := c.storage.serverEventsStream

	// 1. First, fetch the root message by event ID
	rootEvent, err := c.GetRoomEventByEventID(ctx, space_id, room_id, threadRootEventId)
	if err != nil {
		return nil, fmt.Errorf("failed to get root message: %w", err)
	}
	if rootEvent == nil {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventId)
	}

	// Verify it's actually a message (not some other event type)
	if rootEvent.GetMessagePosted() == nil {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventId)
	}

	// 2. Fetch all thread replies using subject filter
	// Thread replies are published to: space.{s}.room.{r}.msg.{rootEventId}.replies.{eventId}
	threadFilterSubject := subjects.RoomThreadFilter(kindForSpace(space_id), room_id, threadRootEventId)

	// Create ephemeral consumer to fetch all thread replies
	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject:     threadFilterSubject,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	})
	if err != nil {
		// If consumer creation fails, still return the root message
		c.logger.Warn("Failed to create thread consumer", "error", err)
		return []*corev1.SpaceEvent{rootEvent}, nil
	}

	// Ensure consumer is deleted when we're done
	consumerName := consumer.CachedInfo().Name
	defer func() {
		if err := stream.DeleteConsumer(ctx, consumerName); err != nil {
			c.logger.Debug("Failed to delete thread consumer", "consumer", consumerName, "error", err)
		}
	}()

	// Collect all thread replies by fetching in batches until exhausted
	events := []*corev1.SpaceEvent{rootEvent}
	const batchSize = 500

	for {
		msgs, err := consumer.Fetch(batchSize, jetstream.FetchMaxWait(100*time.Millisecond))
		if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
			c.logger.Warn("Failed to fetch thread replies", "error", err)
			break
		}

		if msgs == nil {
			break
		}

		fetchedCount := 0
		for msg := range msgs.Messages() {
			fetchedCount++

			var event corev1.SpaceEvent
			if err := proto.Unmarshal(msg.Data(), &event); err != nil {
				msg.Ack()
				continue
			}

			// Skip events with unknown/removed inner types (e.g., old ThreadReplyEchoEvent)
			if event.Event == nil {
				msg.Ack()
				continue
			}

			events = append(events, &event)
			msg.Ack()
		}

		// If we got fewer messages than batch size, we've exhausted the stream
		if fetchedCount < batchSize {
			break
		}
	}

	c.logger.Debug("Fetched thread events", "space_id", space_id, "room_id", room_id, "thread_root_event_id", threadRootEventId, "count", len(events))

	return events, nil
}

// ThreadMetadata contains reply count, last reply timestamp, and participants for a thread.
type ThreadMetadata struct {
	ReplyCount     int
	LastReplyAt    *time.Time
	ParticipantIDs []string
}

// FollowedThread represents a thread the user is following, enriched with metadata for display.
type FollowedThread struct {
	SpaceID           string
	RoomID            string
	ThreadRootEventID string
	ReplyCount        int
	LastReplyAt       *time.Time
	ParticipantIDs    []string
	HasUnread         bool
}

// threadMetadataKey returns the KV key for thread metadata: {roomId}.{rootEventId}
func threadMetadataKey(roomID string, rootEventId string) string {
	return fmt.Sprintf("%s.%s", roomID, rootEventId)
}

// maxThreadParticipants is the maximum number of participant IDs tracked per thread.
const maxThreadParticipants = 50

// updateThreadMetadata updates the thread metadata in KV with optimistic locking.
// Called when a reply is posted to a thread. Tracks reply count, last reply time, and participants.
// The rootAuthorID is the author of the thread root message - they're included as the first participant.
func (c *ChattoCore) updateThreadMetadata(ctx context.Context, spaceID, roomID string, rootEventId string, rootAuthorID, replyAuthorID string, replyTime time.Time) error {
	const maxRetries = 5

	bucket := c.storage.serverThreadsKV

	key := threadMetadataKey(roomID, rootEventId)

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get current entry (if any) with its revision
		var revision uint64
		var metadata *corev1.ThreadMetadata

		entry, err := bucket.Get(ctx, key)
		if err == nil {
			// Key exists - unmarshal and get revision
			revision = entry.Revision()
			metadata = &corev1.ThreadMetadata{}
			if unmarshalErr := proto.Unmarshal(entry.Value(), metadata); unmarshalErr != nil {
				c.logger.Warn("Failed to unmarshal thread metadata, creating new", "error", unmarshalErr)
				metadata = nil
			}
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to get thread metadata: %w", err)
		}

		// Create or update metadata
		if metadata == nil {
			// First reply to this thread - initialize participant list with root author first
			participants := []string{}
			if rootAuthorID != "" {
				participants = append(participants, rootAuthorID)
			}
			// Add reply author if different from root author
			if replyAuthorID != rootAuthorID {
				participants = append(participants, replyAuthorID)
			}
			metadata = &corev1.ThreadMetadata{
				RootEventId:    rootEventId,
				ReplyCount:     1,
				LastReplyAt:    timestamppb.New(replyTime),
				ParticipantIds: participants,
			}
		} else {
			metadata.ReplyCount++
			metadata.LastReplyAt = timestamppb.New(replyTime)

			// Ensure root author is in participants (for backward compatibility with existing threads)
			if rootAuthorID != "" {
				rootAuthorExists := false
				for _, pid := range metadata.ParticipantIds {
					if pid == rootAuthorID {
						rootAuthorExists = true
						break
					}
				}
				if !rootAuthorExists && len(metadata.ParticipantIds) < maxThreadParticipants {
					// Insert root author at the beginning
					metadata.ParticipantIds = append([]string{rootAuthorID}, metadata.ParticipantIds...)
				}
			}

			// Add reply author if not already present and under cap
			replyAuthorExists := false
			for _, pid := range metadata.ParticipantIds {
				if pid == replyAuthorID {
					replyAuthorExists = true
					break
				}
			}
			if !replyAuthorExists && len(metadata.ParticipantIds) < maxThreadParticipants {
				metadata.ParticipantIds = append(metadata.ParticipantIds, replyAuthorID)
			}
		}

		// Marshal and store
		data, err := proto.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal thread metadata: %w", err)
		}

		// Try atomic update
		var updateErr error
		if revision == 0 {
			// No existing key - use Create for atomic insert
			_, updateErr = bucket.Create(ctx, key, data)
		} else {
			// Existing key - use Update with revision check
			_, updateErr = bucket.Update(ctx, key, data, revision)
		}

		if updateErr == nil {
			c.logger.Debug("Updated thread metadata",
				"space_id", spaceID,
				"room_id", roomID,
				"root_event_id", rootEventId,
				"reply_count", metadata.ReplyCount,
				"participants", len(metadata.ParticipantIds))
			return nil
		}

		// Check if it's a revision conflict (concurrent update)
		if errors.Is(updateErr, jetstream.ErrKeyExists) {
			c.logger.Debug("Thread metadata revision conflict, retrying",
				"room_id", roomID,
				"root_event_id", rootEventId,
				"attempt", attempt+1)
			continue
		}

		// Some other error
		return fmt.Errorf("failed to store thread metadata: %w", updateErr)
	}

	return fmt.Errorf("failed to update thread metadata after %d retries due to concurrent modifications", maxRetries)
}

// notifyThreadFollowers creates persistent notifications for all thread followers when someone replies.
// Followers are users who have explicitly or automatically followed the thread (stored in RUNTIME KV).
// Users in skipIDs are excluded (e.g., already notified via inReplyTo).
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifyThreadFollowers(ctx context.Context, spaceID, roomID, replyAuthorID, replyEventID, threadRootID string, skipIDs []string) {
	// Get all users following this thread
	followerIDs, err := c.GetThreadFollowers(ctx, spaceID, roomID, threadRootID)
	if err != nil {
		c.logger.Warn("Failed to get thread followers for notification",
			"thread_root_id", threadRootID,
			"error", err)
		return
	}

	// Build skip set for O(1) lookups
	skipSet := make(map[string]bool, len(skipIDs))
	for _, id := range skipIDs {
		skipSet[id] = true
	}

	// Notify each follower except the reply author and skipped users
	notifiedCount := 0
	for _, followerID := range followerIDs {
		// Don't notify the person who posted the reply
		if followerID == replyAuthorID {
			continue
		}

		// Skip users already notified via other means (e.g., inReplyTo)
		if skipSet[followerID] {
			continue
		}

		// Skip if user has muted this room
		level, err := c.GetEffectiveNotificationLevel(ctx, spaceID, followerID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for thread follower, continuing",
				"user_id", followerID, "error", err)
		} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}

		// Create persistent notification (for bell icon and notification center)
		// This also publishes NotificationCreatedEvent for real-time updates
		_, err = c.CreateNotification(ctx, followerID, replyAuthorID, &corev1.Notification{
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{
					SpaceId:     spaceID,
					RoomId:      roomID,
					EventId:     replyEventID,
					InReplyToId: threadRootID,
					InThread:    threadRootID,
				},
			},
		})
		if err != nil {
			c.logger.Warn("Failed to create reply notification",
				"recipient_id", followerID,
				"reply_author_id", replyAuthorID,
				"space_id", spaceID,
				"room_id", roomID,
				"error", err)
		} else {
			notifiedCount++
		}
	}

	if notifiedCount > 0 {
		c.logger.Debug("Created reply notifications for thread followers",
			"thread_root_id", threadRootID,
			"reply_author_id", replyAuthorID,
			"notified_count", notifiedCount,
			"space_id", spaceID,
			"room_id", roomID)
	}
}

// notifyInReplyToAuthor creates a persistent notification for the author of a message
// that received a reply (via inReplyTo). Works for both room-level and in-thread replies.
// Returns the notified user ID so the caller can add it to the already-notified set,
// or empty string if no notification was sent.
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifyInReplyToAuthor(ctx context.Context, spaceID, roomID, replyAuthorID, replyEventID, inReplyToEventID, inThread string, alreadyNotifiedIDs []string) string {
	// Look up the original message to find its author
	originalEvent, err := c.GetRoomEventByEventID(ctx, spaceID, roomID, inReplyToEventID)
	if err != nil || originalEvent == nil {
		c.logger.Warn("Failed to get in-reply-to message for notification",
			"in_reply_to_id", inReplyToEventID,
			"error", err)
		return ""
	}

	originalAuthorID := originalEvent.ActorId
	if originalAuthorID == "" {
		return ""
	}

	// Don't notify yourself
	if originalAuthorID == replyAuthorID {
		return ""
	}

	// Don't notify if the user was already notified (e.g., via @mention)
	for _, notifiedID := range alreadyNotifiedIDs {
		if notifiedID == originalAuthorID {
			return ""
		}
	}

	// Skip if user has muted this room
	level, err := c.GetEffectiveNotificationLevel(ctx, spaceID, originalAuthorID, roomID)
	if err != nil {
		c.logger.Warn("Failed to get notification level for in-reply-to author, continuing",
			"user_id", originalAuthorID, "error", err)
	} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		return ""
	}

	// Create persistent notification (for bell icon and notification center)
	_, err = c.CreateNotification(ctx, originalAuthorID, replyAuthorID, &corev1.Notification{
		Notification: &corev1.Notification_Reply{
			Reply: &corev1.ReplyNotification{
				SpaceId:     spaceID,
				RoomId:      roomID,
				EventId:     replyEventID,
				InReplyToId: inReplyToEventID,
				InThread:    inThread,
			},
		},
	})
	if err != nil {
		c.logger.Warn("Failed to create in-reply-to notification",
			"recipient_id", originalAuthorID,
			"reply_author_id", replyAuthorID,
			"space_id", spaceID,
			"room_id", roomID,
			"error", err)
		return ""
	}

	c.logger.Debug("Created in-reply-to notification",
		"recipient_id", originalAuthorID,
		"reply_author_id", replyAuthorID,
		"space_id", spaceID,
		"room_id", roomID)

	return originalAuthorID
}

// GetThreadMetadata returns the reply count, last reply timestamp, and participants for a thread root message.
// Returns zero values if the message has no replies.
// Reads from the THREADS KV bucket which is updated on each reply.
func (c *ChattoCore) GetThreadMetadata(ctx context.Context, spaceID, roomID string, rootEventId string) (*ThreadMetadata, error) {
	bucket := c.storage.serverThreadsKV

	entry, err := bucket.Get(ctx, threadMetadataKey(roomID, rootEventId))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// No thread metadata = no replies
			return &ThreadMetadata{ReplyCount: 0}, nil
		}
		return nil, fmt.Errorf("failed to get thread metadata: %w", err)
	}

	var pbMetadata corev1.ThreadMetadata
	if err := proto.Unmarshal(entry.Value(), &pbMetadata); err != nil {
		c.logger.Warn("Failed to unmarshal thread metadata", "error", err)
		return &ThreadMetadata{ReplyCount: 0}, nil
	}

	metadata := &ThreadMetadata{
		ReplyCount:     int(pbMetadata.ReplyCount),
		ParticipantIDs: pbMetadata.ParticipantIds,
	}

	if pbMetadata.LastReplyAt != nil {
		t := pbMetadata.LastReplyAt.AsTime()
		metadata.LastReplyAt = &t
	}

	return metadata, nil
}

// StreamRoomEventsLive creates a continuous stream of live events for a specific room.
// Only delivers new events that occur after subscription starts (no historical fetch).
// For historical events, use GetRoomEvents query instead.
// The returned channel will be closed when the context is cancelled or after unrecoverable errors.
//
// Reliability: Transient JetStream errors (heartbeat missed, leadership change) trigger automatic
// retry with backoff. Terminal errors (connection closed, consumer deleted) close the channel.
// Clients should handle channel closure by resubscribing if they want to continue receiving events.
func (c *ChattoCore) StreamRoomEventsLive(ctx context.Context, space_id, room_id string) (<-chan *corev1.SpaceEvent, error) {
	// Get the space stream (room events are stored in the unified space stream)
	stream := c.storage.serverEventsStream

	// Create an ordered consumer for live events only, filtered to this room
	// InactiveThreshold ensures the consumer is cleaned up if the client disconnects
	filterSubject := subjects.RoomAllEvents(kindForSpace(space_id), room_id)
	cons, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects:    []string{filterSubject},
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ordered consumer: %w", err)
	}

	eventChan := make(chan *corev1.SpaceEvent)

	// Track current iterator for cleanup
	var currentIter jetstream.MessagesContext
	var iterMu sync.Mutex

	go func() {
		c.logger.Debug("Starting live room event stream", "room_id", room_id)

		defer func() {
			c.logger.Debug("Live room event stream closed", "room_id", room_id)
			close(eventChan)
		}()

		const maxRetries = 3
		retryCount := 0

		for {
			// Get message iterator (retry on recoverable errors)
			iter, err := cons.Messages()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.logger.Error("Failed to get message iterator", "error", err)
				return
			}

			// Store iterator reference for external cleanup
			iterMu.Lock()
			currentIter = iter
			iterMu.Unlock()

			c.logger.Debug("Live subscription active", "room_id", room_id)

			// Read messages until error
			for {
				msg, err := iter.Next()
				if err != nil {
					iter.Stop()

					if ctx.Err() != nil {
						return
					}

					// Terminal errors - cannot recover
					if isTerminalIteratorError(err) {
						c.logger.Debug("Iterator terminated", "room_id", room_id, "error", err)
						return
					}

					// Recoverable error - retry with backoff
					retryCount++
					if retryCount > maxRetries {
						c.logger.Warn("Max retries exceeded for room event iterator", "room_id", room_id, "error", err, "retries", retryCount)
						return
					}

					c.logger.Debug("Iterator error, retrying", "room_id", room_id, "error", err, "retry", retryCount)
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Duration(retryCount) * 100 * time.Millisecond):
						// Continue to outer loop to create new iterator
					}
					break
				}

				// Success - reset retry count
				retryCount = 0

				var event corev1.SpaceEvent
				if err := proto.Unmarshal(msg.Data(), &event); err != nil {
					c.logger.Warn("Failed to unmarshal live event", "error", err)
					continue
				}

				select {
				case <-ctx.Done():
					iter.Stop()
					return
				case eventChan <- &event:
					// Event delivered
				}
			}
		}
	}()

	// Goroutine to stop the iterator when context is cancelled
	go func() {
		<-ctx.Done()
		iterMu.Lock()
		if currentIter != nil {
			currentIter.Stop()
		}
		iterMu.Unlock()
	}()

	return eventChan, nil
}

// ============================================================================
// Unread Message Tracking
// ============================================================================
//
// Per-user, per-room read state is keyed on the last-read root message's stable
// event ID (14-char NanoID, see ADR-026), not on the (volatile) JetStream
// sequence number. Event IDs are embedded in NATS subjects, so they survive
// stream renumbering and rebuilds. See docs/adr/ADR-028 for rationale.
//
// The legacy `room_read_status.*` keys (uint64 sequence numbers) are orphaned
// and ignored. Users with no `room_read_event.*` key are lazy-initialized to
// the room's current last root event on first read — the "caught up at deploy
// time" semantic.

// GetRoomLastEvent returns the last root message's event ID and proto-level
// `created_at` timestamp for a room. Excludes thread replies — only root
// messages affect room-level unread tracking. exists is false if the room
// has no root messages.
//
// Uses the proto's `created_at` rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes messages
// with fresh JetStream timestamps but leaves the proto payloads intact).
func (c *ChattoCore) GetRoomLastEvent(ctx context.Context, spaceID, roomID string) (eventID string, ts time.Time, exists bool, err error) {
	msg, err := c.getRoomLastRootMessage(ctx, spaceID, roomID)
	if err != nil {
		return "", time.Time{}, false, err
	}
	if msg == nil {
		return "", time.Time{}, false, nil
	}
	createdAt, err := rawMsgEventCreatedAt(msg)
	if err != nil {
		return "", time.Time{}, false, err
	}
	return subjects.ParseEventIDFromSubject(msg.Subject), createdAt, true, nil
}

// roomReadEventKey returns the KV key for tracking the user's last-read root
// event ID in a room.
func roomReadEventKey(userID, roomID string) string {
	return fmt.Sprintf("room_read_event.%s.%s", userID, roomID)
}

// GetLastReadEventID returns the user's last-read root-message event ID for a
// room. If no marker exists yet, it lazy-initializes the marker to the room's
// current last root event ("caught up at deploy time"); if the room has no
// messages, it returns "".
func (c *ChattoCore) GetLastReadEventID(ctx context.Context, spaceID, userID, roomID string) (string, error) {
	bucket := c.storage.serverRuntimeKV

	key := roomReadEventKey(userID, roomID)
	entry, err := bucket.Get(ctx, key)
	if err == nil {
		return string(entry.Value()), nil
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return "", fmt.Errorf("failed to get read marker: %w", err)
	}

	// No marker yet — initialize to the room's current last root event so the
	// user starts caught up rather than seeing a wall of unreads.
	lastID, _, exists, err := c.GetRoomLastEvent(ctx, spaceID, roomID)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	// Use Create (atomic insert) rather than Put: a concurrent writer
	// (PostMessage auto-mark, MarkRoomAsRead) may have set a real marker
	// between our Get and our write, and we must not clobber it.
	if _, err := bucket.Create(ctx, key, []byte(lastID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			entry, getErr := bucket.Get(ctx, key)
			if getErr != nil {
				return "", fmt.Errorf("failed to re-read read marker after concurrent init: %w", getErr)
			}
			return string(entry.Value()), nil
		}
		c.logger.Warn("Failed to lazy-initialize read marker", "user_id", userID, "room_id", roomID, "error", err)
	}
	return lastID, nil
}

// SetLastReadEventID stores the user's last-read root-message event ID.
//
// Callers MUST pass either a root message event ID (one published with subject
// `space.{s}.room.{r}.msg.{eventId}`) or the empty string. Thread-reply event
// IDs would not resolve via GetEventTimestamp's root-subject lookup and would
// keep the room permanently flagged as unread.
func (c *ChattoCore) SetLastReadEventID(ctx context.Context, spaceID, userID, roomID, eventID string) error {
	bucket := c.storage.serverRuntimeKV
	if _, err := bucket.Put(ctx, roomReadEventKey(userID, roomID), []byte(eventID)); err != nil {
		return fmt.Errorf("failed to set read marker: %w", err)
	}
	c.logger.Debug("Set last read event", "user_id", userID, "room_id", roomID, "event_id", eventID)
	return nil
}

// GetEventTimestamp returns the proto-level `created_at` timestamp for a
// root message event ID in a room. Returns zero time if the event ID is
// empty or the message doesn't exist (e.g. deleted or never published).
//
// Reads from the proto payload rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes with fresh
// JetStream timestamps but preserves the proto payload).
func (c *ChattoCore) GetEventTimestamp(ctx context.Context, spaceID, roomID, eventID string) (time.Time, error) {
	if eventID == "" {
		return time.Time{}, nil
	}
	stream := c.storage.serverEventsStream
	msg, err := stream.GetLastMsgForSubject(ctx, subjects.RoomMessage(kindForSpace(spaceID), roomID, eventID))
	if err != nil {
		if errors.Is(err, jetstream.ErrMsgNotFound) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("failed to get event: %w", err)
	}
	return rawMsgEventCreatedAt(msg)
}

// HasUnread reports whether a room has unread messages for a user. Returns
// false if the user is not a member, the room is muted, or there are no
// messages. Compares the user's stored read marker (event ID) against the
// room's current last root message.
func (c *ChattoCore) HasUnread(ctx context.Context, spaceID, userID, roomID string) (bool, error) {
	isMember, err := c.RoomMembershipExists(ctx, spaceID, userID, roomID)
	if err != nil {
		return false, fmt.Errorf("failed to check room membership: %w", err)
	}
	if !isMember {
		return false, nil
	}

	level, err := c.GetEffectiveNotificationLevel(ctx, spaceID, userID, roomID)
	if err != nil {
		c.logger.Warn("Failed to get notification level for unread check, continuing with default",
			"user_id", userID, "space_id", spaceID, "room_id", roomID, "error", err)
	} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		return false, nil
	}

	lastID, lastTime, exists, err := c.GetRoomLastEvent(ctx, spaceID, roomID)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	readID, err := c.GetLastReadEventID(ctx, spaceID, userID, roomID)
	if err != nil {
		return false, err
	}
	if readID == "" {
		// Member has a marker but no specific event read yet (joined an
		// empty room, then messages arrived). Anything counts as unread.
		return true, nil
	}
	if readID == lastID {
		return false, nil // Caught up — fast path
	}

	// Read marker points to an older (or deleted) message. Resolve its
	// timestamp and compare. A missing message means the marker is stale —
	// treat as unread; the user re-marks and state self-corrects.
	readTime, err := c.GetEventTimestamp(ctx, spaceID, roomID, readID)
	if err != nil {
		return false, err
	}
	if readTime.IsZero() {
		return true, nil
	}
	return lastTime.After(readTime), nil
}

// threadLastOpenedKey returns the KV key for tracking when a user last opened a thread.
func threadLastOpenedKey(userID, roomID, threadRootEventID string) string {
	return fmt.Sprintf("thread_last_opened.%s.%s.%s", userID, roomID, threadRootEventID)
}

// GetThreadLastOpened retrieves the timestamp when a user last opened a thread.
// Returns zero time if the thread has never been opened.
func (c *ChattoCore) GetThreadLastOpened(ctx context.Context, spaceID, userID, roomID, threadRootEventID string) (time.Time, error) {
	bucket := c.storage.serverRuntimeKV

	entry, err := bucket.Get(ctx, threadLastOpenedKey(userID, roomID, threadRootEventID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return time.Time{}, nil // Never opened
		}
		return time.Time{}, fmt.Errorf("failed to get thread last opened: %w", err)
	}

	// Decode int64 (Unix nano) from bytes using binary.BigEndian
	if len(entry.Value()) != 8 {
		return time.Time{}, fmt.Errorf("invalid thread last opened value")
	}
	nanos := int64(binary.BigEndian.Uint64(entry.Value()))
	return time.Unix(0, nanos), nil
}

// SetThreadLastOpened stores the current timestamp as when the user last opened a thread.
// Returns the previous last-opened time (zero if never opened before).
func (c *ChattoCore) SetThreadLastOpened(ctx context.Context, spaceID, userID, roomID, threadRootEventID string) (time.Time, error) {
	bucket := c.storage.serverRuntimeKV

	key := threadLastOpenedKey(userID, roomID, threadRootEventID)

	// Get previous value first
	var previousTime time.Time
	entry, err := bucket.Get(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return time.Time{}, fmt.Errorf("failed to get previous thread last opened: %w", err)
	}
	if err == nil && len(entry.Value()) == 8 {
		nanos := int64(binary.BigEndian.Uint64(entry.Value()))
		previousTime = time.Unix(0, nanos)
	}

	// Store current time
	now := time.Now()
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(now.UnixNano()))

	_, err = bucket.Put(ctx, key, buf)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to set thread last opened: %w", err)
	}

	c.logger.Debug("Set thread last opened", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "now", now)
	return previousTime, nil
}

// threadFollowKey returns the KV key for tracking whether a user is following a thread.
func threadFollowKey(userID, roomID, threadRootEventID string) string {
	return fmt.Sprintf("thread_follow.%s.%s.%s", userID, roomID, threadRootEventID)
}

// FollowThread marks a user as following a thread so they receive reply notifications.
// Stores a single byte in the RUNTIME KV bucket. Idempotent.
// Publishes a ThreadFollowChangedEvent for multi-tab sync.
func (c *ChattoCore) FollowThread(ctx context.Context, spaceID, userID, roomID, threadRootEventID string) error {
	bucket := c.storage.serverRuntimeKV

	if _, err := bucket.Put(ctx, threadFollowKey(userID, roomID, threadRootEventID), []byte{0x01}); err != nil {
		return fmt.Errorf("failed to follow thread: %w", err)
	}

	c.publishThreadFollowChangedEvent(ctx, userID, spaceID, roomID, threadRootEventID, true)
	return nil
}

// UnfollowThread removes a user's follow on a thread so they stop receiving reply notifications.
// Idempotent - calling when not following is a no-op.
// Publishes a ThreadFollowChangedEvent for multi-tab sync.
func (c *ChattoCore) UnfollowThread(ctx context.Context, spaceID, userID, roomID, threadRootEventID string) error {
	bucket := c.storage.serverRuntimeKV

	if err := bucket.Delete(ctx, threadFollowKey(userID, roomID, threadRootEventID)); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to unfollow thread: %w", err)
	}

	c.publishThreadFollowChangedEvent(ctx, userID, spaceID, roomID, threadRootEventID, false)
	return nil
}

// publishThreadFollowChangedEvent publishes a live event when a user's thread follow state changes.
// User-scoped: only delivered to the user who changed their follow state.
func (c *ChattoCore) publishThreadFollowChangedEvent(ctx context.Context, userID, spaceID, roomID, threadRootEventID string, isFollowing bool) {
	event := &corev1.InstanceEvent{
		Id:        NewEventID(),
		ActorId:   userID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.InstanceEvent_ThreadFollowChanged{
			ThreadFollowChanged: &corev1.ThreadFollowChangedEvent{
				SpaceId:           spaceID,
				RoomId:            roomID,
				ThreadRootEventId: threadRootEventID,
				IsFollowing:       isFollowing,
			},
		},
	}

	subject := subjects.LiveInstanceUserEvent(userID, "thread_follow_changed")
	if err := c.publishInstanceEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish thread follow changed event", "error", err, "user_id", userID, "thread_root_event_id", threadRootEventID)
	}
}

// IsFollowingThread checks if a user is following a thread.
func (c *ChattoCore) IsFollowingThread(ctx context.Context, spaceID, userID, roomID, threadRootEventID string) (bool, error) {
	bucket := c.storage.serverRuntimeKV

	if _, err := bucket.Get(ctx, threadFollowKey(userID, roomID, threadRootEventID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check thread follow: %w", err)
	}
	return true, nil
}

// GetThreadFollowers returns all user IDs following a specific thread.
// Uses ListKeysFiltered to scan for thread_follow.*.{roomID}.{threadRootEventID} keys.
func (c *ChattoCore) GetThreadFollowers(ctx context.Context, spaceID, roomID, threadRootEventID string) ([]string, error) {
	bucket := c.storage.serverRuntimeKV

	pattern := fmt.Sprintf("thread_follow.*.%s.%s", roomID, threadRootEventID)
	lister, err := bucket.ListKeysFiltered(ctx, pattern)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list thread followers: %w", err)
	}

	var userIDs []string
	for key := range lister.Keys() {
		// Key format: thread_follow.{userID}.{roomID}.{threadRootEventID}
		parts := strings.Split(key, ".")
		if len(parts) >= 4 {
			userIDs = append(userIDs, parts[1])
		}
	}
	return userIDs, nil
}

// ListFollowedThreads returns all threads followed by the user in the given spaces,
// sorted by last activity (newest first). Threads with no metadata are skipped.
// Authorization: Caller must verify space membership before calling.
func (c *ChattoCore) ListFollowedThreads(ctx context.Context, userID string, spaceIDs []string) ([]*FollowedThread, error) {
	var allThreads []*FollowedThread

	for _, spaceID := range spaceIDs {
		threads, err := c.listFollowedThreadsInSpace(ctx, userID, spaceID)
		if err != nil {
			c.logger.Warn("Failed to list followed threads for space", "space_id", spaceID, "error", err)
			continue
		}
		allThreads = append(allThreads, threads...)
	}

	// Sort by LastReplyAt descending (newest first), nil values last
	sort.Slice(allThreads, func(i, j int) bool {
		if allThreads[i].LastReplyAt == nil {
			return false
		}
		if allThreads[j].LastReplyAt == nil {
			return true
		}
		return allThreads[i].LastReplyAt.After(*allThreads[j].LastReplyAt)
	})

	return allThreads, nil
}

// listFollowedThreadsInSpace returns all threads followed by the user in a single space.
func (c *ChattoCore) listFollowedThreadsInSpace(ctx context.Context, userID, spaceID string) ([]*FollowedThread, error) {
	bucket := c.storage.serverRuntimeKV

	// List all thread_follow keys for this user across all rooms
	// Use ">" to match remaining parts: thread_follow.{userId}.{roomId}.{threadRootEventId}
	pattern := fmt.Sprintf("thread_follow.%s.>", userID)
	lister, err := bucket.ListKeysFiltered(ctx, pattern)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list followed threads: %w", err)
	}

	var result []*FollowedThread
	for key := range lister.Keys() {
		// Key format: thread_follow.{userID}.{roomID}.{threadRootEventID}
		parts := strings.Split(key, ".")
		if len(parts) < 4 {
			continue
		}
		roomID := parts[2]
		threadRootEventID := parts[3]

		// Get thread metadata (reply count, last reply, participants)
		metadata, err := c.GetThreadMetadata(ctx, spaceID, roomID, threadRootEventID)
		if err != nil {
			c.logger.Warn("Failed to get thread metadata for followed thread", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
			continue
		}

		// Determine unread status: thread has unread if lastReplyAt > threadLastOpened
		hasUnread := false
		if metadata.LastReplyAt != nil {
			lastOpened, err := c.GetThreadLastOpened(ctx, spaceID, userID, roomID, threadRootEventID)
			if err != nil {
				hasUnread = true // Can't determine, assume unread
			} else {
				hasUnread = lastOpened.IsZero() || metadata.LastReplyAt.After(lastOpened)
			}
		}

		result = append(result, &FollowedThread{
			SpaceID:           spaceID,
			RoomID:            roomID,
			ThreadRootEventID: threadRootEventID,
			ReplyCount:        metadata.ReplyCount,
			LastReplyAt:       metadata.LastReplyAt,
			ParticipantIDs:    metadata.ParticipantIDs,
			HasUnread:         hasUnread,
		})
	}

	return result, nil
}

// ============================================================================
// Room Layout
// ============================================================================

// roomLayoutKey is the KV key for the room layout document within the space CONFIG bucket.
const roomLayoutKey = "room_layout"

// maxLayoutRetries is the maximum number of OCC retry attempts for room layout updates.
const maxLayoutRetries = 5

// GetRoomLayout retrieves the room layout for a space from the CONFIG bucket.
// Returns nil if no layout has been configured.
func (c *ChattoCore) GetRoomLayout(ctx context.Context, spaceID string) (*corev1.RoomLayout, error) {
	bucket := c.storage.serverConfigKV

	entry, err := bucket.Get(ctx, roomLayoutKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil // No layout configured
		}
		return nil, fmt.Errorf("failed to get room layout: %w", err)
	}

	layout := &corev1.RoomLayout{}
	if err := proto.Unmarshal(entry.Value(), layout); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room layout: %w", err)
	}

	return layout, nil
}

// UpdateRoomLayout atomically updates the room layout using optimistic concurrency control.
// The layout is stored as a single KV entry for atomic reorders.
// Retries up to maxLayoutRetries times on concurrent modification conflicts.
func (c *ChattoCore) UpdateRoomLayout(ctx context.Context, spaceID string, layout *corev1.RoomLayout) (*corev1.RoomLayout, error) {
	bucket := c.storage.serverConfigKV

	data, err := proto.Marshal(layout)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room layout: %w", err)
	}

	for attempt := 0; attempt < maxLayoutRetries; attempt++ {
		// Get current entry to obtain revision
		entry, getErr := bucket.Get(ctx, roomLayoutKey)

		var revision uint64
		if getErr != nil {
			if !errors.Is(getErr, jetstream.ErrKeyNotFound) {
				return nil, fmt.Errorf("failed to get room layout: %w", getErr)
			}
			// Key doesn't exist — will use Create
			revision = 0
		} else {
			revision = entry.Revision()
		}

		// Attempt atomic update
		var writeErr error
		if revision == 0 {
			_, writeErr = bucket.Create(ctx, roomLayoutKey, data)
		} else {
			_, writeErr = bucket.Update(ctx, roomLayoutKey, data, revision)
		}

		if writeErr == nil {
			return layout, nil
		}

		if errors.Is(writeErr, jetstream.ErrKeyExists) {
			continue // Retry on conflict
		}

		return nil, fmt.Errorf("failed to store room layout: %w", writeErr)
	}

	return nil, ErrConfigConflict
}

// removeRoomFromLayout removes a room ID from the room layout (best-effort).
// Called when a room is deleted to keep the layout consistent.
func (c *ChattoCore) removeRoomFromLayout(ctx context.Context, spaceID, roomID string) {
	bucket := c.storage.serverConfigKV

	for attempt := 0; attempt < maxLayoutRetries; attempt++ {
		entry, err := bucket.Get(ctx, roomLayoutKey)
		if err != nil {
			return // No layout exists or error — nothing to clean up
		}

		layout := &corev1.RoomLayout{}
		if err := proto.Unmarshal(entry.Value(), layout); err != nil {
			return
		}

		// Remove the room ID from all sections and the unsectioned list
		changed := false
		for _, section := range layout.Sections {
			filtered := section.RoomIds[:0]
			for _, id := range section.RoomIds {
				if id != roomID {
					filtered = append(filtered, id)
				} else {
					changed = true
				}
			}
			section.RoomIds = filtered
		}
		if len(layout.UnsortedRoomIds) > 0 {
			filtered := layout.UnsortedRoomIds[:0]
			for _, id := range layout.UnsortedRoomIds {
				if id != roomID {
					filtered = append(filtered, id)
				} else {
					changed = true
				}
			}
			layout.UnsortedRoomIds = filtered
		}

		if !changed {
			return // Room wasn't in the layout
		}

		data, err := proto.Marshal(layout)
		if err != nil {
			return
		}

		_, err = bucket.Update(ctx, roomLayoutKey, data, entry.Revision())
		if err == nil {
			return // Success
		}

		if errors.Is(err, jetstream.ErrKeyExists) {
			continue // Retry on conflict
		}
		return // Other error — give up
	}
}

// PublishRoomLayoutUpdated publishes a live event notifying clients that the room layout was updated.
// Authorization: The event is published to the instance space subject, so it is delivered
// to all space members via the existing instance event authorization filter.
func (c *ChattoCore) PublishRoomLayoutUpdated(ctx context.Context, actorID, spaceID string) error {
	event := &corev1.InstanceEvent{
		CreatedAt: timestamppb.Now(),
		ActorId:   actorID,
		Event: &corev1.InstanceEvent_RoomLayoutUpdated{
			RoomLayoutUpdated: &corev1.RoomLayoutUpdatedEvent{
				SpaceId: spaceID,
			},
		},
	}

	subject := subjects.LiveInstanceSpaceEvent(spaceID, "room_layout_updated")
	return c.publishInstanceEvent(ctx, subject, event)
}

