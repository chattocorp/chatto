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
- Each row shows the room, root message, latest visible reply when one exists,
  last activity, reply count, and a participant preview.
- The Unread filter includes only threads with replies after the user's thread
  read cursor.
- A row with a matching unread notification uses notification orange for
  Important attention and a neutral marker for Ambient attention. The client
  reads this decoration from its current Notifications view.
- A user can mark a thread with unread replies as read, or stop following any
  displayed thread, from its row.
- Opening a thread uses the normal thread read behavior. This advances the read
  cursor and clears notification attention that the displayed content covers.
- A thread can remain in My Threads after all replies and notifications are
  read. It remains until the user stops following it or loses access.
- Thread activity can change the live sort order. The bundled client restarts
  loaded offset pages after a reply post, edit, or retraction before it
  continues pagination.

## Design Decisions

### 1. My Threads composes existing state

**Decision:** Follow state selects rows, and the thread read cursor determines
unread replies. The client can decorate a followed-thread row from matching
unread notification occurrences that it already has. Thread viewer state does
not contain notification attention.
**Why:** One authority for each fact prevents contradictory badges and keeps
Mark read consistent with an open thread and the Notifications view.
**Tradeoff:** A row can have unread replies without a notification, or a
notification without an unread reply. Badge-only activity does not decorate a
thread row because it does not create a notification occurrence.

### 2. Unread means unread replies

**Decision:** The Unread filter uses only the thread read cursor. A notification
does not put a read thread in this filter.
**Why:** Users can predict the filter from the conversation content that they
have read. Notification policy remains an independent way to prioritize work.
**Tradeoff:** A thread with important attention can appear only in All after
its replies are read.

### 3. The activity-list presentation makes the order clear

**Decision:** My Threads uses flat activity rows and date sections. Each row
shows the latest visible reply first when one exists and keeps compact
root-message context below it. A thread without replies shows its root as the
primary activity.
**Why:** The latest reply explains why the thread is active. Flat rows and date
sections distinguish this newest-first activity list from a room timeline,
where newer messages appear at the bottom.
**Tradeoff:** My Threads and room timelines use different reading directions.
The list response must also hydrate more message and user data. API clients
must restart offset pagination after activity changes the live order.

### 4. The navigation indicator covers followed threads

**Decision:** The My Threads navigation indicator summarizes unread replies
and loaded unread notifications only for followed threads. Important
notification attention takes visual priority over the neutral indicator.
**Why:** The indicator must lead to a row that the user can find in My Threads.
**Tradeoff:** A notification for an unfollowed thread can still appear in
Notifications without lighting the My Threads indicator.

### 5. Chatto 0.5 uses the explicit viewer-state contract

**Decision:** The 0.5 client and server use an explicit reply-unread field.
They do not preserve the ambiguous pre-0.5 client field or add notification
state to the thread contract.
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
