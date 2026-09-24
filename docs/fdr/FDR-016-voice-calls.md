# FDR-016: Voice Calls

**Status:** Active
**Last reviewed:** 2026-09-24

## Overview

Rooms support real-time voice conversations with optional camera video and screen/window/tab sharing. Supported browsers can include audio from a shared browser tab. A phone tab in the room sidebar lets members start or join the room call; the call panel shows screen-share tiles first, then video-enabled participant cards, then compact voice-only participant cards, and provides mute, camera, screen-share, device-selection, and hang-up controls. Audio and video are routed through LiveKit (an external WebRTC service); Chatto only handles authorization, participant state, and the UI.

## Behavior

- **Connection quality** comes from LiveKit, independently of microphone activity.
  Participant cards and the current-user card show an amber network icon for a
  poor connection and a red disconnected icon for a lost connection. Hover or
  click the icon to read its status. Healthy and unknown connections show no
  warning. Recovery removes the warning and closes its explanation.

- Open a participant's user context menu with the three-dot button or a
  right-click on their card. Touch users can long-press the card. For remote
  participants in the active call, participant and camera card menus include
  **Voice volume** alongside profile actions. This control is also available
  from the members list. Screen-share card menus show **Stream volume** instead.
  Adjust each independently from 0% to 200%. 100% is the original
  level. A perceptual curve maps 50% to -10 dB and 200% to +10 dB;
  percentages approximate relative loudness, not signal amplitude. Both sources
  use the same curve, including saved slider positions. These settings change
  only what this listener hears. The browser saves
  levels per server and user for later calls. Native game-share audio uses the
  sharing user's stream level.
- **Mute locally** silences both sources without changing saved levels.
  Sliders move in 5% steps and mark 100% as the original level.
  Boost needs Web Audio; if it is unavailable, playback is limited to 100%
  and the controls explain the limit. Saved boosted levels are retained.
- Microphone activity appears as a glow on the participant card. Quiet input
  does not show a warning because normal pauses do not indicate a microphone
  fault. The call toolbar's gear opens Voice & video preferences for an input
  level check and a microphone test.
- Calls use the system output when the browser cannot select a Web Audio
  output device. If the browser blocks playback, **Enable call audio** resumes it.

- **Voice Boosting** in Voice & video settings is a checkbox that is enabled by default
  for new and existing users. It adds warmth, clarity, and loudness with EQ and
  moderate compression. Users can turn it off if it causes audio problems.
  Previous slider and preset choices do not disable the new default. An explicit
  opt-out with the checkbox is saved.
  Saturation is excluded because the combined effects must not distort ordinary
  speech. Automatic plosive control reduces brief bass thumps, and adaptive bass
  control reduces sustained boom while preserving vocal warmth. These cuts run
  before compression so excessive bass is less likely to lower the whole voice.
  Automatic de-essing reduces sharp S and SH sounds, and a final limiter catches
  near-clipping sample peaks. Automatic corrections stay conservative.
  An enabled gate gets a softer closing transition to preserve quiet word
  endings; Off still disables the gate. These additions need no separate controls
  and are bypassed when Voice Boosting is off. Changes use short ramps to avoid
  clicks. The checkbox is disabled when processing is unavailable.
  The noise gate stays separate because its threshold depends on the microphone
  and room. Voice Boosting affects microphone audio in calls and the local test,
  not received audio or screen sharing. Its state is saved per browser and server
  without changing gate, device, or join-muted choices.
  Compression receives the boosted EQ signal to reduce loud peaks and uses the
  browser compressor's built-in makeup gain. A small additional gain after
  compression increases loudness before the final peak limiter. Browser automatic gain control is disabled
  to avoid competing volume adjustments. Calls retain browser echo cancellation
  and noise suppression. The local test disables echo cancellation so it does
  not cancel its own playback.
- **Noise gate threshold** is available beside a live microphone meter in call
  devices and **App Preferences → Voice & video**. It defaults to **Off**
  and is saved per browser and server. Higher thresholds suppress quieter
  sounds. The meter shows input before the gate, on the same scale as its
  threshold marker. Muted calls show no input activity.
- Sensitivity changes affect outgoing microphone audio and the local test.
  The gate uses a short attack, a hold period, a lower closing threshold,
  and a slower release to limit abrupt changes and repeated opening near
  the threshold. It does not change participant mute state. Basic audio
  remains available if the browser cannot run the optional gate; the UI
  shows that microphone processing is unavailable. This does not add a media service
  or change room permissions.

- **App Preferences → Voice & video** stores microphone, speaker, camera, and
  join-muted choices in this browser for the selected server. Successful device
  changes during a call update those choices. Missing devices use a system
  default without erasing the saved choice. The next call can use a device
  that returns.
  Changes from this preferences page also switch the connected call. An explicit
  microphone uses an exact capture constraint; only a missing device permits
  fallback.
- Camera selection does not start video. Joining muted does not request
  microphone access merely to list devices. Browser support controls whether
  a speaker can be selected.
- Device choices use compact dropdowns so long device lists do not expand the
  settings page. The controls follow the app's depth preference where browser
  support permits. A local microphone test shows an input meter and records a
  sample without playing live audio. Stop or the 10-second limit ends capture
  before replay starts through the selected speaker. The sample stays in browser
  memory until the next test, microphone change, or navigation. Users can replay
  it. The test does not connect to LiveKit or verify network connectivity.
  Testing is unavailable during the selected server's call. Output changes
  keep capture active. A failed explicit output selection stops the test instead
  of silently playing through another device. Input changes restart an active
  test. Camera choices do not interrupt it. Navigation discards the sample,
  and late capture results are stopped. With Voice Boosting off and the gate
  Off, the test records the original capture stream. It disables browser noise
  suppression and does not initialize custom processing. Switching between
  this bypass and processing starts a new sample and keeps the selected speaker.
- Opening the settings page requests microphone and camera access separately
  when their device names are unavailable and no call is active on the selected
  server. The browser may show permission dialogs. Each successful request
  refreshes the device choices and stops capture immediately. A missing or
  blocked camera does not prevent audio device discovery. Discovery does not
  play, record, or send media to an external service.

- Room membership and `call.join` are required to enter a call. Starting a
  new call also requires `call.start`. Media permissions do not grant entry.
- `call.voice`, `call.camera`, and `call.screenshare` independently control
  microphone, camera, and screen or application sharing. Captured share audio
  belongs to `call.screenshare`, including native game sharing.
- Members with `call.join` and no media permissions can listen and watch.
  Their client does not request microphone or camera access to join.
- Members can still view an active call and leave it after call permissions
  are denied. Call controls show which actions are unavailable.
- The client stops revoked media when it receives updated room permissions.
  The server also checks connected participants on its 30-second LiveKit
  reconciliation cycle. It updates media grants or disconnects participants
  who can no longer join. Failures retry on later cycles. Revocation is not
  instantaneous, and an old unexpired token can briefly reconnect before a
  subsequent check removes that participant again.
- Existing human call access is preserved on upgrade through initial server
  grants for `everyone`. An existing grant, deny, or clear is never overwritten.
  Bots need explicit grants and their owner's current authority.

- Members of a room with the right permission see a phone tab alongside the room sidebar's members/files tabs when LiveKit is configured.
- Opening the call tab shows the current room call. If no call is active, it offers a "Start call" action. If a call is active and the viewer has not joined, it shows projected participants as ungrouped participant cards and a "Join call" action.
- When the current room has an active call, the phone tab is accent-highlighted and pulses while another sidebar tab is selected.
- Joining the call switches the call tab into participant mode with pinned screen-share tiles first, larger camera video participant cards next, and compact voice-only participant cards after that, without separate Video or Voice section headings. Participant mode exposes a voice activity glow, mute state, camera toggle, screen-share toggle, device selector, and hang-up controls.
- The pane header shows maximize and fullscreen controls only while the viewer is connected to that room's call. An active call alone does not show these controls.
- On desktop, an active call sidebar can be maximized from the pane header. Maximized mode keeps the app's left navigation sidebars visible, hides the room timeline/content area, and turns the call panel into a stage layout: the first screen share is featured, otherwise the first camera participant is featured, otherwise the first voice participant is featured; remaining screen shares, camera feeds, and voice cards stay visible as secondary tiles.
- A desktop active call pane can be placed into browser fullscreen from the pane header, whether it is in the normal sidebar width or maximized across the chat route. This is separate from maximizing the pane inside the chat route.
- Camera and screen-share tiles expose a compact fullscreen button in their header. Joined participant cards expose a compact mute button directly in the header; remote cards keep volume controls in their three-dot menu. Voice cards use the same height for local and remote participants. In a wide sidebar with a screen share or multiple video feeds, participant cards use equal-width columns; screen shares span the full row. Narrow sidebars use one column. Fullscreen is local to the viewer's browser. Remote participant mute is also local to the viewer and does not change server state or other participants' audio. The local participant card controls the viewer's own microphone.
- Call controls form one joined pill with separators at the bottom of the call pane. Its height and rounded corners match the composer and other bottom-row controls. Participant content scrolls above it.
- While the viewer is in a call, a compact joined pill above the lower-left current-user card provides the active call room link plus mute, camera, screen-share, and leave controls. It matches the user card width and remains visible when the call sidebar is open. Both toolbars place the microphone before the camera.
- While the viewer is connected to a call, supported browsers request a screen wake lock so the display does not automatically dim or lock. The lock is released when the call ends and requested again when the app returns to the foreground. Browsers that do not support or grant wake locks continue the call without this enhancement; a wake lock does not prevent mobile operating systems from suspending an app that the user backgrounds or manually locks.
- Other rooms with an active call replace the normal room/DM icon with the same accent phone icon and animated pulse twin used by the call tab so members know there's a conversation happening; clicking that icon opens the room with the call tab selected.
- Message author names show a compact call presence icon when the author is in the current room's active call: phone for voice-only participants, video camera when the viewer has joined the LiveKit call and can see an active camera track.
- A call start appears in the room timeline as a system row naming the member who started it. While that exact call remains active, the row includes a "Join call" action that opens the room's call sidebar. The final leave adds a separate "The active call has ended" row. Individual participant joins and leaves update call indicators and participant lists without adding timeline rows. Explicit user intent is recorded immediately, and LiveKit webhooks/reconciliation confirm or correct the active participant projection.
- Losing room membership also removes the user from the room's active call. This includes voluntarily leaving the room, being removed by a moderator, being banned, and account-deletion cleanup. The affected client immediately hides that room's call roster and disconnects its local media when the membership change arrives. Chatto records the call leave from the membership transition and best-effort asks LiveKit to disconnect the participant; if that LiveKit removal fails, the room membership change still succeeds and reconciliation can catch up later.
- Joined call participants hear fixed synthesized cues from durable participant join/leave events, including their own join/leave events and other participants in the same active call. These call cues are separate from configurable notification sounds and do not use notification sound filters; `CallEndedEvent` does not play a separate cue.
- The first join starts a call session, creates fresh per-call E2EE key material, and records durable call lifecycle facts. The final leave ends the call, records the end fact, and shreds the call key.
- Hanging up disconnects from LiveKit and clears the participant from everyone else's view.
- New clients always enable LiveKit E2EE before connecting. Chatto distributes a KMS-backed per-call shared key with the LiveKit join token; the raw key is never written to EVT and is shredded when the call ends.
- Screen sharing requests browser-tab audio when the browser supports it. In Chrome, the presenter must select a browser tab and enable **Share tab audio** in the browser picker. Chatto excludes whole-system audio so remote call playback is not captured and fed back into the room.
- Screen-share state is LiveKit track state only. Users who have not joined the call still see who is in the active call, but they do not see whether a participant is sharing a screen.
- Dismissing or denying the browser screen-share picker leaves the call connected without an error toast. The control returns to its ready state so the user can try again. Other capture failures still show an error.
- Screen sharing is one call control on every host. In a web browser it opens the browser's own window/tab/screen picker. On macOS 15 and newer, Chatto Desktop instead opens a Chatto picker with static previews of ordinary visible application windows and complete displays. Selecting a window publishes its video and isolated owning-application audio; selecting a display is video-only so Chatto's remote call playback cannot be captured and echoed upstream. Native sharing offers multiple quality layers so LiveKit can match receiver size and network conditions while pausing unused layers. Browser and native sharing occupy one mutually exclusive share slot, while camera and microphone remain independent.
- When LiveKit is not configured on the server, all voice UI is hidden — no button, no panel, no indicator.

## Design Decisions

### 1. Call lifecycle and join/leave are durable room facts with internal source

**Decision:** `CallStartedEvent`, `CallParticipantJoinedEvent`, `CallParticipantLeftEvent`, and `CallEndedEvent` are persisted in the room EVT aggregate keyed by room ID, on `evt.room.{roomId}.call_started`, `evt.room.{roomId}.call_joined`, `evt.room.{roomId}.call_left`, and `evt.room.{roomId}.call_ended`. Explicit frontend join/leave writes use source `USER`; LiveKit webhook writes use source `LIVEKIT`; reconciliation writes use source `RECONCILIATION`. Public APIs expose active call state and call-start/end timeline rows without the internal source or E2EE key ref. Start and end facts drive room history as well as active call state, live indicators, and key lifecycle; participant transitions remain hidden from normal room history.
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

### 5. Voice activity fills the identity row

**Decision:** Soft, flowing accent-coloured fog illuminates each speaking
participant's identity row. Local capture and received microphone audio use the
same measured amplitude scale. Remote levels come from decoded audio before
listener volume or local mute, rather than server speaker-status updates.
Unavailable or unsubscribed remote audio has no glow. Microphone volume controls the fog's brightness,
spread, and movement speed. Quiet speech produces a gentle drift; louder speech
moves the fog faster. Three translucent layers of broad wisps move independently
and overlap. Animated simplex noise gives them uneven density and shape.
Speed changes smoothly. The glow responds quickly to speech
and fades away in silence. Video and screen-share content stays clear. Muted
microphones show no activity. Muting someone locally does not hide their
speaking activity. Reduced motion keeps the glow stationary and updates its
intensity without animation.
Screen-share tiles use their screen audio track's level, including native
companion publishers. They stay quiet when the share has no audio; microphone
activity and microphone mute do not affect this separate meter.
**Why:** The card background makes active speakers easy to find without a
pulsing outline or an extra status icon. The effect uses existing call audio
levels and needs no additional audio capture or external connection.
**Tradeoff:** Glow intensity conveys relative activity rather than a calibrated
volume measurement. Only users who joined the call receive this feedback.

### 6. Screen sharing is joined-client LiveKit track state

**Decision:** Screen/window/tab sharing uses LiveKit's browser screen-share publishing path and is represented by screen-share video plus optional browser-provided tab audio on joined clients. Chatto requests tab audio, publishes it with media-oriented stereo settings, and excludes whole-system audio. Chatto does not persist separate screen-share events, add public API fields, or expose screen-share state to call observers before they join.
**Why:** Screen sharing is media-session state, and the existing durable room facts already answer the server-owned question of who is in the call. Keeping screen-share state inside LiveKit avoids adding durable state that can become stale when browser capture ends.
**Tradeoff:** Non-joined observers know a call is active and who is in it, but not whether someone is sharing. Audio capture remains browser- and surface-dependent, and presenters must opt into tab audio in the browser picker.

### 7. Big-call mode is a desktop pane state, not a separate route

**Decision:** Maximized call mode expands the room call sidebar across the chat route content area while leaving the app's left navigation sidebars in place. It is session-only UI state and uses one featured stage plus a secondary strip, preserving the normal ordering of screen shares before cameras before voice-only participants.
**Why:** Calls remain room-scoped context, not a separate destination. Keeping the left navigation visible lets users stay oriented and move between rooms while giving the call enough canvas for screen shares and active video.
**Tradeoff:** The maximized layout is desktop-first. Mobile keeps the existing overlay sidebar model instead of adding a second maximize/fullscreen interaction layer.

### 8. Fullscreen and local mute are viewer-local controls

**Decision:** Fullscreen controls use the browser Fullscreen API on either an individual media tile or the desktop call pane. Local mute changes only this viewer's local audio: remote participants are muted through LiveKit remote participant volume, and local participant tiles reuse the viewer's microphone mute.
**Why:** Fullscreen and "I don't want to hear this feed/user right now" are personal presentation choices. They should not create durable call facts, alter room state, or surprise other participants.
**Tradeoff:** Local mute is intentionally not visible to other participants and does not change the remote participant's published mute state. Users need to distinguish it from the normal microphone mute indicator.

### 9. Test endpoints bypass webhook validation in build-tag mode

**Decision:** E2E tests use special `/webhooks/test/call-join` and `/webhooks/test/call-leave` endpoints that skip HMAC validation and call the core methods directly. Available only with `-tags test_endpoints`.
**Why:** Real LiveKit isn't realistic to run in CI, but webhook flow is exactly the thing E2E tests need to exercise. Build-tag gating keeps the endpoints out of production. See ADR-020.
**Tradeoff:** Two webhook entry points (real + test); test ones are well-isolated and trivially removable from prod builds.

### 10. E2EE keys are KMS-backed per-call secrets

**Decision:** `voiceCallToken` returns both `token` and `e2eeKey`. The first join for a room creates a new call ID and per-call E2EE key through Chatto's KMS boundary, stores the raw key in `ENCRYPTION_KEYS` under `call.e2ee.{callId}`, and records only the key ref in `CallStartedEvent`. The final leave records `CallEndedEvent`, then attempts idempotent key shredding. That event is also the durable trigger for the shared call-key cleanup consumer, which retries unfinished shredding across crashes and replicas. The frontend creates an `ExternalE2EEKeyProvider`, configures the LiveKit E2EE worker, sets the key, enables E2EE, then connects.
**Why:** LiveKit E2EE key generation/distribution is application responsibility. Chatto already authorizes token access by room membership, so the token resolver is the narrow place to distribute the shared call key. Keeping the raw key out of EVT and normal backups avoids turning event-log copies into permanent decrypt material for captured media. Making the end fact the retry source closes the post-commit crash window without coupling recovery to LiveKit reconciliation.
**Tradeoff:** Always-on E2EE breaks media compatibility with older clients that do not enable E2EE. Key deletion is at least once, so replicas may repeat the same idempotent shredding attempt; an unavailable KMS can briefly retain an ended call's key until retry succeeds. Restoring a backup without `ENCRYPTION_KEYS` cannot recover active call keys; active calls should be considered interrupted across such restores.

### 11. Screen sharing is one action with an optional native host capability

**Decision:** Voice controls expose one screen-share action. When the host advertises the narrow native screen-share capability, Chatto presents short-lived, single-use opaque window and display offers with in-memory previews in its own picker. Enumeration requires a user action, supersedes any earlier enumeration, and excludes Chatto Desktop's own windows. Without that capability, the action delegates to LiveKit and the browser's picker. Selecting a native source starts a publish-only LiveKit companion connection with its own opaque identity and the same per-call E2EE key. The frontend merges that connection's media into the owning member's logical participant, hides the companion as a separate tile, and never attaches the local owner's companion audio. Window capture publishes owning-application audio; full-display capture is video-only because system audio would contain remote call playback. Starting either implementation replaces the other; camera and microphone publications are unaffected.
**Why:** One action matches the user's intent independently of where Chatto runs. The browser keeps its required security chooser, while native capture can add previews, deterministic source choice, and owning-application audio that Chromium's path cannot provide consistently. Direct native publication also avoids an H.264 decode, canvas copy, browser capture, and second WebRTC encode. The optional capability pattern from ADR-072 keeps platform knowledge out of the shared product state and leaves macOS, Windows, and feasible Linux providers independent.
**Tradeoff:** Desktop source enumeration requires macOS Screen & System Audio Recording permission, and static previews can become stale before selection; the helper re-resolves the offered source, verifies a window still belongs to the enumerated application, and fails safely. Full-display viewers do not receive source audio until the native path can exclude Chatto's own playback before capture. The server must mint a separate short-lived publisher credential because reusing the member's LiveKit identity would disconnect their primary call connection. Webhooks and reconciliation exclude companion identities from durable call membership, while stale-room cleanup still removes them. The first macOS target publishes aspect-ratio-preserving H.264 quality classes with maximum edges of 1920, 1280, and 640 pixels at 60, 60, and 30 fps respectively. Encoding multiple layers can consume more publisher CPU and upload when receivers simultaneously request different qualities, while dynacast avoids that cost for unused layers. Actual cadence and quality remain dependent on the source, hardware encoder, network, SFU, and receiving device.

### 12. Active calls request a best-effort screen wake lock

**Decision:** While the viewer is connected to any call, the web client requests a screen wake lock from supporting browsers. Because browsers release locks when a document is hidden, the client requests a fresh lock after returning to the foreground. Ending the call releases the lock. Unsupported or rejected requests do not interrupt the call.
**Why:** Preventing automatic display sleep reduces avoidable call interruption and friction on mobile devices while respecting the browser's power and permission policy.
**Tradeoff:** The API can keep a visible screen awake, but it cannot guarantee background execution or override manual device locking, operating-system power policy, or browser suspension. Keeping the display on also uses more battery.

## Permissions

All five permissions support Server, Room group, Room, and Direct messages
scopes. DM checks use the shared Direct messages scope, not individual DM rules.

- `call.start` — start a call; also requires `call.join`.
- `call.join` — join an active call, including as a listener.
- `call.voice` — publish microphone audio.
- `call.camera` — publish camera video.
- `call.screenshare` — share a screen, window, tab, or native application,
  including captured audio.

Call hosts, participant removal, and other management actions are deferred.

## Related

- **ADRs:** ADR-009 (webhook-driven voice call state), ADR-012 (two-tier real-time events), ADR-020 (build-tag gated test endpoints), ADR-067 (Electron desktop packaging), ADR-069 (explicit durable consumer lifecycle), ADR-072 (optional host capabilities), ADR-091 (semantic realtime events)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-019 (Room Lifecycle), FDR-034 (Chatto Desktop), FDR-045 (Realtime Event Stream)

## Open Questions

- Should there be a dedicated `voice.join` permission so operators can disable voice in specific rooms/groups without touching room membership? Currently any room member can call.
