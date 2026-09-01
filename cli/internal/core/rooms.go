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

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// getRoomLastRootEvent returns the most recent root MessagePostedEvent
// (excluding thread replies) in a room, or nil if none have been
// projected yet. Bounded O(walk-until-found) via the projection's
// LastVisibleRoomEntry helper.
func (c *ChattoCore) getRoomLastRootEvent(roomID string) *evtv1.Event {
	entry, ok := c.roomModel.lastVisibleRoomEntry(roomID, func(e *evtv1.Event) bool {
		msg := e.GetMessagePosted()
		return msg != nil && msg.GetInThread() == ""
	})
	if !ok {
		return nil
	}
	return entry.Event
}

// getRoomLastMessageEvent returns the most recent MessagePostedEvent
// of any kind (root or thread reply) in a room, or nil. It uses the
// projection's message-post index because thread replies are not part of the
// visible room timeline.
func (c *ChattoCore) getRoomLastMessageEvent(roomID string) *evtv1.Event {
	entry, ok := c.roomModel.lastRoomMessageEntry(roomID)
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
// ENABLED. Unknown future values fail closed to DISABLED. DMs remain
// threadless.
func EffectiveRoomThreadingMode(room *evtv1.Room) evtv1.RoomThreadingMode {
	if room == nil || room.GetKind() == evtv1.RoomKind_ROOM_KIND_DM {
		return evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED
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
	return EffectiveRoomThreadingMode(&evtv1.Room{Kind: kind, ThreadingMode: mode})
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
func (c *ChattoCore) CreateRoom(ctx context.Context, actorID string, kind RoomKind, groupID, name, description string, opts ...CreateRoomOption) (*evtv1.Room, error) {
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}
	options := collectCreateRoomOptions(opts)
	if kind == KindDM && options.universal {
		return nil, fmt.Errorf("DM rooms cannot be universal")
	}
	if kind == KindDM && options.applyAnnouncementsDefaults {
		return nil, fmt.Errorf("DM rooms cannot use announcements defaults")
	}
	if kind == KindDM && options.threadingMode != evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED {
		return nil, invalidArgument("DM rooms cannot configure threading")
	}
	if kind == KindChannel && options.threadingMode != evtv1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED && !IsValidRoomThreadingMode(options.threadingMode) {
		return nil, invalidArgument("invalid room threading mode")
	}
	threadingMode := normalizedRoomThreadingMode(ProtoKindForRoomKind(kind), options.threadingMode)

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

	name = normalizeRoomName(name)
	room_id := NewRoomID()

	room := &evtv1.Room{
		Id:            room_id,
		Kind:          ProtoKindForRoomKind(kind),
		Name:          name,
		Description:   description,
		GroupId:       groupID,
		Universal:     options.universal,
		ThreadingMode: threadingMode,
	}

	createdEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomCreated{
			RoomCreated: &evtv1.RoomCreatedEvent{
				RoomId:        room_id,
				Name:          name,
				Description:   description,
				Kind:          ProtoKindForRoomKind(kind),
				Universal:     options.universal,
				ThreadingMode: threadingMode,
			},
		},
	})

	var defaultPermissionEntries []evtstream.BatchEntry
	if kind == KindChannel && options.applyAnnouncementsDefaults {
		defaultPermissionEntries = rbacSeedEntries(nil, nil, defaultAnnouncementsRoomDecisions(room_id))
	}
	additionalEntries := func(ctx context.Context) ([]evtstream.BatchEntry, error) {
		entries := make([]evtstream.BatchEntry, 0, len(defaultPermissionEntries)+1)
		if groupID != "" {
			groupPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
			if err != nil {
				return nil, fmt.Errorf("read room-group OCC position before room creation: %w", err)
			}
			if !groupPosition.IsZero() {
				if err := c.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
					return nil, fmt.Errorf("wait for room group before room creation: %w", err)
				}
			}
			if _, ok := c.roomModel.roomGroup(groupID); !ok {
				return nil, ErrRoomGroupNotFound
			}
			added := newEvent(actorID, &evtv1.Event{
				Event: &evtv1.Event_RoomAddedToGroup{
					RoomAddedToGroup: &evtv1.RoomAddedToGroupEvent{GroupId: groupID, RoomId: room_id},
				},
			})
			entries = append(entries, evtstream.BatchEntry{
				Subject:       evtstream.GroupAggregate(groupID).SubjectFor(added),
				Event:         added,
				HasOCC:        true,
				ExpectedSeq:   groupPosition.Seq,
				FilterSubject: evtstream.GroupSubjectFilter(),
			})
		}
		entries = append(entries, defaultPermissionEntries...)
		return entries, nil
	}
	seqs, err := c.publishRoomEventWithNameOCCEntries(ctx, name, createdEvent, room_id, additionalEntries)
	if err != nil {
		return nil, err
	}
	createdSeq := seqs[0]

	c.logger.Info("Room created", "kind", kind, "room_id", room_id, "name", name, "group_id", groupID)

	createdSubject := evtstream.RoomAggregate(room_id).SubjectFor(createdEvent)
	if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(createdSubject, createdSeq)); err != nil {
		return nil, err
	}
	if groupID != "" {
		groupEntryIndex := 1
		groupSubject := evtstream.GroupAggregate(groupID).Subject(evtstream.EventRoomAddedToGroup)
		if err := c.roomModel.waitForGroupLayout(ctx, events.SubjectPosition(groupSubject, seqs[groupEntryIndex])); err != nil {
			return nil, fmt.Errorf("wait for created room group membership: %w", err)
		}
		c.notifyRoomLayoutChanged(ctx, actorID, "create_room")
	}
	if len(defaultPermissionEntries) > 0 {
		last := len(defaultPermissionEntries) - 1
		if err := c.rbacModel.waitFor(ctx, events.SubjectPosition(defaultPermissionEntries[last].Subject, seqs[len(seqs)-1])); err != nil {
			return nil, fmt.Errorf("wait for channel room defaults: %w", err)
		}
	}
	return room, nil
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
	return decisions
}

// SetRoomUniversal updates a channel room's universal membership flag.
// Authorization: Caller must verify CanManageAnyRoom before calling.
func (c *ChattoCore) SetRoomUniversal(ctx context.Context, actorID string, kind RoomKind, roomID string, universal bool) (*evtv1.Room, error) {
	if kind == KindDM {
		return nil, fmt.Errorf("DM rooms cannot be universal")
	}
	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
		expectedSeq, err := c.EventPublisher.LastSubjectSeq(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("read universal-room OCC tail: %w", err)
		}
		if expectedSeq > 0 {
			if err := c.roomModel.waitForDirectory(ctx, events.SubjectPosition(filter, expectedSeq)); err != nil {
				return nil, fmt.Errorf("wait for room before universal-room change: %w", err)
			}
		}
		room, err := c.GetRoom(ctx, kind, roomID)
		if err != nil {
			return nil, err
		}
		if room.GetUniversal() == universal {
			return room, nil
		}
		event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomUniversalChanged{
			RoomUniversalChanged: &evtv1.RoomUniversalChangedEvent{RoomId: roomID, Universal: universal},
		}})
		subject := agg.SubjectFor(event)
		seqs, err := c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{
			Subject: subject, Event: event, HasOCC: true, ExpectedSeq: expectedSeq, FilterSubject: filter,
		}})
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("publish RoomUniversalChangedEvent: %w", err)
		}
		pos := events.SubjectPosition(subject, seqs[0])
		if err := c.roomModel.waitForDirectoryAndTimeline(ctx, pos); err != nil {
			return nil, err
		}
		c.logger.Info("Room universal flag updated", "kind", kind, "room_id", roomID, "universal", universal)
		return c.GetRoom(ctx, kind, roomID)
	}
	return nil, fmt.Errorf("publish universal-room change retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
}

// SetRoomSlowMode updates a channel room's per-user posting interval.
// Authorization: Caller must verify room.manage before calling.
func (c *ChattoCore) SetRoomSlowMode(ctx context.Context, actorID string, kind RoomKind, roomID string, seconds uint32) (*evtv1.Room, error) {
	if kind == KindDM {
		return nil, invalidArgument("DM rooms cannot use slow mode")
	}
	if seconds > MaxRoomSlowModeSeconds {
		return nil, invalidArgument("slow mode cannot exceed 21600 seconds")
	}
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	if room.GetSlowModeSeconds() == seconds {
		return room, nil
	}

	event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomSlowModeChanged{
		RoomSlowModeChanged: &evtv1.RoomSlowModeChangedEvent{RoomId: roomID, SlowModeSeconds: seconds},
	}})
	pos, err := c.roomModel.appendDirectoryEventually(ctx, c.EventPublisher, evtstream.RoomAggregate(roomID), event)
	if err != nil {
		return nil, fmt.Errorf("publish RoomSlowModeChangedEvent: %w", err)
	}
	if err := c.roomModel.waitForTimeline(ctx, pos); err != nil {
		return nil, err
	}

	c.logger.Info("Room slow mode updated", "kind", kind, "room_id", roomID, "seconds", seconds)
	return c.GetRoom(ctx, kind, roomID)
}

// SetRoomThreadingMode updates a channel room's threading policy. The room
// aggregate protects room state, while authorization is evaluated from stable
// request-time inputs.
// Authorization: Caller must verify room.manage before calling.
func (c *ChattoCore) SetRoomThreadingMode(ctx context.Context, actorID string, kind RoomKind, roomID string, mode evtv1.RoomThreadingMode) (*evtv1.Room, error) {
	return c.setRoomThreadingMode(ctx, actorID, kind, roomID, mode, nil)
}

func (c *ChattoCore) setRoomThreadingMode(
	ctx context.Context,
	actorID string,
	kind RoomKind,
	roomID string,
	mode evtv1.RoomThreadingMode,
	authorize func(context.Context) error,
) (*evtv1.Room, error) {
	if kind == KindDM {
		return nil, invalidArgument("DM rooms cannot configure threading")
	}
	if !IsValidRoomThreadingMode(mode) {
		return nil, invalidArgument("invalid room threading mode")
	}

	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
		prepared, err := c.prepareMessageAppendAttempt(ctx, agg, func(attemptCtx context.Context) error {
			if authorize != nil {
				return authorize(attemptCtx)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		room, err := c.GetRoom(ctx, kind, roomID)
		if err != nil {
			return nil, err
		}
		if EffectiveRoomThreadingMode(room) == mode {
			return room, nil
		}
		event := newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RoomThreadingModeChanged{
			RoomThreadingModeChanged: &evtv1.RoomThreadingModeChangedEvent{RoomId: roomID, ThreadingMode: mode},
		}})
		subject := agg.SubjectFor(event)
		seqs, err := c.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{
			Subject: subject, Event: event, HasOCC: true, ExpectedSeq: prepared.roomSeq, FilterSubject: filter,
		}})
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("publish RoomThreadingModeChangedEvent: %w", err)
		}
		pos := events.SubjectPosition(subject, seqs[0])
		if err := c.roomModel.waitForDirectoryAndTimeline(ctx, pos); err != nil {
			return nil, err
		}
		c.logger.Info("Room threading mode updated", "kind", kind, "room_id", roomID, "threading_mode", mode.String())
		return c.GetRoom(ctx, kind, roomID)
	}
	return nil, fmt.Errorf("publish threading-mode change retry exhausted after %d attempts: %w", maxJoinRoomRetries, events.ErrConflict)
}

// publishRoomEventWithNameOCC publishes a name-claiming room event
// (RoomCreated or RoomUpdated) with cluster-wide name uniqueness enforced via
// JetStream wildcard OCC against `evt.room.>`. When additional entries are
// supplied, the name-claiming event and those entries commit atomically.
//
// The flow per attempt:
//  1. Read the catalog name-claim snapshot for the desired `name`;
//     if any other room holds it, return ErrRoomNameExists immediately.
//  2. Publish the event, and any additional entries, with the snapshot's
//     applied evt.room.> seq.
//     The projected state and OCC token describe the same observed
//     event-log prefix.
//  3. JetStream
//     rejects with ErrConflict if any evt.room.> message landed in the
//     read-publish window — backoff briefly and retry.
//
// excludeRoomID is the ID to exclude from the uniqueness check —
// used by UpdateRoom so a room can keep a name it already holds
// (e.g. case-only changes, or no-op renames).
func (c *ChattoCore) publishRoomEventWithNameOCC(ctx context.Context, name string, event *evtv1.Event, excludeRoomID string, additionalEntries ...evtstream.BatchEntry) ([]uint64, error) {
	return c.publishRoomEventWithNameOCCEntries(ctx, name, event, excludeRoomID, func(context.Context) ([]evtstream.BatchEntry, error) {
		return append([]evtstream.BatchEntry(nil), additionalEntries...), nil
	})
}

// publishRoomEventWithNameOCCEntries is the retry-aware form of
// publishRoomEventWithNameOCC. It rebuilds additional entries after each OCC
// conflict so their expected positions and projection-derived facts describe
// the same event-log prefix as the next commit attempt.
func (c *ChattoCore) publishRoomEventWithNameOCCEntries(
	ctx context.Context,
	name string,
	event *evtv1.Event,
	excludeRoomID string,
	buildAdditionalEntries func(context.Context) ([]evtstream.BatchEntry, error),
) ([]uint64, error) {
	// Determine publish subject from the event payload. Room events
	// all target the per-room aggregate subject; this doesn't change
	// across retries.
	var roomID string
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RoomCreated:
		roomID = e.RoomCreated.GetRoomId()
	case *evtv1.Event_RoomUpdated:
		roomID = e.RoomUpdated.GetRoomId()
	default:
		return nil, fmt.Errorf("publishRoomEventWithNameOCC: unsupported event type %T", e)
	}
	publishSubject := evtstream.RoomAggregate(roomID).SubjectFor(event)
	occFilter := evtstream.RoomSubjectFilter()

	for attempt := 0; attempt < maxRoomNameClaimRetries; attempt++ {
		snapshot := c.roomModel.nameClaimSnapshot(name, excludeRoomID)
		if snapshot.ConflictingRoomID != "" {
			return nil, ErrRoomNameExists
		}
		additionalEntries, err := buildAdditionalEntries(ctx)
		if err != nil {
			return nil, err
		}

		var seqs []uint64
		if len(additionalEntries) == 0 {
			var seq uint64
			seq, err = c.EventPublisher.AppendAtFilter(ctx, publishSubject, event, occFilter, snapshot.Seq)
			seqs = []uint64{seq}
		} else {
			entries := make([]evtstream.BatchEntry, 1, len(additionalEntries)+1)
			entries[0] = evtstream.BatchEntry{
				Subject:       publishSubject,
				Event:         event,
				ExpectedSeq:   snapshot.Seq,
				FilterSubject: occFilter,
				HasOCC:        true,
			}
			entries = append(entries, additionalEntries...)
			seqs, err = c.EventPublisher.AppendBatch(ctx, entries)
		}
		if err == nil {
			return seqs, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return nil, err
		}

		if err := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); err != nil {
			return nil, fmt.Errorf("wait for room directory after OCC conflict: %w", err)
		}

		// Filter advanced under us after the snapshot. Backoff briefly
		// and retry — the next attempt reads a fresh projection snapshot.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("room name OCC retry exhausted after %d attempts: %w", maxRoomNameClaimRetries, events.ErrConflict)
}

// UpdateRoom updates an existing room's mutable fields (name +
// description). Authorization: Caller must verify CanManageAnyRoom
// before calling.
//
// ADR-035 phase 6: event-only. Renames go through the wildcard-OCC
// path to enforce cluster-wide name uniqueness (see
// publishRoomEventWithNameOCC); description-only edits skip the
// uniqueness check and use a plain per-subject OCC.
func (c *ChattoCore) UpdateRoom(ctx context.Context, actorID string, kind RoomKind, room_id, name, description string) (*evtv1.Room, error) {
	if err := ValidateRoomName(name); err != nil {
		return nil, err
	}
	if err := ValidateRoomDescription(description); err != nil {
		return nil, err
	}

	name = normalizeRoomName(name)

	room, err := c.GetRoom(ctx, kind, room_id)
	if err != nil {
		return nil, err
	}

	// "Rename" here means the derived compatibility-normalized, case-folded
	// comparison key changed. Equivalent display-only edits (for example,
	// "general" → "General") can skip the wildcard OCC dance.
	renamed := canonicalRoomName(room.Name) != canonicalRoomName(name)

	room.Name = name
	room.Description = description

	updatedEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomUpdated{
			RoomUpdated: &evtv1.RoomUpdatedEvent{
				RoomId:      room_id,
				Name:        name,
				Description: description,
			},
		},
	})

	var updatedSeq uint64
	if renamed {
		seqs, publishErr := c.publishRoomEventWithNameOCC(ctx, name, updatedEvent, room_id)
		err = publishErr
		if err == nil {
			updatedSeq = seqs[0]
		}
		if err != nil {
			return nil, err
		}
	} else {
		updatedSeq, err = c.EventPublisher.Append(ctx, evtstream.RoomAggregate(room_id).SubjectFor(updatedEvent), updatedEvent)
		if err != nil {
			return nil, fmt.Errorf("publish RoomUpdatedEvent: %w", err)
		}
	}

	c.logger.Info("Room updated", "kind", kind, "room_id", room_id, "name", name)

	updatedSubject := evtstream.RoomAggregate(room_id).SubjectFor(updatedEvent)
	if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(updatedSubject, updatedSeq)); err != nil {
		return nil, err
	}
	return room, nil
}

// DeleteRoom deletes a room.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only. Atomically publishes RoomDeletedEvent (which the
// room directory applies to both catalog and membership indexes) and, for a
// channel room in a group, RoomRemovedFromGroupEvent per ADR-086. Historical
// room events are retained in EVT; the legacy KV room record is no longer
// touched here.
func (c *ChattoCore) DeleteRoom(ctx context.Context, actorID string, kind RoomKind, room_id string) error {
	agg := evtstream.RoomAggregate(room_id)
	filter := agg.AllEventsFilter()
	var deletedSubject string
	var deletedSeq uint64
	var groupRemovedSubject string
	var groupRemovedSeq uint64
	for attempt := 0; attempt < maxJoinRoomRetries; attempt++ {
		streamSeq, err := c.EventPublisher.LastStreamSeq(ctx)
		if err != nil {
			return fmt.Errorf("read stream OCC tail before room deletion: %w", err)
		}
		roomPosition, err := c.EventPublisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return fmt.Errorf("read room deletion OCC tail: %w", err)
		}
		groupPosition, err := c.EventPublisher.LastSubjectPosition(ctx, evtstream.GroupSubjectFilter())
		if err != nil {
			return fmt.Errorf("read room-group OCC tail before room deletion: %w", err)
		}
		if !roomPosition.IsZero() {
			if err := c.roomModel.waitForDirectory(ctx, roomPosition); err != nil {
				return fmt.Errorf("wait for room before deletion: %w", err)
			}
		}
		if !groupPosition.IsZero() {
			if err := c.roomModel.waitForGroupLayout(ctx, groupPosition); err != nil {
				return fmt.Errorf("wait for room-group layout before room deletion: %w", err)
			}
		}
		room, err := c.GetRoom(ctx, kind, room_id)
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
			Subject: deletedSubject, Event: event, HasOCC: true, ExpectedSeq: roomPosition.Seq, FilterSubject: filter,
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
				Subject:       groupRemovedSubject,
				Event:         removed,
				HasOCC:        true,
				ExpectedSeq:   groupPosition.Seq,
				FilterSubject: evtstream.GroupSubjectFilter(),
			})
		} else if kind == KindChannel {
			// Legacy unassigned rooms have no group event that can carry the
			// group OCC guard. The stream guard prevents a concurrent repair or
			// move from adding the room to a group after this decision.
			entries[0].HasStreamOCC = true
			entries[0].ExpectedStreamSeq = streamSeq
		}
		seqs, err := c.EventPublisher.AppendBatch(ctx, entries)
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

	c.logger.Info("Room deleted", "kind", kind, "room_id", room_id)

	// Read-your-writes: every projection that needs to drop state
	// must have applied its event before we return.
	if err := c.roomModel.waitForDirectoryAndTimeline(ctx, events.SubjectPosition(deletedSubject, deletedSeq)); err != nil {
		return err
	}
	if groupRemovedSeq > 0 {
		if err := c.roomModel.waitForGroupLayout(ctx, events.SubjectPosition(groupRemovedSubject, groupRemovedSeq)); err != nil {
			return err
		}
	}
	if kind == KindChannel {
		c.notifyRoomLayoutChanged(ctx, actorID, "delete_room")
	}
	return nil
}

// ArchiveRoom sets a room's archived flag. Archived rooms are hidden
// from sidebars and Browse Rooms; existing memberships are preserved.
// Authorization: Caller must verify CanManageAnyRoom before calling.
//
// ADR-035 phase 6: event-only.
func (c *ChattoCore) ArchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*evtv1.Room, error) {
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	room.Archived = true

	archivedEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomArchived{
			RoomArchived: &evtv1.RoomArchivedEvent{
				RoomId: roomID,
			},
		},
	})
	pos, err := c.roomModel.appendDirectoryEventually(ctx, c.EventPublisher, evtstream.RoomAggregate(roomID), archivedEvent)
	if err != nil {
		return nil, fmt.Errorf("publish RoomArchivedEvent: %w", err)
	}
	if err := c.roomModel.waitForTimeline(ctx, pos); err != nil {
		return nil, err
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
func (c *ChattoCore) UnarchiveRoom(ctx context.Context, actorID string, kind RoomKind, roomID string) (*evtv1.Room, error) {
	room, err := c.GetRoom(ctx, kind, roomID)
	if err != nil {
		return nil, err
	}
	room.Archived = false

	unarchivedEvent := newEvent(actorID, &evtv1.Event{
		Event: &evtv1.Event_RoomUnarchived{
			RoomUnarchived: &evtv1.RoomUnarchivedEvent{
				RoomId: roomID,
			},
		},
	})
	pos, err := c.roomModel.appendDirectoryEventually(ctx, c.EventPublisher, evtstream.RoomAggregate(roomID), unarchivedEvent)
	if err != nil {
		return nil, fmt.Errorf("publish RoomUnarchivedEvent: %w", err)
	}
	if err := c.roomModel.waitForTimeline(ctx, pos); err != nil {
		return nil, err
	}

	if err := c.PublishRoomGroupsUpdated(ctx, actorID, kind); err != nil {
		c.logger.Error("failed to publish room layout updated event after unarchive", "error", err)
	}

	c.logger.Info("Room unarchived", "kind", kind, "room_id", roomID)
	return room, nil
}

// GetRoom retrieves a room by id.
//
// Reads come from RoomModel's room catalog and group-layout state for the
// group_id field. Returns ErrNotFound (wrapped) if the room isn't
// projected OR if its kind doesn't match the requested kind —
// keeping the "the wrong kind is not found" semantic so callers
// don't accidentally read a DM via a channel-kind probe.
func (c *ChattoCore) GetRoom(ctx context.Context, kind RoomKind, room_id string) (*evtv1.Room, error) {
	room, ok := c.roomModel.room(room_id)
	if !ok || room.Kind != ProtoKindForRoomKind(kind) {
		return nil, fmt.Errorf("room not found: %w", jetstream.ErrKeyNotFound)
	}
	if gid := c.roomModel.roomGroupForRoom(room_id); gid != "" {
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
func (c *ChattoCore) FindRoomByID(ctx context.Context, room_id string) (*evtv1.Room, error) {
	room, ok := c.roomModel.room(room_id)
	if !ok {
		return nil, ErrNotFound
	}
	if gid := c.roomModel.roomGroupForRoom(room_id); gid != "" {
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
// RoomModel's room catalog, composed with its group-layout state for the
// group_id field.
func (c *ChattoCore) ListRooms(ctx context.Context, kind RoomKind) ([]*evtv1.Room, error) {
	rooms := c.roomModel.roomsByKind(ProtoKindForRoomKind(kind))
	for _, r := range rooms {
		if gid := c.roomModel.roomGroupForRoom(r.Id); gid != "" {
			r.GroupId = gid
		}
	}
	return rooms, nil
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
func (c *ChattoCore) ListMemberRooms(ctx context.Context, kind RoomKind, userID string, opts MemberRoomListOptions) ([]*evtv1.Room, error) {
	roomIDs := c.roomModel.explicitRoomIDsForUser(userID)
	seen := make(map[string]struct{}, len(roomIDs))

	type listedRoom struct {
		room          *evtv1.Room
		lastMessageAt time.Time
	}
	listed := make([]listedRoom, 0, len(roomIDs))

	for _, roomID := range roomIDs {
		room, err := c.GetRoom(ctx, kind, roomID)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("lookup room %s: %w", roomID, err)
		}

		var lastMessageAt time.Time
		if opts.RequireLastMessage || opts.SortByLastMessageDesc {
			lastMessageAt, err = c.GetRoomLastMessageAt(ctx, kind, room.Id)
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
		all, err := c.ListRooms(ctx, kind)
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
			canJoin, err := c.CanJoinRoomAt(ctx, userID, kind, room.Id)
			if err != nil {
				return nil, err
			}
			if !canJoin {
				continue
			}

			var lastMessageAt time.Time
			if opts.RequireLastMessage || opts.SortByLastMessageDesc {
				lastMessageAt, err = c.GetRoomLastMessageAt(ctx, kind, room.Id)
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

	rooms := make([]*evtv1.Room, len(listed))
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
