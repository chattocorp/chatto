# ADR-036: Persist Runtime State in RUNTIME_STATE

**Date:** 2026-05-27

## Context

[ADR-033](ADR-033-event-sourced-state-with-projections.md) moves Chatto's
content and domain state to `EVT`, with projections derived from that event
history. Not every durable value belongs in that log. Some state is operational
or user-runtime state: it needs to survive process restarts, but it is a
latest-value cursor, token, job status, cache marker, or runtime preference
rather than reconstructable Chatto content.

Historically these values have lived in a mix of buckets:

- `SERVER_RUNTIME` for read cursors, mention flags, per-room runtime indexes,
  and video processing state.
- `AUTH_TOKENS` for bearer tokens.
- Feature-specific KV buckets or object stores for caches and processing
  metadata.

That spread makes the event-sourcing boundary fuzzy. It also keeps
`SERVER_RUNTIME` as an architectural name even though "server" is a
user-facing product concept, not a useful storage category.

## Decision

Persist all non-content runtime state in one JetStream KV bucket named
`RUNTIME_STATE`.

The storage boundary is:

- If a value is needed to reconstruct Chatto content or domain state, it belongs
  in `EVT`.
- If a value is durable latest-value runtime/user/operational state, it belongs
  in `RUNTIME_STATE`.
- If a value is purely transient process state, it can remain in memory or a
  memory-backed KV bucket.
- If a value is binary/object data, it belongs in the appropriate object store.
- Administrator-managed configuration stays in the existing configuration
  buckets because it is configuration, not runtime state.
- User encryption keys stay in `ENCRYPTION_KEYS` because their backup and
  security rules are intentionally different from ordinary runtime state.

`RUNTIME_STATE` is persisted and configured for latest-value use:

- File storage.
- One version per key (`History: 1`).
- Replicated with the deployment's configured replica count.
- Compression enabled.
- Per-key TTL support enabled via a limit-marker TTL; no global TTL is applied.

Initial and planned occupants include:

- Room read cursors: `read.room.{userId}.{roomId}`.
- Thread read cursors: `read.thread.{userId}.{roomId}.{threadRootEventId}`.
- Pending notifications: `notification.{userId}.{notificationId}`, with per-key
  90-day TTL.
- Auth, verification, reset, and revocation tokens after their migration from
  token-specific buckets.
- Asset and video processing state after its migration from `SERVER_RUNTIME`.

Mention flags are not a target runtime-state model. Orange-dot behavior derives
from pending notifications instead of preserving `room_mention_status.*` as
canonical state.

## Consequences

- `EVT` remains focused on reconstructable content and domain history.
- Runtime state has one persisted operational home with uniform backup, TTL,
  and history semantics.
- The old `SERVER_RUNTIME` bucket becomes a legacy migration source, not a
  place for new state.
- Runtime values in `RUNTIME_STATE` are not replayable from `EVT`; backup and
  restore procedures must include this bucket when preserving user/runtime
  continuity matters.
- Per-key TTL becomes available for tokens, processing records, and similar
  values without splitting each feature into its own bucket.
- Security-sensitive exceptions remain explicit. In particular,
  `ENCRYPTION_KEYS` is not folded into this bucket.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md) — defines the
  event-sourced content/domain boundary.
- [ADR-028](ADR-028-event-id-keyed-read-state.md) — defines the read-cursor
  shape that now lives in `RUNTIME_STATE`.
