# ADR-072: Present Notifications as One Persistent Occurrence List

**Date:** 2026-08-10

## Context

The legacy model equates a notification record with pending unread attention.
Reading covered activity or dismissing the notification deletes the record, so
users cannot retain read history. GitHub's inbox is useful inspiration, but its
separate Inbox and Done lifecycle is more state than Chatto needs.

Chatto also needs to reduce repetitive presentation without allowing mutable
server-side groups to become another source of truth. Grouping rules are a
client concern: different clients may present the same exact activities
differently, while counts, jump targets, reads, and deletion must remain
occurrence-exact.

## Decision

### Attention and deletion

Each visible notification occurrence is either **Unread** or **Read**. Unread
occurrences use Chatto's semantic notification orange and contribute one to
the exact unread count. Read occurrences remain in the same chronological list
until deletion or the 90-day expiry. There is no Done view and no Mark Unread
operation.

Opening an unread occurrence navigates to its exact room, thread, and event;
the existing target-display handshake marks it read only after successful
display. Reading a room or thread also marks covered occurrences read.

Delete is independent of reading. `DeleteNotificationOccurrence` deletes one
exact occurrence. `BatchDeleteNotificationOccurrences` idempotently deletes an
explicit bounded set of occurrence IDs, which lets a client dismiss one of its
temporary presentation groups without a mutable server group boundary.
`DeleteAllNotificationOccurrences` deletes everything current at the server's
authoritative mutation boundary. Muting an origin remains future work.

The persisted core enum retains the numeric development-era `DONE` and
`CLAIMED` values because `RUNTIME_STATE` protobufs are storage contracts.
Current code writes neither state; retained Done rows are treated as Read.

### The server exposes exact occurrences

`ListNotificationOccurrences` returns a newest-first page of exact occurrences
plus totals independent of pagination:

- total unread occurrence count;
- unread occurrence count per target room; and
- the earliest expiry in the complete retained list.

Each occurrence carries its exact target and cause data, including reaction
emoji and a current, visibility-checked thread-root excerpt where applicable.
Presentation text is hydrated only after authorization and is not persisted in
the occurrence.

The realtime projection replaces the same finite occurrence page and totals.
Realtime transitions accelerate convergence; list/reconnect state remains
authoritative. The bundled multi-server client preserves fulfilled servers
when another server fails and reports the failure as partial.

### Clients derive presentation groups

The bundled client derives temporary groups from occurrences:

- direct messages group by DM room;
- reactions group by reacted-to room/thread/message target and consolidate
  actors and emoji;
- followed-thread activity may group by room and thread root;
- ambient followed-room activity may group by room; and
- mentions and replies remain separate per exact jump target.

A temporary group opens its newest unread occurrence, or its newest occurrence
when every member is read. It contains the exact occurrence IDs used for an
optimistic batch delete. It is never persisted, transmitted, counted, or
mutated as a server resource. A client may revise these presentation rules
without a protocol or storage migration.

The bundled client presents unread and read rows in one chronological view,
groups them into Today, Yesterday, This Week, and month sections using the
account's preferred time zone, and renders full-sentence descriptions. Thread
activity includes the current root excerpt. Reaction rows show the reaction
emoji and consolidate activity for the same target. The list does not show a
redundant `1` counter.

The bell, server indicator, room indicators, installed-app badge, and push app
badge use exact unread occurrence counts. Presentation consolidation never
changes attention counts: two unread DMs in one displayed row still count as
two.

### Read state and policy remain separate

Room/thread read cursors describe content consumption. Notification policy
decides whether new activity creates an occurrence and whether it is eligible
to interrupt. Notification attention describes whether that occurrence is
new. Disabling a cause does not mark content read, reading a notification does
not necessarily advance a room cursor, and changing policy does not erase
existing history.

### Reconciliation and visibility

Before list/realtime assembly, the server fences the notification materializer
and the recipient, room, group-layout, and RBAC projections used to validate
current visibility. Retracted targets, removed reactions, and inaccessible
rooms are tombstoned before they can be returned. The complete retained list is
validated before exact totals are derived, including occurrences outside the
requested page.

## Consequences

- Read notifications remain useful history until deletion or expiry.
- Dismissal does not pretend the source was handled.
- Exact occurrence counts are stable regardless of presentation grouping.
- Mentions cannot collapse distinct jump targets, while high-volume reactions
  can remain one compact row.
- Batch deletion is safely retryable because its membership is explicit.
- Clients own grouping complexity, but the public API is smaller and no
  server-side group state can drift from occurrences.
