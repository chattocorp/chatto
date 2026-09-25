# FDR-044: My Threads

**Status:** Active
**Last reviewed:** 2026-09-25

## Overview

My Threads is a conversation inbox for channel-room and DM threads that the
current user follows. It helps the user return to active conversations and
find replies that they have not read.

## Behavior

- My Threads lists followed threads in newest-activity-first order and groups
  them by activity date.
- Each row shows the room, root message, latest visible reply when one exists,
  last activity, reply count, and a participant preview.
- A DM row uses participant names and avatars instead of a channel name. It
  does not show a `#` channel prefix.
- The Unread filter includes only threads with replies after the user's thread
  read cursor. The server applies this filter before pagination, so the first
  page shows unread threads or confirms that none exist.
- When server search is enabled, a search input filters followed threads by
  their root messages and all replies, including replies outside the current
  list preview. Each matching thread appears once in activity order.
- Search uses the message-search syntax, including `from:username`. Plain words
  search message text. All/Unread still applies to
  the matching threads. Clearing the input restores the ordinary list.
- The search input receives focus when My Threads opens.
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

### 2a. The server filters unread threads

**Decision:** `ListFollowedThreads` accepts `unread_only`. The server reads
the cursors of all followed threads, keeps the unread threads, and then
paginates. The page total counts only unread threads. The client also hides a
thread that becomes read while the list shows it. For search results and for
servers without this field, the client filters loaded pages and continues to
load pages until it finds a match or reaches the end. It shows the normal
loading state during this work.
**Why:** A client-side filter over pages of all followed threads needs many
requests to show an empty Unread view. The server already loads every followed
thread to sort it, so one more cursor read for each thread is small.
**Tradeoff:** Unread pages use offsets over a set that shrinks when the user
reads a thread. The next page can skip a thread until the list loads again.

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
must restart pagination after activity changes the live order. The ordinary
list uses offsets. Search uses opaque cursors through the shared search API;
each search page counts distinct threads.

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

Search requires the optional message-search provider. If the provider is not
ready, the page shows its status and a retry control. The ordinary thread list
remains available after the search input is cleared.

- `message.read` — read room and thread messages.
- `message.read-interactions` — read an accessible interaction thread when the
  user does not have the general message-read permission.

## Related

- **ADRs:** ADR-038, ADR-076, ADR-077, ADR-080, ADR-082
- **FDRs:** FDR-002 (Replies & Threads), FDR-012 (Notifications), FDR-039
  (Message Access & Interactions)
