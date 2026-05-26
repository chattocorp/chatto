package subjects

import (
	"fmt"
	"strings"
)

// This file is the single source of truth for all NATS subject patterns in
// the system. All functions are pure and construct subjects from entity
// IDs.
//
// Subject shapes:
//
//	Room messages (root):
//	  server.room.{kind}.{roomId}.msg.{eventId}
//	Room messages (thread reply):
//	  server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}
//	Room meta (lifecycle/membership):
//	  server.room.{kind}.{roomId}.meta
//	Server-level events (membership lifecycle):
//	  server.member.{verb}
//
// `kind` is one of `"channel"` or `"dm"` and lets list operations prefix-
// filter without loading the room record.

// ===== SERVER STREAM SUBJECTS =====

// AllEvents returns the wildcard subject matching every event in the
// SERVER_EVENTS stream. Pattern: `server.>`.
func AllEvents() string {
	return "server.>"
}

// Member returns the subject for a server-level membership event.
//
// The verb is the event type with any leading `member_` stripped (so the
// token is `joined`/`left`/`deleted` rather than `member_joined` etc.).
//
// Pattern: `server.member.{verb}`.
func Member(eventType string) string {
	return fmt.Sprintf("server.member.%s", strings.TrimPrefix(eventType, "member_"))
}

// ===== ROOM EVENT SUBJECTS =====

// RoomMessage returns the subject for a root message event.
// Pattern: `server.room.{kind}.{roomId}.msg.{eventId}`.
func RoomMessage(kind, roomID, eventID string) string {
	return fmt.Sprintf("server.room.%s.%s.msg.%s", kind, roomID, eventID)
}

// RoomThread returns the subject for a thread reply message.
// Pattern: `server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}`.
func RoomThread(kind, roomID, rootEventID, eventID string) string {
	return fmt.Sprintf("server.room.%s.%s.msg.%s.replies.%s", kind, roomID, rootEventID, eventID)
}

// RoomThreadFilter returns the wildcard subject for all replies in a
// specific thread.
// Pattern: `server.room.{kind}.{roomId}.msg.{rootEventId}.replies.>`.
func RoomThreadFilter(kind, roomID, rootEventID string) string {
	return fmt.Sprintf("server.room.%s.%s.msg.%s.replies.>", kind, roomID, rootEventID)
}

// RoomThreadLookup returns the wildcard subject for looking up a thread
// reply by event ID via GetLastMsgForSubject.
// Pattern: `server.room.{kind}.{roomId}.msg.*.replies.{eventId}`.
func RoomThreadLookup(kind, roomID, eventID string) string {
	return fmt.Sprintf("server.room.%s.%s.msg.*.replies.%s", kind, roomID, eventID)
}

// RoomAllThreads returns the wildcard subject for all thread events in a
// room. Pattern: `server.room.{kind}.{roomId}.msg.*.replies.>`.
func RoomAllThreads(kind, roomID string) string {
	return fmt.Sprintf("server.room.%s.%s.msg.*.replies.>", kind, roomID)
}

// RoomMeta returns the subject for non-message room events (lifecycle,
// membership). Pattern: `server.room.{kind}.{roomId}.meta`.
func RoomMeta(kind, roomID string) string {
	return fmt.Sprintf("server.room.%s.%s.meta", kind, roomID)
}

// RoomAllMessages returns the wildcard subject for all messages (root +
// thread) in a room. Pattern: `server.room.{kind}.{roomId}.msg.>`.
func RoomAllMessages(kind, roomID string) string {
	return fmt.Sprintf("server.room.%s.%s.msg.>", kind, roomID)
}

// RoomRootMessages returns the wildcard subject for root messages only in
// a room. Pattern: `server.room.{kind}.{roomId}.msg.*` — the single-token
// wildcard excludes thread replies (which carry an extra suffix).
func RoomRootMessages(kind, roomID string) string {
	return fmt.Sprintf("server.room.%s.%s.msg.*", kind, roomID)
}

// RoomAllEvents returns the filter subject for all events in a specific
// room. Matches messages, threads, and meta events.
// Pattern: `server.room.{kind}.{roomId}.>`.
func RoomAllEvents(kind, roomID string) string {
	return fmt.Sprintf("server.room.%s.%s.>", kind, roomID)
}

// AllRoomEvents returns the wildcard subject for all room events of a
// given kind. Pattern: `server.room.{kind}.>`.
func AllRoomEvents(kind string) string {
	return fmt.Sprintf("server.room.%s.>", kind)
}

// RoomRootEventsFilters returns filter subjects for root messages and
// meta events in a single room. Excludes thread replies.
//
// Returns: [`server.room.{kind}.{roomId}.msg.*`, `server.room.{kind}.{roomId}.meta`].
//
// Use with JetStream consumer FilterSubjects for efficient server-side
// filtering.
func RoomRootEventsFilters(kind, roomID string) []string {
	return []string{
		fmt.Sprintf("server.room.%s.%s.msg.*", kind, roomID),
		fmt.Sprintf("server.room.%s.%s.meta", kind, roomID),
	}
}

// AllRoomEventsFilters returns filter subjects for all messages (root +
// thread) and meta events across all rooms of a kind.
//
// Returns: [`server.room.{kind}.*.msg.>`, `server.room.{kind}.*.meta`].
//
// Use with JetStream consumer FilterSubjects for live subscriptions that
// need all messages.
func AllRoomEventsFilters(kind string) []string {
	return []string{
		fmt.Sprintf("server.room.%s.*.msg.>", kind),
		fmt.Sprintf("server.room.%s.*.meta", kind),
	}
}

// AllRoomEventsFiltersAnyKind returns filter subjects for all room events
// (channel + dm), used by the unified deployment-wide live subscription.
//
// Returns: [`server.room.*.*.msg.>`, `server.room.*.*.meta`].
func AllRoomEventsFiltersAnyKind() []string {
	return []string{
		"server.room.*.*.msg.>",
		"server.room.*.*.meta",
	}
}

// LiveRoomAllEventsAnyKind returns the live subject for all transient room
// events across both kinds. Pattern: `live.server.room.>`.
func LiveRoomAllEventsAnyKind() string {
	return "live.server.room.>"
}

// ===== PARSERS =====
//
// Subject shape recap (parsers are used at message-receive time, where
// the subject is fully concrete):
//
//	server.room.{kind}.{roomId}.msg.{eventId}                            (root)
//	server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}      (thread)
//	server.room.{kind}.{roomId}.meta                                     (meta)
//
// JetStream republish (`server.>` → `live.server.>`) means the same
// subjects also arrive with a leading `live` segment. Parsers normalize
// via stripLivePrefix so both shapes share one set of length checks.

// ParseRoomIDFromSubject extracts the room ID from a room event subject.
// Returns "" for non-room subjects or unrecognized shapes.
func ParseRoomIDFromSubject(subject string) string {
	parts := stripLivePrefix(splitSubject(subject))
	if len(parts) >= 5 && isRoomEventSubject(parts) {
		return parts[3]
	}
	return ""
}

// ParseKindFromRoomSubject extracts the room kind ("channel" or "dm") from a
// room-event subject — durable (`server.room.{kind}.>`) or live
// (`live.server.room.{kind}.>`). Returns "" for non-room subjects.
func ParseKindFromRoomSubject(subject string) string {
	parts := stripLivePrefix(splitSubject(subject))
	if len(parts) >= 3 && parts[0] == "server" && parts[1] == "room" {
		return parts[2]
	}
	return ""
}

// ParseThreadRootEventIDFromSubject extracts the root event ID from a
// thread reply subject. Returns ("", false) for non-thread subjects.
func ParseThreadRootEventIDFromSubject(subject string) (string, bool) {
	parts := stripLivePrefix(splitSubject(subject))
	if len(parts) == 8 && isRoomEventSubject(parts) && parts[4] == "msg" && parts[6] == "replies" {
		return parts[5], true
	}
	return "", false
}

// IsRootMessageSubject reports whether a subject is for a top-level (root)
// message — 6 segments with `msg` at index 4.
func IsRootMessageSubject(subject string) bool {
	parts := stripLivePrefix(splitSubject(subject))
	return len(parts) == 6 && isRoomEventSubject(parts) && parts[4] == "msg"
}

// IsMetaSubject reports whether a subject is for a meta event — 5
// segments with `meta` at index 4.
func IsMetaSubject(subject string) bool {
	parts := stripLivePrefix(splitSubject(subject))
	return len(parts) == 5 && isRoomEventSubject(parts) && parts[4] == "meta"
}

// IsThreadSubject reports whether a subject is for a thread reply — 8
// segments with `msg` at index 4 and `replies` at index 6.
func IsThreadSubject(subject string) bool {
	parts := stripLivePrefix(splitSubject(subject))
	return len(parts) == 8 && isRoomEventSubject(parts) && parts[4] == "msg" && parts[6] == "replies"
}

// ParseEventIDFromSubject extracts the event ID from a message subject.
// Returns "" for non-message subjects.
func ParseEventIDFromSubject(subject string) string {
	parts := stripLivePrefix(splitSubject(subject))
	if len(parts) < 5 || !isRoomEventSubject(parts) {
		return ""
	}
	if len(parts) == 6 && parts[4] == "msg" {
		return parts[5]
	}
	if len(parts) == 8 && parts[4] == "msg" && parts[6] == "replies" {
		return parts[7]
	}
	return ""
}

// stripLivePrefix removes a leading `live` segment so that durable
// (`server.>`) and republished/live (`live.server.>`) subjects share
// one canonical shape. Returns the original slice if not prefixed.
func stripLivePrefix(parts []string) []string {
	if len(parts) > 0 && parts[0] == "live" {
		return parts[1:]
	}
	return parts
}

// isRoomEventSubject reports whether the dot-split segments belong to a
// room event subject of the shape `server.room.{kind}.{roomId}.>`.
// Callers can read parts[3] as the room ID after this returns true.
func isRoomEventSubject(parts []string) bool {
	return len(parts) >= 4 && parts[0] == "server" && parts[1] == "room"
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

// ===== LIVE SUBJECTS =====
//
// Live subjects are used for transient events that bypass JetStream
// storage. Same `server.>` namespace as the durable subjects above.

// LiveUserAllEvents returns the live subject wildcard for all events
// scoped to a specific user. Pattern: `live.server.user.{userId}.>`.
func LiveUserAllEvents(userID string) string {
	return fmt.Sprintf("live.server.user.%s.>", userID)
}

// LiveUserScopedAllEvents returns the live subject wildcard for all
// user-scoped events (any user). Pattern: `live.server.user.>`.
func LiveUserScopedAllEvents() string {
	return "live.server.user.>"
}

// LiveUserEvent returns the live subject for a specific user's event.
// Pattern: `live.server.user.{userId}.{eventType}`.
func LiveUserEvent(userID, eventType string) string {
	return fmt.Sprintf("live.server.user.%s.%s", userID, eventType)
}

// LiveAllEvents returns the live subject for all server-scoped live events.
// Pattern: `live.server.>`.
func LiveAllEvents() string {
	return "live.server.>"
}

// LiveMemberAllEvents returns the live subject for non-room server-level
// live events. Pattern: `live.server.member.>`.
func LiveMemberAllEvents() string {
	return "live.server.member.>"
}

// LiveMember returns the live subject for a server-level membership event.
// `member_` prefix is stripped from `eventType` (mirrors Member).
// Pattern: `live.server.member.{verb}`.
func LiveMember(eventType string) string {
	return fmt.Sprintf("live.server.member.%s", strings.TrimPrefix(eventType, "member_"))
}

// LiveRoomEvent returns the live subject for a room event.
// Pattern: `live.server.room.{kind}.{roomId}.{eventType}`.
func LiveRoomEvent(kind, roomID, eventType string) string {
	return fmt.Sprintf("live.server.room.%s.%s.%s", kind, roomID, eventType)
}

// LiveRoomAllEvents returns the live subject for all transient room events
// of a given kind. Pattern: `live.server.room.{kind}.>`.
func LiveRoomAllEvents(kind string) string {
	return fmt.Sprintf("live.server.room.%s.>", kind)
}

// LiveRoomReactionEvents returns the subscription subject for all reaction
// live mirrors of a given kind.
// Pattern: `live.server.room.{kind}.*.reaction_*`.
func LiveRoomReactionEvents(kind string) string {
	return fmt.Sprintf("live.server.room.%s.*.reaction_*", kind)
}

// ===== SERVER-SCOPED LIVE SUBJECT PATTERNS =====
// For transient deployment-wide events that bypass JetStream (config
// changes, server branding, room layout, etc.). Fanout to all members;
// server-side authorization filtering happens in the subscriber.

// LiveConfigEvent returns the live subject for a deployment-wide config
// event. Pattern: `live.server.config.{eventType}`. Fanout — every
// connected user receives it and the subscriber applies authorization.
func LiveConfigEvent(eventType string) string {
	return fmt.Sprintf("live.server.config.%s", eventType)
}

// LiveConfigAllEvents returns the wildcard subject for all server config
// events. Pattern: `live.server.config.>`.
func LiveConfigAllEvents() string {
	return "live.server.config.>"
}
