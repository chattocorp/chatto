# FDR-012: Notifications

**Status:** Active
**Last reviewed:** 2026-07-20

## Overview

Chatto has a persistent notification system surfaced through a bell icon and notification center. Notifications represent things the user should pay attention to: DMs, @mentions of users/roles/virtual groups, replies to their own messages, new posts in threads they follow, and (optionally) all messages in rooms they've subscribed to. Notification levels are configurable per server and per joined channel room; DMs inherit the server level.

## Behavior

- A bell icon shows an unread count and opens the notification center listing recent notifications.
- A notification appears for: a DM message, a mention that resolves to the user, a reply to one of the user's messages, a new reply in a thread the user follows, or any root message in a room set to ALL_MESSAGES.
- Mention notifications may come from direct `@username`, role `@role`, `@all`, or `@here` mentions. The bundled composer asks for confirmation before sending role, `@all`, or `@here` mentions, while API callers can post authorized messages directly.
- Notifications auto-expire after 90 days.
- Dismissing a notification removes it everywhere — across all the user's open tabs and devices.
- Marking a room or thread read dismisses pending notifications whose source messages are covered by that read position.
- A notification sound plays for non-silent creations, and in-app and installed PWA notification badges update as authoritative pending state changes.
- While the installed PWA is visible, its app-icon badge shows the aggregate pending DM count when every authenticated server's current notification page is complete and at least one DM is pending, even if other notification kinds are also present. A non-numeric attention flag appears when only non-DM notifications are pending or any page is incomplete. Ordinary unread rooms stay in the in-app sidebar unless the user has configured them to create notifications.
- Initial load and reconnect replace the current pending-notification page and room counts from server state, so a missed live transition does not permanently leave client badges out of sync.
- Users can choose and locally shape the notification sound on each browser with volume, tone, and effect controls.
- Sidebar notification badges aggregate pending mentions, replies, DMs, and all-message subscriptions by room.
- A recipient's Do Not Disturb presence still stores new notifications and updates counts, but those creation events are silent: no notification sound and no web push while DND is active.

## Notification Levels

Per server and per joined channel room, the user picks one of four levels. DMs inherit the server level.

- **DEFAULT** — inherit from the parent (room → server → system default of NORMAL).
- **MUTED** — suppress everything for this scope, including @mentions. The room doesn't even show as unread in the sidebar.
- **NORMAL** — notifications for mentions, DMs, and thread replies. Default behavior.
- **ALL_MESSAGES** — like NORMAL plus every root message in the room.

## Thread Follow

- Posting a reply in a thread automatically subscribes the user to that thread's reply notifications.
- The thread-root author is subscribed when the first reply is posted unless they previously made an explicit follow choice.
- A direct `@username` mention in a thread subscribes the mentioned user if they have never followed or explicitly unfollowed that thread before. Role mentions, `@all`, and `@here` notify according to mention rules but do not subscribe recipients.
- Thread followers can manually unfollow, and non-posters can manually follow.
- Followers receive a notification for new replies in the thread (skipping their own).
- Thread notifications respect room mute: a muted room produces no thread notifications even for followed threads.

## Design Decisions

### 1. Persistent notification model with authoritative projection sync

**Decision:** Notifications are persistent per-user objects in `RUNTIME_STATE` with a 90-day TTL. Internal create and dismiss signals cause authoritative pending-page and room-count replacements on the resumable client projection; every subscription also reconciles that finite current state.
**Why:** Notifications need to survive a tab close, agree across devices, and recover after a client misses a transient signal. They are pending user-runtime state rather than content history. See ADR-012, ADR-028, ADR-036, and ADR-051.
**Tradeoff:** The projected list is a finite newest page even though its total and room counts are authoritative. A client that cannot classify every pending item must use a generic attention indicator instead of inventing an exact kind-specific count.

### 2. Mute suppresses notifications AND unread

**Decision:** MUTED is stronger than "no pings": a muted room doesn't appear unread in the sidebar either.
**Why:** "Quiet" in chat apps often means "ignore this room completely". A user who mutes a room wants it out of their face, not just out of their alerts.
**Tradeoff:** Users who want "quiet but I still want to see if there's new stuff" don't have a third state. The two main modes (engage / ignore) cover the dominant use cases.

### 3. Mute trumps mentions

**Decision:** Mentioning a user in a muted room produces no notification. The mention text still highlights in the body if the user opens the room.
**Why:** Mute is the strongest "I don't want pings" signal. Allowing mentions through would defeat the muscle-memory of "mute the room to stop the spam".
**Tradeoff:** Coordinators can't reliably ping someone in a muted room. The mention still renders, so eventual visibility is preserved.

### 4. Thread auto-follow on post and direct mention

**Decision:** Posting in a thread automatically follows it, even if the poster previously unfollowed. A delivered direct `@username` mention inside a thread also follows the thread for that recipient, unless they explicitly unfollowed it before. Follow and unfollow state is represented by durable room-aggregate `ThreadFollowedEvent` and `ThreadUnfollowedEvent` facts, with a projection used for notification fanout and My Threads.
**Why:** People who participate in a thread almost always want to see the replies, and a direct mention makes the thread relevant to the recipient. Manual unfollow handles both the "I posted once and don't care any more" case and the "do not put this mentioned thread back in My Threads" case.
**Tradeoff:** A user who posts in many threads or is directly mentioned in many threads accumulates followed-thread subscriptions over time. The 90-day TTL on notifications limits the blast radius; the thread follow state itself is cheap to store.

### 5. Broadcast mentions are sender-controlled with bundled-client friction

**Decision:** `@all`, `@here`, and role mentions are allowed. The bundled
composer asks for confirmation before sending them, and muted recipients still
do not receive notifications. The server does not require a confirmation token
from API callers.
**Why:** Chatto needs explicit operational pings for small teams and rooms, but broad pings should be deliberate in the main client. Keeping the safeguard in the client avoids making the integration API carry a client-shaped confirmation token that does not provide meaningful abuse protection.
**Tradeoff:** Operators and integrations can force attention in a room unless recipients have muted it. This is acceptable because mute remains authoritative and integrations can add their own policy or UX friction where appropriate.

### 6. ALL_MESSAGES is a per-room subscription, not a per-message setting

**Decision:** "Notify me for every message" is configured per room by the user, not per message by the poster.
**Why:** Receiver-controlled subscription puts the ongoing ambient-notification choice with the person who has to live with the noise. Sender-controlled broadcasts are reserved for explicit mentions; the bundled client adds confirmation friction for role and room-wide mentions.
**Tradeoff:** Users who want every message still need to opt into ALL_MESSAGES; senders should use mentions only for attention events.

### 7. Push notifications piggyback on persistent notifications

**Decision:** A push notification fires when a persistent notification is created. If no persistent notification is created (because the room is muted, etc.), no push is sent either.
**Why:** Pushes and in-app notifications are the same logical event presented in two surfaces. Sharing the gating logic ensures they can't diverge. See FDR-013.
**Tradeoff:** No way to receive a push without also generating a persistent notification. Considered desirable: a push you can't find later in the app would be annoying.

### 8. No parallel mention-status flag

**Decision:** A mention's contribution to the aggregate room badge is derived from pending notifications. Chatto does not maintain a separate `room_mention_status.*` flag.
**Why:** The separate flag duplicated notification state and had to be cleared in lockstep with notification dismissals and room reads. A single pending-notification model gives one source of truth for mention, reply, DM, and all-message attention indicators.
**Tradeoff:** Mention attention now has the same retention and dismissal semantics as notifications. Removing one mention's contribution does not clear the aggregate room badge while other pending notifications remain.

### 9. Notification sound choice and shaping are local

**Decision:** Notification sound selection and sound-shaping controls are stored in browser-local preferences.
**Why:** They are playback-device preferences, not server behavior. Keeping them local matches the existing sound picker and avoids adding durable compatibility surface for an annoyance/subtlety control.
**Tradeoff:** A user who signs in on a new browser reconfigures sound taste there. Server-synced display settings remain separate.

### 10. Do Not Disturb silences alert delivery

**Decision:** Do Not Disturb is checked at notification creation time. While the recipient has live DND presence, Chatto still creates the persistent notification and publishes silent transition metadata with the authoritative replacement, but it suppresses legacy attention hints, notification sounds, and Web Push delivery.
**Why:** DND means "do not interrupt me now", not "discard things I should review later". Storing the notification preserves missed activity in the notification center and sidebar counts, while the silent marker lets clients update state without making noise.
**Tradeoff:** A user may see badge/sidebar changes while actively viewing Chatto in DND. That is less disruptive than sound or push, and it avoids losing important mentions or DMs.

### 11. Reading resolves attention through its read boundary

**Decision:** Advancing a room or thread read position dismisses pending notifications whose source messages are covered by that position. Explicit dismissal remains a separate acknowledgement action.
**Why:** Opening and consuming content should resolve the attention it generated without clearing newer messages or unrelated rooms and threads.
**Tradeoff:** The current read-side cleanup cannot catch a notification created after the read scan has already completed. ADR-053 adds the complementary creation-side read check required for convergence in either order.

## Permissions

Notification preferences require no RBAC permission string and are always self-scoped. Room-level preferences require current membership in the channel room.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-028 (event-ID-keyed read state), ADR-036 (runtime state in `RUNTIME_STATE`), ADR-038 (room-owned thread state), ADR-051 (server-scoped resumable client projection), ADR-053 (convergent notification policy and pending state)
- **FDRs:** FDR-006 (@Mentions), FDR-007 (Direct Messages), FDR-013 (Web Push Notifications)

## Open Questions

[Notifications 2.0](https://github.com/chattocorp/chatto/issues/1556) tracks a
planned redesign of this feature under ADR-053. Before this FDR can describe
the replacement behavior, the project still needs to settle the user-facing
names and defaults for cause-specific delivery intensities, the exact legacy
preference mapping, and whether changing a preference clears existing pending
notifications. Until that behavior ships, the four notification levels above
remain the supported contract.
