# ADR-072: Present Notifications as One Persistent List with Derived Groups

**Date:** 2026-08-10

## Context

The legacy notification model equates a notification record with pending
unread attention. Reading covered activity or dismissing the notification
deletes the record, so users cannot keep a read notification for later.

GitHub notifications provide useful inspiration for durable attention and
grouped activity, but their separate Inbox and Done lifecycle is more state
than Chatto needs. A notification should be new or already read. If a user no
longer wants it in the list, they can delete it without opening the source.
Chatto also needs to group related occurrences without allowing mutable group
records to become another source of truth.

## Decision

### Attention state and deletion

Each visible notification occurrence has one attention state:

- **Unread** — counted as new attention and highlighted with Chatto's semantic
  notification orange; or
- **Read** — retained in the same chronological list without unread emphasis.

There is no Done state, no separate notification view, and no operation that
marks a notification unread. Opening an unread notification marks it read only
after its target is displayed successfully. Reading a room or thread also
marks covered occurrences read.

Delete is independent of reading. It removes an occurrence from the visible
list and leaves only the minimal anti-recreation tombstone until the original
expiry. The bundled client exposes group deletion as its sole row action, so a
user can discard a notification without opening or otherwise handling its
source activity. The public API also retains single-occurrence deletion for
precise integration and privacy workflows.

The persisted core enum retains the numeric legacy-development `DONE` value
because `RUNTIME_STATE` protobufs are storage contracts. Current code never
writes that state; any retained value decodes as visible read history.

### User actions

The public behavior follows these rules:

- Opening a notification navigates to its exact room, thread, and event. The
  occurrence is marked read after that target is displayed.
- `MarkNotificationRead` is an idempotent one-way transition. There is no
  general state-update RPC and no Mark Unread operation.
- Reading a room or thread marks covered notification occurrences read.
- `DeleteNotificationOccurrence` permanently dismisses one occurrence.
- `DeleteNotificationGroup` dismisses the occurrences that belong to the
  derived group at the server's authoritative mutation boundary.
- `DeleteAllNotificationOccurrences` dismisses every occurrence current at the
  server's authoritative mutation boundary. Later activity remains new.

Read and delete mutations are server-owned and synchronize across sessions.
A later source event can create a new occurrence with the same derived group
ID, so clients must not automatically retry an ambiguous group or whole-list
deletion.

Muting a notification's source may be added as a separate policy action in the
future. It is not part of this lifecycle or the initial bundled UI.

### Groups are derived presentation resources

A **notification group** is a read model over occurrences, not an authoritative
mutable record. Its stable grouping target is derived from the exact activity
destination:

- a DM conversation groups by room;
- thread activity groups by room and thread root;
- reactions group by the reacted-to message;
- ambient channel activity groups by room; and
- an ungroupable future cause falls back to its source occurrence.

A group exposes its current member occurrences, matched reasons, newest
activity, unread state, strongest intensity, and deterministic open target. It
opens the newest unread visible occurrence, or the newest visible occurrence
when every member is read.

Group list responses contain a bounded newest-member preview, always including
the open occurrence, plus total count and aggregate state. Bounding the preview
keeps ConnectRPC pages and realtime replacement frames finite even when a busy
room, DM, or thread has thousands of retained occurrences. Clients render the
first page immediately and automatically append later pages.

All unread and read groups are assembled into one chronological server view.
The bundled client separates that view into Today, Yesterday, This Week, and
month sections using each server account's preferred time zone. Rows describe
the activity in a full sentence rather than presenting a terse actor/reason
tuple.

The public API therefore needs only one paginated list request. The bundled
multi-server client makes one request per authenticated server, preserves
fulfilled results when another server fails, and reports the failure as
partial.

Single-group and whole-list dismissal update the bundled client optimistically.
Whole-list dismissal uses one authoritative request per authenticated server;
it never loops over only the groups currently loaded by pagination. A failed
server restores its rows without restoring successful servers' rows.

The bell, server indicator, room indicators, and installed-app badge derive
from the same unread occurrences and grouping rules. The bell and server
indicator are active when at least one unread group exists; the notification
list uses the semantic `attention` orange for the same state.

### Read state and policy remain separate

Room and thread read cursors describe content consumption. Notification policy
decides whether new activity creates an occurrence and whether it is eligible
to interrupt. Notification attention state describes whether the resulting
occurrence is new. None of these concepts substitutes for another.

Consequently, disabling a cause does not mark content read, marking a
notification read does not necessarily advance the room cursor, and changing a
room's policy does not erase existing notification history. The legacy
`MUTED` behavior that also hid ordinary room unread state is not carried into
Notifications 2.0.

### Reconciliation and visibility

Notification groups and counts are finite authoritative state in the
server-scoped client projection. Reconnect and reset replace them from the
current notification index. Realtime operations accelerate convergence but do
not define correctness. Responses and realtime replacements expose the next
expiry boundary, while each group exposes its earliest expiry, so continuously
connected clients refresh even when KV TTL removal produces no live watcher
transition.

If a target is retracted, a reaction is removed, or authorization no longer
allows the recipient to open the target, the server removes the affected
occurrence. Assemblers never use a retained notification as authority to reveal
an otherwise inaccessible room, actor, or message.

## Consequences

- Read notifications remain available until the user deletes them or they
  expire.
- Users can dismiss notifications without pretending they opened or handled
  the source content.
- The one-way Unread-to-Read transition avoids a third triage state and removes
  general occurrence and group-update API surface.
- Derived groups reduce noise without introducing mutable group records that
  can drift from their member occurrences.
- Group deletion requires an authoritative boundary so concurrent later
  occurrences are not accidentally included.
- Whole-list deletion has the same boundary semantics and is not safe for an
  automatic retry after an ambiguous transport failure.
- Read cursors, delivery policy, and notification attention can evolve without
  overloading one another's semantics.
