# FDR-044: Threads

**Status:** Active
**Last reviewed:** 2026-10-30

## Overview

Threads is a conversation inbox for channel-room and DM threads that the
current user follows. It helps the user return to active conversations and
find replies that they have not read.

## Behavior

- Threads defaults to newest activity first. The Activity / Age control can
  instead sort by root-message creation, newest first.
  Calendar-day groups and row times use the selected sort date. Today has a
  visible heading and relative times. Earlier days show local clock times.
  Grouping follows the viewer's timezone. Unknown dates appear last, without a row time.
- The sort selection stays with the page when users change tabs, filter,
  search, or return from a thread. It is not a device-wide preference.
- Returning to the page or a cached tab refreshes its loaded pages from the
  server. Cached rows remain visible while the refresh runs.
- The Mine tab shows followed threads started by the current user. The
  Followed tab shows all followed threads, including those started by the
  current user. Mine is a subset of Followed. A thread whose root author is
  unavailable is omitted from Mine.
- Followed is the first and default tab. Switching tabs keeps the search text and
  All/Unread selection. Both tabs require follow state: unfollowing removes a
  thread even when the current user started it.
- Each row shows the original thread message on one line, with no reply preview
  or byline. It includes the selected sort time and a participant preview.
  The room has its own column before
  the time. Long room labels truncate; the room column disappears on narrow
  rows. Reply counts are not shown. The participant preview shows the
  root author as the avatar and one other participant in a
  small badge.
- A thread with unread replies shows its first line in bold and a neutral
  avatar dot. Read threads use regular weight and muted text. Metadata uses
  subdued text. Rows use compact spacing and small participant avatars.
  Desktop row actions replace trailing metadata on hover or keyboard focus.
  Touch inputs keep the actions visible beside the content.
- A DM row uses participant names and avatars instead of a channel name. It
  does not show a `#` channel prefix.
- The Unread filter includes only threads with replies after the user's thread
  read cursor. The server applies this filter before pagination, so the first
  page shows unread threads or confirms that none exist.
- The search input, thread tabs, All/Unread filter, and sort control share one toolbar above
  the list. The switchers have the same size and stay together to the right
  of the input. On narrow screens, the input and switchers use separate rows.
- When server search is enabled, a search input filters followed threads by
  their root messages and all replies, including replies outside the current
  list preview. Each matching thread appears once in the selected order.
- Search uses the message-search syntax, including `from:username`. Plain words
  search message text. All/Unread still applies to
  the matching threads. Clearing the input restores the ordinary list.
- Search stays within the selected tab. The tab selects the root author;
  `from:username` selects the author of a matching root message or reply.
- The search input receives focus when Threads opens.
- A row with a matching unread notification uses a neutral marker for all
  attention levels. The client reads this decoration from its current
  Notifications view. Row backgrounds remain neutral.
- A user can mark a thread with unread replies as read, or stop following any
  displayed thread, from its row.
- Opening a thread uses the normal thread read behavior. This advances the read
  cursor and clears notification attention that the displayed content covers.
- A thread can remain in Threads after all replies and notifications are
  read. It remains until the user stops following it, loses access, or its
  root and all replies are deleted. A deleted root with visible replies stays
  in the list. A live root with no replies also stays in the list.
  The server excludes fully deleted threads before page counts and pagination.
  Their follow state stays unchanged.
- Thread activity can change activity order; it does not change creation order. The bundled client restarts
  loaded offset pages after a reply post, edit, or retraction before it
  continues pagination.

## Design Decisions

### Sort threads by activity or creation

**Decision:** Sort the full result set before pagination, including search.
Keep latest activity as the default and show the original message in each row.
**Why:** Users can find new conversations without an older thread moving ahead
of them each time it receives a reply.
**Compatibility:** The API additions require no migration for existing callers.
Older servers ignore the list order field and can reject creation-order search.
The minimum server version stays unchanged.

### Mine narrows the followed inbox

**Decision:** Followed includes every accessible followed thread with content.
Mine filters this set
to the current user's root messages before pagination and search.
Keep the navigation indicator combined across both tabs, without tab counts.
**Why:** Users can find conversations they started without removing them from
the complete followed inbox. Follow state controls which threads stay in the inbox.
**Tradeoff:** Threads is not a history of every thread the user started.
Unfollowed threads remain available through their rooms and general search.

### 1. Threads composes existing state

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
**Tradeoff:** Unread pages use offsets over a set that becomes smaller when
the user reads a thread. The client counts only loaded threads that are still
unread to calculate the next offset. A read in a different client can still
move the offset until the list loads again.

### 3. The activity-list presentation makes the order clear

**Decision:** Threads uses flat activity rows and calendar-day sections with
visible headings, including Today, as in Notifications. Each row shows the original
message without an author prefix; the avatar identifies its author.
Missing or deleted roots use the corresponding
placeholder rather than switching to a reply. New replies update unread state,
activity order, and time without replacing the row's subject.
**Why:** The original message identifies the conversation. Flat rows and date
sections distinguish this newest-first activity list from a room timeline,
where newer messages appear at the bottom.
**Tradeoff:** Threads and room timelines use different reading directions.
The list response must also hydrate more message and user data. API clients
must restart pagination after activity changes the live order. The ordinary
list uses offsets. Search uses opaque cursors through the shared search API;
each search page counts distinct threads.

### 4. The navigation indicator covers followed threads

**Decision:** The Threads navigation indicator counts loaded unread
notifications for followed threads. The badge uses notification orange when
one notification has Important attention, and the neutral badge color when all
have Ambient attention. Unread replies without an unread notification do not
show a navigation indicator.
**Why:** One notification source supplies the navigation badge and thread-row
notification markers. The navigation indicator does not depend on
unread flags in cached room timelines.
**Tradeoff:** A notification for an unfollowed thread can still appear in
Notifications without lighting the Threads indicator. The badge counts only
the notifications that the client has loaded. Unread replies without a
notification, including Badge and Off activity, remain available through the
Unread filter.

### 5. Chatto 0.5 uses the explicit viewer-state contract

**Decision:** The 0.5 client and server use an explicit reply-unread field.
They do not preserve the ambiguous pre-0.5 client field or add notification
state to the thread contract.
**Why:** Chatto 0.5 already has a breaking client and server boundary. Keeping
the ambiguous field would make it easy for new clients to rebuild the same
second unread model that this feature removes.
**Tradeoff:** A new client cannot use Threads with a pre-0.5 server. Existing
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
