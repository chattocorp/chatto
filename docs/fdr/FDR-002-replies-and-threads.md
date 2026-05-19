# FDR-002: Replies & Threads

**Status:** Active
**Last reviewed:** 2026-05-19

## Overview

Chatto messages can link to one another via reply attribution, and they can live inside threads — conversations branching off a root message. Replies and threads are independent concepts: a message can reply without being in a thread, or live in a thread without referencing a specific parent. Rooms can be configured to promote one shape over another.

## Behavior

- A message in a room can optionally reference another message as the one it's in reply to.
- A reply renders with a byline above the message body: the referenced author's small avatar, name, and a single-line excerpt of the referenced message.
- Clicking the byline transports the user to the referenced message and briefly highlights it.
- Clicking the avatar or name in the byline opens the user's context menu.
- A thread is a sequence of messages starting from a root message and continuing inside a dedicated thread pane. Threads can contain plain messages or reply-attributed messages; both are valid.
- A user can post a plain message into a room, a reply into the room timeline, a plain message into a thread, or a reply inside a thread — each gated by separate permissions, so a room can be configured for many threading styles.

## Design Decisions

### 1. Replies and threads are orthogonal in the data model

**Decision:** A message's reply target and its containing thread are independent fields. The system enforces no rule like "replies must be in a thread" or "thread messages must reply to the root".
**Why:** Different communities want different shapes. Some want strict thread-everything; some want flat-with-replies; some want both. Encoding either as a structural constraint forecloses on the alternatives.
**Tradeoff:** Operators have to configure room permissions to enforce their desired model. Without configuration, all four shapes are technically possible in any room.

### 2. Posting permissions are split along reply × thread axes

**Decision:** Four separate permissions: `message.post`, `message.post-in-thread`, `message.reply`. (No separate "start thread" permission — starting a thread is the same action as posting in one.)
**Why:** Operators want to express patterns like "everyone can reply in threads, but only certain roles can post root messages" without inventing custom roles. Each axis (root-vs-thread, reply-or-not) needs its own gate.
**Tradeoff:** Four permission keys to learn instead of one. The permission resolver handles cascade.

### 3. Reply attribution doesn't change storage

**Decision:** A reply is a normal message with an extra `inReplyTo` field. It's not stored differently.
**Why:** Reply attribution is a presentation concern. Special-casing the storage would mean every read path has to handle two flavors of message.
**Tradeoff:** Bulk operations (deleting a message, etc.) need to consider whether replies still make sense after the target is gone. The UI handles this by gracefully degrading the byline.

## Permissions

- `message.post` — post a root message in a room.
- `message.post-in-thread` — post a message in a thread (whether starting it or replying inside).
- `message.reply` — attach `inReplyTo` to a posted message, in either the room timeline or a thread.

## Related

- **ADRs:** ADR-011 (message body/event split), ADR-026 (event identity via NanoID)
- **FDRs:** FDR-003 (Thread Reply Echo)
