# ADR-073: Define the Loom Architecture

**Date:** 2026-08-14

**Status:** Accepted

## Context

Chatto's event-sourced state, single `EVT` stream, projections, optional
projection persistence, and durable effect workers were established through
[ADR-033](ADR-033-event-sourced-state-with-projections.md),
[ADR-034](ADR-034-single-event-stream.md),
[ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md),
[ADR-054](ADR-054-optional-projection-persistence.md), and
[ADR-069](ADR-069-explicit-durable-consumer-lifecycle.md). ADR-056 then began
extracting the reusable mechanics into the application-neutral `pkg/events`
module. Authling's own ADR-001 applies the same shape as the concrete second
application while preserving a separate product and NATS account.

"Event-sourcing framework" describes the reusable package, but not the wider
application architecture. That architecture also includes the account and
trust boundary, application-owned events and subjects, derived read models,
snapshot repositories, durable effects, and the division between framework
mechanics and product policy. Without a shared name, those pieces are easy to
discuss as unrelated implementation choices or accidentally generalize from
one product's composition.

## Decision

Name this shared architectural style **the Loom Architecture**, expanded as
**Log-Oriented Outcomes and Materializations**.

- **Log-oriented** means durable domain decisions converge on one
  optimistic-concurrency-controlled event log rather than a collection of
  independently authoritative mutable stores.
- **Outcomes** are reliable asynchronous effects derived from committed facts
  through durable workers.
- **Materializations** are disposable read models derived from the log,
  including in-memory projections, NATS-backed state, local indexes, and
  external stores.

The name describes an architecture, not a package, executable, public API, or
new product. Existing module names and imports remain unchanged.

### Application boundary and authoritative log

Each independent application operates through its own NATS account. A Chatto
runtime and an Authling runtime never share an application account, even when
they are deployed in one NATS server or may eventually be composed in one
operating-system process.

Inside that account, the application owns one primary JetStream stream with the
logical role `EVT`. The application owns its physical resource name, subjects,
event vocabulary, aggregate boundaries, retention and replication policy,
stream-incarnation identity, and resource lifecycle. Durable domain facts in
that stream are authoritative; projections and effect-delivery state do not
become alternate authorities for the same facts.

Applications own their encoded event envelope and compatibility policy.
Chatto and Authling currently use application-owned Protobuf envelopes, but
Protobuf is not a Loom Architecture requirement and the shared framework stays
envelope-neutral.

```text
commands
   |
   | decide from caught-up state, append with OCC
   v
primary EVT stream in the application's NATS account
   |                                      |
   | ordered replay                       | durable delivery
   v                                      v
materializations                       workers
(RAM, NATS, local or external)            |
   |                                      v
   `- optional snapshots/checkpoints   external outcomes
```

Commands that decide from projected state must wait for the relevant
projection frontier and append with OCC over the same aggregate subject or
subject filter represented by that state. Conflict retries re-evaluate the
complete decision from current state. A successful append returns a durable
position that can serve as a local read-your-writes barrier.

The single stream provides one durable sequence for replay and readiness, but
applications must not infer domain causality between unrelated aggregates from
their incidental global sequence order. Cross-aggregate invariants need an
explicit OCC boundary or durable application facts.

Not every value belongs in `EVT`. Expiring workflow state, sessions, secrets,
content-addressed blobs, and purely transient signals use stores appropriate to
their lifecycle. A durable security or business fact remains an event even
when related credentials, payload objects, or caches live elsewhere.

### Materializations and restore acceleration

A projection declares the facts it consumes and applies them in stream order.
Its state may live in process RAM, NATS, a local durable index, or another
external store. `Apply` may update the materialization's own derived store and,
for a checkpointed projection, atomically commit its EVT cutoff. It must not
perform an outcome or mutate state outside that materialization, such as
sending an email, calling a webhook, or updating an unrelated system, because
replay and multiple application replicas may apply the same event again.

The shared events framework owns reusable ordering, replay, readiness,
read-your-writes, failure, and shutdown mechanics. It may provide
storage-specific bases where a concrete shared use case proves them. Its
current `MemoryProjection` base provides in-process locking, while persistence
remains an optional capability rather than part of every projection contract.

Snapshots and checkpoints are disposable replay accelerators, not event-log
backups. Every restored artifact must be bound to its projection key, opaque
contract ID, stream name and incarnation, and replay cutoff. Missing, corrupt,
incompatible, future, or retention-gapped state falls back to cold replay when
safe; it must never silently become authority.

The intended shared-framework boundary includes reusable snapshot repository
interfaces and proven storage implementations, including NATS Object Store and
S3-compatible backends, once concrete needs in more than one application show
the smallest useful contract. Until that extraction occurs, the framework
provides snapshot capture and restore hooks while applications own repository
implementation, encryption and privacy policy, retention, publication,
cleanup, storage configuration, and key management. Extracted repository code
must accept application-owned opaque payloads and metadata without importing a
product's configuration, Protobufs, resource names, or storage paths.

### Outcomes and durable workers

An external side effect required by a committed fact must remain discoverable
after crashes, replica changes, and temporary dependency failures. Named
JetStream durable consumers provide that recovery position; handlers must be
safe under at-least-once delivery.

The shared framework may own bounded pulling, progress heartbeats,
acknowledgement, retry, poison-delivery termination, cancellation, and shutdown
handoff. The application owns consumer creation, names, filters, delivery
policy, rollout and retirement, event decoding, idempotency, and the definition
of successful or terminal domain work. A worker process stopping must not erase
the deployment-wide durable consumer.

### Shared packages

`pkg/events` is the core reusable implementation of Loom mechanics.
`pkg/natsruntime` optionally supplies application-neutral embedded NATS process
lifecycle. `pkg/datacrypto` and `pkg/appconfig` are supporting shared modules,
not defining parts of the Loom Architecture. None of these packages may absorb
Chatto or Authling domain policy merely to shorten application wiring.

Use the repository-local `loom-architecture` skill when designing,
implementing, reviewing, debugging, or documenting this architecture. Apply
the product-specific instructions and skills in addition when a change affects
Chatto or Authling behavior.

## Consequences

Chatto, Authling, and the shared framework gain concise vocabulary for the
architecture they use without making either product part of the other. Design
reviews can distinguish architecture invariants from current package surface
and from application policy.

The name does not make `pkg/events` stable or require APIs to use Loom
terminology. A later ADR can rename the architecture without migrating stored
data or public APIs.

One primary event stream keeps backup, replay, readiness, and operational
reasoning small, but it also shares stream-level replication, retention, and
write capacity across the application's durable domain. A future application
that needs multiple authoritative streams would be a deliberate variation or
revision of the Loom Architecture rather than an invisible implementation
detail.

Reusable snapshot repositories become an explicit framework direction, but
their extraction remains evidence-driven. Product storage and security policy
must not leak into the shared API, and snapshots remain optional acceleration
rather than a prerequisite for adopting the architecture.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-050](ADR-050-ephemeral-encrypted-projection-snapshots.md)
- [ADR-054](ADR-054-optional-projection-persistence.md)
- [ADR-056](ADR-056-extractable-nats-event-sourcing-framework.md)
- [ADR-057](ADR-057-temporarily-incubate-authling.md)
- [ADR-058](ADR-058-application-neutral-embedded-nats-runtime.md)
- [ADR-069](ADR-069-explicit-durable-consumer-lifecycle.md)
- [Authling ADR-001](../../authling/docs/adr/ADR-001-event-sourced-nats-architecture.md)
