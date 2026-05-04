package subjects

import "fmt"

// Single source of truth for NATS subject patterns. Per ADR-029 (PR 6 of the
// ADR-021 server consolidation) all subjects drop the historical
// `space.{spaceId}.` / `live.instance.` / `live.space.{spaceId}.` prefixes
// in favor of a flat server-wide namespace.
//
// Stored event subjects (in CHAT_EVENTS):
//   - room.{roomId}.msg.{eventId}                              — root messages
//   - room.{roomId}.msg.{rootEventId}.replies.{eventId}        — thread replies
//   - room.{roomId}.meta                                       — lifecycle + membership
//   - joined / left / member_deleted                           — server-wide structural
//
// Live subjects (bypass JetStream, ADR-012 two-tier split preserved):
//   - live.user.{userId}.{eventType}                           — per-user private
//   - live.server.{eventType}                                  — server-wide
//   - live.server.config.updated                               — server config change
//   - live.server.room.{roomId}.{eventType}                    — per-room
//
// Many functions still take a `spaceID` parameter because their callers
// pass one through — the parameter is ignored as of PR 6. The signature
// cleanup happens in PR 9 alongside the Instance → Server rename.

// ===== STORED EVENT SUBJECTS (CHAT_EVENTS stream) =====

// ServerJoinedSubject is the subject for server-wide "user joined" events.
const ServerJoinedSubject = "joined"

// ServerLeftSubject is the subject for server-wide "user left" events.
const ServerLeftSubject = "left"

// ServerMemberDeletedSubject is the subject for "user/membership deleted" events.
const ServerMemberDeletedSubject = "member_deleted"

// ChatEventsSubjects returns the explicit subject filter list for the
// CHAT_EVENTS stream. Per ADR-029 the filter must be explicit — never `>` —
// so future subject namespaces (live.>, audit.>, presence.>) cannot
// accidentally land in the durable stream.
func ChatEventsSubjects() []string {
	return []string{
		"room.>",
		ServerJoinedSubject,
		ServerLeftSubject,
		ServerMemberDeletedSubject,
	}
}

// SpaceEvent returns the subject for a server-level structural event.
// Pre-PR-6 this took a spaceID; the parameter is ignored now.
// Pattern: {eventType} — e.g. "joined", "left", "member_deleted".
func SpaceEvent(_unusedSpaceID, eventType string) string {
	return eventType
}

// SpaceAllEvents returns the wildcard for all server-level structural events
// across all rooms. Used for stream consumers.
// Pattern: {joined,left,member_deleted,room.>} — ChatEventsSubjects covers it.
func SpaceAllEvents(_unusedSpaceID string) string {
	// Single-subject wildcard equivalent isn't possible after PR 6 because
	// the stored subjects are split between top-level structural events
	// and `room.>`. Callers should use ChatEventsSubjects() for filter
	// lists; this helper returns the room half, which is what every
	// runtime caller historically wanted.
	return "room.>"
}

// ===== ROOM EVENT SUBJECTS =====

// SpaceRoomMessage returns the subject for a root message in a room.
// Pattern: room.{roomId}.msg.{eventId}
func SpaceRoomMessage(_unusedSpaceID, roomID, eventID string) string {
	return fmt.Sprintf("room.%s.msg.%s", roomID, eventID)
}

// SpaceRoomThread returns the subject for a thread reply.
// Pattern: room.{roomId}.msg.{rootEventId}.replies.{eventId}
func SpaceRoomThread(_unusedSpaceID, roomID, rootEventID, eventID string) string {
	return fmt.Sprintf("room.%s.msg.%s.replies.%s", roomID, rootEventID, eventID)
}

// SpaceRoomThreadFilter returns the wildcard for all replies in a specific thread.
// Pattern: room.{roomId}.msg.{rootEventId}.replies.>
func SpaceRoomThreadFilter(_unusedSpaceID, roomID, rootEventID string) string {
	return fmt.Sprintf("room.%s.msg.%s.replies.>", roomID, rootEventID)
}

// SpaceRoomThreadLookup returns the lookup pattern for a thread reply by
// event ID (any thread root).
// Pattern: room.{roomId}.msg.*.replies.{eventId}
func SpaceRoomThreadLookup(_unusedSpaceID, roomID, eventID string) string {
	return fmt.Sprintf("room.%s.msg.*.replies.%s", roomID, eventID)
}

// SpaceRoomAllThreads returns the wildcard for all thread events in a room.
// Pattern: room.{roomId}.msg.*.replies.>
func SpaceRoomAllThreads(_unusedSpaceID, roomID string) string {
	return fmt.Sprintf("room.%s.msg.*.replies.>", roomID)
}

// SpaceRoomMeta returns the subject for room meta events (lifecycle + membership).
// Pattern: room.{roomId}.meta
func SpaceRoomMeta(_unusedSpaceID, roomID string) string {
	return fmt.Sprintf("room.%s.meta", roomID)
}

// SpaceRoomAllMessages returns the wildcard for all messages (root + thread)
// in a room.
// Pattern: room.{roomId}.msg.>
func SpaceRoomAllMessages(_unusedSpaceID, roomID string) string {
	return fmt.Sprintf("room.%s.msg.>", roomID)
}

// SpaceRoomRootMessages returns the wildcard for root messages only in a room.
// The single wildcard matches one token (eventId) but excludes thread
// replies whose subjects have an extra `.replies.{eventId}` segment.
// Pattern: room.{roomId}.msg.*
func SpaceRoomRootMessages(_unusedSpaceID, roomID string) string {
	return fmt.Sprintf("room.%s.msg.*", roomID)
}

// SpaceRoomAllEvents returns the wildcard for all events in a specific room
// (messages + threads + meta).
// Pattern: room.{roomId}.>
func SpaceRoomAllEvents(_unusedSpaceID, roomID string) string {
	return fmt.Sprintf("room.%s.>", roomID)
}

// SpaceAllRoomEvents returns the wildcard for all room events server-wide.
// Pattern: room.>
func SpaceAllRoomEvents(_unusedSpaceID string) string {
	return "room.>"
}

// SpaceRoomRootEventsFilters returns the consumer filter list for root
// messages and meta events in a room (excludes thread replies).
// Returns: ["room.{r}.msg.*", "room.{r}.meta"]
func SpaceRoomRootEventsFilters(_unusedSpaceID, roomID string) []string {
	return []string{
		fmt.Sprintf("room.%s.msg.*", roomID),
		fmt.Sprintf("room.%s.meta", roomID),
	}
}

// SpaceAllRoomEventsFilters returns the consumer filter list for all
// messages and meta events across all rooms.
// Returns: ["room.*.msg.>", "room.*.meta"]
func SpaceAllRoomEventsFilters(_unusedSpaceID string) []string {
	return []string{
		"room.*.msg.>",
		"room.*.meta",
	}
}

// ===== SUBJECT PARSING =====

// ParseRoomIDFromSubject extracts the room ID from a room event subject.
// Returns the room ID for room.{r}.* subjects, or empty string for
// server-level structural events (joined/left/member_deleted) or any other
// subject that doesn't start with `room.`.
func ParseRoomIDFromSubject(subject string) string {
	parts := splitSubject(subject)
	// room.{roomId}.{...} — room ID at index 1, minimum 3 parts (room.{r}.meta)
	if len(parts) >= 3 && parts[0] == "room" {
		return parts[1]
	}
	return ""
}

// ParseThreadRootEventIDFromSubject extracts the root event ID from a thread
// reply subject.
// Pattern: room.{roomId}.msg.{rootEventId}.replies.{eventId} — 6 parts.
func ParseThreadRootEventIDFromSubject(subject string) (string, bool) {
	parts := splitSubject(subject)
	if len(parts) == 6 && parts[0] == "room" && parts[2] == "msg" && parts[4] == "replies" {
		return parts[3], true
	}
	return "", false
}

// IsRootMessageSubject reports whether a subject identifies a root (non-thread)
// message.
// Pattern: room.{roomId}.msg.{eventId} — 4 parts.
func IsRootMessageSubject(subject string) bool {
	parts := splitSubject(subject)
	return len(parts) == 4 && parts[0] == "room" && parts[2] == "msg"
}

// IsMetaSubject reports whether a subject identifies a room meta event.
// Pattern: room.{roomId}.meta — 3 parts.
func IsMetaSubject(subject string) bool {
	parts := splitSubject(subject)
	return len(parts) == 3 && parts[0] == "room" && parts[2] == "meta"
}

// IsThreadSubject reports whether a subject identifies a thread reply.
// Pattern: room.{roomId}.msg.{rootEventId}.replies.{eventId} — 6 parts.
func IsThreadSubject(subject string) bool {
	parts := splitSubject(subject)
	return len(parts) == 6 && parts[0] == "room" && parts[2] == "msg" && parts[4] == "replies"
}

// ParseEventIDFromSubject extracts the event ID from a message subject
// (root or thread reply).
// Patterns:
//   - room.{roomId}.msg.{eventId}                              → eventId at index 3
//   - room.{roomId}.msg.{rootEventId}.replies.{eventId}        → eventId at index 5
func ParseEventIDFromSubject(subject string) string {
	parts := splitSubject(subject)
	if len(parts) < 3 || parts[0] != "room" {
		return ""
	}
	if len(parts) == 4 && parts[2] == "msg" {
		return parts[3]
	}
	if len(parts) == 6 && parts[2] == "msg" && parts[4] == "replies" {
		return parts[5]
	}
	return ""
}

// splitSubject splits a NATS subject by dots.
func splitSubject(subject string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(subject); i++ {
		if subject[i] == '.' {
			parts = append(parts, subject[start:i])
			start = i + 1
		}
	}
	if start < len(subject) {
		parts = append(parts, subject[start:])
	}
	return parts
}

// ===== LIVE SUBJECT PATTERNS (live.>) =====
//
// Live subjects bypass JetStream — they're transient real-time notifications.
// Per ADR-012 / ADR-029 the namespace splits two-tier:
//   - live.user.{userId}.*    — private to that user
//   - live.server.*           — server-wide and per-room

// LiveAllEvents returns the wildcard for all live events.
// Pattern: live.>
func LiveAllEvents() string {
	return "live.>"
}

// LiveInstanceAllEvents is retained as a compatibility name for the wildcard
// across all live events. Pattern: live.>
func LiveInstanceAllEvents() string {
	return LiveAllEvents()
}

// LiveUserAllEvents returns the wildcard for all live events targeted at a
// specific user.
// Pattern: live.user.{userId}.>
func LiveUserAllEvents(userID string) string {
	return fmt.Sprintf("live.user.%s.>", userID)
}

// LiveInstanceUserAllEvents — compatibility shim, see LiveUserAllEvents.
func LiveInstanceUserAllEvents(userID string) string {
	return LiveUserAllEvents(userID)
}

// LiveUserEvent returns the live subject for a per-user private event.
// Pattern: live.user.{userId}.{eventType}
func LiveUserEvent(userID, eventType string) string {
	return fmt.Sprintf("live.user.%s.%s", userID, eventType)
}

// LiveInstanceUserEvent — compatibility shim, see LiveUserEvent.
func LiveInstanceUserEvent(userID, eventType string) string {
	return LiveUserEvent(userID, eventType)
}

// LiveServerAllEvents returns the wildcard for all server-wide live events
// (including per-room ones).
// Pattern: live.server.>
func LiveServerAllEvents() string {
	return "live.server.>"
}

// LiveSpaceAllEvents — compatibility shim. The spaceID parameter is ignored.
func LiveSpaceAllEvents(_unusedSpaceID string) string {
	return LiveServerAllEvents()
}

// LiveServerLevelEvents returns the wildcard for direct-children live.server
// events only (excludes the per-room subtree). The single wildcard matches
// one token, so live.server.room.{roomId}.{eventType} is excluded.
// Pattern: live.server.*
func LiveServerLevelEvents() string {
	return "live.server.*"
}

// LiveSpaceLevelEvents — compatibility shim, see LiveServerLevelEvents.
func LiveSpaceLevelEvents(_unusedSpaceID string) string {
	return LiveServerLevelEvents()
}

// LiveServerEvent returns the live subject for a server-wide event.
// Pattern: live.server.{eventType}
func LiveServerEvent(eventType string) string {
	return fmt.Sprintf("live.server.%s", eventType)
}

// LiveSpaceEvent — compatibility shim. The spaceID parameter is ignored.
func LiveSpaceEvent(_unusedSpaceID, eventType string) string {
	return LiveServerEvent(eventType)
}

// LiveServerRoomEvent returns the live subject for a per-room transient event.
// Pattern: live.server.room.{roomId}.{eventType}
func LiveServerRoomEvent(roomID, eventType string) string {
	return fmt.Sprintf("live.server.room.%s.%s", roomID, eventType)
}

// LiveSpaceRoomEvent — compatibility shim. The spaceID parameter is ignored.
func LiveSpaceRoomEvent(_unusedSpaceID, roomID, eventType string) string {
	return LiveServerRoomEvent(roomID, eventType)
}

// LiveServerRoomAllEvents returns the wildcard for all per-room live events.
// Pattern: live.server.room.>
func LiveServerRoomAllEvents() string {
	return "live.server.room.>"
}

// LiveSpaceRoomAllEvents — compatibility shim, see LiveServerRoomAllEvents.
func LiveSpaceRoomAllEvents(_unusedSpaceID string) string {
	return LiveServerRoomAllEvents()
}

// LiveServerRoomReactionEvents returns the wildcard for live reaction events
// across all rooms.
// Pattern: live.server.room.*.reaction_*
func LiveServerRoomReactionEvents() string {
	return "live.server.room.*.reaction_*"
}

// LiveSpaceRoomReactionEvents — compatibility shim, see LiveServerRoomReactionEvents.
func LiveSpaceRoomReactionEvents(_unusedSpaceID string) string {
	return LiveServerRoomReactionEvents()
}

// LiveServerConfigUpdated returns the live subject for server config update events.
// Pattern: live.server.config.updated
func LiveServerConfigUpdated() string {
	return "live.server.config.updated"
}

// LiveInstanceConfigUpdated — compatibility shim, see LiveServerConfigUpdated.
func LiveInstanceConfigUpdated() string {
	return LiveServerConfigUpdated()
}

// LiveServerConfigAllEvents returns the wildcard for all server config events.
// Pattern: live.server.config.>
func LiveServerConfigAllEvents() string {
	return "live.server.config.>"
}

// LiveInstanceConfigAllEvents — compatibility shim, see LiveServerConfigAllEvents.
func LiveInstanceConfigAllEvents() string {
	return LiveServerConfigAllEvents()
}

// LiveInstanceSpaceEvent — compatibility shim. Pre-ADR-029 this was a
// distinct subject namespace from LiveSpaceEvent; per the ADR they collapse
// to the same `live.server.{eventType}` subject. The spaceID parameter is
// ignored.
func LiveInstanceSpaceEvent(_unusedSpaceID, eventType string) string {
	return LiveServerEvent(eventType)
}
