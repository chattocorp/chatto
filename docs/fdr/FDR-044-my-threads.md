# FDR-044: My Threads

**Status:** Active
**Last reviewed:** 2026-08-30

## Overview

My Threads is a conversation inbox for channel-room threads that the current
user follows. It helps the user return to active conversations and find replies
that they have not read.

## Behavior

- My Threads lists followed threads in newest-activity-first order and groups
  them by activity date.
- Each row shows the room, root message, latest visible reply, last activity,
  reply count, and a participant preview.
- The Unread filter includes only threads with replies after the user's thread
  read cursor.
- Notification attention is separate from reply unread state. Important
  attention uses notification orange. Ambient attention uses a neutral marker.
- A user can mark the displayed thread activity as read or stop following the
  thread from its row.
- Opening a thread uses the normal thread read behavior. This advances the read
  cursor and clears notification attention that the displayed content covers.
- A thread can remain in My Threads after all replies and notification
  attention are read. It remains until the user stops following it or loses
  access.

## Design Decisions

### 1. My Threads composes existing state

**Decision:** Follow state selects rows, the thread read cursor determines
unread replies, and notification state determines attention. My Threads does
not own another unread or notification state.
**Why:** One authority for each fact prevents contradictory badges and keeps
Mark read consistent with an open thread and the Notifications view.
**Tradeoff:** A row can have unread replies without notification attention, or
notification attention without an unread reply.

### 2. Unread means unread replies

**Decision:** The Unread filter uses only the thread read cursor. Notification
attention does not put a read thread in this filter.
**Why:** Users can predict the filter from the conversation content that they
have read. Notification policy remains an independent way to prioritize work.
**Tradeoff:** A thread with important attention can appear only in All after
its replies are read.

### 3. The activity-list presentation makes the order clear

**Decision:** My Threads uses flat activity rows and date sections. Each row
shows the latest visible reply first and keeps compact root-message context
below it.
**Why:** The latest reply explains why the thread is active. Flat rows and date
sections distinguish this newest-first activity list from a room timeline,
where newer messages appear at the bottom.
**Tradeoff:** My Threads and room timelines use different reading directions.
The list response must also hydrate more message and user data.

### 4. Chatto 0.5 uses the explicit viewer-state contract

**Decision:** The 0.5 client and server use separate reply unread and attention
fields. They do not preserve the ambiguous pre-0.5 client field.
**Why:** Chatto 0.5 already has a breaking client and server boundary. Keeping
the ambiguous field would make it easy for new clients to rebuild the same
second unread model that this feature removes.
**Tradeoff:** A new client cannot use My Threads with a pre-0.5 server. Existing
persisted follow, read, and notification data still upgrades without changes.

## Permissions

- `message.read` — read channel-room and thread messages.
- `message.read-interactions` — read an accessible interaction thread when the
  user does not have the general message-read permission.

## Related

- **ADRs:** ADR-038, ADR-076, ADR-077, ADR-080, ADR-082
- **FDRs:** FDR-002 (Replies & Threads), FDR-012 (Notifications), FDR-039
  (Message Access & Interactions)
