# FDR-012: Notifications

**Status:** Active
**Last reviewed:** 2026-07-01

## Overview

Chatto has a persistent notification system surfaced through a bell icon and notification center. Notifications represent things the user should pay attention to: DMs, @mentions of users/roles/virtual groups, replies to their own messages, new posts in threads they follow, and (optionally) all messages in rooms they've subscribed to. Notification levels are configurable per space and per room.

## Behavior

- A bell icon shows an unread count and opens the notification center listing recent notifications.
- A notification appears for: a DM message, a mention that resolves to the user, a reply to one of the user's messages, a new reply in a thread the user follows, or any root message in a room set to ALL_MESSAGES.
- Mention notifications may come from direct `@username`, role `@role`, `@all`, or `@here` mentions. Sends that would notify more than 10 users require confirmation before notifications are created.
- Notifications auto-expire after 90 days.
- Dismissing a notification removes it everywhere — across all the user's open tabs and devices.
- A notification sound plays and the in-app and installed PWA notification badges update in real time as new notifications arrive.
- The installed PWA dock badge reflects pending notifications only; ordinary unread rooms stay in the in-app sidebar unless the user has configured them to create notifications.
- Users can choose and locally shape the notification sound on each browser with volume, tone, and effect controls.
- Sidebar orange dots for mentions, replies, DMs, and all-message subscriptions derive from pending notification records.
- A recipient's Do Not Disturb presence still stores new notifications and updates counts, but those creation events are silent: no notification sound and no web push while DND is active.

## Notification Levels

Per space and per room, the user picks one of four levels:

- **DEFAULT** — inherit from the parent (room → space → system default of NORMAL).
- **MUTED** — suppress everything for this scope, including @mentions. The room doesn't even show as unread in the sidebar.
- **NORMAL** — notifications for mentions, DMs, and thread replies. Default behavior.
- **ALL_MESSAGES** — like NORMAL plus every root message in the room.

## Thread Follow

- Posting a reply in a thread automatically subscribes the user to that thread's reply notifications.
- A direct `@username` mention in a thread subscribes the mentioned user if they have never followed or explicitly unfollowed that thread before. Role mentions, `@all`, and `@here` notify according to mention rules but do not subscribe recipients.
- Thread followers can manually unfollow, and non-posters can manually follow.
- Followers receive a notification for new replies in the thread (skipping their own).
- Thread notifications respect room mute: a muted room produces no thread notifications even for followed threads.

## Design Decisions

### 1. Persistent notification model with live-event sync

**Decision:** Notifications are persistent objects stored per user in `RUNTIME_STATE` (`notification.{userId}.{notificationId}`), with a 90-day per-key TTL. Live events fire on create and dismiss to keep all the user's connected sessions in sync.
**Why:** Notifications need to survive a tab close (so the badge count is right when you come back tomorrow), and they need to be the same across devices. They are pending user-runtime state, not reconstructable content history, so `RUNTIME_STATE` is the right home. See ADR-012, ADR-028, and ADR-036.
**Tradeoff:** A notification dismissal anywhere clears it everywhere, even if the user wanted to dismiss only locally. The simpler model wins here — "I've seen it" is not device-specific.

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

### 5. Broadcast mentions are sender-controlled but bounded

**Decision:** `@all`, `@here`, and role mentions are allowed, but sends that would notify more than 10 users require confirmation and muted recipients still do not receive notifications.
**Why:** Chatto needs explicit operational pings for small teams and rooms, but broad pings should be deliberate. Confirmation catches accidental broadcasts without removing the tool.
**Tradeoff:** Operators can force attention in a room unless recipients have muted it. This is acceptable because the prompt adds friction and mute remains authoritative.

### 6. ALL_MESSAGES is a per-room subscription, not a per-message setting

**Decision:** "Notify me for every message" is configured per room by the user, not per message by the poster.
**Why:** Receiver-controlled subscription puts the ongoing ambient-notification choice with the person who has to live with the noise. Sender-controlled broadcasts are reserved for explicit mentions with confirmation.
**Tradeoff:** Users who want every message still need to opt into ALL_MESSAGES; senders should use mentions only for attention events.

### 7. Push notifications piggyback on persistent notifications

**Decision:** A push notification fires when a persistent notification is created. If no persistent notification is created (because the room is muted, etc.), no push is sent either.
**Why:** Pushes and in-app notifications are the same logical event presented in two surfaces. Sharing the gating logic ensures they can't diverge. See FDR-013.
**Tradeoff:** No way to receive a push without also generating a persistent notification. Considered desirable: a push you can't find later in the app would be annoying.

### 8. No parallel mention-status flag

**Decision:** @mention orange dots are derived from pending mention notifications. Chatto does not maintain a separate `room_mention_status.*` flag.
**Why:** The separate flag duplicated notification state and had to be cleared in lockstep with notification dismissals and room reads. A single pending-notification model gives one source of truth for mention, reply, DM, and all-message attention indicators.
**Tradeoff:** Pending mention dots now have the same retention and dismissal semantics as notifications. This is deliberate: a mention that is no longer a pending notification is no longer pending attention.

### 9. Notification sound choice and shaping are local

**Decision:** Notification sound selection and sound-shaping controls are stored in browser-local preferences.
**Why:** They are playback-device preferences, not server behavior. Keeping them local matches the existing sound picker and avoids adding durable compatibility surface for an annoyance/subtlety control.
**Tradeoff:** A user who signs in on a new browser reconfigures sound taste there. Server-synced display settings remain separate.

### 10. Do Not Disturb silences alert delivery

**Decision:** Do Not Disturb is checked at notification creation time. While the recipient has live DND presence, Chatto still creates the persistent notification and publishes a silent live sync event, but it suppresses legacy attention live events, notification sounds, and web push delivery.
**Why:** DND means "do not interrupt me now", not "discard things I should review later". Storing the notification preserves missed activity in the notification center and sidebar counts, while the silent marker lets clients update state without making noise.
**Tradeoff:** A user may see badge/sidebar changes while actively viewing Chatto in DND. That is less disruptive than sound or push, and it avoids losing important mentions or DMs.

## Permissions

Notification preferences are user-scoped and don't require special permissions to manage. There's no permission gating the ability to mute or change levels.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-028 (event-ID-keyed read state), ADR-036 (runtime state in `RUNTIME_STATE`), ADR-038 (room-owned thread state)
- **FDRs:** FDR-006 (@Mentions), FDR-007 (Direct Messages), FDR-013 (Web Push Notifications)
