# FDR-002: Replies & Threads

**Status:** Active
**Last reviewed:** 2026-08-19

## Overview

Chatto messages can link to one another via reply attribution, and channel-room messages can live inside threads — conversations branching off a root message. Replies and threads are independent concepts: a message can reply without being in a thread, or live in a thread without referencing a specific parent. Channel rooms can be configured to promote one shape over another; DMs support reply attribution but not threads.

## Behavior

- A message in a room can optionally reference another message as the one it's in reply to.
- DMs keep replies in their single room timeline and do not offer thread actions. Historical DM threads remain readable but cannot receive new replies.
- A reply renders with a byline above the message body: the referenced author's small avatar, name, and a single-line excerpt of the referenced message.
- Clicking the byline transports the user to the referenced message and briefly highlights it.
- Clicking the avatar or name in the byline opens the user's context menu.
- If the user selects text inside a message body before choosing Reply or Reply in thread, the target composer inserts that selected plain text as a Markdown blockquote while preserving any existing draft text.
- A thread is a sequence of messages starting from a root message and continuing inside a dedicated thread pane. Threads can contain plain messages or reply-attributed messages; both are valid.
- Posting a reply attempts to follow the thread for its author, even after an
  earlier unfollow. The first reply also attempts to follow the root author when
  they have never made a follow choice. These post-commit subscription writes
  are best-effort and cannot roll back the message.
- When posting a root message in a channel room, an author with both root and thread posting permissions can choose **Post as thread**. The root is immediately marked as a thread and the author follows it, while the composer remains in the room timeline. The author or another member can open the empty thread from its thread link when they want to reply.
- Posting an ordinary root message remains unchanged. If nobody explicitly establishes its thread, the first thread reply establishes one implicitly.
- Thread badges in the room timeline are normal links to the thread URL, so users can copy or open the thread link through browser-native link actions.
- Links copied from messages inside a thread reopen that thread and focus the linked message. A root message can be opened in its thread pane before the thread has any replies.
- When the room area is wide enough, its timeline and the open thread appear side by side and both remain interactive. In narrower room areas, the thread overlays the dimmed, inactive room timeline. The wide thread pane is resizable, and the device remembers the preferred width.
- A user can post a plain message into a room, a reply into the room timeline, a plain message into a thread, or a reply inside a thread — each gated by separate permissions, so a room can be configured for many threading styles.

## Design Decisions

### 1. Replies and threads are orthogonal in the data model

**Decision:** A message's reply target and its containing thread are independent fields. The system enforces no rule like "replies must be in a thread" or "thread messages must reply to the root".
**Why:** Different communities want different shapes. Some want strict thread-everything; some want flat-with-replies; some want both. Encoding either as a structural constraint forecloses on the alternatives.
**Tradeoff:** Operators have to configure room permissions to enforce their desired model. Without configuration, all four shapes are technically possible in any room.

### 2. Posting permissions are split by location only, not by reply attribution

**Decision:** Two posting permissions: `message.post` (room timeline) and `message.post-in-thread` (inside a thread). Reply attribution (`inReplyTo`) is **not** separately gated — anyone who can post can reply.
**Why:** Operators want to express patterns like "everyone can reply in threads, but only certain roles can post root messages" — that's the room-vs-thread axis, which the two permissions cover. Reply attribution is message presentation and notification semantics, not a distinct posting location or moderation boundary. A reply may create its own notification occurrence independently of a direct mention in the same message; the recipient controls that reply cause through notification policy.
**Tradeoff:** Operators who genuinely want to disable reply attribution as a UI affordance cannot do so through permissions. Recipients can suppress reply notifications without disabling the reply feature itself.

### 3. Reply attribution doesn't change storage

**Decision:** A reply is a normal message with an extra `inReplyTo` field. It's not stored differently.
**Why:** Reply attribution is a presentation concern. Special-casing the storage would mean every read path has to handle two flavors of message.
**Tradeoff:** Bulk operations (deleting a message, etc.) need to consider whether replies still make sense after the target is gone. The UI handles this by gracefully degrading the byline.

### 4. Thread replies use a cursor-paginated event connection

**Decision:** `MessagePostedEvent.threadReplies(limit, before, after)` returns a `RoomEventsConnection` page of replies, in chronological order, excluding the root event. Cursors use the same opaque sequence shape as `Room.events`.
**Why:** Threads are append-only timelines and can grow large. A connection keeps the release API from baking in an unbounded reply list while matching the room timeline pagination model clients already understand.
**Tradeoff:** Thread panes now load reply pages rather than a bare array. The current UI still asks for the default page, and can add older/newer reply paging without another schema change.

### 5. Anchored thread reads preserve the visible window

**Decision:** `MessagePostedEvent.threadRepliesAround(eventId, limit)` returns a reply page centered around a reply event ID, or around the top of the thread when the root event ID is supplied. The root event itself is still resolved separately and is not included in the reply connection.
**Why:** Reconnect and wake refreshes need to reload the current thread window without jumping the reader to the newest replies. Anchoring by event ID lets the UI preserve scroll position in the same way room timelines use `eventsAround`.
**Tradeoff:** This adds a second thread read shape, but keeps the existing forward/backward pagination API simple and avoids teaching cursor pagination how to express "refresh around this visible row."

### 6. Thread message links identify both the thread and focused message

**Decision:** A link copied from the thread pane preserves the thread root separately from the message it focuses. Opening the link shows the thread pane even when the focused message is the root and no replies exist.
**Why:** A message identifier alone can locate a reply's thread after a lookup, but it cannot express that a root message should open as an empty thread. Carrying both identities makes the intended view explicit and directly shareable.
**Tradeoff:** Thread message links contain two event identifiers, making them longer than ordinary room message links.

### 7. Root authors can establish a thread before the first reply

**Decision:** A channel-room root post can explicitly create its thread when the author has both `message.post` and `message.post-in-thread`. The root message, `ThreadCreatedEvent`, and root-author `ThreadFollowedEvent` are one atomic room-aggregate write. The durable thread exists even with zero replies, and public messages expose that state by including `Message.thread`; ordinary roots without an established thread omit it.
**Why:** The author can signal the intended conversation shape at posting time instead of leaving the decision to the first person who replies. Atomic creation prevents a visible root from briefly or permanently losing that intent. Keeping the room view stable makes **Post as thread** a posting choice rather than an unexpected navigation action.
**Tradeoff:** Clients must distinguish an established empty thread from an ordinary root with zero replies by checking `Message.thread` presence. An author who wants to reply immediately must open the new thread from its link.

**Compatibility:** `CreateMessageRequest.create_thread` is additive, while `Message.thread` presence now distinguishes established threads from ordinary roots. Older clients keep posting ordinary roots and infer established threads from non-zero reply counts. The bundled client only offers **Post as thread** to servers in the 0.5 compatibility line, preventing an older server from silently ignoring `create_thread`. Requiring both posting permissions is a behavioral authorization tightening during 0.5 development; it needs no data migration and does not change existing threads.

### 8. Thread presentation follows the room's available space

**Decision:** An open thread shares the room area when both panes remain useful; otherwise it overlays the room. The decision follows the room area's width after surrounding sidebars, not the browser viewport. Split panes are independently usable, and the thread width is a device-local preference.
**Why:** A wide browser can still leave a narrow conversation area when either resizable sidebar is open. Responding to the actual room area uses spare space without squeezing both conversations into unusable panes.
**Tradeoff:** Resizing surrounding panes can change an open thread between split and overlay presentation. The thread remains open and keeps the same URL, but the room becomes inactive whenever the overlay presentation takes effect.

## Permissions

- `message.post` — post a root message (with or without `inReplyTo`) in a room. Explicitly establishing that root as a thread also requires `message.post-in-thread`.
- `message.post-in-thread` — post a message inside a channel-room thread (with or without `inReplyTo`), and—together with `message.post`—explicitly establish a root as a thread. This permission does not make threads available in DMs.

## Related

- **ADRs:** ADR-011 (message body/event split), ADR-026 (event identity via NanoID), ADR-038 (room-owned thread state), ADR-050 (ephemeral encrypted projection snapshots), ADR-076 (deterministic notification occurrences), ADR-077 (persistent notification list)
- **FDRs:** FDR-003 (Thread Reply Echo), FDR-012 (Notifications)
