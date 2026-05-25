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
	"hmans.de/chatto/internal/events"
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

// getRoomLastRootEvent returns the most recent root MessagePostedEvent
// (excluding thread replies) in a room, or nil if none have been
// projected yet. Bounded O(walk-until-found) via the projection's
// LastVisibleRoomEntry helper.
func (c *ChattoCore) getRoomLastRootEvent(roomID string) *corev1.Event {
	entry, ok := c.RoomTimeline.LastVisibleRoomEntry(roomID, func(e *corev1.Event) bool {
		msg := e.GetMessagePosted()
		return msg != nil && msg.GetInThread() == ""
	})
	if !ok {
		return nil
	}
	return entry.Event
}

// getRoomLastMessageEvent returns the most recent MessagePostedEvent
// of any kind (root or thread reply) in a room, or nil.
func (c *ChattoCore) getRoomLastMessageEvent(roomID string) *corev1.Event {
	entry, ok := c.RoomTimeline.LastVisibleRoomEntry(roomID, func(e *corev1.Event) bool {
		return e.GetMessagePosted() != nil
	})
	if !ok {
		return nil
	}
	return entry.Event
}

// GetRoomLastMessageAt returns the timestamp of the last message in a
// room, including thread replies. Reads from the in-memory room
// timeline projection.
func (c *ChattoCore) GetRoomLastMessageAt(ctx context.Context, kind RoomKind, roomID string) (time.Time, error) {
	ev := c.getRoomLastMessageEvent(roomID)
	if ev == nil {
		return time.Time{}, nil
	}
	if ev.GetCreatedAt() == nil {
		return time.Time{}, nil
	}
	return ev.GetCreatedAt().AsTime(), nil
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

// maxRoomNameClaimRetries bounds the OCC retry loop for cross-room
// uniqueness checks. Each retry refreshes the projection and re-checks
// the name; conflicts come from other processes publishing room events
// concurrently. Five attempts with exponential backoff (~31ms worst
// case) is generous for normal workloads.
const maxRoomNameClaimRetries = 5

// CreateRoom creates a new room.
// Authorization: Caller must verify CanCreateRoom before calling.
//
// groupID identifies the RoomGroup the room belongs to. DM rooms pass
// empty. For channel rooms an empty groupID auto-routes to the first
// group in the layout (seed "Lobby" group on fresh deployments) — see
// ADR-031.
//
// ADR-035 phase 6: event-only. Name uniqueness is enforced via
// JetStream wildcard OCC against `evt.room.>` — the room manager
// reads its projection's LastSeq, checks the name is unused, then
// publishes RoomCreatedEvent with that seq as the expected-last for
// the filter. Concurrent room mutations from any process (this one
// or another replica) advance the filter's seq and cause our publish
// to fail; we re-check uniqueness from the (now-caught-up) projection
// and retry.
func (c *ChattoCore) CreateRoom(ctx context.Context, actorID string, kind RoomKind, groupID, name, description string) (*corev1.Room, error) {
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}

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

	name = strings.TrimSpace(name)
	room_id := NewRoomID()

	room := &corev1.Room{
		Id:          room_id,
		Kind:        ProtoKindForRoomKind(kind),
		Name:        name,
		Description: description,
		GroupId:     groupID,
	}

	createdEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId:      room_id,
				Name:        name,
				Description: description,
				Kind:        ProtoKindForRoomKind(kind),
			},
		},
	})

	createdSeq, err := c.publishRoomEventWithNameOCC(ctx, name, createdEvent, room_id)
	if err != nil {
		return nil, err
	}

	// Move the room into its group's room_ids list. Best-effort — a
	// failed move leaves a room in the catalog with no group
	// membership; an admin can repair via re-move. (Channel rooms only;
	// DMs don't belong to groups.)
	if groupID != "" {
		if err := c.MoveRoomToGroup(ctx, actorID, room_id, groupID); err != nil {
			c.logger.Warn("Failed to add new room to set layout",
				"error", err, "room_id", room_id, "group_id", groupID)
		}
	}

	// Legacy live broadcast on the SERVER_EVENTS path — keeps the
	// frontend's myEvents subscription working until that pipe is
	// retired.
	subject := subjects.RoomMeta(string(kind), room_id)
	if _, err := c.publishServerEventWithAck(ctx, subject, createdEvent); err != nil {
		c.logger.Error("failed to publish room created event (legacy)", "error", err, "room_id", room_id)
	}

	if strings.EqualFold(name, AnnouncementsRoomName) {
		if err := c.SetupAnnouncementsRoomPermissions(ctx, room_id); err != nil {
			c.logger.Warn("Failed to set up announcements room permissions", "error", err, "room_id", room_id)
		}
	}

	c.logger.Info("Room created", "kind", kind, "room_id", room_id, "name", name, "group_id", groupID)

	if kind == KindChannel && groupID == "" {
		c.notifyRoomLayoutChanged(ctx, actorID, "create_room")
	}

	if err := c.RoomCatalogProjector.WaitForSeq(ctx, createdSeq); err != nil {
		return nil, fmt.Errorf("wait for catalog projection: %w", err)
	}
	return room, nil
}

// publishRoomEventWithNameOCC publishes a name-claiming room event
// (RoomCreated or RoomUpdated) with cluster-wide name uniqueness
// enforced via JetStream wildcard OCC against `evt.room.>`.
//
// The flow per attempt:
//  1. Check the catalog for the desired `name`; if any other room
//     holds it, return ErrRoomNameExists immediately.
//  2. Read the stream's actual last seq for the OCC filter directly
//     (NOT from the catalog projector — that subscribes to a narrower
//     set of subjects than `evt.room.>` and its LastSeq would fall
//     permanently behind, deterministically failing every retry on a
//     server that's had any joins/leaves since the last catalog write).
//  3. Publish the event with the freshly-read filter seq. JetStream
//     rejects with ErrConflict if any evt.room.> message landed in the
//     read-publish window — backoff briefly and retry.
//
// excludeRoomID is the ID to exclude from the uniqueness check —
// used by UpdateRoom so a room can keep a name it already holds
// (e.g. case-only changes, or no-op renames).
func (c *ChattoCore) publishRoomEventWithNameOCC(ctx context.Context, name string, event *corev1.Event, excludeRoomID string) (uint64, error) {
	// Determine publish subject from the event payload. Room events
	// all target the per-room aggregate subject; this doesn't change
	// across retries.
	var roomID string
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		roomID = e.RoomCreated.GetRoomId()
	case *corev1.Event_RoomUpdated:
		roomID = e.RoomUpdated.GetRoomId()
	default:
		return 0, fmt.Errorf("publishRoomEventWithNameOCC: unsupported event type %T", e)
	}
	publishSubject := events.RoomAggregate(roomID).SubjectFor(event)
	occFilter := events.RoomSubjectFilter()

	for attempt := 0; attempt < maxRoomNameClaimRetries; attempt++ {
		if owner := c.RoomCatalog.FindByName(name); owner != "" && owner != excludeRoomID {
			return 0, ErrRoomNameExists
		}

		filterSeq, err := c.EventPublisher.LastSubjectSeq(ctx, occFilter)
		if err != nil {
			return 0, fmt.Errorf("read OCC filter seq: %w", err)
		}

		seq, err := c.EventPublisher.AppendAtFilter(ctx, publishSubject, event, occFilter, filterSeq)
		if err == nil {
			return seq, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return 0, err
		}

		// Filter advanced under us between LastSubjectSeq and the
		// publish. Backoff briefly and retry — the next attempt's
		// fresh LastSubjectSeq read will see the landed event.
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return 0, fmt.Errorf("room name OCC retry exhausted after %d attempts: %w", maxRoomNameClaimRetries, events.ErrConflict)
}

// UpdateRoom updates an existing room's mutable fields (name +
// description). Authorization: Caller must verify CanManageAnyRoom
// before calling.
//
// ADR-035 phase 6: event-only. Renames go through the wildcard-OCC
// path to enforce cluster-wide name uniqueness (see
// publishRoomEventWithNameOCC); description-only edits skip the
// uniqueness check and use a plain per-subject OCC.
func (c *ChattoCore) UpdateRoom(ctx context.Context, actorID string, kind RoomKind, room_id, name, description string) (*corev1.Room, error) {
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)

	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}

	// "Rename" here means the case-folded name changed. Case-only
	// edits (e.g. "general" → "General") don't change the uniqueness
	// slot and can skip the wildcard OCC dance.
	renamed := !strings.EqualFold(room.Name, name)

	room.Name = name
	room.Description = description

	updatedEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{
				RoomId:      room_id,
				Name:        name,
				Description: description,
			},
		},
	})

	var updatedSeq uint64
	if renamed {
		updatedSeq, err = c.publishRoomEventWithNameOCC(ctx, name, updatedEvent, room_id)
		if err != nil {
			return nil, err
		}
	} else {
		updatedSeq, err = c.EventPublisher.Append(ctx, events.RoomAggregate(room_id).SubjectFor(updatedEvent), updatedEvent)
		if err != nil {
			return nil, fmt.Errorf("publish RoomUpdatedEvent: %w", err)
		}
	}

	subject := subjects.RoomMeta(string(kind), room_id)
	if err := c.publishServerEvent(ctx, subject, updatedEvent); err != nil {
		c.logger.Error("failed to publish room updated event (legacy)", "error", err, "room_id", room_id)
	}

	c.logger.Info("Room updated", "kind", kind, "room_id", room_id, "name", name)

	if err := c.RoomCatalogProjector.WaitForSeq(ctx, updatedSeq); err != nil {
		return nil, fmt.Errorf("wait for catalog projection: %w", err)
	}
	return room, nil
}

// DeleteRoom deletes a room.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only. Publishes RoomDeletedEvent (which both
// the catalog and membership projections apply as a drop) and, for
// channel rooms in a group, a RoomRemovedFromGroupEvent cascade per
// ADR-034 Approach A. Stream events are purged after the projections
// catch up; the legacy KV room record is no longer touched here.
func (c *ChattoCore) DeleteRoom(ctx context.Context, actorID string, kind RoomKind, room_id string) error {
	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return err
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomDeleted{
			RoomDeleted: &corev1.RoomDeletedEvent{
				RoomId: room_id,
			},
		},
	})
	seq, err := c.EventPublisher.Append(ctx, events.RoomAggregate(room_id).SubjectFor(event), event)
	if err != nil {
		return fmt.Errorf("publish RoomDeletedEvent: %w", err)
	}

	// Cascade (ADR-034 Approach A): a channel room that lives in a
	// group emits a per-group event so the group projection drops the
	// room from its room_ids. DMs don't belong to groups.
	var groupRemovedSeq uint64
	if kind == KindChannel && room.GetGroupId() != "" {
		removed := newEvent(actorID, &corev1.Event{
			Event: &corev1.Event_RoomRemovedFromGroup{
				RoomRemovedFromGroup: &corev1.RoomRemovedFromGroupEvent{
					GroupId: room.GetGroupId(),
					RoomId:  room_id,
				},
			},
		})
		groupRemovedSeq, err = c.EventPublisher.Append(ctx, events.GroupAggregate(room.GetGroupId()).SubjectFor(removed), removed)
		if err != nil {
			c.logger.Error("failed to publish RoomRemovedFromGroupEvent for delete cascade", "error", err, "room_id", room_id, "group_id", room.GetGroupId())
		}
	}

	subject := subjects.RoomMeta(string(kind), room_id)
	if err := c.publishServerEvent(ctx, subject, event); err != nil {
		c.logger.Error("failed to publish room deleted event (legacy)", "error", err, "room_id", room_id)
	}

	// Purge room events from the space stream (best-effort — orphan
	// events can be cleaned up manually if this fails).
	if err := c.purgeRoomEvents(ctx, kind, room_id); err != nil {
		c.logger.Error("failed to purge room events", "error", err, "kind", kind, "room_id", room_id)
	}

	// (Phase-6 note: pre-phase-6 we had to walk room_group docs to
	// drop the deleted room from group.room_ids. The cascade
	// RoomRemovedFromGroupEvent above handles that automatically
	// via the RoomGroups projection now.)

	c.logger.Info("Room deleted", "kind", kind, "room_id", room_id)

	if kind == KindChannel {
		c.notifyRoomLayoutChanged(ctx, actorID, "delete_room")
	}

	// Read-your-writes: every projection that needs to drop state
	// must have applied its event before we return.
	if err := c.RoomMembershipProjector.WaitForSeq(ctx, seq); err != nil {
		return fmt.Errorf("wait for membership projection: %w", err)
	}
	if err := c.RoomCatalogProjector.WaitForSeq(ctx, seq); err != nil {
		return fmt.Errorf("wait for catalog projection: %w", err)
	}
	if groupRemovedSeq > 0 {
		if err := c.RoomGroupsProjector.WaitForSeq(ctx, groupRemovedSeq); err != nil {
			return fmt.Errorf("wait for groups projection: %w", err)
		}
	}
	return nil
}

// ArchiveRoom sets a room's archived flag. Archived rooms are hidden
// from sidebars and Browse Rooms; existing memberships are preserved.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only.
func (c *ChattoCore) ArchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*corev1.Room, error) {
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	room.Archived = true

	archivedEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomArchived{
			RoomArchived: &corev1.RoomArchivedEvent{
				RoomId: roomID,
			},
		},
	})
	if _, err := c.RoomCatalogProjector.AppendAndWait(ctx, c.EventPublisher, events.RoomAggregate(roomID), archivedEvent); err != nil {
		return nil, fmt.Errorf("publish RoomArchivedEvent: %w", err)
	}

	subject := subjects.RoomMeta(string(kind), roomID)
	if err := c.publishServerEvent(ctx, subject, archivedEvent); err != nil {
		c.logger.Error("failed to publish room archived event (legacy)", "error", err, "room_id", roomID)
	}
	if err := c.PublishRoomGroupsUpdated(ctx, actorID, kind); err != nil {
		c.logger.Error("failed to publish room layout updated event after archive", "error", err)
	}

	c.logger.Info("Room archived", "kind", kind, "room_id", roomID)
	return room, nil
}

// UnarchiveRoom clears a room's archived flag. The room keeps its set
// position throughout the archive/unarchive cycle.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only.
func (c *ChattoCore) UnarchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*corev1.Room, error) {
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	room.Archived = false

	unarchivedEvent := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_RoomUnarchived{
			RoomUnarchived: &corev1.RoomUnarchivedEvent{
				RoomId: roomID,
			},
		},
	})
	if _, err := c.RoomCatalogProjector.AppendAndWait(ctx, c.EventPublisher, events.RoomAggregate(roomID), unarchivedEvent); err != nil {
		return nil, fmt.Errorf("publish RoomUnarchivedEvent: %w", err)
	}

	subject := subjects.RoomMeta(string(kind), roomID)
	if err := c.publishServerEvent(ctx, subject, unarchivedEvent); err != nil {
		c.logger.Error("failed to publish room unarchived event (legacy)", "error", err, "room_id", roomID)
	}
	if err := c.PublishRoomGroupsUpdated(ctx, actorID, kind); err != nil {
		c.logger.Error("failed to publish room layout updated event after unarchive", "error", err)
	}

	c.logger.Info("Room unarchived", "kind", kind, "room_id", roomID)
	return room, nil
}

// GetRoom retrieves a room by id.
//
// Reads come from RoomCatalog composed with RoomGroups for the
// group_id field. Returns ErrNotFound (wrapped) if the room isn't
// projected OR if its kind doesn't match the requested kind —
// keeping the "the wrong kind is not found" semantic so callers
// don't accidentally read a DM via a channel-kind probe.
func (c *ChattoCore) GetRoom(ctx context.Context, kind RoomKind, room_id string) (*corev1.Room, error) {
	room, ok := c.RoomCatalog.Get(room_id)
	if !ok || room.Kind != ProtoKindForRoomKind(kind) {
		return nil, fmt.Errorf("room not found: %w", jetstream.ErrKeyNotFound)
	}
	if gid := c.RoomGroups.GroupForRoom(room_id); gid != "" {
		room.GroupId = gid
	}
	return room, nil
}

// FindRoomByID resolves a room from its ID alone (no kind probe).
// Returns ErrNotFound if the room isn't in the catalog.
//
// Live events carry only a room ID (no kind discriminator on the
// wire), so resolvers and consumers downstream of those events use
// this to recover both the room and the kind context (via
// KindOfRoom on the result).
func (c *ChattoCore) FindRoomByID(ctx context.Context, room_id string) (*corev1.Room, error) {
	room, ok := c.RoomCatalog.Get(room_id)
	if !ok {
		return nil, ErrNotFound
	}
	if gid := c.RoomGroups.GroupForRoom(room_id); gid != "" {
		room.GroupId = gid
	}
	return room, nil
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

// ListRooms retrieves all rooms of the given kind from the
// RoomCatalog projection, composed with RoomGroups for the group_id
// field.
func (c *ChattoCore) ListRooms(ctx context.Context, kind RoomKind) ([]*corev1.Room, error) {
	rooms := c.RoomCatalog.AllByKind(ProtoKindForRoomKind(kind))
	for _, r := range rooms {
		if gid := c.RoomGroups.GroupForRoom(r.Id); gid != "" {
			r.GroupId = gid
		}
	}
	return rooms, nil
}

// RoomNameExists reports whether a channel room with the given name
// (case-insensitive, whitespace-trimmed) currently exists. ADR-035
// phase 6: served from RoomCatalog.FindByName.
func (c *ChattoCore) RoomNameExists(_ context.Context, _ RoomKind, name string) (bool, error) {
	return c.RoomCatalog.FindByName(name) != "", nil
}

// RoomNameExistsExcluding is like RoomNameExists but treats
// excludeRoomID as "free." Used by callers checking whether a rename
// would collide.
func (c *ChattoCore) RoomNameExistsExcluding(_ context.Context, _ RoomKind, name, excludeRoomID string) (bool, error) {
	owner := c.RoomCatalog.FindByName(name)
	return owner != "" && owner != excludeRoomID, nil
}
