---
name: loom-architecture
description: Apply the repository-wide Loom Architecture—Log-Oriented Outcomes and Materializations—when designing, implementing, reviewing, debugging, or documenting NATS and JetStream event-sourced applications. Use for work involving NATS account boundaries, a primary event stream, optimistic concurrency control, projections and read-your-writes, snapshots or checkpoints, snapshot repositories, durable workers, or reliable external effects.
---

# Loom Architecture

Loom stands for **Log-Oriented Outcomes and Materializations**. It is an
architecture for building event-sourced applications on NATS and JetStream.

```text
commands -> event log (EVT role) -> materializations
                                 -> durable workers -> outcomes
```

## Log-oriented

Each application runs in its own NATS account. Within that account, one primary
JetStream stream holds the application's durable domain events. Loom calls the
stream's logical role `EVT`; the application chooses its physical resource
name.

The event log is the source of truth. Commands decide what should happen and
append new events only if the relevant history has not changed. This is
optimistic concurrency control: concurrent changes cannot silently overwrite
each other. Applications define their own events, subjects, and aggregate
boundaries. Events may use Protobuf, but Loom does not require a particular
encoding.

## Materializations

Materializations are views of the event log built for a particular use. They
are commonly called projections or read models. A materialization may live in
RAM, in NATS, in a local database, or in an external system.

Materializations are derived state, not another source of truth. They can be
discarded and rebuilt by replaying the event log. Snapshots and checkpoints can
make that rebuild faster, but they are still disposable and are not backups of
the event log.

A Loom framework can provide reusable projection bases, especially for
in-memory projections, plus snapshot repository interfaces and implementations
for storage such as NATS Object Store or S3-compatible object storage.

## Outcomes

Outcomes are reliable asynchronous work caused by committed events: sending an
email, calling a webhook, updating another system, or performing any other
follow-up that must survive a restart.

Durable workers use named JetStream consumers so unfinished work remains
available after crashes or handoffs between processes. Delivery is at least
once, so outcome handlers must tolerate receiving the same event more than
once. Projections only derive state; durable workers perform external effects.

## Storage conventions

[ADR-114](../../../docs/adr/ADR-114-jetstream-storage-conventions.md) gives
the storage rules for each Loom application:

- Resource names, configurations, subjects, key shapes, value encodings, and
  durable consumer names are persisted contracts. Change them only with a
  migration that is safe when replicas of different versions run together.
- Use an existing resource first. A new resource needs a lifecycle, retention,
  storage type, or backup policy that no existing resource gives.
- Declare each resource once: name, configuration, and backup policy. Pin the
  configuration with a test.
- Encode new records as protobuf from the package for their storage contract.
- Build each key family in one place: prefix first, then owner identifiers
  from the widest to the narrowest scope. Each application selects one key
  style.
- Classify OCC conflicts with `jetstreamutil.IsSequenceConflict`, and read the
  key again after a `Create` conflict.
- Report every durable consumer in the operator diagnostics.

## Operating cost

The building blocks of Loom are not free while they are idle. Count their
cost when you design, and measure it when you change them.

- **Most of the load is inside NATS.** For a replicated stream or a durable
  consumer, the servers coordinate on each pull, acknowledgement, and write.
  The requests that a client sends are a small part of the traffic. On one
  three-node cluster, more than 90% of the messages were between the servers.
- **A worker without work must not poll.** Hold one long pull while the
  worker is idle. A loop of short pulls on many durable consumers was the
  largest cost that we found. The durable worker of the framework does this.
- **Each durable consumer has a permanent cost,** with or without work. The
  number of consumers is a design decision. State it for each application,
  and weigh it against the isolation that separate consumers give.
- **A read from the stream leader is a request.** Do not send one for each
  request and each subject. Wait only for the subjects that the next read
  depends on, and read their positions concurrently. If a bounded delay is
  acceptable, record that as a decision.
  [ADR-127](../../../docs/adr/ADR-127-bounded-staleness-for-credential-validation.md)
  is an example.
- **A periodic scan costs in proportion to the data, for as long as the
  application runs.** Run a safety net at a long interval, or make one pass
  cheap: read the records in the scan, not one at a time. State the interval
  and the number of requests for one pass where the scan is defined.
- **Measure with real data.** An empty development server does not show a
  cost that grows with the data. Restore a backup into a local server, record
  the NATS traffic, and measure retained memory together with time.

## Framework and application

The framework supplies the reusable mechanics for publishing events, replaying
materializations, taking and restoring snapshots, and running durable workers.
The application supplies the domain: its event types, subjects, decisions,
projection logic, outcome handlers, and operational policy.

In short: events record what happened, materializations make those facts easy
to use, and durable workers reliably turn them into external outcomes.
