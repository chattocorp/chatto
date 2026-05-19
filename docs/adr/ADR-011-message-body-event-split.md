# ADR-011: Message Body / Event Split

**Date:** 2026-03-01

**Naming note:** This ADR refers to per-space buckets `SPACE_{id}_EVENTS` and `SPACE_{id}_BODIES`. Those were consolidated into the unified `SERVER_EVENTS` stream and `SERVER_BODIES` KV bucket by ADR-030 (Retire the Space tier); the body key is now `{userId}.{eventId}` directly in `SERVER_BODIES`. The body/event split decision itself — write-body-then-publish-event, mutable KV body keyed by event ID, GDPR shredding via per-user prefix — still holds.

## Context

Chat messages have two fundamentally different lifecycles: the *event* (who posted, when, in which room, in reply to what) is immutable, but the *content* (body text, attachments, link previews) is mutable — users can edit and delete messages. JetStream streams are append-only logs; updating a message in-place would require rewriting the stream.

Additionally, Chatto supports thread-reply echo ("also send to channel"), where the same message body appears in both the thread and the room timeline as two distinct events. And GDPR crypto-shredding needs to efficiently destroy all of a user's message content without touching the event stream.

## Decision

Split message storage into two layers:

- **Immutable event records** in the per-space JetStream stream (`SPACE_{id}_EVENTS`). These contain metadata (author, timestamp, room, thread, reply references) and a `messageBodyId` reference — but not the body text itself.
- **Mutable message bodies** in a per-space KV bucket (`SPACE_{id}_BODIES`), keyed by `{userId}.{bodyId}`. Edits update the KV entry; deletes remove it.

The body is always written *before* the event is published. If the event publish fails, the orphaned body is harmless. If the body write fails, the event is never published.

## Consequences

- **Edits and deletes are simple KV operations**: No stream rewriting, no tombstone events needed for the core data model.
- **Thread-reply echo shares a body**: The echo event and the original event reference the same `messageBodyId`. Editing the body is instantly reflected in both locations.
- **GDPR crypto-shredding works by user prefix**: All bodies for a user live under `{userId}.*` in the KV bucket. Deleting a user's encryption key makes all their content unreadable without touching the event stream. Prefix-based key listing enables bulk operations.
- **Two reads to display a message**: Field resolvers must fetch the body from KV when resolving message content. This is a tradeoff for mutability.
- **Body-before-event ordering is a consistency choice**: The event is the "commit point" — if it's published, the body is guaranteed to exist. The reverse would risk events referencing bodies that were never written.
