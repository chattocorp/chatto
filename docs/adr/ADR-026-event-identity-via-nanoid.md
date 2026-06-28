# ADR-026: Event Identity via NanoID, Not JetStream Sequence Numbers

**Date:** 2026-03-26

## Context

Events in Chatto were historically identified by two values: a NanoID (`event.id`) and a JetStream stream sequence number (`sequence_id`). The sequence number was populated at read time from JetStream metadata and exposed through the then-current public API as `SpaceEvent.sequenceId`.

This created several problems:

- **Leaky abstraction**: JetStream sequence numbers are a storage transport detail. Exposing them in the API coupled clients to the underlying infrastructure. If we ever migrated to a different event store, every client would break.
- **Fragile under backup/restore**: JetStream sequence numbers may not survive backup/restore cycles, making them unreliable as stable identifiers.
- **Dual identity**: Having two identifiers for the same entity (NanoID and sequence number) created confusion about which to use. Reactions, threads, and replies already used NanoID event IDs — but unread tracking and single-event fetches used sequence numbers.
- **Enumerable**: Monotonically increasing integers let clients guess or enumerate message IDs, which is undesirable.

## Decision

Remove `sequence_id` from the event model entirely. Events are identified exclusively by their NanoID (`id` field) in all APIs and client-facing contexts.

Specific changes:

- **Proto**: Reserved field 9001 (`sequence_id`) on `SpaceEvent`. The field is no longer populated or read.
- **Public API**: Removed the exposed `sequenceId` field and sequence-number lookup. Single-event lookup uses stable event IDs with O(1) subject-based JetStream lookup.
- **Unread tracking**: `markRoomAsRead` returns nanosecond-precision JetStream timestamps (`lastReadAt`, `previousLastReadAt`) instead of sequence numbers. The frontend compares `event.createdAt` against these timestamps to place the unread separator.
- **Event sorting**: The frontend sorts events by `createdAt` with NanoID string comparison as tiebreaker, instead of `parseInt(sequenceId)`.
- **Internal use**: JetStream sequence numbers are still used internally for stream operations (consumer start positions, `GetMsg` lookups) via `GetEventSequence()` and `GetSequenceTimestamp()`, but never exposed to clients or stored on the event model.
- **Publish idempotency**: Durable event publishes set the JetStream `Nats-Msg-Id` header to `event.id`, so retrying the same event envelope within the stream duplicate window returns the original ack instead of appending a second fact.

## Consequences

- **Cleaner API contract**: Clients interact only with stable NanoIDs and timestamps. No infrastructure details leak through.
- **Infrastructure flexibility**: The event store could be swapped without changing the API contract. Sequence numbers are confined to the core package.
- **Operational idempotency**: `event.id` is now the event's retry/deduplication key as well as its API identity. Durable event writers must populate it before publishing.
- **Simpler frontend**: One identifier (`event.id`) for everything — dedup keys, refetch targets, virtual list keys, E2E test locators.
- **Timestamp precision matters**: Unread tracking now depends on `createdAt` timestamp ordering. JetStream timestamps have nanosecond precision (via `RFC3339Nano`), which is more than sufficient. Sub-millisecond concurrent posts could theoretically share a timestamp, but the NanoID tiebreaker handles this.
- **Two extra JetStream lookups on `markRoomAsRead`**: The resolver fetches timestamps for the last-read and previous-last-read sequence numbers via `GetSequenceTimestamp()`. This adds two `GetMsg` calls per mark-as-read operation — negligible given this is a low-frequency user action.
