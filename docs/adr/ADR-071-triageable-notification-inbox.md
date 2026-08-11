# ADR-071: Model Notifications as a Triageable Inbox with Derived Groups

**Date:** 2026-08-10

## Context

Chatto currently equates a notification record with pending unread attention.
Reading covered activity or dismissing the notification deletes the record.
Users therefore cannot keep a read notification for later, dismiss one without
opening its message, review dismissed history, or separate inbox organization
from room read state.

[GitHub's notification inbox](https://docs.github.com/en/subscriptions-and-notifications/how-tos/viewing-and-triaging-notifications/managing-notifications-from-your-inbox)
provides a useful interaction model: notifications can be read or unread,
moved to Done, and deleted without treating all of those actions as content
reads. Chatto adopts the Inbox/Done distinction while keeping the first version
focused on the two actions needed for ordinary triage: Done and Delete. Chatto
also needs to group related occurrences, such as several DM messages, thread
replies, or reactions to one message, without letting mutable group records
become another source of truth.

## Decision

### Inbox state and views

Each visible notification occurrence has exactly one inbox state:

- **Unread** — in Inbox and counted as new attention;
- **Read** — still in Inbox, but not counted as new attention; or
- **Done** — removed from Inbox and visible in Done until expiry.

Deleting an occurrence is distinct from Done. Delete removes it from every
user-visible view and leaves only the minimal anti-recreation tombstone until
the original expiry. Normal triage should prefer Done; Delete is the explicit
request to discard history.

### User actions

The public behavior follows these rules:

- Opening a notification navigates to its exact room, thread, and event and
  marks the occurrence read. The room or thread read cursor advances only once
  that target is actually displayed as read.
- Mark Read and Mark Unread change inbox state without changing a room or
  thread read cursor.
- Reading a room or thread marks covered notification occurrences read, but
  does not move them to Done or delete them.
- Done can be applied directly from Inbox without opening or otherwise handling
  the source activity.
- Moving a Done item back to Inbox restores it as read by default; the user may
  then mark it unread.

These mutations are server-owned and synchronized across every session.
Single-occurrence mutations are idempotent. Group mutations capture the
members present when the server handles the request and are deliberately not
advertised as idempotent: a later occurrence may reuse the same derived group
ID, so clients must not automatically retry an ambiguous group mutation.
They are exposed as resource-oriented public API operations; the bundled UI is
not the API's only intended consumer.

### Groups are derived presentation resources

A **notification group** is a read model over occurrences, not an authoritative
mutable record. Its stable grouping target is derived from the exact activity
destination:

- a DM conversation groups by room;
- thread activity groups by room and thread root;
- reactions group by the reacted-to message;
- ambient channel activity groups by room;
- an ungroupable future cause falls back to its source occurrence.

A group exposes its current member occurrence IDs, matched reasons, newest
activity, unread state, strongest intensity, and deterministic open target. It
opens the newest unread visible occurrence, or the newest visible occurrence
when all members are read.

Group list responses contain a bounded newest-member preview, always including
the open occurrence, plus total count and aggregate state. The first API does
not expose a separate exact-member listing; one can be added when a product
flow needs group expansion. Bounding the preview keeps ConnectRPC pages and
realtime replacement frames finite even when one busy room, DM, or thread has
thousands of retained occurrences. Clients render the first group page
immediately and automatically append later pages as the trailing sentinel
becomes visible; broad realtime invalidations never eagerly download an entire
90-day view.

Groups are assembled within a view. Inbox membership includes only Unread and
Read occurrences, while Done membership includes only Done occurrences. The
same stable grouping target may therefore have an Inbox row for new activity
and a Done row for older history at the same time.

Group actions operate on the occurrences that are members at the mutation's
authoritative boundary. Moving a group to Done does not create a permanent
group-level dismissal: later activity creates a new occurrence and makes the
derived group appear in Inbox again. This prevents a race from silently
dismissing activity that arrived after the user's action. Group mutations
return a bounded affected-count acknowledgement and publish one coalesced
realtime invalidation after their ordered member writes; they never return or
broadcast the full member set.

The bell shows the number of unread groups. A group row may also show its
occurrence count. APIs expose explicit group and occurrence counts so clients
do not have to infer whether a number represents grouped conversations or raw
activity. Room and installed-app indicators are assembled from the same unread
occurrences and grouping rules.

### Read state, policy, and triage remain separate

Room/thread read cursors describe content consumption. Notification policy
decides whether new activity creates an occurrence and whether it is eligible
to interrupt. Inbox state organizes the resulting occurrences. None of these
three concepts stands in for another.

Consequently, disabling a cause does not mark content read, marking a
notification read does not necessarily advance the room cursor, and changing a
room's policy does not erase existing notification history. The legacy
`MUTED` behavior that also hid ordinary room unread state is not carried into
the per-cause Notifications 2.0 model.

### Reconciliation and visibility

Inbox groups and counts are finite authoritative state in the server-scoped
client projection. Reconnect and reset replace them from the current
notification index. Live operations may optimize a single transition but do
not define correctness. Realtime changes invalidate both views so an open Done
view refetches across sessions. Responses and realtime Inbox
replacements expose the next Inbox expiry boundary, while each group exposes
its own earliest expiry, so continuously connected clients refresh every open
view even when KV TTL removal itself produces no live watcher transition.
One room/thread read-through may update many occurrences, but publishes one
revision-fenced invalidation after its ordered writes.

If a target is retracted, a reaction is removed, or authorization no longer
allows the recipient to open the target, the server removes the affected
occurrence from all views and groups. Assemblers never use a retained
notification as authority to reveal an otherwise inaccessible room, actor, or
message.

## Consequences

- Read notifications remain available until the user moves them to Done,
  deletes them, or they expire.
- Users can clear Inbox directly without pretending they opened or read the
  source content.
- Done supplies dismissible history, while Delete has a clear privacy and
  anti-recreation meaning.
- Derived groups reduce noise without introducing mutable group records that
  can drift from their member occurrences.
- Group mutations require an authoritative boundary so concurrent later
  occurrences are not accidentally included.
- Read cursors, delivery policy, and inbox organization can evolve without
  overloading one another's semantics.
