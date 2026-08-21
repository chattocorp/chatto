# ADR-036: Persist Runtime State in RUNTIME_STATE

**Date:** 2026-05-27

**Updated:** 2026-08-22

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
- KMS KEK material stays in `ENCRYPTION_KEYS` because its backup and security
  rules are intentionally different from ordinary runtime state. App-owned
  wrapped DEK records live in `RUNTIME_STATE`; they are backed up with the
  encrypted data they describe, but are unusable without the excluded KEKs.

`RUNTIME_STATE` is persisted and configured for latest-value use:

- File storage.
- One version per key (`History: 1`).
- Replicated with the deployment's configured replica count.
- Compression enabled.
- Per-key TTL support enabled via a limit-marker TTL; no global TTL is applied.

Current occupants include:

- Room read cursors: `read.room.{userId}.{roomId}`.
- Thread read cursors: `read.thread.{userId}.{roomId}.{threadRootEventId}`.
- Bounded notification read-boundary and visibility-boundary records used to
  coordinate read reconciliation and persistent privacy boundaries between
  `EVT` and the separate `NOTIFICATIONS` event stream. Notification derivation
  work, occurrences, and lifecycle facts do not live in `RUNTIME_STATE`; see
  ADR-076.
- Web Push subscriptions: `push_subscription.{userId}.{endpointHash}`.
- Runtime credential verifiers: `session.{hmac}`. Cookie-presentation records
  keep `auth.token_ttl` sliding expiry. Human bearer access records instead use
  fixed `auth.access_token_ttl` expiry and point to a stable
  `renewable_session.{hmac}` authority whose TTL is the non-renewable remaining
  `auth.token_ttl` maximum. Values include credential kind
  (`first_party_session` or `oauth_access_token`), presentation (`bearer` or
  `cookie`), source, safe request metadata, fresh-auth metadata, and the user
  auth generation they were issued against. User-wide cleanup scans these
  records and deletes entries whose stored user ID matches.
- Renewable human bearer-session authorities: `renewable_session.{hmac}` with
  user/client binding, absolute expiry, auth generation, current refresh
  generation, last refresh request ID/time, and fresh-auth metadata. Rotation
  uses KV revision OCC across replicas. The raw refresh credential is never
  stored; deleting this key invalidates every access generation.
- OAuth authorization-code verifiers: `grant.{hmac}`, with per-key 5-minute
  TTL. Values include the user auth generation they were issued against.
- Account workflow credential verifiers: `email_otp.{hmac(subject)}.{hmac(code)}`,
  `email_otp.{hmac(subject)}.challenge`, `registration_completion.{hmac}`,
  `password_reset.{hmac}`, and `account_deletion_token.{hmac}`, with per-key
  TTLs appropriate to each workflow.
- Link-preview cache entries: `link_preview.{urlHash}`, with per-key 24-hour
  TTL for successful previews and 1-hour TTL for failed fetches.
- App-owned wrapped DEK records: `dek.{contentKeyRef}`, one protobuf
  `UserDataEncryptionKey` per purpose-scoped user DEK epoch. These records have
  no TTL and are shredded on account deletion.

`ReadStateModel` mirrors the room and thread cursor keyspaces into memory with
one filtered watcher per Chatto process. The watcher supplies both its initial
latest-value snapshot and ongoing changes; core startup does not complete until
that snapshot is applied. KV remains authoritative, writes retain revision OCC,
and write paths wait for the watcher to observe their successful revision when
they require local read-your-writes.

`NotificationBoundaryIndex` applies the same process-wide filtered-watcher
pattern to notification read and visibility boundaries. Core startup waits for
its initial snapshot. Writes retain KV revision OCC and wait for the successful
revision to reach the index when local read-your-writes matters. One owning
model therefore serves all requests and background repair without creating a
watcher per request, user, or WebSocket.

The HMAC keys for runtime credential handles, renewable sessions, OAuth codes,
and account workflow tokens are derived with `[core].secret_key` from opaque
credential material plus a per-flow scope string. `RUNTIME_STATE` is included
in backups, so active sessions and pending flows survive restore when the same
secret is used; restoring with a different secret intentionally invalidates
those credentials. Backup archives do not contain raw cookie credential
handles, bearer access or refresh credentials, links, or OAuth codes.

Attachment declarations and video derivative manifests are not a `RUNTIME_STATE`
target. Uploaded assets are content and are declared with `AssetCreatedEvent`;
generated video thumbnails and variants are content metadata, so completed/failed
outcomes live in durable room EVT events that reference the created asset ID.
The current video processor does not write new runtime progress or claim state;
legacy `SERVER_RUNTIME video.*` records are historical pre-0.1 state and are
not written by current code.

Mention flags are not a target runtime-state model. Attention indicators derive
from exact unread notification occurrences instead of preserving
`room_mention_status.*` as canonical state.

## Consequences

- `EVT` remains focused on reconstructable content and domain history.
- Runtime state has one persisted operational home with uniform backup, TTL,
  and history semantics.
- Hot read-state assembly avoids one KV round trip per room or thread, at the
  cost of process memory proportional to the persisted marker count and one
  process-wide filtered watcher per replica.
- Notification read and visibility reconciliation gets the same bounded,
  process-wide watcher and startup/read-your-writes guarantees.
- The old `SERVER_RUNTIME` bucket is historical pre-0.1 storage, not a place
  for new state.
- Runtime values in `RUNTIME_STATE` are not replayable from `EVT`; backup and
  restore procedures must include this bucket when preserving user/runtime
  continuity matters.
- Runtime credential records can be backed up without storing redeemable raw
  tokens, because their keys are HMAC-derived from `[core].secret_key`.
- Per-key TTL becomes available for tokens and similar runtime values without
  splitting each feature into its own bucket.
- Security-sensitive exceptions remain explicit. In particular, KMS KEKs in
  `ENCRYPTION_KEYS` are not folded into this bucket; only app-owned wrapped DEK
  records live in `RUNTIME_STATE`.
- `MEMORY_CACHE` is the companion non-durable bucket for cross-process
  volatile state. It uses memory storage, one version per key, the deployment's
  replica count, no global TTL, and a limit-marker TTL for expiring presence
  sessions. It is excluded from backups and is expected to clear on a full
  JetStream restart. Current occupants are user presence records
  `presence.{userId}` with per-key TTL. Active voice call participants are
  projection-backed durable room facts, not `MEMORY_CACHE` state. LiveKit E2EE
  call keys are stored behind the KMS boundary in `ENCRYPTION_KEYS` under
  `call.e2ee.{callId}` refs and shredded when the corresponding call ends. The
  retired `USER_PRESENCE` and `CALL_STATE` buckets are historical pre-0.1
  shapes; fresh boots do not provision or import them.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md) — defines the
  event-sourced content/domain boundary.
- [ADR-028](ADR-028-event-id-keyed-read-state.md) — defines the read-cursor
  shape that now lives in `RUNTIME_STATE`.
- [ADR-076](ADR-076-deterministic-notification-occurrences.md) — defines the
  bounded notification log and its cross-log runtime boundaries.
