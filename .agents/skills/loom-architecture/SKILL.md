---
name: loom-architecture
description: Apply the Loom Architecture—Log-Oriented Outcomes and Materializations—when designing, implementing, reviewing, debugging, or documenting NATS and JetStream event-sourced applications. Use for work involving NATS account boundaries, a primary event stream, optimistic concurrency control, projections and read-your-writes, snapshots or checkpoints, snapshot repositories, durable workers, or reliable external effects.
---

# Loom Architecture

Use this skill as an implementation and review checklist for applications and
frameworks that follow the Loom Architecture: one authoritative event log,
disposable materializations, and durable outcomes.

## Preserve the Loom Invariants

- Give each independent application its own NATS account. Do not share an
  application account merely because applications use the same NATS server or
  run in the same operating-system process.
- Keep one primary JetStream event stream with the logical role `EVT` inside
  that account. Let the application own its physical name, subjects,
  vocabulary, aggregate boundaries, identity, configuration, and lifecycle.
- Treat committed durable facts as authoritative. Do not make a projection,
  snapshot, checkpoint, durable-consumer position, cache, or external index an
  alternate source of truth.
- Require OCC for event mutations. Match the OCC subject or filter to the state
  used by the decision, wait for that projection frontier, and re-run the
  complete decision after a conflict.
- Use committed stream positions for readiness and local read-your-writes.
  Do not infer business causality between unrelated aggregates from incidental
  global stream order.
- Keep event envelopes and codecs application-owned. Protobuf is a valid
  choice, but it is not a framework or Loom invariant.
- Keep runtime state, expiring workflows, secrets, binary objects, and
  transient signals outside `EVT` unless they represent a durable domain fact.

## Design Materializations

Treat projections as disposable materializations of retained events:

- Let a projection store its derived state in RAM, NATS, local durable
  storage, or an external system according to its access and recovery needs.
- Keep `Apply` deterministic and side-effect-free. Replay and multiple replicas
  must not multiply an external effect.
- Declare consumed subjects precisely and keep the projector's readiness
  frontier with the projection it owns.
- Fail the affected capability when decode, apply, or startup replay fails.
  Never serve partial state as if it were current.

Choose one restore strategy per projection:

- Use no persistence and cold-replay `EVT` by default.
- Use a portable snapshot for replaceable state that benefits from shared
  object storage.
- Use a local checkpoint when the projection can atomically persist its
  derived changes and EVT cutoff in its own store.
- Never combine a snapshot and checkpoint as competing restore authorities.

Bind every restored artifact to its projection key, opaque contract ID, stream
name and incarnation, and replay cutoff. Treat missing, corrupt, incompatible,
future, or retention-gapped artifacts as unavailable acceleration, not truth.
Review snapshot contents for privacy, deletion, cryptographic-erasure, and key
exposure risks.

A framework may provide a snapshot repository interface and implementations
for stores such as NATS Object Store or S3-compatible object storage. Keep
these contracts payload-agnostic: the application owns serialization,
encryption and privacy policy, retention, cleanup, resource names, storage
configuration, and key management.

## Design Outcomes

Use durable workers when a committed fact requires work that must survive a
crash, dependency outage, replica handoff, or scale-to-zero interval.

- Configure a named durable JetStream consumer at the application boundary.
- Make handlers safe under at-least-once delivery through idempotency,
  terminal-state checks, or an OCC-protected completion record.
- Let the shared worker own bounded fetch execution, heartbeats,
  acknowledgement, retry, poison termination, cancellation, and shutdown
  handoff.
- Keep consumer names, filters, starting policy, creation, rollout,
  replacement, and retirement application-owned.
- Do not delete a deployment-wide consumer when one worker or replica stops.
- Define mixed-version and rollback behavior before changing a durable
  consumer contract.

Do not use a process-local queue, timer, or lease as the only recovery path for
a required external effect. A lease may reduce duplicate work, but it does not
replace durable discovery or fence writes.

## Review a Change

Answer these questions before considering Loom work complete:

1. Which application and NATS account own the data?
2. Is the value a durable fact, materialization, outcome state, runtime value,
   secret, object, or transient signal?
3. Which aggregate subject or filter is the OCC boundary?
4. Which projections consume the fact, and which must be current before the
   command or subsequent read returns?
5. Can every materialization rebuild from retained facts, and is its restore
   artifact bound to the correct contract, stream incarnation, and cutoff?
6. Does every required external effect have durable discovery and an
   at-least-once-safe handler?
7. Who owns each durable consumer's creation, versioned identity, rollout, and
   retirement?
8. What happens during startup, replay, conflict, dependency failure, rolling
   deployment, rollback, and stream recreation?
9. Which focused tests demonstrate those answers?
