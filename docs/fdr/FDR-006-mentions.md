# FDR-006: @Mentions

**Status:** Active
**Last reviewed:** 2026-06-13

## Overview

Users can mention users, roles, and room-scoped virtual groups with `@handle` syntax. A delivered mention notifies the recipient users, contributes to the room's pending-notification indicator in the sidebar, and renders the mention as styled text in the message body.

## Behavior

- Typing `@` followed by at least one character opens the autocomplete popup in the composer.
- Matching is fuzzy against room-member logins, room-member display names, the virtual handles `all` and `here`, and server role names. Prefix matches rank higher than substring matches.
- Pressing Tab completes the first match and appends a space. Pressing Tab again cycles to the next candidate.
- `@username` mentions notify that user if they are a current room member.
- Custom `@role` mentions notify current room members who currently hold that server role.
- System role mentions follow the role hierarchy: `@owner` reaches room owners, `@admin` reaches room admins and owners, and `@moderator` reaches room moderators, admins, and owners. Custom role mentions remain explicit membership only.
- `@all` mentions every current room member, regardless of presence.
- `@here` mentions current room members whose presence is not offline.
- `@everyone` is not a message mention handle. Use `@all` for room-wide delivery; `everyone` remains the implicit RBAC role.
- Valid user, role, and virtual mentions render with highlight styling in the posted message. Self-mentions get additional styling.
- Mentions inside code blocks, pre-formatted text, and blockquotes are not styled — they render as plain text.
- Mentioning yourself does not produce a notification.
- Mentioning a user who isn't a room member leaves the `@name` as plain text — the mention is not delivered.
- If a message would notify more than 10 users, the composer asks for confirmation before sending. The backend enforces the same guard for API callers.
- Mentions are resolved when a message is first posted. Editing a message later does not add, remove, dismiss, or re-send mention notifications.

## Design Decisions

### 1. One shared `@` namespace

**Decision:** Users, roles, and virtual handles all use `@handle`. User logins cannot use existing role names or virtual handles, and custom role names cannot use existing user logins or virtual handles. User and role handles are claimed through a shared atomic `RUNTIME_STATE` KV key so concurrent user/role writes cannot commit the same mention handle.
**Why:** Users already understand `@` as "direct attention". A second prefix would make role mentions harder to discover and harder to tab-complete. A single case-insensitive namespace keeps parsing and autocomplete predictable.
**Tradeoff:** A server cannot have a user and role with the same mention handle. Early 0.1.x servers can resolve any existing collisions manually.

### 2. Mentions are room-scoped

**Decision:** Mentions only deliver to users who are current members of the room being posted to. Custom role mentions intersect explicit role membership with room membership. System role mentions intersect room membership with RBAC rank, so higher-ranked users are included in lower system-role mentions.
**Why:** Room membership is the visibility boundary for the message. Notifying a non-member would either leak context or create a notification they cannot open.
**Tradeoff:** A role mention may reach fewer people than the full server role assignment list. Authors who need a broader audience must post in a room that contains that audience.

### 3. Mentions are post-time facts

**Decision:** Mention delivery is decided when the message is posted. Later edits may change the visible message body, but they do not re-resolve mentions or change who was notified by the original post.
**Why:** A mention notification is an attention event that already happened. Re-resolving mentions on edit would allow quiet retroactive pings, would make notifications depend on mutable usernames and edited body text, and would complicate replay now that message bodies are private payload facts.
**Tradeoff:** An author who forgot to mention someone must send a new message rather than editing the old one to ping them. Removing an `@name` from the edited body also does not revoke an already-created notification.

### 4. Echo events carry mentions but don't re-notify

**Decision:** When a thread reply is echoed to the channel, `mentionedUserIds` is copied to the echo. The echo doesn't fire a second notification — the original reply already did.
**Why:** The echo's mention rendering (highlight, link to profile) needs the field present, but the user shouldn't get notified twice. See FDR-003.
**Tradeoff:** The frontend has to know that echo mentions don't trigger room-level mention indicators twice. The backend skips the notification on echo events.

### 5. Mute trumps mention

**Decision:** If the recipient has muted the room, the mention is rendered but does not produce a notification.
**Why:** Mute is the user's strongest signal that they don't want pings from this room. Honoring it for everything except mentions would create surprise notifications.
**Tradeoff:** Users in muted rooms might miss directed pings. The mute affordance is loud enough that this is a reasonable default; users who want differently shouldn't mute.

### 6. Mention attention state is a notification

**Decision:** A delivered mention creates a pending notification. Sidebar mention dots derive from pending notifications, not from a separate room-level mention-status key.
**Why:** Mention attention state has the same lifecycle as other notifications: it is pending until the user views or dismisses it, syncs across devices, and expires with notification retention. Keeping it in the notification model avoids duplicated state.
**Tradeoff:** Mention dots follow notification dismissal semantics. Dismissing a mention notification clears the corresponding sidebar attention signal.

### 7. Large mention sends require confirmation

**Decision:** A message whose mentions would notify more than 10 users requires explicit confirmation. The count is computed after deduplication, excluding the author, excluding users muted for the room, and applying room-membership constraints. The backend returns a short-lived confirmation token scoped to the author, room, message body, thread target, and echo flag; the retry uses that token so live recipient-count drift does not force repeated prompts.
**Why:** Role and room-wide mentions are useful operational tools, but accidental broad pings are costly. Confirmation preserves the feature while catching the common "I did not realize this reaches everyone" mistake.
**Tradeoff:** Senders occasionally see one extra prompt for intentional broadcasts.

## Permissions

No dedicated mention permission. Anyone who can post in a room can mention any user, role, or virtual handle that resolves inside that room.

## Related

- **ADRs:** ADR-026 (event identity via NanoID)
- **FDRs:** FDR-002 (Replies & Threads), FDR-003 (Thread Reply Echo), FDR-012 (Notifications), FDR-013 (Web Push Notifications)
