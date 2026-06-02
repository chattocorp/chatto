# FDR-016: Voice Calls

**Status:** Active
**Last reviewed:** 2026-06-01

## Overview

Rooms support real-time voice conversations. A small phone icon in the room header lets members join the call; a panel shows participants with mute, device selection, and hang-up controls. Audio is routed through LiveKit (an external WebRTC service); Chatto only handles authorization, participant state, and the UI.

## Behavior

- Members of a room with the right permission see a "Join call" button in the room header.
- Joining the call opens a panel showing each participant's avatar (with a speaking-indicator ring), a mute toggle, an audio device selector, and a hang-up button.
- Other rooms with an active call show a small headphone icon in the sidebar so members know there's a conversation happening.
- A member's join/leave is visible to the others in the call within roughly a round-trip — the local participant sees themselves in the call instantly, but others see the indicator after the LiveKit webhook reaches Chatto.
- Hanging up disconnects from LiveKit and clears the participant from everyone else's view.
- When LiveKit is not configured on the server, all voice UI is hidden — no button, no panel, no indicator.

## Design Decisions

### 1. LiveKit webhooks are the sole source of participant state changes

**Decision:** Participant join/leave events come from LiveKit's webhooks, never from client-side mutations. Chatto's `MEMORY_CACHE` call state (`call.{spaceId}.{roomId}`) is updated only in response to a `participant_joined`, `participant_left`, or `room_finished` webhook.
**Why:** Client-driven join/leave is brittle — a crashed tab or lost network never sends the leave mutation, and the participant looks stuck in the call forever. LiveKit detects WebRTC transport-level disconnects and fires `participant_left` whether the client cooperated or not. See ADR-009.
**Tradeoff:** Joining is slightly delayed for remote observers (the webhook has to round-trip from LiveKit to Chatto before others see you). Acceptable in exchange for never-stuck participants. The local user sees themselves immediately because the LiveKit Room object exposes local state without the webhook.

### 2. Call state is memory-backed, deliberately ephemeral

**Decision:** Call state lives in the memory-backed, non-backed-up `MEMORY_CACHE` bucket. If JetStream restarts, the call state vanishes. The retired `CALL_STATE` bucket is copied into `MEMORY_CACHE` on boot when present so active calls survive the storage rename during upgrade.
**Why:** Call state is "who's connected right now". After a restart, the source of truth is LiveKit — and as participants reconnect, the webhooks repopulate the state automatically. Persisting it would waste storage and risk showing a stale "who's in the call" list across restarts.
**Tradeoff:** A brief window after restart where the API reports no active calls until participants reconnect and webhooks land. Acceptable for an ephemeral concept.

### 3. Graceful degradation when LiveKit isn't configured

**Decision:** When LiveKit credentials are absent, the call APIs return null/empty and the frontend hides the entire voice UI.
**Why:** Self-hosters who don't want to run LiveKit (or haven't yet) shouldn't see dead UI affordances. Hiding the surface entirely is clearer than disabled buttons. See ADR-009.
**Tradeoff:** Operators have to know LiveKit setup exists. Documented in setup guides.

### 4. Audio tracks must be explicitly attached

**Decision:** The frontend listens for `RoomEvent.TrackSubscribed` and calls `track.attach()` to wire LiveKit audio into a hidden `<audio>` element. On leave or `TrackUnsubscribed`, it calls `track.detach()`.
**Why:** LiveKit delivers audio data over WebRTC, but the browser doesn't autoplay it without an attached element. Without explicit attach, the UI looks like everything works — participant rings even animate — but nobody hears anything. The pattern lives in `frontend/src/lib/state/voiceCall.svelte.ts`; any refactor that touches LiveKit subscription handling needs to keep the `track.attach()` / `track.detach()` calls intact.
**Tradeoff:** A subtle requirement that's easy to miss when refactoring; the skill warns explicitly.

### 5. Audio levels poll at ~60ms instead of using ActiveSpeakersChanged

**Decision:** Speaking indicators (avatar rings) read audio levels via a 60ms `setInterval` poll instead of relying on LiveKit's `ActiveSpeakersChanged` event.
**Why:** `ActiveSpeakersChanged` fires roughly every 100ms — fast enough for "who's talking" but visibly choppy for animated speaker rings. The 60ms poll feels smoother.
**Tradeoff:** A small recurring poll cost. Worth it for the visual quality.

### 6. Test endpoints bypass webhook validation in build-tag mode

**Decision:** E2E tests use special `/webhooks/test/call-join` and `/webhooks/test/call-leave` endpoints that skip HMAC validation and call the core methods directly. Available only with `-tags test_endpoints`.
**Why:** Real LiveKit isn't realistic to run in CI, but webhook flow is exactly the thing E2E tests need to exercise. Build-tag gating keeps the endpoints out of production. See ADR-020.
**Tradeoff:** Two webhook entry points (real + test); test ones are well-isolated and trivially removable from prod builds.

## Permissions

- `voiceCallToken` query — requires room membership.
- `callParticipants` query — requires room membership.
- `activeCallRoomIds` query — requires server membership.

Voice calling doesn't have a dedicated permission today; room membership is the gate.

## Related

- **ADRs:** ADR-009 (webhook-driven voice call state), ADR-012 (two-tier real-time events), ADR-020 (build-tag gated test endpoints)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-019 (Room Lifecycle)

## Open Questions

- Should there be a dedicated `voice.join` permission so operators can disable voice in specific rooms/groups without touching room membership? Currently any room member can call.
