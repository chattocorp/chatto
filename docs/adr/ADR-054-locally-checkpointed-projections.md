# ADR-054: Locally Checkpointed Projections

**Date:** 2026-07-20

## Context

ADR-033 introduced process-local in-memory projections rebuilt from `EVT`, and
ADR-050 added optional encrypted snapshots as disposable startup accelerators.
The projection interface consequently requires every projection to implement
snapshot methods, even when it has no snapshot codec.

Search introduces a different derived-state shape. Its Bleve index lives on a
local volume, may be much larger than memory, and should resume from the event
position committed inside that index. Exporting the complete index through the
projection-snapshot repository would add cost without improving correctness.
Building a separate EVT consumer for search would instead duplicate ordering,
readiness, replay, failure, and stream-incarnation logic already owned by the
projector framework.

## Decision

Allow a projection to own a disposable, locally checkpointed read model while
continuing to use the common `events.Projector` lifecycle.

The base projection contract contains only its logical subjects and ordered,
idempotent event application. Snapshot persistence and local checkpoint
restoration become separate optional interfaces. Existing in-memory
projections can opt into ADR-050 snapshots; a locally checkpointed projection
can open and validate its own state; a projection without either mechanism
cold-replays `EVT`.

A checkpointed projection receives a framework-owned restore request containing
its stable registration key, opaque projection contract ID, EVT stream name
and incarnation, and the stream's current retained bounds. It returns the
highest EVT stream sequence durably represented by its local state.

For a checkpointed projection, a successful `Apply(event, sequence)` means the
derived-state mutation and that sequence have been committed atomically. The
projector advances readiness only after `Apply` succeeds and starts a restored
consumer at the following sequence. Duplicate application remains harmless.
Projection code receives the framework-provided stream sequence and does not
parse JetStream message metadata itself.

A projection may optionally batch events while replaying the history captured
at startup. A successful startup batch must be exactly equivalent to applying
its events individually in stream order, and must atomically commit all derived
mutations with the batch's final sequence. The projector advances only after
that commit. A failed or decode-interrupted batch remains wholly uncommitted
from the projector's perspective and reports failure at its first sequence.
After reaching the captured startup target, live events return to individual
`Apply` calls so read-your-writes latency is not coupled to a batch window.

The projection contract ID covers every input that determines whether the
local state at its checkpoint is equivalent to replaying `EVT` through that
sequence, including indexed fields, analyzers, event handling, and checkpoint
meaning. A contract mismatch, different EVT incarnation, checkpoint ahead of
the stream, checkpoint behind deleted history, corrupt local state, or an
explicit operator rebuild causes the projection to discard its local state
and cold-replay. Locally checkpointed state is derived cache data and is not
required for backup correctness.

One projection has one restore authority. A projection cannot simultaneously
restore from an ADR-050 snapshot and from locally checkpointed state. This
avoids ambiguous precedence and mismatched replay frontiers.

Projection registration and diagnostics move far enough outside `ChattoCore`
that runtime units can reuse the same projector metadata, startup readiness,
failure state, replay completion, waits, and graceful close lifecycle. Core
continues to own the registration catalogue for its in-process read models;
standalone units own their registrations without constructing or running
`ChattoCore`.

## Consequences

Search and future disk-backed indexes reuse the established projection
consumer and readiness guarantees instead of implementing custom EVT replay.
Restart cost is proportional to events after the local checkpoint when the
index remains valid, while deleting the local directory always falls back to
correct reconstruction from `EVT`.

The base projection interface becomes smaller and describes the behavior every
projection actually shares. Snapshot codecs and local checkpoint stores remain
explicit capabilities with independently testable contracts.

Disk-backed projections take on a strong transactional requirement: returning
success before both the materialized change and checkpoint are durable can
silently skip an event after restart. A storage backend that cannot provide
that atomicity cannot implement this interface without an additional durable
journal or transaction boundary.

Startup batching reduces transaction and write-amplification costs for
disk-backed rebuilds. Implementations must maintain batch-local state so that
multiple events affecting the same entity behave exactly like sequential
application. A crash safely replays the whole last uncommitted batch.

Local indexes may contain decrypted or otherwise privacy-sensitive derived
data. Each feature must define removal, rebuild, backup-exclusion, filesystem,
and operator trust requirements appropriate to its contents. Local checkpoint
support by itself does not authorize persisting plaintext.

ADR-050 remains the mechanism for portable, encrypted snapshots of eligible
in-memory projections. Locally checkpointed indexes are machine-local caches,
are not uploaded through the snapshot repository, and are not restored by
`chatto backup`.
