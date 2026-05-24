package events

import "strings"

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
)

// ConfigSingletonID is the sentinel aggregate ID for server-wide config
// (ADR-034 singleton convention: server-scoped aggregates use a stable
// sentinel rather than introducing a different subject shape).
const ConfigSingletonID = "server"

// Aggregate identifies one event-sourced aggregate by type and ID. Every
// event for an aggregate lives on the single subject Subject() returns —
// per-subject sequence therefore equals per-aggregate sequence, and OCC
// against `Nats-Expected-Last-Subject-Sequence` serialises every write
// to one aggregate against every other.
//
// The event type intentionally does NOT appear in the subject. Event
// type is a property of the payload (the protobuf oneof variant on
// corev1.Event), not of the routing key. Keeping it out of the subject
// preserves single-source-of-truth for "what kind of event is this?"
// and makes adding new event types a zero-subject-change operation
// (one new oneof case + one new projection switch arm).
type Aggregate struct {
	Type string
	ID   string
}

// Subject returns the per-aggregate subject under SubjectRoot.
// Pattern: evt.{type}.{id}.
func (a Aggregate) Subject() string {
	return SubjectRoot + a.Type + "." + a.ID
}

// RoomAggregate is the typed constructor for a room-aggregate handle.
// All room lifecycle events (joins, leaves, deletes, future renames,
// permission overrides, etc.) publish to RoomAggregate(roomID).Subject().
func RoomAggregate(roomID string) Aggregate {
	return Aggregate{Type: AggregateRoom, ID: roomID}
}

// RoomSubjectFilter returns the wildcard subject filter for every room
// aggregate's events. Used by projections that consume across all rooms.
// Pattern: evt.room.>
func RoomSubjectFilter() string {
	return SubjectRoot + AggregateRoom + ".>"
}

// ConfigAggregate is the typed constructor for the singleton server-
// config aggregate. All server-config lifecycle events publish to
// ConfigAggregate().Subject() — pattern evt.config.server.
func ConfigAggregate() Aggregate {
	return Aggregate{Type: AggregateConfig, ID: ConfigSingletonID}
}

// ConfigSubjectFilter returns the wildcard subject filter for config
// aggregate events. Used by the ServerConfig projection.
// Pattern: evt.config.>
func ConfigSubjectFilter() string {
	return SubjectRoot + AggregateConfig + ".>"
}

// GroupAggregate is the typed constructor for a room-group aggregate
// handle. All group lifecycle events (created, renamed, deleted) and
// group room-membership events (room added/removed/reordered within
// the group) publish to GroupAggregate(groupID).Subject().
func GroupAggregate(groupID string) Aggregate {
	return Aggregate{Type: AggregateGroup, ID: groupID}
}

// GroupSubjectFilter returns the wildcard subject filter for every
// group aggregate's events. Used by the RoomGroup projection.
// Pattern: evt.group.>
func GroupSubjectFilter() string {
	return SubjectRoot + AggregateGroup + ".>"
}

// ParseGroupSubject extracts the groupID from a group aggregate
// subject. Accepts both the durable shape (evt.group.{groupID}) and
// the republished live shape (live.evt.group.{groupID}). Returns
// ok=false if the subject doesn't match.
func ParseGroupSubject(subject string) (groupID string, ok bool) {
	s := stripLivePrefix(subject)
	prefix := SubjectRoot + AggregateGroup + "."
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	rest := s[len(prefix):]
	if rest == "" || strings.Contains(rest, ".") {
		return "", false
	}
	return rest, true
}

// ParseRoomSubject extracts the roomID from a room aggregate subject.
// Accepts both the durable shape (evt.room.{roomID}) and the
// republished live shape (live.evt.room.{roomID}). Returns ok=false if
// the subject doesn't match either form.
func ParseRoomSubject(subject string) (roomID string, ok bool) {
	s := stripLivePrefix(subject)
	prefix := SubjectRoot + AggregateRoom + "."
	if !strings.HasPrefix(s, prefix) {
		return "", false
	}
	rest := s[len(prefix):]
	if rest == "" || strings.Contains(rest, ".") {
		return "", false
	}
	return rest, true
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
