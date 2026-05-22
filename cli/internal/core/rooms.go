package core

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// getRoomLastMessage fetches the last message in a room directly from JetStream.
// Returns nil if no messages exist for this room yet.
func (c *ChattoCore) getRoomLastMessage(ctx context.Context, kind RoomKind, roomID string) (*jetstream.RawStreamMsg, error) {
	stream := c.storage.serverEventsStream

	msg, err := stream.GetLastMsgForSubject(ctx, subjects.RoomAllMessages(string(kind), roomID))
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
func (c *ChattoCore) getRoomLastRootMessage(ctx context.Context, kind RoomKind, roomID string) (*jetstream.RawStreamMsg, error) {
	stream := c.storage.serverEventsStream

	msg, err := stream.GetLastMsgForSubject(ctx, subjects.RoomRootMessages(string(kind), roomID))
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
func (c *ChattoCore) GetRoomLastMessageAt(ctx context.Context, kind RoomKind, roomID string) (time.Time, error) {
	msg, err := c.getRoomLastMessage(ctx, kind, roomID)
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
	var event corev1.Event
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
//
// groupID identifies the RoomGroup the room belongs to. For channel rooms
// this should be a real set's ID once the room-sets feature is fully
// wired (see ADR-031); during the transition, an empty string is still
// accepted and the room is created without a set membership. DM rooms
// always pass an empty groupID. When a non-empty groupID is provided the
// set must exist; the room is automatically added to its room_ids list.
func (c *ChattoCore) CreateRoom(ctx context.Context, actorID string, kind RoomKind, groupID, name, description string) (*corev1.Room, error) {
	// Validate room name
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}

	// Validate room description
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}

	// If a groupID is provided, verify it exists before creating the room.
	// DM rooms always pass empty. For channel rooms, an empty groupID
	// auto-routes to the first group in the layout (the seed "Lobby" group
	// on fresh deployments) so existing callers don't need to pick one
	// explicitly. See ADR-031.
	if groupID != "" {
		if _, err := c.GetRoomGroup(ctx, groupID); err != nil {
			return nil, err
		}
	} else if kind == KindChannel {
		groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			return nil, fmt.Errorf("lookup default group: %w", err)
		}
		if len(groups) > 0 {
			groupID = groups[0].Id
		}
	}

	// Trim whitespace from name
	name = strings.TrimSpace(name)

	bucket := c.storage.serverConfigKV

	// Backfill name index for any pre-existing rooms (no-op after first call per process).
	if err := c.ensureRoomNameIndex(ctx, kind, bucket); err != nil {
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
		SpaceId:     SpaceIDForKind(kind),
		Kind:        ProtoKindForRoomKind(kind),
		Name:        name,
		Description: description,
		GroupId:     groupID,
	}

	roomData, err := proto.Marshal(room)
	if err != nil {
		// Roll back the name claim so the name doesn't end up reserved with no room behind it.
		c.bestEffortReleaseRoomNameClaim(ctx, bucket, indexKey, room_id)
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	if _, err := bucket.Put(ctx, roomKey(kind, room.Id), roomData); err != nil {
		c.bestEffortReleaseRoomNameClaim(ctx, bucket, indexKey, room_id)
		return nil, fmt.Errorf("failed to store room: %w", err)
	}

	// If the room belongs to a set, append it to the set's room_ids.
	// Best-effort — if this fails the room exists with its GroupId stamped
	// but the layout isn't updated; the inconsistency is detectable and
	// can be repaired by an admin re-move.
	if groupID != "" {
		if err := c.MoveRoomToGroup(ctx, actorID, room_id, groupID); err != nil {
			c.logger.Warn("Failed to add new room to set layout; room.GroupId is set but set membership is not reflected in the layout",
				"error", err, "room_id", room_id, "group_id", groupID)
		}
	}

	// Create and publish audit event to space stream
	// Room events are stored in the unified space stream
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId:      room_id,
				Name:        name,
				Description: description,
			},
		},
	})
	subject := subjects.RoomMeta(string(kind), room_id)
	_, err = c.publishServerEventWithAck(ctx, subject, event)
	if err != nil {
		// Room was created in KV but event failed - log but don't fail
		c.logger.Error("failed to publish room created event", "error", err, "room_id", room_id)
	}

	// Set up special permissions for announcements rooms
	if strings.EqualFold(name, AnnouncementsRoomName) {
		if err := c.SetupAnnouncementsRoomPermissions(ctx, room_id); err != nil {
			c.logger.Warn("Failed to set up announcements room permissions", "error", err, "room_id", room_id)
			// Don't fail room creation if permission setup fails
		}
	}

	c.logger.Info("Room created", "kind", kind, "room_id", room_id, "name", name, "group_id", groupID)

	// Notify connected clients so they pick up the new room in the
	// directory and (if joined) the sidebar. Channel rooms only — DMs
	// live outside the channel layout. When the room was placed in a
	// group, MoveRoomToGroup already published this event; only emit
	// here as a fallback for the (rare) groupless channel-room case.
	if kind == KindChannel && groupID == "" {
		c.notifyRoomLayoutChanged(ctx, actorID, "create_room")
	}

	return room, nil
}

// UpdateRoom updates an existing room.
// KV store is updated first, then an event is published for audit trail (best-effort).
// Authorization: Caller must verify CanManageAnyRoom before calling.
func (c *ChattoCore) UpdateRoom(ctx context.Context, actorID string, kind RoomKind, room_id, name, description string) (*corev1.Room, error) {
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

	// Fetch existing room (preserves all fields like Archived)
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}

	bucket := c.storage.serverConfigKV

	// Backfill name index for pre-existing rooms (no-op after first call per process).
	if err := c.ensureRoomNameIndex(ctx, kind, bucket); err != nil {
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
	if _, err := bucket.Put(ctx, roomKey(kind, room.Id), roomData); err != nil {
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
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{
				RoomId:      room_id,
				Name:        name,
				Description: description,
			},
		},
	})
	subject := subjects.RoomMeta(string(kind), room_id)
	if err := c.publishServerEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room updated event", "error", err, "room_id", room_id)
	}

	c.logger.Info("Room updated", "kind", kind, "room_id", room_id, "name", name)

	return room, nil
}

// DeleteRoom deletes a room.
// Publishes event first, then deletes from KV store, then deletes the stream.
// Authorization: Caller must verify CanManageAnyRoom before calling.
func (c *ChattoCore) DeleteRoom(ctx context.Context, actorID string, kind RoomKind, room_id string) error {
	// Verify room exists
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return err
	}

	// Create and publish audit event to space stream BEFORE deletion (best-effort)
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomDeleted{
			RoomDeleted: &corev1.RoomDeletedEvent{
				RoomId: room_id,
			},
		},
	})
	subject := subjects.RoomMeta(string(kind), room_id)
	if err := c.publishServerEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room deleted event", "error", err, "room_id", room_id)
	}

	// Delete from KV store (source of truth)
	bucket := c.storage.serverConfigKV
	err = bucket.Delete(ctx, roomKey(kind, room_id))
	if err != nil {
		return fmt.Errorf("failed to delete room: %w", err)
	}

	// Release the room name so a new room can claim it. Best-effort: a stale entry only
	// blocks reuse of that exact name, and the next CreateRoom for it would log a clear
	// ErrRoomNameExists rather than silently wedging anything.
	c.bestEffortReleaseRoomNameClaim(ctx, bucket, roomNameIndexKey(room.Name), room_id)

	// Purge room events from the space stream
	if err := c.purgeRoomEvents(ctx, kind, room_id); err != nil {
		c.logger.Error("failed to purge room events", "error", err, "kind", kind, "room_id", room_id)
		// Continue anyway - orphaned events can be cleaned up manually if needed
	}

	// Best-effort: remove room from layout if present
	c.removeRoomFromLayout(ctx, kind, room_id)

	c.logger.Info("Room deleted", "kind", kind, "room_id", room_id)

	if kind == KindChannel {
		c.notifyRoomLayoutChanged(ctx, actorID, "delete_room")
	}

	return nil
}

// ArchiveRoom sets a room's archived flag to true.
// Archived rooms are hidden from sidebars and Browse Rooms. Existing memberships are preserved.
// Authorization: Caller must verify CanManageAnyRoom before calling.
func (c *ChattoCore) ArchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*corev1.Room, error) {
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}

	room.Archived = true

	bucket := c.storage.serverConfigKV
	roomData, err := proto.Marshal(room)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	_, err = bucket.Put(ctx, roomKey(kind, room.Id), roomData)
	if err != nil {
		return nil, fmt.Errorf("failed to archive room: %w", err)
	}

	// Publish persisted event to space stream (best-effort)
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomArchived{
			RoomArchived: &corev1.RoomArchivedEvent{
				RoomId: roomID,
			},
		},
	})
	subject := subjects.RoomMeta(string(kind), roomID)
	if err := c.publishServerEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room archived event", "error", err, "room_id", roomID)
	}

	// Publish live event for real-time sync (sidebar/layout updates)
	if err := c.PublishRoomGroupsUpdated(ctx, actorID, kind); err != nil {
		c.logger.Error("failed to publish room layout updated event after archive", "error", err)
	}

	c.logger.Info("Room archived", "kind", kind, "room_id", roomID)
	return room, nil
}

// UnarchiveRoom sets a room's archived flag to false. Archive/unarchive
// only toggles the flag — the room keeps its set position throughout the
// cycle.
// Authorization: Caller must verify CanManageAnyRoom before calling.
func (c *ChattoCore) UnarchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*corev1.Room, error) {
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}

	room.Archived = false

	bucket := c.storage.serverConfigKV
	roomData, err := proto.Marshal(room)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal room: %w", err)
	}
	_, err = bucket.Put(ctx, roomKey(kind, room.Id), roomData)
	if err != nil {
		return nil, fmt.Errorf("failed to unarchive room: %w", err)
	}

	// Publish persisted event to space stream (best-effort)
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomUnarchived{
			RoomUnarchived: &corev1.RoomUnarchivedEvent{
				RoomId: roomID,
			},
		},
	})
	subject := subjects.RoomMeta(string(kind), roomID)
	if err := c.publishServerEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room unarchived event", "error", err, "room_id", roomID)
	}

	// Publish live event for real-time sync (sidebar/layout updates)
	if err := c.PublishRoomGroupsUpdated(ctx, actorID, kind); err != nil {
		c.logger.Error("failed to publish room layout updated event after unarchive", "error", err)
	}

	c.logger.Info("Room unarchived", "kind", kind, "room_id", roomID)
	return room, nil
}

// GetRoom retrieves a room from the space-specific CONFIG bucket.
func (c *ChattoCore) GetRoom(ctx context.Context, kind RoomKind, room_id string) (*corev1.Room, error) {
	bucket := c.storage.serverConfigKV

	entry, err := bucket.Get(ctx, roomKey(kind, room_id))
	if err != nil {
		return nil, fmt.Errorf("room not found: %w", err)
	}

	room := &corev1.Room{}
	if err := proto.Unmarshal(entry.Value(), room); err != nil {
		return nil, fmt.Errorf("failed to unmarshal room: %w", err)
	}

	return room, nil
}

// FindRoomByID resolves a room from its ID alone by probing the channel
// bucket first, then DMs. Returns ErrNotFound if neither has the room.
//
// Live events carry only a room ID (no kind discriminator on the wire),
// so resolvers and consumers downstream of those events use this to
// recover both the room and the kind context the core API still needs
// for KV partitioning.
func (c *ChattoCore) FindRoomByID(ctx context.Context, room_id string) (*corev1.Room, error) {
	if room, err := c.GetRoom(ctx, KindChannel, room_id); err == nil {
		return room, nil
	}
	if room, err := c.GetRoom(ctx, KindDM, room_id); err == nil {
		return room, nil
	}
	return nil, ErrNotFound
}

// FindRoomKind is a thin wrapper around FindRoomByID for callers that
// only need the kind. The room load is paid either way; the wrapper is
// just there for ergonomics.
func (c *ChattoCore) FindRoomKind(ctx context.Context, room_id string) (RoomKind, error) {
	room, err := c.FindRoomByID(ctx, room_id)
	if err != nil {
		return "", err
	}
	return KindOfRoom(room), nil
}

// ListRooms retrieves all rooms of the given kind from the CONFIG bucket.
//
// Post-#330 phase 4b: channels and DM rooms share SERVER_CONFIG, with the
// kind encoded in the key prefix (`room.channel.{X}` vs `room.dm.{X}`).
// The prefix scan returns only the matching kind, so no in-memory filter
// is needed.
func (c *ChattoCore) ListRooms(ctx context.Context, kind RoomKind) ([]*corev1.Room, error) {
	bucket := c.storage.serverConfigKV

	prefix := roomKeyPrefix(kind)
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
func (c *ChattoCore) RoomNameExists(ctx context.Context, kind RoomKind, name string) (bool, error) {
	return c.RoomNameExistsExcluding(ctx, kind, name, "")
}

// RoomNameExistsExcluding checks if a room with the given name exists, excluding a specific room.
// This is used by UpdateRoom to allow a room to keep its own name (with different casing).
// It performs a case-insensitive comparison after trimming whitespace.
//
// Backed by the room_name_index.* keys, so it's O(1) per call after the per-space backfill
// has run once. CreateRoom and UpdateRoom enforce uniqueness via atomic kv.Create rather
// than calling this — this method exists for callers that want to query without mutating.
func (c *ChattoCore) RoomNameExistsExcluding(ctx context.Context, kind RoomKind, name, excludeRoomID string) (bool, error) {
	bucket := c.storage.serverConfigKV

	if err := c.ensureRoomNameIndex(ctx, kind, bucket); err != nil {
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
func (c *ChattoCore) ensureRoomNameIndex(ctx context.Context, kind RoomKind, bucket jetstream.KeyValue) error {
	if _, ok := c.roomNameIndexBackfilled.Load(kind); ok {
		return nil
	}

	// Channels only — DM rooms have empty names so there's nothing to index.
	keyLister, err := bucket.ListKeysFiltered(ctx, roomKeyPrefix(kind))
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			c.roomNameIndexBackfilled.Store(kind, struct{}{})
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
			c.logger.Warn("Failed to backfill room name index entry", "kind", kind, "room_id", room.Id, "error", err)
		}
	}

	c.roomNameIndexBackfilled.Store(kind, struct{}{})
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
