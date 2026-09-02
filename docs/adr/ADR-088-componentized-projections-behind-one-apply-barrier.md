# ADR-088: Coordinate Projection Components Behind One Apply Barrier

**Date:** 2026-09-02

**Status:** Accepted

## Context

The shared `pkg/events` framework gives each projection one projector. The
projector owns ordered event delivery, an applied sequence, readiness, and an
optional restore mechanism. The projection owns one read model and its apply
lock.

This structure is useful for materializations that have different source logs
or lifecycles. It is less useful when many related read models consume the same
event log. Chatto currently runs one `EVT` consumer for each registered
projection. Related models can reach different sequences, and a caller cannot
capture their combined state and one exact sequence through one barrier.

The current snapshot contract also produces one payload for each projector.
Combining related read models into one projector must not require one large
state type or one large payload. A later message-history design needs bounded
storage parts, while all parts still represent one event-log cutoff. This
decision supplies that structural capability. It does not define message
retention or time-based storage.

## Decision

Separate three projection responsibilities in the Loom framework:

1. A **projector** owns event-log consumption, decoding, replay, readiness,
   failure, waits, process lifecycle, one ordered apply barrier, and the exact
   event-log sequence represented behind that barrier.
2. A **componentized projection** combines related projection components. It
   routes events, prepares one complete mutation, and exposes their snapshot
   state as one projection.
3. **Projection components** own focused reducers, read models, and optional
   persistence codecs. They do not own independent consumers or sequence
   frontiers while they are part of a componentized projection.

The exported framework API uses these terms. Application-specific code can
give a componentized projection a domain name, such as `ServerContentView`.

### Ordered application

One projector delivers each decoded event to one componentized projection.
The projector holds its exclusive apply barrier while the componentized
projection prepares and commits the event.

Each applicable component first prepares an immutable mutation. Preparation
can validate input, decrypt data, clone retained messages, and calculate
changes. It must not change live component state. If any preparation fails,
the projector discards all prepared mutations, leaves component state and the
applied sequence unchanged, and marks itself as failed.

After every applicable component prepares successfully, the projector commits
the prepared mutations in a fixed order. Commit must not return an error,
perform external I/O, or call an external outcome. The projector then advances
its applied sequence before it releases the barrier. State changes and the
sequence are therefore one atomic change for projection readers.

An event that does not change a component still advances the projector after
preparation completes. The sequence then means that the componentized
projection has examined every retained source-log record through that
position.

A component must reduce events deterministically and can update only state
owned by the materialization. A component whose reduction cannot fail can use
a direct prepared-mutation adapter. The same prepare-and-commit rule applies
to an optional startup batch.

The global stream sequence is an ordering and readiness coordinate. It does
not create domain causality between unrelated aggregates and does not replace
aggregate OCC.

### Consistent capture

The projector supplies one capture operation. The operation obtains canonical
component payloads from the componentized projection and the applied sequence
under the same barrier.
The initial component codecs serialize their state during this operation.
Compression, encryption, and storage I/O occur after the barrier is released.
A later codec can add a detached capture handle when large-component copy time
must be shorter than serialization time.

A capture therefore represents all included components at exactly one source
sequence. A caller must not claim this contract after it reads live component
getters separately.

### Projection snapshot cohorts

A componentized projection can opt in to a projection snapshot cohort. Each
persistent component declares:

- a stable component key;
- an opaque component contract ID;
- a codec that captures and restores only that component's state; and
- one or more stable part keys when its state needs bounded storage parts.

One authenticated and encrypted cohort manifest binds the source-log name and
incarnation, the shared cutoff, the componentized-projection contract, and the
required component keys, contract IDs, part keys, checksums, sizes, and
immutable object references. Component parts are stored independently. Every
manifest and component object must use authenticated encryption. The
repository publishes the cohort only after every required object is stored and
validated.

A protobuf-backed component contract ID continues to combine a manual restore-
semantics token with the reachable protobuf schema fingerprint from ADR-050.
A protobuf package or full-name change therefore selects a new component
contract as described by ADR-084.

The componentized projection configuration defines the exact required
component set. Each component contract defines its maximum part count and
maximum encoded, compressed, encrypted, and decompressed size. The repository
calculates the cumulative cohort limits from those registered maxima with
checked arithmetic.
A loader rejects extra components, missing components, extra parts, and sizes
outside these limits before it allocates or downloads component state. It
loads and validates bounded parts one at a time instead of retaining every
encoded cohort object in memory.

Chatto initially permits one part for each `ServerContentView` component and
keeps ADR-050's current payload, encrypted-object, and decompression limits for
that part. A later message-history decision can give the timeline component a
larger bounded part count and more specific part-key rules without changing
the componentized-projection contract.

Restore accepts only one complete cohort. It validates every required
component before it installs a new component graph under the apply barrier. A
missing, corrupt, incompatible, future, or retention-gapped component rejects
the complete cohort and selects cold replay. Projectors do not combine
components from different cutoffs and do not perform selective component
replay in this initial design.

For a shared repository, one encrypted pointer is the publication boundary.
The repository must update it with revision OCC, reject a cutoff regression
for the same stream incarnation and contract, and retain current and previous
cohort manifests. A loader tries the current complete cohort and then the
previous complete cohort before it selects cold replay. Objects are scoped to
one cohort generation and are not shared between cohort manifests.

A failed pointer update must not expose the new cohort and must attempt to
remove all objects uploaded for that unpublished cohort. Normal bounded expiry
reclaims any orphan that remains after failed cleanup. Dropping an older
manifest removes only objects owned by that manifest and contract.

A later replica-local repository can use a repository-specific atomic
publication mechanism instead of distributed revision OCC. It must preserve
the complete-cohort and no-partial-publication guarantees.

Applications still own encryption, key management, storage selection,
publication, cleanup, size limits, and privacy approval. A process lease can
reduce duplicate publication work, but it is not a correctness boundary.

A component that is not safe to persist cannot join a componentized projection
whose projector starts after a restored cohort cutoff. The application must
keep that component in a separate cold-replayed projection or define a later
selective-replay design.

### Framework compatibility

A projection with one component remains a normal use of the framework.
Applications do not have to combine independent materializations only because
the capability exists.

The framework stays application-neutral. It does not know Chatto or Authling
event envelopes, component names, protobuf schemas, storage paths, or privacy
policy. Chatto and Authling must both compile and pass their focused tests when
the shared API changes.

This decision changes framework structure and snapshot mechanics. It does not
change application event bytes, subjects, OCC headers, replay order, or stream
positions.

When accepted, this decision partially supersedes the one-projector-per-read-
model and one-payload-per-projector parts of ADR-033, ADR-050, ADR-054, and
ADR-056. Their event authority, optional persistence, stream-incarnation,
privacy, and application-boundary rules remain in effect.

## Consequences

- Related read models can share one consumer, apply barrier, readiness state,
  and exact source-log sequence.
- Domain code can keep focused models instead of creating one large state
  structure.
- Persistence can split large materializations into bounded component parts
  without losing the shared cutoff.
- One component contract change rejects the current cohort and causes a cold
  replay of the complete componentized projection. The first design favors a
  simple exact sequence over partial restore.
- One slow or failed component delays or fails the complete componentized
  projection.
  Purpose-specific and security-sensitive materializations should remain
  independent when they need failure isolation.
- Shared repositories retain revision OCC, cutoff non-regression,
  current/previous fallback, authenticated encryption, and orphan cleanup.
- A broad consumer can deliver more records to one process path, but it avoids
  duplicate broker delivery and decoding across related components.
- The initial component codecs serialize under the apply barrier. Large
  captures can delay event application. A later detached-capture API can move
  serialization outside the barrier at the cost of temporary memory.
- Existing single-component Authling projections can keep their current
  lifecycle. Shared API changes still require Authling verification because
  Authling consumes `pkg/events` directly.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-054](ADR-054-optional-projection-persistence.md)
- [ADR-056](ADR-056-extractable-nats-event-sourcing-framework.md)
- [ADR-073](ADR-073-define-the-loom-architecture.md)
- [ADR-084](ADR-084-separate-internal-protobufs-by-storage-contract.md)
- [Authling ADR-001](../../authling/docs/adr/ADR-001-event-sourced-nats-architecture.md)
