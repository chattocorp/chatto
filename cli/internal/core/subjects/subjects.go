package subjects

import "fmt"

// This file is the single source of truth for all NATS subject patterns in
// the system. All functions are pure and construct subjects from entity
// IDs.
//
// Two subject namespaces coexist during the #330 phase 4d migration:
//
//   - Legacy per-space (`space.{spaceId}.>`): for non-primary, non-DM
//     spaces. Used by the per-space `SPACE_{spaceId}_EVENTS` JetStream
//     streams.
//
//   - Consolidated (`server.>`): for the configured primary space and the
//     DM system space. Used by the deployment-wide `SERVER_EVENTS` stream
//     introduced in phase 4d.
//
// Each helper picks one of the two via `shouldUseServerSubjects(spaceID)`.
// The room kind (channel/dm) is part of the consolidated subject so list
// operations can prefix-filter without loading the room record — same
// principle as the SERVER_CONFIG key prefix introduced in phase 4b.
//
// Subject shapes:
//
//   Room messages (root):
//     space.{spaceId}.room.{roomId}.msg.{eventId}
//     server.room.{kind}.{roomId}.msg.{eventId}
//   Room messages (thread reply):
//     space.{spaceId}.room.{roomId}.msg.{rootEventId}.replies.{eventId}
//     server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}
//   Room meta (lifecycle/membership):
//     space.{spaceId}.room.{roomId}.meta
//     server.room.{kind}.{roomId}.meta
//   Space-level (currently membership only):
//     space.{spaceId}.{eventType}
//     server.member.{eventType}
//
// Both formats place roomID at the same dot-segment index (3) and eventID
// at index 5 (root) / 7 (thread reply), so parsers only need a prefix
// check, not separate offset tables.

// ===== SPACE STREAM SUBJECTS =====

// SpaceEvent returns the subject for space-level events.
//
// Legacy: space.{spaceId}.{eventType}
// Server: server.member.{eventType}
//
// Currently used only for membership lifecycle (member_deleted etc.).
// Other "space-level" lifecycle (joined/left) is published via
// LiveInstanceUserEvent against the user, not the space.
func SpaceEvent(spaceID, eventType string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.member.%s", eventType)
	}
	return fmt.Sprintf("space.%s.%s", spaceID, eventType)
}

// SpaceAllEvents returns the wildcard subject for all space-level events.
//
// Legacy: space.{spaceId}.>
// Server: server.>
func SpaceAllEvents(spaceID string) string {
	if shouldUseServerSubjects(spaceID) {
		return "server.>"
	}
	return fmt.Sprintf("space.%s.>", spaceID)
}

// ===== ROOM EVENT SUBJECTS =====

// SpaceRoomMessage returns the subject for a root message event.
//
// Legacy: space.{spaceId}.room.{roomId}.msg.{eventId}
// Server: server.room.{kind}.{roomId}.msg.{eventId}
func SpaceRoomMessage(spaceID, roomID, eventID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.msg.%s", roomKind(spaceID), roomID, eventID)
	}
	return fmt.Sprintf("space.%s.room.%s.msg.%s", spaceID, roomID, eventID)
}

// SpaceRoomThread returns the subject for a thread reply message.
//
// Legacy: space.{spaceId}.room.{roomId}.msg.{rootEventId}.replies.{eventId}
// Server: server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}
func SpaceRoomThread(spaceID, roomID, rootEventID, eventID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.msg.%s.replies.%s", roomKind(spaceID), roomID, rootEventID, eventID)
	}
	return fmt.Sprintf("space.%s.room.%s.msg.%s.replies.%s", spaceID, roomID, rootEventID, eventID)
}

// SpaceRoomThreadFilter returns the wildcard subject for all replies in a
// specific thread.
//
// Legacy: space.{spaceId}.room.{roomId}.msg.{rootEventId}.replies.>
// Server: server.room.{kind}.{roomId}.msg.{rootEventId}.replies.>
func SpaceRoomThreadFilter(spaceID, roomID, rootEventID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.msg.%s.replies.>", roomKind(spaceID), roomID, rootEventID)
	}
	return fmt.Sprintf("space.%s.room.%s.msg.%s.replies.>", spaceID, roomID, rootEventID)
}

// SpaceRoomThreadLookup returns the wildcard subject for looking up a
// thread reply by event ID via GetLastMsgForSubject.
//
// Legacy: space.{spaceId}.room.{roomId}.msg.*.replies.{eventId}
// Server: server.room.{kind}.{roomId}.msg.*.replies.{eventId}
func SpaceRoomThreadLookup(spaceID, roomID, eventID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.msg.*.replies.%s", roomKind(spaceID), roomID, eventID)
	}
	return fmt.Sprintf("space.%s.room.%s.msg.*.replies.%s", spaceID, roomID, eventID)
}

// SpaceRoomAllThreads returns the wildcard subject for all thread events
// in a room.
//
// Legacy: space.{spaceId}.room.{roomId}.msg.*.replies.>
// Server: server.room.{kind}.{roomId}.msg.*.replies.>
func SpaceRoomAllThreads(spaceID, roomID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.msg.*.replies.>", roomKind(spaceID), roomID)
	}
	return fmt.Sprintf("space.%s.room.%s.msg.*.replies.>", spaceID, roomID)
}

// SpaceRoomMeta returns the subject for non-message room events
// (lifecycle, membership).
//
// Legacy: space.{spaceId}.room.{roomId}.meta
// Server: server.room.{kind}.{roomId}.meta
func SpaceRoomMeta(spaceID, roomID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.meta", roomKind(spaceID), roomID)
	}
	return fmt.Sprintf("space.%s.room.%s.meta", spaceID, roomID)
}

// SpaceRoomAllMessages returns the wildcard subject for all messages
// (root + thread) in a room. Used for deriving last-message timestamps.
//
// Legacy: space.{spaceId}.room.{roomId}.msg.>
// Server: server.room.{kind}.{roomId}.msg.>
func SpaceRoomAllMessages(spaceID, roomID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.msg.>", roomKind(spaceID), roomID)
	}
	return fmt.Sprintf("space.%s.room.%s.msg.>", spaceID, roomID)
}

// SpaceRoomRootMessages returns the wildcard subject for root messages
// only in a room. The single-token wildcard (*) excludes thread replies
// (which have an additional `.replies.{eventId}` suffix).
//
// Legacy: space.{spaceId}.room.{roomId}.msg.*
// Server: server.room.{kind}.{roomId}.msg.*
func SpaceRoomRootMessages(spaceID, roomID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.msg.*", roomKind(spaceID), roomID)
	}
	return fmt.Sprintf("space.%s.room.%s.msg.*", spaceID, roomID)
}

// SpaceRoomAllEvents returns the filter subject for all events in a
// specific room. Matches messages, threads, and meta events.
//
// Legacy: space.{spaceId}.room.{roomId}.>
// Server: server.room.{kind}.{roomId}.>
func SpaceRoomAllEvents(spaceID, roomID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.%s.>", roomKind(spaceID), roomID)
	}
	return fmt.Sprintf("space.%s.room.%s.>", spaceID, roomID)
}

// SpaceAllRoomEvents returns the wildcard subject for all room events
// across a space.
//
// Legacy: space.{spaceId}.room.>
// Server: server.room.{kind}.>
//
// In the server format, this filter is scoped to a specific kind so
// channels and DMs stay distinguishable in subscriptions that are
// already context-aware (a primary-space subscriber shouldn't see DM
// traffic, and vice versa).
func SpaceAllRoomEvents(spaceID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("server.room.%s.>", roomKind(spaceID))
	}
	return fmt.Sprintf("space.%s.room.>", spaceID)
}

// SpaceRoomRootEventsFilters returns filter subjects for root messages
// and meta events in a single room. Excludes thread replies.
//
// Legacy: ["space.{s}.room.{r}.msg.*", "space.{s}.room.{r}.meta"]
// Server: ["server.room.{kind}.{r}.msg.*", "server.room.{kind}.{r}.meta"]
//
// Use with JetStream consumer FilterSubjects for efficient server-side
// filtering.
func SpaceRoomRootEventsFilters(spaceID, roomID string) []string {
	if shouldUseServerSubjects(spaceID) {
		kind := roomKind(spaceID)
		return []string{
			fmt.Sprintf("server.room.%s.%s.msg.*", kind, roomID),
			fmt.Sprintf("server.room.%s.%s.meta", kind, roomID),
		}
	}
	return []string{
		fmt.Sprintf("space.%s.room.%s.msg.*", spaceID, roomID),
		fmt.Sprintf("space.%s.room.%s.meta", spaceID, roomID),
	}
}

// SpaceAllRoomEventsFilters returns filter subjects for all messages
// (root + thread) and meta events across all rooms in a space.
//
// Legacy: ["space.{s}.room.*.msg.>", "space.{s}.room.*.meta"]
// Server: ["server.room.{kind}.*.msg.>", "server.room.{kind}.*.meta"]
//
// Use with JetStream consumer FilterSubjects for live subscriptions
// that need all messages.
func SpaceAllRoomEventsFilters(spaceID string) []string {
	if shouldUseServerSubjects(spaceID) {
		kind := roomKind(spaceID)
		return []string{
			fmt.Sprintf("server.room.%s.*.msg.>", kind),
			fmt.Sprintf("server.room.%s.*.meta", kind),
		}
	}
	return []string{
		fmt.Sprintf("space.%s.room.*.msg.>", spaceID),
		fmt.Sprintf("space.%s.room.*.meta", spaceID),
	}
}

// ===== PARSERS =====
//
// All parsers accept either subject format. Index alignment makes this
// cheap: roomID is always at index 3 and eventID at index 5 (root) / 7
// (thread reply), regardless of whether the subject starts with `space.`
// or `server.room.`.

// ParseRoomIDFromSubject extracts the room ID from a room event subject.
// Returns "" for space-level events or unrecognized subjects.
//
// Subjects:
//
//	space.{spaceId}.room.{roomId}.msg.{eventId}                          (root)
//	space.{spaceId}.room.{roomId}.msg.{rootEventId}.replies.{eventId}    (thread)
//	space.{spaceId}.room.{roomId}.meta                                   (meta)
//	server.room.{kind}.{roomId}.msg.{eventId}                            (root)
//	server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}      (thread)
//	server.room.{kind}.{roomId}.meta                                     (meta)
func ParseRoomIDFromSubject(subject string) string {
	parts := splitSubject(subject)
	if len(parts) >= 5 && isRoomEventSubject(parts) {
		return parts[3]
	}
	return ""
}

// ParseThreadRootEventIDFromSubject extracts the root event ID from a
// thread reply subject. Returns ("", false) for non-thread subjects.
func ParseThreadRootEventIDFromSubject(subject string) (string, bool) {
	parts := splitSubject(subject)
	if len(parts) == 8 && isRoomEventSubject(parts) && parts[4] == "msg" && parts[6] == "replies" {
		return parts[5], true
	}
	return "", false
}

// IsRootMessageSubject reports whether a subject is for a top-level
// (root) message — 6 segments with `msg` at index 4.
func IsRootMessageSubject(subject string) bool {
	parts := splitSubject(subject)
	return len(parts) == 6 && isRoomEventSubject(parts) && parts[4] == "msg"
}

// IsMetaSubject reports whether a subject is for a meta event — 5
// segments with `meta` at index 4.
func IsMetaSubject(subject string) bool {
	parts := splitSubject(subject)
	return len(parts) == 5 && isRoomEventSubject(parts) && parts[4] == "meta"
}

// IsThreadSubject reports whether a subject is for a thread reply — 8
// segments with `msg` at index 4 and `replies` at index 6.
func IsThreadSubject(subject string) bool {
	parts := splitSubject(subject)
	return len(parts) == 8 && isRoomEventSubject(parts) && parts[4] == "msg" && parts[6] == "replies"
}

// ParseEventIDFromSubject extracts the event ID from a message subject.
// Returns "" for non-message subjects.
func ParseEventIDFromSubject(subject string) string {
	parts := splitSubject(subject)
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

// isRoomEventSubject reports whether the dot-split segments belong to a
// room event in either the legacy `space.{id}.room.{r}.>` shape or the
// consolidated `server.room.{kind}.{r}.>` shape. Both shapes place the
// roomID at index 3, so callers can read parts[3] directly after this
// returns true.
//
// Discriminator:
//   - Legacy starts with parts[0] == "space" and parts[2] == "room".
//   - Server starts with parts[0] == "server" and parts[1] == "room".
func isRoomEventSubject(parts []string) bool {
	if len(parts) < 4 {
		return false
	}
	if parts[0] == "space" && parts[2] == "room" {
		return true
	}
	if parts[0] == "server" && parts[1] == "room" {
		return true
	}
	return false
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
// storage. Same primary-aware dispatch as the durable subjects above.

// LiveInstanceAllEvents returns the live subject for all instance-level
// events. Pattern: live.instance.>
//
// Instance-scoped, not space-scoped — unaffected by the migration.
func LiveInstanceAllEvents() string {
	return "live.instance.>"
}

// LiveInstanceUserAllEvents returns the live subject for all events for
// a specific user. Pattern: live.instance.user.{userId}.>
func LiveInstanceUserAllEvents(userID string) string {
	return fmt.Sprintf("live.instance.user.%s.>", userID)
}

// LiveInstanceUserEvent returns the live subject for a specific user's
// instance event. Pattern: live.instance.user.{userId}.{eventType}
func LiveInstanceUserEvent(userID, eventType string) string {
	return fmt.Sprintf("live.instance.user.%s.%s", userID, eventType)
}

// LiveSpaceAllEvents returns the live subject for all events tied to a
// space.
//
// Legacy: live.space.{spaceId}.>
// Server: live.server.>
func LiveSpaceAllEvents(spaceID string) string {
	if shouldUseServerSubjects(spaceID) {
		return "live.server.>"
	}
	return fmt.Sprintf("live.space.%s.>", spaceID)
}

// LiveSpaceLevelEvents returns the live subject for non-room space-level
// events only.
//
// Legacy: live.space.{spaceId}.* — single-token wildcard excludes
//
//	`live.space.{id}.room.>` (different segment count).
//
// Server: live.server.member.> — non-room live events all live under the
//
//	`member` namespace post-4d.
func LiveSpaceLevelEvents(spaceID string) string {
	if shouldUseServerSubjects(spaceID) {
		return "live.server.member.>"
	}
	return fmt.Sprintf("live.space.%s.*", spaceID)
}

// LiveSpaceEvent returns the live subject for a space-level event
// (currently membership lifecycle).
//
// Legacy: live.space.{spaceId}.{eventType}
// Server: live.server.member.{eventType}
func LiveSpaceEvent(spaceID, eventType string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("live.server.member.%s", eventType)
	}
	return fmt.Sprintf("live.space.%s.%s", spaceID, eventType)
}

// LiveSpaceRoomEvent returns the live subject for a room event.
//
// Legacy: live.space.{spaceId}.room.{roomId}.{eventType}
// Server: live.server.room.{kind}.{roomId}.{eventType}
func LiveSpaceRoomEvent(spaceID, roomID, eventType string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("live.server.room.%s.%s.%s", roomKind(spaceID), roomID, eventType)
	}
	return fmt.Sprintf("live.space.%s.room.%s.%s", spaceID, roomID, eventType)
}

// LiveSpaceRoomAllEvents returns the live subject for all transient room
// events in a space.
//
// Legacy: live.space.{spaceId}.room.>
// Server: live.server.room.{kind}.>
func LiveSpaceRoomAllEvents(spaceID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("live.server.room.%s.>", roomKind(spaceID))
	}
	return fmt.Sprintf("live.space.%s.room.>", spaceID)
}

// LiveSpaceRoomReactionEvents returns the subscription subject for all
// live-only reaction events.
//
// Legacy: live.space.{spaceId}.room.*.reaction_*
// Server: live.server.room.{kind}.*.reaction_*
func LiveSpaceRoomReactionEvents(spaceID string) string {
	if shouldUseServerSubjects(spaceID) {
		return fmt.Sprintf("live.server.room.%s.*.reaction_*", roomKind(spaceID))
	}
	return fmt.Sprintf("live.space.%s.room.*.reaction_*", spaceID)
}

// ===== INSTANCE LIVE SUBJECT PATTERNS =====
// For transient instance-level events that bypass JetStream (config
// changes, etc.). Instance-scoped, unaffected by the migration.

// LiveInstanceConfigUpdated returns the subject for instance config
// update events. Pattern: live.instance.config.updated
func LiveInstanceConfigUpdated() string {
	return "live.instance.config.updated"
}

// LiveInstanceConfigAllEvents returns the wildcard subject for all
// instance config events. Pattern: live.instance.config.>
func LiveInstanceConfigAllEvents() string {
	return "live.instance.config.>"
}

// LiveInstanceSpaceEvent returns the live subject for a space-wide
// instance event. Pattern: live.instance.space.{spaceId}.{eventType}
//
// Instance-scoped (used for fanout to space members with server-side
// authorization filtering); unaffected by the migration.
func LiveInstanceSpaceEvent(spaceID, eventType string) string {
	return fmt.Sprintf("live.instance.space.%s.%s", spaceID, eventType)
}
