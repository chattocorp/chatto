package events

import (
	"strings"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Subject roots for the event log. The durable stream stores events on
// SubjectRoot; the stream's RePublish config forwards them onto
// LiveSubjectRoot for live delivery.
//
// We don't use "server.evt." here because the legacy SERVER_EVENTS stream
// claims "server.>", which NATS treats as overlapping. Keeping the new
// roots on a separate top-level token sidesteps that without refactoring
// the legacy stream's subject filters.
const (
	SubjectRoot     = "evt."
	LiveSubjectRoot = "live.evt."
)

// Aggregate type segments. Stable identifiers; once written, never renamed.
const (
	AggregateRoom   = "room"
	AggregateConfig = "config"
	AggregateGroup  = "group"
	AggregateLayout = "layout"
)

// ConfigSingletonID is the sentinel aggregate ID for server-wide config
// (ADR-034 singleton convention: server-scoped aggregates use a stable
// sentinel rather than introducing a different subject shape).
const ConfigSingletonID = "server"

// LayoutSingletonID is the sentinel aggregate ID for the singleton
// sidebar layout. Same convention as ConfigSingletonID.
const LayoutSingletonID = "default"

// Event-type tokens. NATS-idiomatic snake_case; the trailing segment of
// every event subject. Stable identifiers — once written, never renamed.
//
// The canonical mapping lives in EventTypeOf below; constants here are
// just symbolic names for the same strings so call sites don't repeat
// string literals.
const (
	// Room aggregate
	EventRoomCreated    = "room_created"
	EventRoomUpdated    = "room_updated"
	EventRoomArchived   = "room_archived"
	EventRoomUnarchived = "room_unarchived"
	EventRoomDeleted    = "room_deleted"
	EventUserJoinedRoom = "user_joined"
	EventUserLeftRoom   = "user_left"

	// Messages (also under the room aggregate — every message event for
	// a room lives under evt.room.{R}.message_*, so a subscriber on
	// evt.room.{R}.> still receives the complete per-room timeline).
	// See issue #597. The "edited" and "retracted" tokens match the
	// MessageEditedEvent / MessageRetractedEvent proto names; if those
	// proto names are renamed in a future cleanup, subject tokens can
	// stay as-is (subjects are stable once written).
	EventMessagePosted    = "message_posted"
	EventMessageEdited    = "message_edited"
	EventMessageRetracted = "message_retracted"

	// Reactions (also under the room aggregate). Reaction state is
	// derived from these durable events by the reaction projection.
	EventReactionAdded   = "reaction_added"
	EventReactionRemoved = "reaction_removed"

	// Group aggregate
	EventRoomGroupCreated      = "group_created"
	EventRoomGroupUpdated      = "group_updated"
	EventRoomGroupDeleted      = "group_deleted"
	EventRoomAddedToGroup      = "room_added"
	EventRoomRemovedFromGroup  = "room_removed"
	EventRoomsInGroupReordered = "rooms_reordered"

	// Layout aggregate (singleton)
	EventRoomGroupsReordered = "groups_reordered"

	// Config aggregate (singleton)
	EventServerConfigChanged = "config_changed"
)

// EventTypeOf returns the canonical NATS subject token for an event's
// oneof variant. Returns "" if the event is nil or its oneof is unset.
//
// This is the single source of truth: the protobuf oneof drives the
// subject token, so the subject can't disagree with the payload by
// convention. New event types add a case here and nothing else changes.
func EventTypeOf(e *corev1.Event) string {
	if e == nil {
		return ""
	}
	switch e.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		return EventRoomCreated
	case *corev1.Event_RoomUpdated:
		return EventRoomUpdated
	case *corev1.Event_RoomArchived:
		return EventRoomArchived
	case *corev1.Event_RoomUnarchived:
		return EventRoomUnarchived
	case *corev1.Event_RoomDeleted:
		return EventRoomDeleted
	case *corev1.Event_UserJoinedRoom:
		return EventUserJoinedRoom
	case *corev1.Event_UserLeftRoom:
		return EventUserLeftRoom

	case *corev1.Event_MessagePosted:
		return EventMessagePosted
	case *corev1.Event_MessageEdited:
		return EventMessageEdited
	case *corev1.Event_MessageRetracted:
		return EventMessageRetracted

	case *corev1.Event_ReactionAdded:
		return EventReactionAdded
	case *corev1.Event_ReactionRemoved:
		return EventReactionRemoved

	case *corev1.Event_RoomGroupCreated:
		return EventRoomGroupCreated
	case *corev1.Event_RoomGroupUpdated:
		return EventRoomGroupUpdated
	case *corev1.Event_RoomGroupDeleted:
		return EventRoomGroupDeleted
	case *corev1.Event_RoomAddedToGroup:
		return EventRoomAddedToGroup
	case *corev1.Event_RoomRemovedFromGroup:
		return EventRoomRemovedFromGroup
	case *corev1.Event_RoomsInGroupReordered:
		return EventRoomsInGroupReordered

	case *corev1.Event_RoomGroupsReordered:
		return EventRoomGroupsReordered

	case *corev1.Event_ServerConfigChanged:
		return EventServerConfigChanged
	}
	return ""
}

// Aggregate identifies one event-sourced aggregate by type and ID. Every
// event for the aggregate lives under the prefix Subject("") returns;
// per-event subjects add an event-type trailing segment.
//
// Per-subject OCC against `Nats-Expected-Last-Subject-Sequence` operates
// at the (aggregate, event-type) granularity. Cross-event-type invariants
// use wildcard OCC against AllEventsFilter() via the
// `Nats-Expected-Last-Subject-Sequence-Subject` header (see
// Publisher.AppendAtFilter).
type Aggregate struct {
	Type string
	ID   string
}

// Subject returns the per-(aggregate, event-type) subject.
// Pattern: evt.{aggType}.{aggID}.{eventType}.
func (a Aggregate) Subject(eventType string) string {
	return SubjectRoot + a.Type + "." + a.ID + "." + eventType
}

// SubjectFor is like Subject but derives the event-type token from the
// event payload's oneof variant. Convenient when the caller already has
// the event built — pairs naturally with publisher helpers that take an
// Aggregate + Event rather than a raw subject.
func (a Aggregate) SubjectFor(e *corev1.Event) string {
	return a.Subject(EventTypeOf(e))
}

// AllEventsFilter returns the wildcard filter matching every event for
// THIS aggregate instance, across every event type. Used as the filter
// token for cross-event-type wildcard OCC (the "did anything else land
// on this aggregate?" guard) and as a wait-target for "any event on
// this aggregate."
// Pattern: evt.{aggType}.{aggID}.>
func (a Aggregate) AllEventsFilter() string {
	return SubjectRoot + a.Type + "." + a.ID + ".>"
}

// RoomAggregate is the typed constructor for a room-aggregate handle.
// All room lifecycle events (joins, leaves, deletes, renames, future
// additions) publish under RoomAggregate(roomID).
func RoomAggregate(roomID string) Aggregate {
	return Aggregate{Type: AggregateRoom, ID: roomID}
}

// GroupAggregate is the typed constructor for a room-group aggregate
// handle. All group lifecycle events and group room-membership events
// publish under GroupAggregate(groupID).
func GroupAggregate(groupID string) Aggregate {
	return Aggregate{Type: AggregateGroup, ID: groupID}
}

// LayoutAggregate is the typed constructor for the singleton sidebar
// layout aggregate. Owns inter-group ordering for the sidebar; the
// room-group set itself is owned by the group aggregate.
func LayoutAggregate() Aggregate {
	return Aggregate{Type: AggregateLayout, ID: LayoutSingletonID}
}

// ConfigAggregate is the typed constructor for the singleton server-
// config aggregate.
func ConfigAggregate() Aggregate {
	return Aggregate{Type: AggregateConfig, ID: ConfigSingletonID}
}

// RoomSubjectFilter returns the wildcard filter matching every event of
// every room aggregate, across all event types.
// Pattern: evt.room.>
func RoomSubjectFilter() string { return SubjectRoot + AggregateRoom + ".>" }

// GroupSubjectFilter returns the wildcard filter matching every event of
// every room-group aggregate.
// Pattern: evt.group.>
func GroupSubjectFilter() string { return SubjectRoot + AggregateGroup + ".>" }

// LayoutSubjectFilter returns the wildcard filter matching every event
// of the singleton layout aggregate.
// Pattern: evt.layout.>
func LayoutSubjectFilter() string { return SubjectRoot + AggregateLayout + ".>" }

// ConfigSubjectFilter returns the wildcard filter matching every event
// of the singleton config aggregate.
// Pattern: evt.config.>
func ConfigSubjectFilter() string { return SubjectRoot + AggregateConfig + ".>" }

// RoomEventTypeFilter returns a cross-aggregate, event-type-narrow
// filter — every event of the given type across every room. Used by
// projections that only care about a subset of event types and don't
// want to receive the full evt.room.> firehose.
// Pattern: evt.room.*.{eventType}
func RoomEventTypeFilter(eventType string) string {
	return SubjectRoot + AggregateRoom + ".*." + eventType
}

// GroupEventTypeFilter is the group analogue of RoomEventTypeFilter.
// Pattern: evt.group.*.{eventType}
func GroupEventTypeFilter(eventType string) string {
	return SubjectRoot + AggregateGroup + ".*." + eventType
}

// ParseRoomSubject extracts the roomID from a room-aggregate event
// subject. Accepts both the durable form (evt.room.{R}.{type}) and the
// republished live form (live.evt.room.{R}.{type}). Returns ok=false if
// the subject doesn't match either shape.
func ParseRoomSubject(subject string) (roomID string, ok bool) {
	return parseAggregateSubject(subject, AggregateRoom)
}

// ParseGroupSubject extracts the groupID from a group-aggregate event
// subject. Accepts durable and republished live forms.
func ParseGroupSubject(subject string) (groupID string, ok bool) {
	return parseAggregateSubject(subject, AggregateGroup)
}

// parseAggregateSubject extracts the aggregate ID from a subject of the
// form evt.{aggType}.{id}.{eventType} (or its live.evt.* republished
// form). The trailing event-type segment is discarded.
func parseAggregateSubject(subject, aggType string) (string, bool) {
	s := stripLivePrefix(subject)
	prefix := SubjectRoot + aggType + "."
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	rest := s[len(prefix):]
	dot := strings.Index(rest, ".")
	if dot < 1 || dot == len(rest)-1 {
		return "", false
	}
	// rest = {id}.{eventType}[.{anything else — shouldn't happen}]
	return rest[:dot], true
}

// stripLivePrefix returns the subject with the "live." prefix removed if
// present. Lets parsers treat durable and republished subjects uniformly.
func stripLivePrefix(subject string) string {
	const live = "live."
	if strings.HasPrefix(subject, live) {
		return subject[len(live):]
	}
	return subject
}
