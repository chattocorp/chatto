package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

// Room name validation constants
const (
	RoomNameMinLength        = 1
	RoomNameMaxLength        = 30
	RoomDescriptionMaxLength = 500
	// MaxRoomSlowModeSeconds is the longest supported per-user posting interval.
	MaxRoomSlowModeSeconds = 6 * 60 * 60
)

// ErrRoomNameExists is returned when a room with an equivalent normalized,
// case-folded name already exists.
var ErrRoomNameExists = errors.New("a room with this name already exists on this server")

// normalizeRoomName returns the NFC-normalized, whitespace-trimmed room name
// stored in durable room events and returned through public APIs.
func normalizeRoomName(name string) string {
	return norm.NFC.String(strings.TrimSpace(name))
}

var roomNameCaseFolder = cases.Fold()

// canonicalRoomName returns the compatibility-normalized, case-folded key used
// for room-name uniqueness and name-based lookups. Room display names remain
// NFC-normalized; this derived key is never persisted or shown to users.
func canonicalRoomName(name string) string {
	compatibilityNormalized := norm.NFKC.String(normalizeRoomName(name))
	return norm.NFKC.String(roomNameCaseFolder.String(compatibilityNormalized))
}

// ValidateRoomName validates a room name and returns an error if invalid.
// Room names accept visible Unicode text but reject controls and line-breaking
// separators that would disrupt user-interface layout.
func ValidateRoomName(name string) error {
	normalized := normalizeRoomName(name)
	if utf8.RuneCountInString(normalized) < RoomNameMinLength {
		return fmt.Errorf("room name is required")
	}
	if utf8.RuneCountInString(normalized) > RoomNameMaxLength {
		return fmt.Errorf("room name must be %d characters or less", RoomNameMaxLength)
	}

	for _, ch := range normalized {
		if unicode.IsControl(ch) || unicode.Is(unicode.Zl, ch) || unicode.Is(unicode.Zp, ch) {
			return fmt.Errorf("room name must not contain control characters or line breaks")
		}
	}
	if !HasVisibleContent(normalized) {
		return fmt.Errorf("room name must contain at least one visible character")
	}

	return nil
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

type createRoomOptions struct {
	universal                  bool
	threadingMode              evtv1.RoomThreadingMode
	applyAnnouncementsDefaults bool
	importContext              *evtv1.ImportContext
	importReceipt              func(string) evtstream.BatchEntry
}

// CreateRoomOption customizes room creation for trusted/internal callers.
type CreateRoomOption func(*createRoomOptions)

// WithUniversalRoom sets the initial universal membership flag for a channel
// room. DM rooms reject universal membership at CreateRoom validation time.
func WithUniversalRoom(universal bool) CreateRoomOption {
	return func(options *createRoomOptions) {
		options.universal = universal
	}
}

// WithRoomThreadingMode sets the initial threading policy for a channel room.
// Omitting this option preserves the ENABLED default.
func WithRoomThreadingMode(mode evtv1.RoomThreadingMode) CreateRoomOption {
	return func(options *createRoomOptions) {
		options.threadingMode = mode
	}
}

// IsValidRoomThreadingMode reports whether mode is an explicit channel-room
// policy. UNSPECIFIED is reserved for omitted historical data and DMs.
func IsValidRoomThreadingMode(mode evtv1.RoomThreadingMode) bool {
	switch mode {
	case evtv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED,
		evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED,
		evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED,
		evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED:
		return true
	default:
		return false
	}
}

// EffectiveRoomThreadingMode normalizes persisted room data. Historical
// channel events predate the field and therefore resolve UNSPECIFIED to
// ENABLED. DMs always use ENABLED. Unknown future channel values fail closed
// to DISABLED.
func EffectiveRoomThreadingMode(room *readmodel.Room) evtv1.RoomThreadingMode {
	if room == nil {
		return evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED
	}
	if room.GetKind() == evtv1.RoomKind_ROOM_KIND_DM {
		return evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED
	}
	if room.GetThreadingMode() == evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED {
		return evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED
	}
	if IsValidRoomThreadingMode(room.GetThreadingMode()) {
		return room.GetThreadingMode()
	}
	return evtv1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED
}

func normalizedRoomThreadingMode(kind evtv1.RoomKind, mode evtv1.RoomThreadingMode) evtv1.RoomThreadingMode {
	return EffectiveRoomThreadingMode(&readmodel.Room{Kind: kind, ThreadingMode: mode})
}

// WithAnnouncementsRoomDefaults applies the built-in announcements room's
// creation-time posting permissions. It is for first-boot seeding only; a
// user-created room does not gain special permissions from its display name.
func WithAnnouncementsRoomDefaults() CreateRoomOption {
	return func(options *createRoomOptions) {
		options.applyAnnouncementsDefaults = true
	}
}

func collectCreateRoomOptions(opts []CreateRoomOption) createRoomOptions {
	var options createRoomOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

// CreateRoom creates a new room.
// Authorization: Caller must verify CanCreateRoom before calling.
//
// groupID identifies the RoomGroup the room belongs to. DM rooms pass
// empty. For channel rooms an empty groupID auto-routes to the first
// group in the layout (seed "Lobby" group on fresh deployments) — see
// ADR-031.
//
// ADR-035 phase 6: event-only. Name uniqueness is enforced via
// JetStream wildcard OCC against `evt.room.>` — the room service
// reads a catalog snapshot containing both the name owner and the
// applied evt.room.> sequence, then publishes RoomCreatedEvent, any selected
// channel room-group membership fact, and any channel-room default permission
// facts as one atomic batch with that seq as the expected-last for the filter.
// Concurrent room mutations from any process (this one or another replica)
// advance the filter's seq and cause our publish to fail; we re-check
// uniqueness from the (now-caught-up) projection and retry.
func (c *ChattoCore) CreateRoom(ctx context.Context, actorID string, kind RoomKind, groupID, name, description string, opts ...CreateRoomOption) (*readmodel.Room, error) {
	return c.roomCommands.createRoom(ctx, actorID, kind, groupID, name, description, opts...)
}

func defaultAnnouncementsRoomDecisions(roomID string) []rbacSeedDecision {
	var decisions []rbacSeedDecision
	appendRoleDecisions := func(roleName string, permissions []Permission, decision DecisionKind) {
		for _, permission := range permissions {
			decisions = append(decisions, rbacSeedDecision{
				scope:       ScopeRoom,
				scopeID:     roomID,
				subjectKind: evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_ROLE,
				subject:     roleName,
				permission:  permission,
				decision:    decision,
			})
		}
	}

	appendRoleDecisions(RoleEveryone, DefaultAnnouncementsEveryoneDenials(), DecisionDeny)
	appendRoleDecisions(RoleAdmin, DefaultAnnouncementsAdminPermissions(), DecisionAllow)
	appendRoleDecisions(RoleOwner, []Permission{PermMessagePost}, DecisionAllow)
	return decisions
}

// DeleteRoom deletes a room.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only. Atomically publishes RoomDeletedEvent (which the
// room directory applies to both catalog and membership indexes) and, for a
// channel room in a group, RoomRemovedFromGroupEvent per ADR-086. Historical
// room events are retained in EVT; the legacy KV room record is no longer
// touched here.
func (m *RoomCommandModel) DeleteRoom(ctx context.Context, actorID string, kind RoomKind, room_id string) error {
	agg := evtstream.RoomAggregate(room_id)
	filter := agg.AllEventsFilter()
	var deletedSubject string
	var deletedSeq uint64
	var groupRemovedSubject string
	var groupRemovedSeq uint64
	for attempt := range maxJoinRoomRetries {
		streamSeq, err := m.publisher.LastStreamSeq(ctx)
		if err != nil {
			return fmt.Errorf("read stream OCC tail before room deletion: %w", err)
		}
		roomPosition, err := m.publisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return fmt.Errorf("read room deletion OCC tail: %w", err)
		}
		groupPosition, err := m.publisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
		if err != nil {
			return fmt.Errorf("read room-group OCC tail before room deletion: %w", err)
		}
		if !roomPosition.IsZero() {
			if err := m.rooms.waitForDirectory(ctx, roomPosition); err != nil {
				return fmt.Errorf("wait for room before deletion: %w", err)
			}
		}
		if !groupPosition.IsZero() {
			if err := m.rooms.waitForGroupLayout(ctx, groupPosition); err != nil {
				return fmt.Errorf("wait for room-group layout before room deletion: %w", err)
			}
		}
		room, err := m.rooms.GetRoom(ctx, kind, room_id)
		if err != nil {
			return err
		}
		event := newEvent(actorID, &evtv1.Event{
			Event: &evtv1.Event_RoomDeleted{
				RoomDeleted: &evtv1.RoomDeletedEvent{RoomId: room_id},
			},
		})
		deletedSubject = agg.SubjectFor(event)
		entries := []evtstream.BatchEntry{{
			Subject: deletedSubject,
			Event:   event,
			Expect:  events.ExpectFilterSeq(filter, roomPosition.Seq),
		}}
		groupRemovedSubject = ""
		if kind == KindChannel && room.GetGroupId() != "" {
			removed := newEvent(actorID, &evtv1.Event{
				Event: &evtv1.Event_RoomRemovedFromGroup{
					RoomRemovedFromGroup: &evtv1.RoomRemovedFromGroupEvent{GroupId: room.GetGroupId(), RoomId: room_id},
				},
			})
			groupRemovedSubject = evtstream.GroupAggregate(room.GetGroupId()).SubjectFor(removed)
			entries = append(entries, evtstream.BatchEntry{
				Subject: groupRemovedSubject,
				Event:   removed,
				Expect:  events.ExpectFilterSeq(evtstream.GroupSubjectFilter(), groupPosition.Seq),
			})
		} else if kind == KindChannel {
			// Legacy unassigned rooms have no group event that can carry the
			// group OCC guard. The stream guard prevents a concurrent repair or
			// move from adding the room to a group after this decision.
			entries[0].Expect = entries[0].Expect.AndStreamSeq(streamSeq)
		}
		seqs, err := m.publisher.AppendBatch(ctx, entries)
		if errors.Is(err, events.ErrConflict) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("publish RoomDeletedEvent: %w", err)
		}
		if len(seqs) == 0 {
			return errors.New("room deletion committed no event")
		}
		deletedSeq = seqs[0]
		if groupRemovedSubject != "" {
			groupRemovedSeq = seqs[1]
		}
		break
	}
	if deletedSeq == 0 {
		return fmt.Errorf("publish room deletion retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
	}

	m.logger.Info("Room deleted", "kind", kind, "room_id", room_id)

	// Read-your-writes: every projection that needs to drop state
	// must have applied its event before we return.
	if err := m.rooms.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(deletedSubject, deletedSeq)); err != nil {
		return err
	}
	if groupRemovedSeq > 0 {
		if err := m.rooms.waitForGroupLayout(ctx, events.SubjectPosition(groupRemovedSubject, groupRemovedSeq)); err != nil {
			return err
		}
	}
	return nil
}

// ArchiveRoom sets a room's archived flag. Archived rooms are hidden
// from sidebars and Browse Rooms; existing memberships are preserved.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only.
func (c *ChattoCore) ArchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*readmodel.Room, error) {
	return c.roomCommands.archiveRoom(ctx, actorID, kind, roomID)
}

// UnarchiveRoom clears a room's archived flag. The room keeps its set
// position throughout the archive/unarchive cycle.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only.
func (c *ChattoCore) UnarchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*readmodel.Room, error) {
	return c.roomCommands.unarchiveRoom(ctx, actorID, kind, roomID)
}

// GetRoom retrieves a room by id.
//
// Reads come from RoomModel's room catalog and group-layout state for the
// group_id field. Returns ErrNotFound (wrapped) if the room isn't
// projected OR if its kind doesn't match the requested kind —
// keeping the "the wrong kind is not found" semantic so callers
// don't accidentally read a DM via a channel-kind probe.
func (c *ChattoCore) GetRoom(ctx context.Context, kind RoomKind, room_id string) (*readmodel.Room, error) {
	return c.roomModel.GetRoom(ctx, kind, room_id)
}

// FindRoomByID resolves a room from its ID alone (no kind probe).
// Returns ErrNotFound if the room isn't in the catalog.
//
// Live events carry only a room ID (no kind discriminator on the
// wire), so resolvers and consumers downstream of those events use
// this to recover both the room and the kind context (via
// KindOfRoom on the result).
func (c *ChattoCore) FindRoomByID(ctx context.Context, room_id string) (*readmodel.Room, error) {
	return c.roomModel.FindRoomByID(ctx, room_id)
}

// FindRoomKind is a thin wrapper around FindRoomByID for callers that
// only need the kind. The room load is paid either way; the wrapper is
// just there for ergonomics.
func (c *ChattoCore) FindRoomKind(ctx context.Context, room_id string) (RoomKind, error) {
	return c.roomModel.FindRoomKind(ctx, room_id)
}

// ListRooms retrieves all rooms of the given kind from the
// RoomModel's room catalog, composed with its group-layout state for the
// group_id field.
func (c *ChattoCore) ListRooms(ctx context.Context, kind RoomKind) ([]*readmodel.Room, error) {
	return c.roomModel.ListRooms(ctx, kind)
}

// MemberRoomListOptions controls optional filtering/sorting for ListMemberRooms.
type MemberRoomListOptions struct {
	// RequireLastMessage excludes rooms that have never received a message.
	RequireLastMessage bool
	// SortByLastMessageDesc sorts rooms by latest message time, newest first.
	// Rooms without messages sort last when RequireLastMessage is false.
	SortByLastMessageDesc bool
}

// ListMemberRooms retrieves rooms of the given kind that the user participates
// in. It is the shared room-list primitive for member-scoped room surfaces;
// callers layer product policy on top with MemberRoomListOptions.
func (m *RoomMembershipModel) ListMemberRooms(ctx context.Context, kind RoomKind, userID string, opts MemberRoomListOptions) ([]*readmodel.Room, error) {
	roomIDs := m.rooms.explicitRoomIDsForUser(userID)
	seen := make(map[string]struct{}, len(roomIDs))

	type listedRoom struct {
		room          *readmodel.Room
		lastMessageAt time.Time
	}
	listed := make([]listedRoom, 0, len(roomIDs))

	for _, roomID := range roomIDs {
		room, err := m.rooms.GetRoom(ctx, kind, roomID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("lookup room %s: %w", roomID, err)
		}

		var lastMessageAt time.Time
		if opts.RequireLastMessage || opts.SortByLastMessageDesc {
			lastMessageAt, err = m.rooms.GetRoomLastMessageAt(ctx, kind, room.Id)
			if err != nil {
				return nil, fmt.Errorf("lookup last message for room %s: %w", room.Id, err)
			}
			if opts.RequireLastMessage && lastMessageAt.IsZero() {
				continue
			}
		}

		listed = append(listed, listedRoom{room: room, lastMessageAt: lastMessageAt})
		seen[room.Id] = struct{}{}
	}

	if kind == KindChannel {
		all, err := m.rooms.ListRooms(ctx, kind)
		if err != nil {
			return nil, err
		}
		for _, room := range all {
			if room == nil || !room.GetUniversal() {
				continue
			}
			if _, ok := seen[room.Id]; ok {
				continue
			}
			canJoin, err := m.policy.CanJoinRoomAt(ctx, userID, kind, room.Id)
			if err != nil {
				return nil, err
			}
			if !canJoin {
				continue
			}

			var lastMessageAt time.Time
			if opts.RequireLastMessage || opts.SortByLastMessageDesc {
				lastMessageAt, err = m.rooms.GetRoomLastMessageAt(ctx, kind, room.Id)
				if err != nil {
					return nil, fmt.Errorf("lookup last message for room %s: %w", room.Id, err)
				}
				if opts.RequireLastMessage && lastMessageAt.IsZero() {
					continue
				}
			}
			listed = append(listed, listedRoom{room: room, lastMessageAt: lastMessageAt})
			seen[room.Id] = struct{}{}
		}
	}

	if opts.SortByLastMessageDesc {
		sort.SliceStable(listed, func(i, j int) bool {
			return listed[i].lastMessageAt.After(listed[j].lastMessageAt)
		})
	}

	rooms := make([]*readmodel.Room, len(listed))
	for i, r := range listed {
		rooms[i] = r.room
	}
	return rooms, nil
}

// RoomNameExists reports whether a channel room with the given name exists
// after trimming, Unicode compatibility normalization, and full case folding.
// ADR-035 phase 6: served from RoomCatalog.FindByName.
func (c *ChattoCore) RoomNameExists(_ context.Context, _ RoomKind, name string) (bool, error) {
	return c.roomModel.roomIDByName(name) != "", nil
}

// RoomNameExistsExcluding is like RoomNameExists but treats
// excludeRoomID as "free." Used by callers checking whether a rename
// would collide.
func (c *ChattoCore) RoomNameExistsExcluding(_ context.Context, _ RoomKind, name, excludeRoomID string) (bool, error) {
	return c.roomModel.nameClaimSnapshot(name, excludeRoomID).ConflictingRoomID != "", nil
}
