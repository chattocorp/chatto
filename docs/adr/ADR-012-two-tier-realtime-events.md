# ADR-012: Two-Tier Real-Time Event System

**Date:** 2026-03-01

**Naming note:** This ADR refers to `space.{id}.>` and `live.space.{id}.>` subject patterns and the `StreamMySpaceEvents` fan-in function. After ADR-029 (Instance -> Server rename), ADR-030 (Space tier retired), ADR-034 (EVT), and ADR-042 (protobuf-first public API), the live equivalents are `live.evt.>` for republished durable EVT facts, `live.sync.>` for transient `LiveEvent` signals, and realtime websocket delivery for the public app-session stream. `SERVER_EVENTS` no longer republishes to a live subject. The two-tier split itself (durable JetStream vs. transient NATS Core) and the per-event-type channel decision are unchanged.

**Update:** Reactions moved from the original live-only examples into durable
room facts during the event-sourcing rollout. `ReactionAddedEvent` and
`ReactionRemovedEvent` now live in `EVT` and reach clients through
`live.evt.>` after projection readiness and authorization checks. See FDR-005,
ADR-033, and ADR-034.

## Context

Chatto's real-time events span a wide spectrum of persistence and frequency requirements. Messages, joins, room lifecycle events, reactions, and other durable room facts must be durably stored and replayable. Typing indicators, presence changes, and notification sync signals are ephemeral - they matter for the current moment but have no audit or replay value.

Publishing all events to JetStream would waste storage on high-frequency transient signals. Using only bare NATS pub/sub would lose ordering guarantees and replay for messages.

## Decision

Split events into two channels based on persistence:

1. **JetStream events** (messages, joins, leaves, room lifecycle, reactions): Originally published to `space.{id}.>` subjects on a persisted per-space stream; currently published as durable `EVT` facts and exposed internally through `live.evt.>`.
2. **Live-only events** (typing indicators, presence, notification sync, session/user/config invalidations): Originally published to `live.space.{id}.>` subjects via bare NATS Core pub/sub; currently published as `corev1.LiveEvent` messages under `live.sync.>`. Not stored. Consumed via plain NATS subscriptions.

The realtime delivery layer merges durable and transient live channels, then maps authorized events into public protobuf live events for connected clients.

## Consequences

- **Efficient storage**: High-frequency transient events don't accumulate in JetStream streams. A busy space with constant typing indicators doesn't bloat its event stream.
- **Appropriate delivery guarantees**: Messages get ordered, durable delivery. Typing indicators get fire-and-forget delivery, which is correct — a missed typing indicator is harmless.
- **Fan-in complexity**: The realtime delivery layer merges durable committed facts and transient sync signals into one authorized stream. The current `StreamMyEvents` path consumes `live.evt.>` and `live.sync.>` and maps both to public realtime protobuf frames.
- **Delivery mapping must stay explicit**: Every new live event type must be registered in the current `StreamMyEvents` / realtime mapping path so delivery can extract its authorization scope and public protobuf shape. Missing mappings can still hide otherwise-valid events from clients, so live-event changes need tests at the delivery boundary.
- **New event types require a channel decision**: When adding a new event type, developers must decide whether it belongs in JetStream (persistent, ordered) or NATS Core (ephemeral, best-effort). This is an explicit architectural choice, not a default.
