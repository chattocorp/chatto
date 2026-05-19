# ADR-012: Two-Tier Real-Time Event System

**Date:** 2026-03-01

**Naming note:** This ADR refers to `space.{id}.>` and `live.space.{id}.>` subject patterns and the `StreamMySpaceEvents` fan-in function. After ADR-029 (Instance → Server rename) and ADR-030 (Space tier retired), the equivalents are `server.>` / `live.server.>` and `StreamMyEvents`, and the `SERVER_EVENTS` stream's `RePublish` config now forwards every accepted message onto `live.server.>` so a subscriber needs only one NATS Core sub to receive both durable and transient events. The two-tier split itself (durable JetStream vs. transient NATS Core) and the per-event-type channel decision are unchanged.

## Context

Chatto's real-time events span a wide spectrum of persistence and frequency requirements. Messages, joins, and room lifecycle events must be durably stored and replayable. Typing indicators, reactions, presence changes, and message update notifications are ephemeral — they matter for the current moment but have no audit or replay value.

Publishing all events to JetStream would waste storage on high-frequency transient signals. Using only bare NATS pub/sub would lose ordering guarantees and replay for messages.

## Decision

Split events into two channels based on persistence:

1. **JetStream events** (messages, joins, leaves, room lifecycle): Published to `space.{id}.>` subjects on a persisted per-space stream. Consumed via ordered JetStream consumers with replay support.
2. **Live-only events** (reactions, typing indicators, presence, message updates/deletes): Published to `live.space.{id}.>` subjects via bare NATS Core pub/sub. Not stored. Consumed via plain NATS subscriptions.

The `StreamMySpaceEvents` function in core is the central fan-in point that merges both channels (plus a KV presence watcher) into a single Go channel for the GraphQL subscription.

## Consequences

- **Efficient storage**: High-frequency transient events don't accumulate in JetStream streams. A busy space with constant typing indicators doesn't bloat its event stream.
- **Appropriate delivery guarantees**: Messages get ordered, durable delivery. Typing indicators get fire-and-forget delivery, which is correct — a missed typing indicator is harmless.
- **Fan-in complexity**: `StreamMySpaceEvents` must merge three async sources (JetStream consumer, two NATS Core subscriptions) into one output channel. This is the most complex goroutine in the codebase.
- **Silent drops on missing registration**: Every new event type must be added to the type switch in `StreamMySpaceEvents` that extracts the room ID. If it's missing, the event is silently dropped with no error log. This is a known footgun documented in the live-events rules.
- **New event types require a channel decision**: When adding a new event type, developers must decide whether it belongs in JetStream (persistent, ordered) or NATS Core (ephemeral, best-effort). This is an explicit architectural choice, not a default.
