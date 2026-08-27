# Chatto Architecture Inventory

This directory records the current shape of the Chatto runtime: what exists,
where it lives, and which component owns it. It is deliberately not a history
or rationale document.

- Cross-cutting decisions and their consequences live in the
  [Architecture Decision Records](../adr/INDEX.md).
- Feature behaviour and feature-specific design live in the
  [Feature Decision Records](../fdr/INDEX.md).
- Canonical terminology lives in the [glossary](../GLOSSARY.md).
- Public protobuf methods and message fields live in the generated
  [ConnectRPC API reference](../../apps/docs-website/src/content/docs/reference/connectrpc-api/index.mdx).

## System map

Chatto is a real-time chat application with a ConnectRPC public API, a protobuf
realtime WebSocket, and NATS JetStream as its primary data store.

1. API handlers authenticate the caller and delegate domain decisions to core
   operation models.
2. Durable domain writes append protobuf facts to the `EVT` stream using
   JetStream optimistic concurrency control.
3. In-memory projections rebuild read models from `EVT`; writers wait for the
   relevant projection sequence to provide local read-your-writes.
4. `RUNTIME_STATE` stores persisted latest-value operational records that are
   intentionally outside durable domain history.
5. JetStream republishes committed facts to `live.evt.>`, while transient
   invalidations use `live.sync.>`. The realtime delivery model waits for local
   projections and authorizes events before mapping them to public frames.

The runtime is process-local but supports multiple Chatto replicas connected to
the same NATS account. Cross-replica correctness comes from JetStream and KV
atomicity, not process-local locks or singleton goroutines.

Related decisions: [ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md),
[ADR-034](../adr/ADR-034-single-event-stream.md),
[ADR-036](../adr/ADR-036-runtime-state-kv-boundary.md), and
[ADR-045](../adr/ADR-045-public-api-stability-tiers.md).

## Inventories

| Category | Contents |
| -------- | -------- |
| [Runtime components](runtime-components.md) | Core models, facades, publishers, workers, and their responsibilities |
| [Projections](projections.md) | Registered projectors, logical subjects, read models, and snapshot support |
| [NATS resources](nats-resources.md) | Streams, KV buckets, Object Stores, live roots, persistence, and backup status |
| [Subjects and events](subjects-and-events.md) | Durable envelopes, subject namespace, event tokens, and transient subjects |
| [Runtime state](runtime-state.md) | KV and Object Store key shapes, TTLs, security boundaries, and owners |
| [Durable effects](durable-effects.md) | External effects, recovery guarantees, idempotency, and known gaps |
| [Interfaces](interfaces.md) | ConnectRPC packages, mounted services, transports, and authentication boundaries |
| [Realtime delivery](realtime-delivery.md) | WebSocket handshake, server-side fanout, authorization, and catch-up model |

## Inventory rules

- Record current runtime facts, not migration archaeology or design rationale.
- Link to the authoritative source files and relevant ADRs or FDRs.
- Put feature semantics in an FDR and public API details in protobuf comments;
  do not reproduce either here.
- Preserve compatibility facts only while they constrain current reads, writes,
  cleanup, mixed-version operation, or stored data.
