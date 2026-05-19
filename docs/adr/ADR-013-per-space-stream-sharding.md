# ADR-013: Per-Space JetStream Stream Sharding with Lazy Initialization

**Date:** 2026-03-01

**Status:** Superseded by ADR-030 (Retire the Space tier). The per-space stream and KV bucket family (`SPACE_{id}_EVENTS`, `SPACE_{id}_CONFIG`, etc.) was collapsed into the unified `SERVER_*` resources by the Phase-4 migration (#354); the `kind` segment in subjects (`server.room.channel.*` / `server.room.dm.*`) now disambiguates what space-sharding used to. The decision recorded here is preserved as historical context.

## Context

Chatto needs to store ordered event logs for room messages and space lifecycle events. A single global stream would require all consumers to scan all messages across all spaces, creating contention. Per-room streams would create too many JetStream streams for large instances with thousands of rooms.

## Decision

Shard at the space level. Each space gets its own dedicated JetStream stream (`SPACE_{id}_EVENTS`) and a family of per-space KV buckets:

- `SPACE_{id}_CONFIG` — room and space configuration
- `SPACE_{id}_RBAC` — roles and permission overrides
- `SPACE_{id}_RUNTIME` — ephemeral runtime state
- `SPACE_{id}_BODIES` — message body content
- `SPACE_{id}_REACTIONS` — reaction aggregates
- `SPACE_{id}_THREADS` — thread metadata
- `SPACE_{id}_ASSETS` — asset metadata

Streams and buckets are created lazily on first access and cached in-process using `sync.Map` (streams) and `lazycache.Cache` (KV buckets). `CreateOrUpdateStream` is called at most once per space per process lifetime.

## Consequences

- **Space isolation**: Each space's event history is fully independent. Consumers for one space don't contend with others. A noisy space doesn't degrade performance for quiet ones.
- **Per-space sequence counters**: JetStream's sequence numbers are per-stream, so `DeliverByStartSequencePolicy` works within a space. Pagination and catchup are naturally scoped.
- **Lazy initialization avoids startup cost**: Streams and buckets are created on first use, not at space creation time. This means a space with no messages yet has no JetStream overhead.
- **Naming convention is load-bearing**: Backup/restore tooling, admin monitoring, and stream cleanup all depend on the `SPACE_{id}_*` naming pattern. Changing it would require migration tooling.
- **More streams than a single-stream design**: A Chatto instance with 1,000 spaces has 1,000+ JetStream streams. NATS handles this well, but it's more than a single-stream architecture.
- **Cross-space queries are expensive**: Finding "all messages by user X" requires iterating every space's stream. This is an acceptable tradeoff since cross-space queries are rare (admin/moderation only).
