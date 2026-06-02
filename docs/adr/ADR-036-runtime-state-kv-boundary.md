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
  and legacy video processing state.
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
  memory-backed KV bucket. Shared volatile cache state that must be visible
  across Chatto processes belongs in `MEMORY_CACHE`.
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

Current occupants include:

- Room read cursors: `read.room.{userId}.{roomId}`.
- Thread read cursors: `read.thread.{userId}.{roomId}.{threadRootEventId}`.
- Pending notifications: `notification.{userId}.{notificationId}`, with per-key
  90-day TTL.
- Web Push subscriptions: `push_subscription.{userId}.{endpointHash}`.
- Bearer auth token verifiers: `session.{hmac}`, with per-key
  `auth.token_ttl` sliding-window expiry.
- OAuth authorization-code verifiers: `grant.{hmac}`, with per-key 5-minute
  TTL.
- Account workflow token verifiers: `registration.{hmac}`,
  `email_verification.{hmac}`, `password_reset.{hmac}`, and
  `account_deletion_token.{hmac}`, with per-key TTLs appropriate to each
  workflow.
- Link-preview cache entries: `link_preview.{urlHash}`, with per-key 24-hour
  TTL for successful previews and 1-hour TTL for failed fetches.

The HMAC keys for bearer tokens, OAuth codes, and account workflow tokens are
derived with `[core].secret_key` from the raw token/code plus a per-flow scope
string. `RUNTIME_STATE` is included in backups, so active sessions and pending
flows survive restore when the same secret is used; restoring with a different
secret intentionally invalidates those credentials. Backup archives do not
contain raw bearer tokens, links, or OAuth codes.

Attachment declarations and video derivative manifests are not a `RUNTIME_STATE`
target. Uploaded assets are content and are declared with `AssetCreatedEvent`;
generated video thumbnails and variants are content metadata, so completed/failed
outcomes live in durable room EVT events that reference the created asset ID.
The current video processor does not write new runtime progress or claim state;
legacy `SERVER_RUNTIME video.*` records are only a migration source.

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
- Runtime credential records can be backed up without storing redeemable raw
  tokens, because their keys are HMAC-derived from `[core].secret_key`.
- Per-key TTL becomes available for tokens and similar runtime values without
  splitting each feature into its own bucket.
- Security-sensitive exceptions remain explicit. In particular,
  `ENCRYPTION_KEYS` is not folded into this bucket.
- `MEMORY_CACHE` is the companion non-durable bucket for cross-process
  volatile state. It uses memory storage, one version per key, the deployment's
  replica count, no global TTL, and a limit-marker TTL for expiring presence
  sessions. It is excluded from backups and is expected to clear on a full
  JetStream restart. Current occupants are user presence records
  `presence.{userId}` with per-key TTL and active voice calls
  `call.{spaceId}.{roomId}`. The retired `USER_PRESENCE` and `CALL_STATE`
  buckets are legacy import sources only; fresh boots do not provision them.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md) — defines the
  event-sourced content/domain boundary.
- [ADR-028](ADR-028-event-id-keyed-read-state.md) — defines the read-cursor
  shape that now lives in `RUNTIME_STATE`.
