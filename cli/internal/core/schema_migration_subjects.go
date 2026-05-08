package core

import (
	"fmt"
	"strings"
)

// Subject rewriting helper for the #330 phase 4d migration. Pure
// functions; isolated from NATS so they can be unit-tested without a
// running JetStream.
//
// Each legacy subject from a per-space stream maps to exactly one
// server-format subject. The mapping is deterministic from the legacy
// subject and the room kind (channel for the primary, dm for the DM
// space). Membership-event subjects strip the redundant `member_`
// prefix because the new shape namespaces them under `member.` already.

// rewriteSubjectForServerStream maps a legacy space-stream subject to
// its server-format equivalent. `legacySubject` must start with
// `space.{spaceID}.…`. `kind` is "channel" or "dm" — chosen by the
// caller based on which source stream the message came from.
//
// Returns ("", false) for subjects that don't match a known legacy
// shape; the caller logs and skips them rather than corrupting the
// target stream with a misrouted message.
//
// Mappings:
//
//	space.{X}.room.{R}.msg.{E}                 → server.room.{kind}.{R}.msg.{E}
//	space.{X}.room.{R}.msg.{root}.replies.{E}  → server.room.{kind}.{R}.msg.{root}.replies.{E}
//	space.{X}.room.{R}.meta                    → server.room.{kind}.{R}.meta
//	space.{X}.member_{verb}                    → server.member.{verb}
func rewriteSubjectForServerStream(legacySubject, kind string) (string, bool) {
	parts := strings.Split(legacySubject, ".")

	// All legacy space-stream subjects start with `space.{spaceID}.…`.
	if len(parts) < 3 || parts[0] != "space" {
		return "", false
	}

	// Room subjects: space.{X}.room.{R}.…
	if len(parts) >= 5 && parts[2] == "room" {
		roomID := parts[3]
		tail := parts[4:]
		switch {
		case len(tail) == 1 && tail[0] == "meta":
			return fmt.Sprintf("server.room.%s.%s.meta", kind, roomID), true
		case len(tail) == 2 && tail[0] == "msg":
			return fmt.Sprintf("server.room.%s.%s.msg.%s", kind, roomID, tail[1]), true
		case len(tail) == 4 && tail[0] == "msg" && tail[2] == "replies":
			return fmt.Sprintf("server.room.%s.%s.msg.%s.replies.%s", kind, roomID, tail[1], tail[3]), true
		}
		return "", false
	}

	// Space-level (membership) subject: space.{X}.{eventType}. The
	// `member_` prefix on the eventType is stripped to match the
	// server-side `server.member.{verb}` shape.
	if len(parts) == 3 {
		verb := strings.TrimPrefix(parts[2], "member_")
		return fmt.Sprintf("server.member.%s", verb), true
	}

	return "", false
}
