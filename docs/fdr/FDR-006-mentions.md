# FDR-006: @Mentions

**Status:** Active
**Last reviewed:** 2026-05-27

## Overview

Users can mention each other in messages with `@username` syntax. A mention notifies the recipient, contributes to the room's pending-notification indicator in the sidebar, and renders the mention as styled text in the message body.

## Behavior

- Typing `@` followed by at least one character opens the autocomplete popup in the composer.
- Matching is fuzzy against both the user's login and display name. Prefix matches rank higher than substring matches.
- Pressing Tab completes the first match and appends a space. Pressing Tab again cycles to the next candidate.
- Valid mentions render with highlight styling in the posted message. Self-mentions get additional styling.
- Mentions inside code blocks, pre-formatted text, and blockquotes are not styled — they render as plain text.
- Mentioning yourself does not produce a notification.
- Mentioning a user who isn't a member of the server leaves the `@name` as plain text — the mention is not delivered.

## Design Decisions

### 1. Only server members can be mentioned

**Decision:** Mentions only resolve against users who are members of the server. Mentions of non-members are silently dropped (rendered as plain text).
**Why:** Mentioning a non-member would either need to invite them (privacy hazard) or no-op (the current behavior, which preserves the typed text). The no-op is the conservative choice.
**Tradeoff:** Users can't ping someone who hasn't joined yet. They have to invite first, then mention.

### 2. No `@channel` or `@here`

**Decision:** Only individual user mentions exist. There's no `@channel`, `@everyone`, `@here`, or other broadcast form.
**Why:** Broadcast mentions are a common source of noise and abuse in chat apps. Without them, the cost of mentioning is bounded.
**Tradeoff:** Operators who want a "shout into the room" mechanism have to use room-wide notifications (see FDR-012, `ALL_MESSAGES` notification level) — which is opt-in per user per room and lower-stakes.

### 3. Echo events carry mentions but don't re-notify

**Decision:** When a thread reply is echoed to the channel, `mentionedUserIds` is copied to the echo. The echo doesn't fire a second notification — the original reply already did.
**Why:** The echo's mention rendering (highlight, link to profile) needs the field present, but the user shouldn't get notified twice. See FDR-003.
**Tradeoff:** The frontend has to know that echo mentions don't trigger room-level mention indicators twice. The backend skips the notification on echo events.

### 4. Mute trumps mention

**Decision:** If the recipient has muted the room, the mention is rendered but does not produce a notification.
**Why:** Mute is the user's strongest signal that they don't want pings from this room. Honoring it for everything except mentions would create surprise notifications.
**Tradeoff:** Users in muted rooms might miss directed pings. The mute affordance is loud enough that this is a reasonable default; users who want differently shouldn't mute.

### 5. Mention attention state is a notification

**Decision:** A delivered mention creates a pending notification. Sidebar mention dots derive from pending notifications, not from a separate room-level mention-status key.
**Why:** Mention attention state has the same lifecycle as other notifications: it is pending until the user views or dismisses it, syncs across devices, and expires with notification retention. Keeping it in the notification model avoids duplicated state.
**Tradeoff:** Mention dots follow notification dismissal semantics. Dismissing a mention notification clears the corresponding sidebar attention signal.

## Permissions

No dedicated mention permission. Anyone who can post in a room can mention any server member.

## Related

- **ADRs:** ADR-026 (event identity via NanoID)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo), FDR-012 (Notifications), FDR-013 (Web Push Notifications)
