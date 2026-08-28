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

## Framework and application

The framework supplies the reusable mechanics for publishing events, replaying
materializations, taking and restoring snapshots, and running durable workers.
The application supplies the domain: its event types, subjects, decisions,
projection logic, outcome handlers, and operational policy.

In short: events record what happened, materializations make those facts easy
to use, and durable workers reliably turn them into external outcomes.
