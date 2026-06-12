# ADR-009: Webhook-Driven Voice Call State

**Date:** 2026-03-01

## Context

Chatto integrates with LiveKit for WebRTC voice calls. The system needs to track which participants are in which calls so the UI can show call indicators (headphone icons) and participant lists. The question is who reports participant state changes.

Two approaches:

- **Client-driven**: Clients send mutations (`joinCall`, `leaveCall`) when they connect or disconnect. Simple to implement but unreliable — if a client crashes, closes the tab, or loses connectivity, the leave mutation never fires and the participant appears stuck in the call.
- **Webhook-driven**: LiveKit itself notifies the server via HTTP webhooks when participants join or leave. LiveKit detects disconnections at the WebRTC transport level, so leave events fire even if the client crashes.

## Decision

Use LiveKit webhooks as the sole source of participant state changes:

- `POST /webhooks/livekit` receives HMAC-validated events from LiveKit
- `participant_joined` adds the participant to `MEMORY_CACHE` under `call.{spaceId}.{roomId}`
- `participant_left` removes them
- `room_finished` cleans up any remaining participants and deletes the KV key
- NATS live events (`CallParticipantJoined`, `CallParticipantLeft`) are published for frontend subscriptions
- The frontend does **not** publish join/leave events via mutations

## Consequences

- **Crash resilience**: If a client crashes or loses network, LiveKit detects the WebRTC disconnect and fires a `participant_left` webhook. No ghost participants.
- **Single source of truth**: The server only updates call state in response to LiveKit webhooks, never client claims. This eliminates race conditions between client-reported and server-observed state.
- **Memory-backed state is intentional**: `MEMORY_CACHE` uses `MemoryStorage` and is excluded from backups. If JetStream restarts, call state is lost — but participants will reconnect to LiveKit, triggering new join webhooks that repopulate the state. Persistence would be wasted storage. The retired `CALL_STATE` bucket is historical and is no longer imported on boot.
- **Latency**: There's a brief delay between a client connecting to LiveKit and the webhook arriving at Chatto. The user sees themselves in the call immediately (local LiveKit Room state), but other users see the join indicator after the webhook round-trip.
- **Webhook URL must be reachable**: LiveKit must be able to POST to Chatto's webhook endpoint. In development, this typically requires a tunnel or local LiveKit server.
- **Graceful degradation**: When LiveKit is not configured, all voice APIs return null/empty and the frontend hides call UI entirely.
