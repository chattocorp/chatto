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
- **All writes use optimistic concurrency control.** Every event publish carries a `Nats-Expected-Last-Subject-Sequence` header. The framework offers no "publish without OCC" primitive. This guarantees that per-subject (per-aggregate) history is a serialized sequence with no race-induced gaps. The same invariant makes migration replayable and makes uniqueness claims expressible as ordered subject sequences. When a write decision comes from projection state, the owning projection exposes a snapshot containing both the derived state and the applied stream sequence for the same OCC subject/filter; conflict retries wait for catch-up and decide from a fresh snapshot.
- **Read-your-writes via stream positions.** Successful publish returns a stream sequence number, and the writer already knows the subject or subject filter it published/guarded against. Domain code wraps those into an `events.StreamPosition` and asks the relevant projection owner to wait until its projector reaches that position. The projector verifies both sides of the contract: the sequence must belong to the supplied subject/filter, and the projection must consume the sequence's actual subject. Waiting on a raw sequence that the projection will never consume is a programming error, not a timeout-shaped control path. Reads from other actors see the new state on the projection's natural consumer cadence — typically sub-millisecond, never coordinated.
- **Snapshots are deferred but accommodated.** The projection interface includes `Snapshot()` and `Restore()` methods from day one. No snapshot orchestration ships initially — startup replays from the beginning of the stream. We add snapshot persistence when stream length makes startup time unacceptable, without changing the projection contract.
- **Runtime state stays out.** Presence, typing indicators, link-preview cache, auth tokens, image cache, and similar TTL-driven or content-addressed state are not part of the event log. Durable latest-value runtime state uses `RUNTIME_STATE` per [ADR-036](ADR-036-runtime-state-kv-boundary.md); purely transient state can remain memory-backed, and binary/object data stays in object stores. Security-relevant workflow facts may still be appended to EVT for audit, but only as safe facts without raw tokens, links, passwords, auth codes, or raw IP addresses.
- **A thin internal Go package owns the abstractions.** No third-party event-sourcing framework is adopted. The package exposes `Publish`, `StreamPosition`, a `Projection` interface, and a `Projector` (consumer + apply loop). Estimated size: ~1000–1500 lines, fully under our control.

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
- **Crypto-shredding model survives, with a deliberate ES exception for body payloads.** [ADR-007](ADR-007-per-user-encryption-with-crypto-shredding.md) per-user encryption applies to event payloads; deleting a user's key still renders their messages unreadable. The body/event split in [ADR-011](ADR-011-message-body-event-split.md) now lives inside EVT as private body payload events plus public message facts, removing the old body-side-table coordination problem for migrated messages. Public message facts (`MessagePostedEvent`, `MessageEditedEvent`, `MessageRetractedEvent`) remain immutable event-sourced history; private `MessageBodyEvent` payload facts are retention-controlled content carriers and may be securely deleted to satisfy message deletion semantics. This is an explicit, narrow departure from strict event-sourcing immutability: a cold replay after deletion preserves the durable conversation facts but cannot reconstruct deleted body material. If we later introduce per-message DEKs in a separate non-backed-up key store, message deletion can shred that per-message key instead of deleting `MessageBodyEvent`, allowing these payload events to become immutable again and better align with textbook ES practice.
- **No third-party framework dependency.** Trade: we own the abstractions and their evolution. Read both `looplab/eventhorizon` and `blinkinglight/bee` for design vocabulary, but ship our own minimal package shaped to Chatto's needs.
- **Shared replay with projection-local readiness.** Initially each `Projector` ran its own NATS `OrderedConsumer`; projection startup diagnostics showed that duplicate replay delivery, protobuf decode, subject matching, and JetStream metadata parsing were significant costs. `ChattoCore.Run` now replays `evt.>` through one process-local ordered consumer, decodes each event once, and fans it out to projectors whose logical subject filters match. Each projection still has independent status, lag, failure, startup metrics, and `WaitFor` state, so callers keep the same read-your-writes contract while startup replay work scales with the event stream rather than registered projection count.
- **Projection subject declarations have separate roles.** A projection's `Subjects()` value is the logical consumption and readiness contract used by waits, live delivery, diagnostics, and tests. Optional replay subjects are a physical consumer concern and may be broader for shared replay or legacy compatibility. Broad filters remain intentional when a projection's snapshot exposes an OCC token for the full aggregate tail; focused filters are appropriate for stable derived indexes over selected event families.
- **Stable stream sequences are a framework concern.** Projection `Apply` methods receive the global stream sequence from the projector framework. Projections should not parse JetStream metadata or use consumer sequence numbers; reconnect replay, live delivery readiness, and projection-backed OCC all depend on the same stable stream sequence vocabulary.

## Out of scope for this ADR

- The shape of the event log itself (one stream vs. many) — see [ADR-034](ADR-034-single-event-stream.md).
- The migration strategy from today's CRUD+log model — see [ADR-035](ADR-035-per-aggregate-phased-migration.md).
- The runtime-state storage boundary — see [ADR-036](ADR-036-runtime-state-kv-boundary.md).
- Snapshot persistence and projection bootstrapping from snapshots — interface is committed here; mechanism is deferred to a future ADR when stream length forces the issue. The profiling hooks and admin metrics added during replay work are the expected way to decide when snapshots are actually needed.
- Long-term retention, archival, cold storage — also deferred.
- Multi-process coordination beyond "every process maintains its own projection." If we ever want a single authoritative projection across processes, that is a separate decision.
