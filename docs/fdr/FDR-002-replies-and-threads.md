# FDR-002: Replies & Threads

**Status:** Active
**Last reviewed:** 2026-07-20

## Overview

Chatto messages can link to one another via reply attribution, and channel-room messages can live inside threads — conversations branching off a root message. Replies and threads are independent concepts: a message can reply without being in a thread, or live in a thread without referencing a specific parent. Channel rooms can be configured to promote one shape over another; DMs support reply attribution but not threads.

## Behavior

- A message in a room can optionally reference another message as the one it's in reply to.
- Reply attribution notifies the referenced author unless they sent the reply, muted the room, or already receive the message as a direct mention.
- DMs keep replies in their single room timeline and do not offer thread actions. Historical DM threads remain readable but cannot receive new replies.
- A reply renders with a byline above the message body: the referenced author's small avatar, name, and a single-line excerpt of the referenced message.
- Clicking the byline transports the user to the referenced message and briefly highlights it.
- Clicking the avatar or name in the byline opens the user's context menu.
- If the user selects text inside a message body before choosing Reply or Reply in thread, the target composer inserts that selected plain text as a Markdown blockquote while preserving any existing draft text.
- A thread is a sequence of messages starting from a root message and continuing inside a dedicated thread pane. Threads can contain plain messages or reply-attributed messages; both are valid.
- Thread badges in the room timeline are normal links to the thread URL, so users can copy or open the thread link through browser-native link actions.
- Links copied from messages inside a thread reopen that thread and focus the linked message. A root message can be opened in its thread pane before the thread has any replies.
- A user can post a plain message into a room, a reply into the room timeline, a plain message into a thread, or a reply inside a thread. Two location-based permissions control those shapes: one for the room timeline and one for channel-room threads; reply attribution adds no separate gate.
- Posting in a thread follows it. On the first reply, the thread-root author is also followed unless they previously made an explicit follow choice.

## Design Decisions

### 1. Replies and threads are orthogonal in the data model

**Decision:** A message's reply target and its containing thread are independent fields. The system enforces no rule like "replies must be in a thread" or "thread messages must reply to the root".
**Why:** Different communities want different shapes. Some want strict thread-everything; some want flat-with-replies; some want both. Encoding either as a structural constraint forecloses on the alternatives.
**Tradeoff:** Operators have to configure room permissions to enforce their desired model. Without configuration, all four shapes are technically possible in any room.

### 2. Posting permissions are split by location only, not by reply attribution

**Decision:** Two posting permissions: `message.post` (room timeline) and `message.post-in-thread` (inside a thread). Reply attribution (`inReplyTo`) is **not** separately gated — anyone who can post can reply.
**Why:** Operators want to express patterns like "everyone can reply in threads, but only certain roles can post root messages" — that's the room-vs-thread axis, which the two permissions cover. A separate reply-attribution gate was considered and shipped in earlier versions, but it added permission-matrix complexity without defining a useful moderation boundary. Reply notifications remain a consequence of attribution, not a separate posting capability.
**Tradeoff:** Operators who genuinely wanted to disable reply attribution as a UI affordance can't do so via permissions. In practice nobody asked.

### 3. Reply attribution doesn't change storage

**Decision:** A reply is a normal message with an extra `inReplyTo` field. That attribution drives reply rendering, navigation, and notification policy without creating a separate message type.
**Why:** Special-casing storage would make every read path handle two flavours of message even though ordinary messages and replies share the same lifecycle.
**Tradeoff:** Bulk operations (deleting a message, etc.) need to consider whether replies still make sense after the target is gone. The UI handles this by gracefully degrading the byline.

### 4. Thread replies are paginated separately from the root

**Decision:** The thread root is resolved separately, while replies load as bounded chronological pages.
**Why:** Threads can grow large. Keeping the root distinct and paginating replies avoids an unbounded list while matching the room timeline's loading model.
**Tradeoff:** Opening a thread requires both its root and a reply page rather than one self-contained array.

### 5. Anchored thread reads preserve the visible window

**Decision:** A thread can reload a bounded reply page around a focused reply, or around the top of the thread when the root is the focus. The root remains separate from the reply page.
**Why:** Reconnect and wake refreshes need to reload the current thread window without jumping the reader to the newest replies. A stable focused event preserves the reader's position.
**Tradeoff:** The server and client support an anchored page in addition to ordinary older/newer pagination.

### 6. Thread message links identify both the thread and focused message

**Decision:** A link copied from the thread pane preserves the thread root separately from the message it focuses. Opening the link shows the thread pane even when the focused message is the root and no replies exist.
**Why:** A message identifier alone can locate a reply's thread after a lookup, but it cannot express that a root message should open as an empty thread. Carrying both identities makes the intended view explicit and directly shareable.
**Tradeoff:** Thread message links contain two event identifiers, making them longer than ordinary room message links.

## Permissions

- `message.post` — post a root message (with or without `inReplyTo`) in a room.
- `message.post-in-thread` — post a message in a channel-room thread (whether starting it or replying inside, with or without `inReplyTo`). This permission does not make threads available in DMs.

## Related

- **ADRs:** ADR-011 (message body/event split), ADR-026 (event identity via NanoID), ADR-038 (room-owned thread state), ADR-050 (ephemeral encrypted projection snapshots), ADR-053 (convergent notification policy and pending state)
- **FDRs:** FDR-003 (Thread Reply Echo), FDR-012 (Notifications)
