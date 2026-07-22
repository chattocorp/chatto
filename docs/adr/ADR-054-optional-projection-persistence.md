# ADR-054: Projection Persistence Is Optional

**Date:** 2026-07-20

## Context

ADR-033 defines projections as derived state rebuilt from `EVT`. The original
Go `Projection` interface nevertheless required every implementation to expose
snapshot methods, including projections that always cold-replay and had no
snapshot format.

ADR-050 later added portable encrypted snapshots for selected in-memory
projections. Search adds a different case: its Bleve index already lives on a
local volume and can atomically store its own EVT cutoff. Treating either
mechanism as part of every projection would make the common interface describe
storage policy instead of event application.

## Decision

Keep the base projection contract limited to two responsibilities:

- declare the logical EVT subjects it consumes; and
- apply decoded events in stream order with their stable stream sequence.

A projection with no persistence capability starts empty and cold-replays
`EVT`. This is the default and requires no snapshot methods.

Persistence is opt-in through separate interfaces:

- `SnapshotProjection` serializes and restores portable state through the
  encrypted snapshot repository defined by ADR-050.
- `CheckpointedProjection` owns local derived storage and returns the highest
  EVT sequence atomically represented by that storage.
- `StartupBatchProjection` may atomically apply ordered batches while replaying
  the history captured at startup. Live events continue through individual
  `Apply` calls.

One projector may use at most one restore authority. It can use a portable
snapshot, a local checkpoint, or neither, but never both.

The projector validates a local checkpoint against its registration key,
opaque projection contract ID, EVT stream identity, and retained sequence
bounds. The projection owns reset policy. The framework never assumes it may
delete local files: an implementation may explicitly reset safe derived state
or fail startup and require operator intervention.

A successful checkpointed `Apply` or startup batch commits the materialized
changes and final EVT sequence atomically. Returning success before both are
durable could skip events after restart and is therefore invalid.

## Consequences

Ordinary projections implement only `Subjects` and `Apply`. In-memory
projections that need portable acceleration implement ADR-050 explicitly;
snapshot-free projections remain first-class and always cold-replay.

Disk-backed providers can reuse the projector's ordering, readiness, replay,
and failure lifecycle without exporting a large local index as a snapshot.
Their storage backend must provide an atomic mutation-and-checkpoint boundary
or use an additional durable journal.

Persistence formats remain disposable accelerators rather than sources of
truth. Contract changes, storage corruption, or lost history may require a
cold replay. Each feature documents whether that recovery is automatic or an
explicit operator action and defines its own backup and privacy treatment.
