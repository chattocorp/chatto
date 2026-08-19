# ADR-077: Present Notifications as One Persistent Occurrence List

**Date:** 2026-08-10

**Updated:** 2026-08-19

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

Each visible notification occurrence is either **Unread** or **Read**. Every
Unread occurrence contributes one to the exact unread count. Its independent
source-time attention level is **Ambient** for reactions and **Important** for
every other current cause. Ambient unread activity uses a neutral treatment;
Important unread activity uses Chatto's semantic notification orange. Read
occurrences remain in the same chronological list until deletion or the
90-day expiry. There is no Done view and no Mark Unread operation.

Opening an unread occurrence navigates to its exact room, thread, and event;
the existing target-display handshake marks it read only after successful
display. Reading a room or thread also marks covered occurrences read.

Delete is independent of reading. `DeleteNotificationOccurrence` deletes one
exact occurrence. `BatchDeleteNotificationOccurrences` idempotently deletes an
explicit bounded set of occurrence IDs, which lets a client dismiss one of its
temporary presentation groups without a mutable server group boundary.
`DeleteAllNotificationOccurrences` deletes every visible occurrence current at
the server's authoritative mutation boundary. Muting an origin remains future
work.

### The server exposes exact occurrences

`ListNotificationOccurrences` returns a newest-first page of exact occurrences
plus totals independent of pagination:

- total unread occurrence count;
- total unread Important occurrence count;
- unread occurrence count per target room;
- unread Important occurrence count per target room; and
- the earliest expiry in the complete retained list.

`GetNotificationOccurrence` reads one stable ID and returns `NOT_FOUND` when it
is absent or invisible. `BatchGetNotificationOccurrences` reads at most 100
stable IDs, preserves first-seen request order, de-duplicates repeats, and omits
missing or inaccessible rows. Integrations can therefore hydrate IDs received
through listing or realtime without scanning pages.

Each occurrence carries one rich `NotificationSignal` oneof branch, including
its exact destination and any cause-specific data such as reaction emoji. New
signal kinds can be added as protobuf oneof branches. An older client receiving
a newer public signal retains it as a generic, non-navigating row with exact
read/delete identity instead of guessing navigation or hiding its badge. An
older server that cannot validate a stored signal fails the affected operation
with `UNIMPLEMENTED` rather than guessing visibility or mutating it. Each new
variant requires its own visibility and lifecycle behavior. The API does not hydrate or
return message excerpts; clients render concise descriptions from signal,
actor, room, and cause-specific metadata.

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
- followed-room activity may group by room; and
- mentions and replies remain separate per exact jump target.

A temporary group opens its newest unread occurrence, or its newest occurrence
when every member is read. It contains the exact occurrence IDs used for an
optimistic batch delete. It is never persisted, transmitted, counted, or
mutated as a server resource. A client may revise these presentation rules
without a protocol or storage migration. Its unread presentation uses the
strongest attention level among its unread members.

The bundled client presents unread and read rows in one chronological view,
groups them into Today, Yesterday, This Week, and month sections using the
account's preferred time zone, and renders concise full-sentence descriptions
without message previews. Read content is visually muted while its actions keep
their normal contrast. Reaction rows show the reaction emoji and consolidate
activity for the same target. The list does not show a redundant `1` counter.

The bell, server indicator, and room indicators use exact unread occurrence
counts and exact Important subsets. They remain present for Ambient-only
activity but turn notification orange whenever at least one Important item is
unread. The installed app aggregates exact unread counts from all authenticated
servers currently loaded by the client. A declarative push can carry only its
origin server's exact unread count while the app is suspended. Presentation
consolidation never changes either count: two unread DMs in one displayed row
still count as two.

### Read state and policy remain separate

Room/thread read cursors describe content consumption. Notification policy
decides whether new activity creates an occurrence and whether it is eligible
to interrupt. Notification read state describes whether that occurrence is
new, while its visual attention level describes how strongly unread activity
is emphasized. Disabling a cause does not mark content read, reading a
notification does not necessarily advance a room cursor, and changing policy
does not erase existing history. Visual attention is not configurable in this
iteration and is not inferred from delivery mode.

### Reconciliation and visibility

Before occurrence reads, realtime assembly, and every read/delete mutation, the server fences the notification
materializer, notification projection, and the recipient, room, group-layout,
and RBAC projections used to validate current visibility. Retracted targets,
removed reactions, and inaccessible rooms are dismissed before they can be
returned. Delete accepts only opaque occurrence IDs scoped to the authenticated
viewer and does not hydrate target content. The complete retained list is
validated before exact totals are derived, including occurrences outside the
requested page.

## Consequences

- Read notifications remain useful history until deletion or expiry.
- Dismissal does not pretend the source was handled.
- Exact occurrence counts are stable regardless of presentation grouping.
- Mentions cannot collapse distinct jump targets, while high-volume reactions
  can remain one compact row.
- Batch deletion is safely retryable because its membership is explicit.
- A mutation transport error is ambiguous because some members may already be
  committed. Clients reconcile the authoritative occurrence page instead of
  rolling back captured rows and potentially resurrecting deleted content.
- Clients own grouping complexity, but the public API is smaller and no
  server-side group state can drift from occurrences.
