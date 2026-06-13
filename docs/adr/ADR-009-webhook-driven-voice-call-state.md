# ADR-009: Durable LiveKit Call State

**Date:** 2026-03-01
**Updated:** 2026-06-13

## Context

Chatto integrates with LiveKit for WebRTC voice calls. The system needs to track which participants are in which calls so the UI can show call indicators (headphone icons) and participant lists. The question is how to combine user intent, LiveKit observation, and restart/reconciliation behavior.

Earlier designs considered two approaches:

- **Client-driven**: Clients send mutations (`joinCall`, `leaveCall`) when they connect or disconnect. Simple to implement but unreliable — if a client crashes, closes the tab, or loses connectivity, the leave mutation never fires and the participant appears stuck in the call.
- **Webhook-driven**: LiveKit itself notifies the server via HTTP webhooks when participants join or leave. LiveKit detects disconnections at the WebRTC transport level, so leave events fire even if the client crashes.

After the 0.1.x EVT rollout, voice call state also needs to fit the durable room-fact model instead of using a process-local `MEMORY_CACHE` participant snapshot as the API source of truth.

## Decision

Persist voice call join/leave transitions as durable room-call EVT facts:

- `CallParticipantJoinedEvent` and `CallParticipantLeftEvent` live on the room aggregate keyed by room ID.
- The durable subjects are `evt.room.{roomId}.call_joined` and `evt.room.{roomId}.call_left`. Calls are room-scoped facts because rooms are Chatto's core primitive for places where members can communicate.
- Explicit client intent writes `USER`-sourced call facts through `joinVoiceCall` / `leaveVoiceCall`.
- `POST /webhooks/livekit` receives HMAC-validated LiveKit events and writes matching `LIVEKIT`-sourced facts.
- A call-state service/projection consumes durable call facts and serves `activeCallRoomIds` / `callParticipants`.
- The call-state service writes join/leave transition facts idempotently per participant state. Duplicate reports from user intent, LiveKit, or reconciliation are skipped after projection catch-up; a real join/leave/join sequence still appends every state-changing transition.
- On startup and periodically, the call-state service compares projected state to LiveKit's current room/participant state and appends `RECONCILIATION` facts for mismatches. Call transition appends use the call projection's per-room applied sequence as the OCC token against `evt.room.{roomId}.>`; on conflict, the service retries from a fresh projection snapshot and skips the append if another replica already applied the transition.
- Call join/leave EVT facts are delivered through the durable live EVT subscription path, but they are hidden from normal visible room timelines.
- `MEMORY_CACHE` is still acceptable for volatile secrets such as LiveKit E2EE keys; it is no longer the active participant snapshot source.

## Consequences

- **Crash resilience**: If a client crashes or loses network, LiveKit detects the WebRTC disconnect and fires a `participant_left` webhook. No ghost participants.
- **Auditability**: State-changing user intent and LiveKit-observed transitions are durable EVT facts. This makes call lifecycle delivery replayable and inspectable without exposing the internal source enum publicly, while avoiding duplicate facts for the same active-state transition.
- **Projection source of truth**: Active call reads come from a projection/service. The projection may show optimistic `USER` state briefly, then LiveKit or reconciliation facts confirm or remove it.
- **Reconciliation**: A process restart no longer loses the local active participant snapshot permanently; the service queries LiveKit and appends correction facts for rooms/participants that differ from the projection. The room aggregate OCC boundary lets multiple replicas reconcile without a leader lease while avoiding duplicate transition facts after OCC conflicts.
- **Latency**: Remote observers can see user intent before LiveKit webhook confirmation. Incorrect optimistic state is corrected by LiveKit leave events or reconciliation.
- **Webhook URL must be reachable**: LiveKit must be able to POST to Chatto's webhook endpoint. In development, this typically requires a tunnel or local LiveKit server.
- **Graceful degradation**: When LiveKit is not configured, all voice APIs return null/empty and the frontend hides call UI entirely.
- **E2EE compatibility**: New clients always enable LiveKit E2EE using a per-room shared key returned with `voiceCallToken`. Older clients without E2EE will not decode encrypted media.
