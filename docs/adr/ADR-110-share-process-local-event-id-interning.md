# ADR-110: Share Process-Local Event ID Interning Across Projections

**Date:** 2026-09-29

**Status:** Accepted

## Context

Each Chatto replica keeps every projection in RAM. The Server Content View
(ADR-089) and the Notification Decisions projection keep one index entry for
every message in the history. ADR-090 removed complete payloads from these
indexes, but the indexes still grow with the message count.

A measurement on a copy of the largest production server showed the cost of
this growth. The copy had 134,671 EVT records and 38,653 message posts. The
Server Content View retained 28.4 MB of Go heap. The Notification Decisions
projection retained 12.3 MB after ADR-109 added its Badge source index. On the
production server, these values grew by about 300 KB for each 1,000 posted
messages.

A breakdown of the retained heap showed these causes:

- The Room Timeline, Threads, and Reactions components each kept a private
  string-keyed map over the same message IDs. The Notification Decisions
  projection kept a fourth copy.
- Per-message records contained Go strings, `time.Time` values, and map
  entries. These values contain pointers, so the garbage collector scans them.
- Follow state, thread summaries, and content keys used nested maps, string
  keys, or complete protobuf messages.

ADR-089 keeps the focused component models separate behind one apply barrier.
The Notification Decisions projection has its own replay frontier. A shared
index must not connect the state or the frontiers of these models.

## Decision

Chatto keeps one process-wide event ID table. The table interns event IDs as
dense, one-based `uint32` handles. Handle zero means "no ID". The production
wiring gives the same table to the Room Timeline, Threads, and Reactions
components of the Server Content View and to the Notification Decisions
projection. A projection that the production wiring does not create, for
example in a test, owns a private table.

The table follows these rules:

- The table is append-only. A handle stays valid for the life of the process,
  and Chatto never reuses a handle.
- A handle is process-local. Snapshots, events, and API responses store ID
  strings and never store handles. A restore interns the strings again.
- The table contains no projection state. It does not decide projection
  semantics, so projections with independent replay frontiers can share it.
  Each projection keeps its own handle-indexed state.
- The table synchronizes itself, because projections apply and read under
  different locks. Its hash index has 64 shards with separate locks. A read of
  an ID from a handle does not lock.
- The table never calls other code while it holds a lock. Callers can use it
  while they hold a projection lock.
- The table stores ID bytes in immutable, append-only arena chunks. A returned
  ID string refers to arena bytes without a copy. Chatto never writes arena
  bytes that a location names.

Projection state that grows with history follows these conventions:

- Use handles instead of ID strings in indexes.
- Use dense slices indexed by handle instead of maps when most handles have a
  value.
- Keep per-message records free of Go pointers. Store times as Unix
  nanoseconds and small flags in spare bits or bytes.
- Keep derived values out of records when other fields determine them.
- Build detached API values, such as protobuf messages and ID strings, at read
  time.
- Keep snapshot formats and contracts unchanged when only the in-memory
  layout changes.

A change to projection memory layout must show equivalence with the previous
version on a copy of real data. The check compares snapshot bytes, restores
snapshots from the previous version, and compares a digest of all read
results.

## Consequences

- On the same production copy, the Server Content View retains 16.8 MB instead
  of 28.4 MB. The Notification Decisions projection adds about 5.6 MB next to
  the Server Content View instead of 12.3 MB.
- Together, the two projections retain about 600 more bytes for each
  additional message. A replay of 25, 50, and 100 percent of the history gave
  this value. The growth is still linear. A server with one million messages
  needs about 600 MB for each replica. A cold tier for old history can remove
  this limit later.
- Replay time decreases. A single lookup by event ID costs about 12 ns more,
  because it takes a shard read lock. Concurrent lookups of different IDs
  rarely contend: on 14 CPUs, they stay within about 10 percent of their
  earlier cost.
- The table does not shrink until the process restarts. IDs from deleted rooms
  and failed restores stay in it.
- The table uses `unsafe.String` to return IDs without a copy. Its correctness
  depends on the immutability rule for arena bytes.
- Estimates of projection size exclude the shared table from each component
  and count it once as the `event_ids` component of the Server Content View.
- Rollback to an earlier binary does not need a cold replay, because snapshot
  formats do not change.

## Related

- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-088](ADR-088-componentized-projections-behind-one-apply-barrier.md)
- [ADR-089](ADR-089-server-content-view.md)
- [ADR-090](ADR-090-hydrate-room-timeline-payloads-from-evt.md)
- [ADR-109](ADR-109-compute-badge-attention-from-projections.md)
