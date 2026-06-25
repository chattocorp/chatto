# FDR-016: Voice Calls

**Status:** Active
**Last reviewed:** 2026-06-19

## Overview

Rooms support real-time voice conversations with optional camera video and video-only screen/window/tab sharing. A phone tab in the room sidebar lets members start or join the room call; the call panel shows screen-share tiles first, then video-enabled participant cards, then compact voice-only participant cards, and provides mute, camera, screen-share, device-selection, and hang-up controls. Audio and video are routed through LiveKit (an external WebRTC service); Chatto only handles authorization, participant state, and the UI.

## Behavior

- Members of a room with the right permission see a phone tab alongside the room sidebar's members/files tabs when LiveKit is configured.
- Opening the call tab shows the current room call. If no call is active, it offers a "Start call" action. If a call is active and the viewer has not joined, it shows projected participants as ungrouped participant cards and a "Join call" action.
- When the current room has an active call, the phone tab is accent-highlighted and pulses while another sidebar tab is selected.
- Joining the call switches the call tab into participant mode with pinned screen-share tiles first, larger camera video participant cards next, and compact voice-only participant cards after that, without separate Video or Voice section headings. Participant mode exposes neutral speaking indicators, mute state, camera toggle, screen-share toggle, device selector, and hang-up controls.
- While the viewer is in any call, the lower-left current-user card shows the active call room plus quick mute, camera, screen-share, and leave controls so the call remains visible outside the room tab.
- Other rooms with an active call replace the normal room/DM icon with the same accent phone icon and animated pulse twin used by the call tab so members know there's a conversation happening; clicking that icon opens the room with the call tab selected.
- Message author names show a compact call presence icon when the author is in the current room's active call: phone for voice-only participants, video camera when the viewer has joined the LiveKit call and can see an active camera track.
- A member's join/leave updates active call indicators and participant lists, but call lifecycle and participant transitions are not shown as room timeline messages. Explicit user intent is recorded immediately, and LiveKit webhooks/reconciliation confirm or correct the active participant projection.
- Joined call participants hear fixed synthesized cues from durable participant join/leave events, including their own join/leave events and other participants in the same active call. These call cues are separate from configurable notification sounds and do not use notification sound filters; `CallEndedEvent` does not play a separate cue.
- The first join starts a call session, creates fresh per-call E2EE key material, and records durable call lifecycle facts. The final leave ends the call, records the end fact, and shreds the call key.
- Hanging up disconnects from LiveKit and clears the participant from everyone else's view.
- New clients always enable LiveKit E2EE before connecting. Chatto distributes a KMS-backed per-call shared key with the LiveKit join token; the raw key is never written to EVT and is shredded when the call ends.
- Screen sharing is video-only in Chatto's UI. Browser tab audio sharing is not published by Chatto today.
- Screen-share state is LiveKit track state only. Users who have not joined the call still see who is in the active call, but they do not see whether a participant is sharing a screen.
- When LiveKit is not configured on the server, all voice UI is hidden — no button, no panel, no indicator.

## Design Decisions

### 1. Call lifecycle and join/leave are durable room facts with internal source

**Decision:** `CallStartedEvent`, `CallParticipantJoinedEvent`, `CallParticipantLeftEvent`, and `CallEndedEvent` are persisted in the room EVT aggregate keyed by room ID, on `evt.room.{roomId}.call_started`, `evt.room.{roomId}.call_joined`, `evt.room.{roomId}.call_left`, and `evt.room.{roomId}.call_ended`. Explicit frontend join/leave writes use source `USER`; LiveKit webhook writes use source `LIVEKIT`; reconciliation writes use source `RECONCILIATION`. GraphQL exposes the same public event shape without the internal source or E2EE key ref. Call facts drive active call state, live indicators, and key lifecycle, but are hidden from normal room history.
**Why:** Calls are realtime/audit facts that should survive process restarts and be delivered through the same durable live EVT path as other room facts. Chatto's product model treats calls as always happening inside a room, with at most one active call per room. Rooms are intentionally cheap coordination spaces, so future private, temporary, or non-public calls can use short-lived rooms and inherit room membership, authorization, naming, visibility, and live-delivery behavior instead of introducing a separate call-membership model. Keeping source internal lets projections distinguish optimistic user intent from media-server observation without adding public API surface.
**Tradeoff:** Duplicate user/LiveKit/reconciliation reports are collapsed at the call-state write boundary when they do not change participant state. A real join, leave, and later rejoin still records each transition as a distinct call session. The model uses the call projection's per-room applied sequence as the OCC token against `evt.room.{roomId}.>` so lifecycle and participant transitions are guarded by the room aggregate boundary across replicas. The design deliberately favors room-scoped calls over independent call aggregates; if calls later need their own durable lifecycle beyond the room boundary, new writes may need to move to a call aggregate while replaying legacy room-scoped facts.

### 2. Active call state is projection-backed and reconciled

**Decision:** Active participant snapshots and the active call session come from a call-state model/projection over durable call facts, not from `MEMORY_CACHE`. User joins can create pending/optimistic state; LiveKit and reconciliation facts confirm or correct it. Chatto includes the active Chatto `callId` in the LiveKit room name so LiveKit webhooks and reconciliation snapshots are applied only to the matching call session. On startup and periodically, Chatto compares active LiveKit rooms/participants to the projection and appends reconciliation facts for mismatches. If LiveKit cannot list rooms/participants for three consecutive elected reconciliation cycles, Chatto ends all projected active calls with reconciliation facts; before that threshold it defers cleanup. If LiveKit reports a room in `ListRooms` but returns not-found when participants are listed, Chatto treats that room as gone/empty and continues reconciling other rooms.
**Why:** The UI needs current participant state, but it should not depend only on volatile KV state or only on historical replay. EVT gives durable audit/live delivery, while LiveKit reconciliation keeps "who is connected now" grounded in the media server.
**Tradeoff:** The projection can briefly show optimistic state before LiveKit or reconciliation corrects it. If LiveKit reports the same already-active transition, the duplicate report is skipped instead of appending another public call event. A sustained LiveKit listing outage can end active calls after the shared failure threshold, favoring eventual UI recovery and unblocking new sessions while avoiding immediate cleanup for transient API failures. Multiple replicas may reconcile concurrently; call transition facts are OCC-gated on the room aggregate and rechecked after conflicts.

### 3. Graceful degradation when LiveKit isn't configured

**Decision:** When LiveKit credentials are absent, the call APIs return null/empty and the frontend hides the entire voice UI.
**Why:** Self-hosters who don't want to run LiveKit (or haven't yet) shouldn't see dead UI affordances. Hiding the surface entirely is clearer than disabled buttons. See ADR-009.
**Tradeoff:** Operators have to know LiveKit setup exists. Documented in setup guides.

### 4. Audio tracks must be explicitly attached

**Decision:** The frontend listens for `RoomEvent.TrackSubscribed` and calls `track.attach()` to wire LiveKit audio into a hidden `<audio>` element. On leave or `TrackUnsubscribed`, it calls `track.detach()`.
**Why:** LiveKit delivers audio data over WebRTC, but the browser doesn't autoplay it without an attached element. Without explicit attach, the UI looks like everything works — participant rings even animate — but nobody hears anything. The pattern lives in `apps/frontend/src/lib/state/voiceCall.svelte.ts`; any refactor that touches LiveKit subscription handling needs to keep the `track.attach()` / `track.detach()` calls intact.
**Tradeoff:** A subtle requirement that's easy to miss when refactoring; the skill warns explicitly.

### 5. Speaking indicators use neutral inline glyphs

**Decision:** Participant cards read audio levels through the existing 60ms cache and show a neutral inline volume glyph for active speakers instead of an accent outline around the card.
**Why:** The fast audio-level cache gives responsive speaking feedback, while keeping the visual treatment quiet and avoiding the blue outline around participant and screen-share tiles.
**Tradeoff:** The indicator is intentionally more subtle than the previous animated card outline.

### 6. Screen sharing is joined-client LiveKit track state

**Decision:** Screen/window/tab sharing uses LiveKit's browser screen-share publishing path and is represented only by `Track.Source.ScreenShare` on joined clients. Chatto does not persist separate screen-share events, add GraphQL fields, or expose screen-share state to call observers before they join.
**Why:** Screen sharing is media-session state, and the existing durable room facts already answer the server-owned question of who is in the call. Keeping screen-share state inside LiveKit avoids adding durable state that can become stale when browser capture ends.
**Tradeoff:** Non-joined observers know a call is active and who is in it, but not whether someone is sharing. Browser-tab audio sharing is also out of scope for this version.

### 7. Test endpoints bypass webhook validation in build-tag mode

**Decision:** E2E tests use special `/webhooks/test/call-join` and `/webhooks/test/call-leave` endpoints that skip HMAC validation and call the core methods directly. Available only with `-tags test_endpoints`.
**Why:** Real LiveKit isn't realistic to run in CI, but webhook flow is exactly the thing E2E tests need to exercise. Build-tag gating keeps the endpoints out of production. See ADR-020.
**Tradeoff:** Two webhook entry points (real + test); test ones are well-isolated and trivially removable from prod builds.

### 8. E2EE keys are KMS-backed per-call secrets

**Decision:** `voiceCallToken` returns both `token` and `e2eeKey`. The first join for a room creates a new call ID and per-call E2EE key through Chatto's KMS boundary, stores the raw key in `ENCRYPTION_KEYS` under `call.e2ee.{callId}`, and records only the key ref in `CallStartedEvent`. The final leave records `CallEndedEvent` and shreds the key ref. The frontend creates an `ExternalE2EEKeyProvider`, configures the LiveKit E2EE worker, sets the key, enables E2EE, then connects.
**Why:** LiveKit E2EE key generation/distribution is application responsibility. Chatto already authorizes token access by room membership, so the token resolver is the narrow place to distribute the shared call key. Keeping the raw key out of EVT and normal backups avoids turning event-log copies into permanent decrypt material for captured media.
**Tradeoff:** Always-on E2EE breaks media compatibility with older clients that do not enable E2EE. Restoring a backup without `ENCRYPTION_KEYS` cannot recover active call keys; active calls should be considered interrupted across such restores.

## Permissions

- `voiceCallToken` query — requires room membership.
- `callParticipants` query — requires room membership.
- `activeCallRoomIds` query — requires server membership.
- `joinVoiceCall` / `leaveVoiceCall` mutations — require room membership.

Voice calling doesn't have a dedicated permission today; room membership is the gate.

## Related

- **ADRs:** ADR-009 (webhook-driven voice call state), ADR-012 (two-tier real-time events), ADR-020 (build-tag gated test endpoints)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-019 (Room Lifecycle)

## Open Questions

- Should there be a dedicated `voice.join` permission so operators can disable voice in specific rooms/groups without touching room membership? Currently any room member can call.
