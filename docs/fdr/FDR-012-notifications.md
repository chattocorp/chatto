# FDR-012: Notifications

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Chatto has a persistent notification system surfaced through a bell icon and notification center. Notifications represent things the user should pay attention to: DMs, @mentions, replies to their own messages, new posts in threads they follow, and (optionally) all messages in rooms they've subscribed to. Notification levels are configurable per space and per room.

## Behavior

- A bell icon shows an unread count and opens the notification center listing recent notifications.
- A notification appears for: a DM message, a mention of the user, a reply to one of the user's messages, a new reply in a thread the user follows, or any root message in a room set to ALL_MESSAGES.
- Notifications auto-expire after 90 days.
- Dismissing a notification removes it everywhere — across all the user's open tabs and devices.
- A notification sound plays and the badge updates in real time as new notifications arrive.

## Notification Levels

Per space and per room, the user picks one of four levels:

- **DEFAULT** — inherit from the parent (room → space → system default of NORMAL).
- **MUTED** — suppress everything for this scope, including @mentions. The room doesn't even show as unread in the sidebar.
- **NORMAL** — notifications for mentions, DMs, and thread replies. Default behavior.
- **ALL_MESSAGES** — like NORMAL plus every root message in the room.

## Thread Follow

- Posting a reply in a thread automatically subscribes the user to that thread's reply notifications.
- Thread followers can manually unfollow, and non-posters can manually follow.
- Followers receive a notification for new replies in the thread (skipping their own).
- Thread notifications respect room mute: a muted room produces no thread notifications even for followed threads.

## Design Decisions

### 1. Persistent notification model with live-event sync

**Decision:** Notifications are persistent objects stored per user, with a 90-day TTL. Live events fire on create and dismiss to keep all the user's connected sessions in sync.
**Why:** Notifications need to survive a tab close (so the badge count is right when you come back tomorrow), and they need to be the same across devices. A persistent store plus live-event sync gives both. See ADR-012 and ADR-028.
**Tradeoff:** A notification dismissal anywhere clears it everywhere, even if the user wanted to dismiss only locally. The simpler model wins here — "I've seen it" is not device-specific.

### 2. Mute suppresses notifications AND unread

**Decision:** MUTED is stronger than "no pings": a muted room doesn't appear unread in the sidebar either.
**Why:** "Quiet" in chat apps often means "ignore this room completely". A user who mutes a room wants it out of their face, not just out of their alerts.
**Tradeoff:** Users who want "quiet but I still want to see if there's new stuff" don't have a third state. The two main modes (engage / ignore) cover the dominant use cases.

### 3. Mute trumps mentions

**Decision:** Mentioning a user in a muted room produces no notification. The mention text still highlights in the body if the user opens the room.
**Why:** Mute is the strongest "I don't want pings" signal. Allowing mentions through would defeat the muscle-memory of "mute the room to stop the spam".
**Tradeoff:** Coordinators can't reliably ping someone in a muted room. The mention still renders, so eventual visibility is preserved.

### 4. Thread auto-follow on post

**Decision:** Posting in a thread automatically follows it. You can manually unfollow afterwards.
**Why:** People who participate in a thread almost always want to see the replies. Auto-follow saves a manual step in the common case. Manual unfollow handles the "I posted once and don't care any more" case.
**Tradeoff:** A user who posts in many threads accumulates many followed-thread subscriptions over time. The 90-day TTL on notifications limits the blast radius; the thread follow state itself is cheap to store.

### 5. ALL_MESSAGES is a per-room subscription, not a per-message setting

**Decision:** "Notify me for every message" is configured per room by the user, not per message by the poster.
**Why:** Poster-controlled "broadcast" mechanics (like `@channel`) are noise generators. Receiver-controlled subscription puts the choice with the person who has to live with the noise.
**Tradeoff:** Operators can't force-broadcast to a room. They have to rely on users opting in to ALL_MESSAGES (or pinning content visibly).

### 6. Push notifications piggyback on persistent notifications

**Decision:** A push notification fires when a persistent notification is created. If no persistent notification is created (because the room is muted, etc.), no push is sent either.
**Why:** Pushes and in-app notifications are the same logical event presented in two surfaces. Sharing the gating logic ensures they can't diverge. See FDR-013.
**Tradeoff:** No way to receive a push without also generating a persistent notification. Considered desirable: a push you can't find later in the app would be annoying.

## Permissions

Notification preferences are user-scoped and don't require special permissions to manage. There's no permission gating the ability to mute or change levels.

## Related

- **ADRs:** ADR-012 (two-tier real-time events), ADR-028 (event-ID-keyed read state), ADR-036 (runtime state in `RUNTIME_STATE`)
- **FDRs:** FDR-006 (@Mentions), FDR-007 (Direct Messages), FDR-013 (Web Push Notifications)
