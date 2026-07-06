# ADR-006: KV as Source of Truth, Streams as Audit Logs

**Date:** 2026-03-01

> **Superseded by [ADR-033](ADR-033-event-sourced-state-with-projections.md)** (2026-05-24). The CRUD-with-audit-log pattern described below is being replaced, aggregate by aggregate, with event-sourced state and derived projections. See [ADR-035](ADR-035-per-aggregate-phased-migration.md) for the migration approach.

## Context

With NATS JetStream as the data store (ADR-001), there are two storage primitives available: **KV buckets** (key-value with last-writer-wins semantics) and **streams** (ordered, append-only message logs). The question is which serves as the source of truth for current state.

Two approaches:

- **Event sourcing**: Streams are the source of truth. Current state is derived by replaying events. Conceptually pure but requires snapshotting, projection logic, and careful handling of schema evolution.
- **CRUD + audit log**: KV holds current state directly. Streams capture the history of changes as events. Reads go to KV (fast, single lookup). Writes update KV and append to the stream.

## Decision

Use the **CRUD + audit log** pattern:

- **KV buckets** are the source of truth for current state (user profiles, room config, memberships, permissions, roles).
- **Event streams** provide ordered history for timeline-based data (room messages, space events) and real-time delivery via subscriptions.
- **Write path**: Mutations write to KV first, then publish an event to the relevant JetStream stream.
- **Read path**: Queries read directly from KV. The stream is consulted only for historical timeline views (message history, event replay).

## Consequences

- **Fast reads**: Current state is a single KV get, not a stream replay. No projection lag, no eventual consistency for reads.
- **Simple write path**: Update KV, publish event. No saga, no event handler chain to derive state.
- **Dual-write risk**: KV write and stream publish are not transactionally linked. If the process crashes between the two, state can diverge. In practice, NATS's in-process reliability and the ephemeral nature of the gap (next write corrects it) make this acceptable.
- **History is append-only**: Message edits and deletions publish new events referencing the original; they don't modify the stream. The KV entry is updated to reflect current state.
- **Clear separation of concerns**: KV answers "what is the current state?" and streams answer "what happened, in order?" Different parts of the system use the appropriate primitive.
