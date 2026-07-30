# ADR-001: Build Authling on an Event-Sourced NATS Architecture

**Status:** Accepted

**Date:** 2026-07-31

## Context

Authling needs authoritative storage for security-sensitive identity state,
OIDC applications and consent, linked upstream identities, and app-scoped user
documents. It must remain correct when multiple Authling replicas handle
concurrent requests, and operators must be able to recover derived state
without maintaining a second durable source of truth.

The repository provides an independently versioned but unstable
application-neutral event-sourcing module at `hmans.de/chatto/pkg/events`. As
described by root
[ADR-056](../../../docs/adr/ADR-056-extractable-nats-event-sourcing-framework.md),
the framework already owns opaque JetStream event-log access, mandatory
optimistic concurrency control (OCC), atomic publication, ordered projection
replay, readiness barriers, and optional snapshot and checkpoint hooks. Authling
is the concrete second application that will harden this boundary and drive any
remaining extraction needed before the module becomes stable and eventually
moves out of the Chatto repository.

Authling must reuse those mechanics without adopting Chatto's event envelope,
subjects, stream identity, resource names, domain policy, or application
composition. Root
[ADR-057](../../../docs/adr/ADR-057-temporarily-incubate-authling.md) requires
the two products to remain independently buildable and separable.

## Decision

Authling will use event sourcing for all durable product state. Immutable
domain events are the source of truth; mutable databases and projections must
not become alternate authorities for the same state.

### Event log

Authling will own one primary JetStream event stream with the logical role
`EVT`, accessed through credentials for Authling's dedicated NATS account. Its
physical resource name, subjects, stream identity, retention policy, and event
vocabulary are Authling contracts and must not be copied from Chatto.

Durable records will use an Authling-owned protobuf event envelope and
Authling-owned protobuf payloads. The framework will continue to see only
opaque encoded records. Authling will provide the typed adapter responsible for
event validation, stable event identifiers, protobuf encoding and decoding,
and subject policy.

Persisted event schemas are compatibility-sensitive. Fields must not be
removed or renumbered, and existing field types or meanings must not be changed.
Evolution should be additive; incompatible semantic changes require a new
event type and explicit migration or replay reasoning.

Accounts, credentials, linked upstream identities, OIDC applications and
consent, and user documents are durable domain state and will be represented by
events. Short-lived authorization codes, login challenges, nonce material, and
similar workflow state may use dedicated expiring runtime storage when replay
and historical retention would be inappropriate. Such runtime state must not
become the only record of a durable domain fact.

### Models and projections

Authling models will serve state from in-memory projections rebuilt by ordered
replay of `EVT`. Each projection owns its event filters, apply logic, replay
frontier, readiness state, and failure lifecycle. A model must not serve until
its required projection is ready, and projection decode or apply failures must
make the affected capability unavailable rather than expose partial state.

Projection state is derived and disposable. It may be discarded and rebuilt
from retained events at any time. Event handlers must therefore be
deterministic with respect to the persisted event and prior projected state,
and must not perform external side effects.

Commands that decide from projected state must publish with an OCC token for
the same aggregate subject or subject filter represented by that state. On an
OCC conflict, Authling must refresh or wait for the relevant projection,
re-evaluate the command, and only then retry. Process-local locks may reduce
duplicate work but are never a correctness boundary.

After a successful write, request paths that promise read-your-writes behavior
will wait for their serving projections to reach the committed stream position
before returning.

### Framework boundary

Authling will consume the application-neutral `hmans.de/chatto/pkg/events`
incubation module. The shared module owns:

- opaque event-log reads, OCC-only writes, and atomic append mechanics;
- ordered projection replay, readiness, failure, and shutdown lifecycles;
- projection handles and stream-position barriers; and
- application-neutral snapshot and checkpoint hooks.

Authling will own:

- protobuf event envelopes, payloads, codecs, and compatibility policy;
- aggregate boundaries, subjects, event types, and stream identity;
- models, projections, authorization, and command retry policy;
- NATS resource configuration and lifecycle;
- snapshot eligibility, codecs, encryption policy, storage configuration,
  retention, and worker policy; and
- runtime composition.

Authling production code must not import Chatto `internal` packages or
Chatto-owned protobufs. Friction found while implementing Authling should drive
the smallest useful extraction; it is not permission to move product policy
into the shared framework.

### Projection snapshots

Projection snapshots are an optional startup optimization, not authoritative
data, a backup, or a substitute for retaining `EVT`. Authling may initially
ship without snapshots and cold-replay all projections.

When snapshots are implemented, S3-compatible object storage should be a
supported backend. This will likely require extracting Chatto's generic
snapshot repository and blob-store mechanics in addition to the core events
framework. The extracted code must not retain Chatto configuration, protobufs,
metadata names, encryption policy, or storage paths.

Every snapshot must be bound to its projection, snapshot contract, `EVT`
stream incarnation, and replay cutoff. Missing, expired, corrupt,
incompatible, or future snapshots must fall back safely to cold replay.
All persisted snapshots must be encrypted before storage, with encryption keys
managed separately from the snapshot objects.

Snapshot codecs and contents require a security and privacy review. They must
not persist plaintext personal data, raw credentials, unwrapped keys, or other
material that would weaken deletion or cryptographic erasure. A projection may
remain cold-replay-only when a safe snapshot representation is not worthwhile.

## Consequences

Authling gains a single durable source of truth, reproducible read models,
auditable state transitions, multi-replica OCC, and a practical second consumer
for the shared framework. New projections can be added without changing
persisted events, and broken derived state can be repaired through replay.

Commands and tests must account explicitly for stale projections, conflicts,
readiness, replay, and failure. Persisted protobuf events and subject shapes
become long-lived storage contracts, so early schema mistakes are expensive.

In-memory projections make normal reads fast but increase memory use and make
startup time grow with retained history. Snapshot support can bound much of
that startup cost, but introduces encryption, compatibility, retention,
cross-replica publication, and object-store lifecycle work. Because snapshots
are disposable, their failure reduces performance rather than compromising
durable state.

Using the shared framework avoids duplicating difficult JetStream mechanics.
Its pre-1.0 API may evolve as Authling becomes its first production consumer,
so framework and Authling dependency changes require explicit compatibility and
release coordination until the boundary becomes stable.
