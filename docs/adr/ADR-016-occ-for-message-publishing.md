# ADR-016: Optimistic Concurrency Control for Message Publishing

**Date:** 2026-03-01

## Context

When multiple users post messages simultaneously in the same room (or the same thread), the events must be ordered consistently. JetStream assigns sequence numbers to stream entries, but concurrent publishes to the same subject can race. Without coordination, two messages could claim the same logical position.

The options are:

- **Distributed locking**: Acquire a per-room lock before publishing. Guarantees ordering but adds latency and requires lock management (TTLs, deadlock detection).
- **At-least-once without coordination**: Publish freely and accept potential duplicates. Simpler but risks duplicate events for the same logical operation.
- **Optimistic concurrency control (OCC)**: Use JetStream's `ExpectLastSequencePerSubject` header to detect concurrent writes. Retry on conflict.

## Decision

Publish each message body and its `MessagePostedEvent` in one atomic JetStream
batch. The first entry checks the last sequence matching the room aggregate's
`evt.room.{roomId}.>` filter. A concurrent room fact therefore rejects the
whole batch rather than allowing the body and public message fact to split or
land against stale room state.

User-facing message posts also evaluate authorization inside every OCC
attempt. Before the check, the writer:

1. captures the existing server authorization-fence sequence;
2. captures and waits for the room directory, room-group layout, RBAC, and
   posting user's projections through their relevant EVT tails;
3. reruns the complete membership, archive, and permission decision; and
4. attaches the captured authorization-fence expectation to a second entry in
   the same message batch.

Authorization-changing RBAC, room-group, and relevant user writes advance the
singleton fence atomically with their domain facts. Message posts only check
that fence; they do not advance it. A concurrent authority change rejects the
message batch and causes the projections and authorization decision to be
refreshed on retry.

Retry OCC conflicts up to five times with bounded exponential backoff. Reuse
the same event IDs and payloads across attempts so JetStream message
deduplication remains effective.

## Consequences

- **No distributed locks**: Message publishing doesn't require per-room locks, lock servers, or lock TTL management. This simplifies multi-process deployments.
- **Correct ordering under contention**: Concurrent facts in one room are serialized by the room-filter check. One batch succeeds; the other retries from the updated room tail.
- **Commit-time authorization**: Membership removal, room archival, RBAC changes, room moves, and effective-owner input changes cannot race an already-completed preflight and leave an unauthorized message behind.
- **No global message serialization**: Successful posts check but do not advance the authorization fence, so ordinary posts in unrelated rooms do not conflict through that lane.
- **Broader same-room contention**: The room guard intentionally covers every room fact, not only `message_posted`; busy rooms can cause retries when membership, messages, reactions, calls, or other room events race.
- **Bounded retries**: If contention exhausts five attempts, the publish fails instead of weakening either guard.
- **Used beyond messages**: The same filter-scoped OCC mechanism protects room layout, server configuration, RBAC, and other ordered event-sourced mutations.
