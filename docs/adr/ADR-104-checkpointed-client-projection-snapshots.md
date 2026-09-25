# ADR-104: Persist Checkpointed Client Projections

**Date:** 2026-09-25
**Status:** Accepted

## Context

ADR-103 restores saved data into the normal frontend stores before connection.
Its presentation cache selected ten recent rooms after a two-second delay and
kept fifty rows per room. Rapid navigation could leave loaded rooms unsaved.
The copy had no applied checkpoint, so each reload required a fresh snapshot.

The existing realtime protocol exposes one opaque, viewer-bound replay cursor
for a server. It does not expose independent replay positions for each room.

## Decision

Keep the existing Svelte state owners. Do not adopt TinyBase. Use IndexedDB
records for room layout, shared resources, each loaded room and thread timeline,
and each loaded member list. Each record carries its schema version and applied
checkpoint. A manifest names the complete record set. One transaction commits
that set and its common replay checkpoint, or leaves the previous set intact.
Concurrent tabs publish complete sets; they do not merge rows from different
checkpoints.

Store the loaded timeline window with its pagination boundaries and whether
older or newer rows exist. Store member IDs, total count, and completeness.
Keep shared user profiles in their normal owner. Persist no credentials or
verified account state. Optimistic timeline and notification patches delay
persistence until authoritative reconciliation or rollback completes.

The server store schedules persistence when its resource owners change. Writes
that arrive during an active transaction coalesce to the latest pending set.
Navigation does not cancel them. There is no dwell-time rule, recent-room count,
or fixed row count. Thread windows remain owned by the server after their UI
closes and receive the same replay and privacy updates as room windows.

Retain the eight-megabyte device budget and seven-day expiry. Under byte
pressure, remove the largest optional timeline or membership records first.
The manifest excludes these records; their owners load fresh data when opened.
If layout and shared resources alone exceed the budget, skip that write.
Other server/account sets are evicted oldest first when the device budget is
full. These limits constrain storage, not which visited rooms qualify.

Restore only a complete, compatible set for the registered server and viewer.
Populate the normal stores and restore the replay cursor. Verify the viewer
before starting replay. The server's existing expired/invalid cursor fallback
replaces the retained projection. Refresh current presence, calls, and member
lists at disk recovery; they can contain state that durable replay cannot fix.
Actions remain blocked until catch-up and its resource reads succeed.

Advance a checkpoint only after all event-triggered reads finish, including
membership updates. A failed read leaves the earlier checkpoint in place.
Persist the checkpoint acceptance time separately from the snapshot write time.
Privacy purges fence local queued writes and leave persistent invalidation
cutoffs. A stale tab cannot recreate a purged copy by giving old state a newer
write timestamp; a later reconciliation barrier must first accept the state.

Discard the previous uncheckpointed cache on the IndexedDB schema upgrade.
No public protocol changes are required. No new external connections are added.

## Consequences

- Loaded rooms can survive rapid navigation and reload through the same state
  path used by live updates. Equal server data retains the same presentation.
- Data and its checkpoint form one recovery unit. A missing, corrupt, expired,
  or incompatible resource rejects that unit rather than enabling partial replay.
- A stored cursor is a replay position, not authorization. Offline data can be
  stale until the server confirms access and reconciles it.
- Serialization, validation, and byte limits remain application responsibilities.
  A browser can still deny, evict, or interrupt storage. Persistence is best effort.
- The cold restore reads the retained set eagerly. Larger cache budgets or
  independent per-resource replay would require a separate performance and
  protocol decision.
- This supersedes ADR-103's room-selection, row-count, and cursor-free storage
  rules. Its first-paint, normal-store rendering, and authorization rules remain.
