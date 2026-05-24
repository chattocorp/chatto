# ADR-033: Event-Sourced State with Derived Projections

**Date:** 2026-05-24

## Context

[ADR-006](ADR-006-kv-source-of-truth-streams-audit-log.md) established CRUD + audit log: KV buckets hold current state, streams capture history. As the codebase has grown on that pattern, several costs have become hard to ignore:

- **Subject-index RAM.** [ADR-026](ADR-026-event-identity-via-nanoid.md) plus the per-event-ID subject pattern (`server.room.{kind}.{roomId}.msg.{eventId}`) gives us O(1) message lookups via `GetLastMsgForSubject`, but NATS keeps its per-subject index in memory. Subject cardinality scales 1:1 with message count. On busy instances this dominates RAM and grows without bound.
- **Migration ergonomics.** Each schema change against a KV bucket needs a bespoke boot migration (`BackfillAttachmentRecords`, `DropLegacyAttachmentRecords`, `BackfillAttachmentLocatorData`, …). KV gives us neither schema tooling nor a replay primitive; every change is its own one-off script.
- **A zoo of write disciplines.** Best-effort publish for some events, store-then-publish for messages, OCC helpers for others, atomic-claim KVs for uniqueness — each pattern is locally reasonable but there is no shared write discipline across the codebase.
- **The audit log is secondary.** Stream retention can trim history. The audit log isn't really primary; it's a derived signal that happens to be persistent.

Event sourcing inverts the relationship: events are the truth, state is derived. The motivations match the costs above:

- **Subject cardinality drops to O(aggregates).** One subject per room, one per user, one per RBAC namespace — not one per message.
- **Schema changes become projection rebuilds.** Drop the projection, replay the stream, done. No boot migration code.
- **One write primitive.** Append event with OCC. Every mutation in the codebase goes through it.
- **The audit log is the data, by construction.** Not a side effect.

This is a large change. The migration is per-aggregate and phased; that strategy is the subject of [ADR-035](ADR-035-per-aggregate-phased-migration.md). The shape of the event log itself — single stream vs. many — is the subject of [ADR-034](ADR-034-single-event-stream.md). This ADR commits to the model.

## Decision

Adopt event sourcing as the storage pattern for domain state.

- **The event stream is the source of truth.** Domain mutations are expressed as events appended to a single JetStream stream (see ADR-034). Current state for any read is derived from these events.
- **Projections are derived state.** Each aggregate type has one or more projections — in-memory Go data structures rebuilt from the event stream — that serve reads. Projections live entirely in process memory; multiple Chatto processes each maintain their own copies, consuming the same stream.
- **All writes use optimistic concurrency control.** Every event publish carries a `Nats-Expected-Last-Subject-Sequence` header. The framework offers no "publish without OCC" primitive. This guarantees that per-subject (per-aggregate) history is a serialized sequence with no race-induced gaps. The same invariant makes migration replayable and makes uniqueness claims expressible as ordered subject sequences.
- **Read-your-writes via wait-for-seq.** Successful publish returns a stream sequence number. The actor's next read against the projection blocks (briefly) until the projection consumer position has reached that sequence. Reads from other actors see the new state on the projection's natural consumer cadence — typically sub-millisecond, never coordinated.
- **Snapshots are deferred but accommodated.** The projection interface includes `Snapshot()` and `Restore()` methods from day one. No snapshot orchestration ships initially — startup replays from the beginning of the stream. We add snapshot persistence when stream length makes startup time unacceptable, without changing the projection contract.
- **Ephemeral state stays out.** Presence, typing indicators, link-preview cache, auth tokens, image cache, and similar TTL-driven or content-addressed state continue to use plain NATS KV / Object Store. They are not part of the event log.
- **A thin internal Go package owns the abstractions.** No third-party event-sourcing framework is adopted. The package exposes `Publish`, a `Projection` interface, a `Projector` (consumer + apply loop), and `WaitForSeq`. Estimated size: ~1000–1500 lines, fully under our control.

This ADR supersedes ADR-006.

## Consequences

- **Subject-index RAM stops scaling with messages.** The dominant memory cost on busy instances goes away. This is the most measurable single win.
- **Schema changes become projection rebuilds.** Adding a field, changing a derivation, computing a new index — all become "drop projection, replay stream." No boot migration code, no KV backfills. The maintenance surface from ADR-006's CRUD model collapses.
- **One write primitive.** `Publish(subject, event, expectedSeq) -> seq` replaces today's variety of KV writes, best-effort publishes, OCC helpers, and atomic claims. New domain code has exactly one mutation pattern to learn.
- **Mandatory OCC has a real cost.** Every write must read the projection (or know the subject is fresh) to know the expected sequence. For high-contention aggregates this can mean retry loops on conflict. We accept this as the cost of removing every "did I race?" question from the codebase.
- **Read-your-writes is per-process.** A writer on process A waits for A's projection consumer to advance; a reader on process B sees the change on B's natural cadence. Cross-process consistency is eventual. In practice the delay is sub-millisecond and bounded by NATS Core latency. GraphQL responses from two processes can momentarily disagree; we treat this as acceptable for chat.
- **Startup cost grows with stream length.** Until snapshots ship, a long-running instance pays projection-rebuild time on every restart. Acceptable in alpha; will need addressing before GA. The interface is in place; only the implementation is deferred.
- **Migration is per-aggregate and phased.** See ADR-035. Big-bang migration is not feasible. The two systems coexist for the duration of the transition.
- **Ops story shifts.** Operators back up one big stream instead of many KV buckets. Restore is "replay events," which is conceptually simpler than restoring multiple KV snapshots into a coordinated state. Long-term storage and retention are out of scope here and will be revisited.
- **Crypto-shredding model survives unchanged.** [ADR-007](ADR-007-per-user-encryption-with-crypto-shredding.md) per-user encryption applies to event payloads; deleting a user's key still renders their messages unreadable. The body/event split in [ADR-011](ADR-011-message-body-event-split.md) becomes "encrypted body field on the event," simplifying the dual-write pattern that exists today.
- **No third-party framework dependency.** Trade: we own the abstractions and their evolution. Read both `looplab/eventhorizon` and `blinkinglight/bee` for design vocabulary, but ship our own minimal package shaped to Chatto's needs.
- **One consumer per projection, for now.** Each `Projector` runs its own NATS `OrderedConsumer` against the projection's declared `Subjects()`. Simple, isolated, and correct — a slow or buggy projection only affects itself, and `WaitForSeq` is trivially per-projection. The cost is that N projections on one process means N independent startup replays of their slice of the stream and N idle subscriptions. We accept this with one projection live; we will revisit (see "Out of scope") when projection count grows.

## Out of scope for this ADR

- The shape of the event log itself (one stream vs. many) — see [ADR-034](ADR-034-single-event-stream.md).
- The migration strategy from today's CRUD+log model — see [ADR-035](ADR-035-per-aggregate-phased-migration.md).
- Snapshot persistence and projection bootstrapping from snapshots — interface is committed here; mechanism is deferred to a future ADR when stream length forces the issue.
- Long-term retention, archival, cold storage — also deferred.
- Multi-process coordination beyond "every process maintains its own projection." If we ever want a single authoritative projection across processes, that is a separate decision.
- **Sharing one consumer across multiple projections.** With several projections subscribing to overlapping subject filters (e.g. multiple `evt.room.>` consumers), grouping them onto a single shared `OrderedConsumer` per unique filter is the obvious next step — startup replay and NATS overhead would then scale with aggregate-type count rather than projection count. The `Projection` interface is unchanged by this evolution; only the `Projector`'s internal lifecycle moves. Deferred until we have 3+ projections and a measurable cost from the per-projection shape.
