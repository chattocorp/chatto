# Runtime Component Inventory

Key files: [`cli/cmd/run.go`](../../cli/cmd/run.go), [`cli/internal/embedded_nats/nats_server.go`](../../cli/internal/embedded_nats/nats_server.go), [`pkg/natsruntime/server.go`](../../pkg/natsruntime/server.go), [`cli/internal/runtimeunit/runtimeunit.go`](../../cli/internal/runtimeunit/runtimeunit.go), [`cli/internal/core/core.go`](../../cli/internal/core/core.go), [`cli/internal/core/nats_recovery.go`](../../cli/internal/core/nats_recovery.go), [`cli/internal/core/core_infrastructure.go`](../../cli/internal/core/core_infrastructure.go), [`cli/internal/core/storage.go`](../../cli/internal/core/storage.go), [`cli/internal/core/core_services.go`](../../cli/internal/core/core_services.go), [`apps/desktop/main.mjs`](../../apps/desktop/main.mjs), [`apps/desktop/preload.cjs`](../../apps/desktop/preload.cjs), [`apps/desktop/frontend_protocol.mjs`](../../apps/desktop/frontend_protocol.mjs), [`apps/desktop/native/macos-capture-probe`](../../apps/desktop/native/macos-capture-probe), [`apps/desktop/native/windows-capture-probe`](../../apps/desktop/native/windows-capture-probe), [`apps/frontend/src/lib/desktop/nativeScreenShare.ts`](../../apps/frontend/src/lib/desktop/nativeScreenShare.ts), [`apps/frontend/src/lib/components/voice/ScreenShareControlButton.svelte`](../../apps/frontend/src/lib/components/voice/ScreenShareControlButton.svelte), [`apps/frontend/src/lib/oauth/authorizationWindow.ts`](../../apps/frontend/src/lib/oauth/authorizationWindow.ts)

The core runtime is process-local but must be safe under multiple Chatto replicas connected to the same NATS account. Correctness comes from JetStream/KV atomicity and projection catch-up, not in-process serialization.

Related decisions: [ADR-033](../adr/ADR-033-event-sourced-state-with-projections.md),
[ADR-041](../adr/ADR-041-runtime-units.md),
[ADR-049](../adr/ADR-049-process-wide-realtime-event-hub.md),
[ADR-056](../adr/ADR-056-extractable-nats-event-sourcing-framework.md),
[ADR-058](../adr/ADR-058-application-neutral-embedded-nats-runtime.md), and
[ADR-066](../adr/ADR-066-durable-asset-processing-runtime-unit.md).

`chatto run` composes optional runtime units from a validated catalogue. Each
registration supplies the same `runtimeunit.Unit` used by its standalone
command plus a config predicate controlling whether it starts in the main
process. The exporter, bundled search provider, and asset-processing worker are
registered units. An embedded unit failure is logged and degrades that optional
capability without stopping the core server. The catalogue supervisor restarts
it with exponential backoff capped at 30 seconds, while the same failure still
exits a standalone unit for its process supervisor.
Independently deployable providers use this catalogue rather than adding
custom startup blocks.

## Client runtimes

The experimental Electron desktop shell is a Chatto client runtime using a
pinned stable Electron and bundled Chromium release. It embeds the official
static SvelteKit build and intercepts the fixed secure origin
`chatto://desktop` without opening a TCP listener; ordinary HTTP and HTTPS
traffic remains on Chromium's normal network path. The existing standalone
frontend owns server registration, authentication, and routing.
Electron's default persistent session stores browser state in the application's
user-data directory. Browser and desktop deployments use the same popup-based
OAuth flow and return the same-origin callback through `BroadcastChannel`.

The shell owns no Chatto backend, NATS resources, projections, or durable
domain state. Every macOS and Windows x64 build adds a narrow optional
`screenShare` renderer capability and a nested native helper. macOS uses
ScreenCaptureKit and offers windows and displays. Windows also offers windows
and displays, using Windows Graphics Capture plus WASAPI process-tree loopback
for application sources.

The bridge lists bounded, temporary opaque sources with static JPEG previews
and controls a native LiveKit companion. Source-preview bytes cross Electron as
structured-clone data and remain in memory. During Windows publication, the
helper also sends a framed local copy of its encoded H.264 access units over a
private child-process pipe; the renderer decodes these for the sender tile and
does not subscribe to its own companion tracks. Raw video and captured audio
remain outside Electron IPC. Credentials, acknowledged lifecycle control, and
aggregate non-content performance diagnostics use the existing control path.
Window capture includes isolated owning-application audio. Display
capture is video-only because system audio would include remote call playback.

The macOS companion publishes an H.264 simulcast ladder and enables dynacast so
unused qualities pause. Windows prefers direct NVIDIA NVENC H.264 and retains
Media Foundation hardware H.264 as a runtime fallback. The direct NVIDIA path
loads NVENC from the installed display driver and uses registered D3D11 input
textures, a low-latency P5 preset, quarter-resolution multipass, and spatial
adaptive quantisation. A pinned public LiveKit C++/Rust fork forwards encoded
access units into WebRTC and returns keyframe and rate-control requests to the
helper.

Windows publishes one full-cadence layer because the SDK does not expose custom
screen-share simulcast layers and its default low layer is capped at 3 fps. It
disables dynacast for that layer, allowing receivers to unsubscribe without
suspending the independent native publisher. The sender's preview uses the
local encoded-frame pipe rather than a server-routed subscription.
The capture callback hands CPU-backed frames to a bounded, freshness-first
worker. That worker scales BGRA, converts it to NV12, hardware-encodes it, and
synchronously submits the access unit; slow work drops superseded frames
instead of blocking capture or growing latency without bound.

When WGC changes texture dimensions during a window/fullscreen transition, the
worker scales each frame from its current dimensions into the LiveKit track's
stable output size rather than renegotiating or discarding resized frames.

The Windows helper watches both source-window validity and frame cadence. It
reattaches the existing publisher to a same-application replacement window—or
restarts capture on the same valid window—after a temporary closure or stall;
without a replacement it disconnects. When the stalled window owns its complete
foreground monitor, the helper immediately switches from window WGC to monitor
WGC; it does not wait for sparse window-WGC heartbeat frames to stop.

If monitor WGC also stalls, it tries DXGI Desktop Duplication and crops frames
to the window bounds. Direct-presentation exit returns capture to window WGC. A
fallback setup failure is non-fatal and retried after a cooldown. Expected DXGI
access loss during a presentation-producer transition recreates the duplication
interface instead of failing the fallback. Metrics identify the active capture
backend.

A line-delimited stdin control command provides cooperative stop, with Desktop
retaining a bounded process-termination fallback. Aggregate publisher
diagnostics continue on a fixed heartbeat even
when no frame publishes; they include the latest non-content source dimensions
and resize count so capture stalls can be distinguished from downstream WebRTC
stalls. Desktop also acknowledges receipt of a stop request on the renderer
control port before waiting for helper exit.

An explicitly selected Windows display uses monitor WGC directly and remains
bound to that monitor until the user stops sharing or the monitor disappears.
It does not enter the window replacement or presentation-fallback state
machine, and it publishes no audio so remote call playback cannot echo into the
room.

The shared frontend feature-detects the capability through its focused desktop
adapter. One screen-share control opens Chatto's source chooser when the
capability exists and otherwise invokes the complete browser/LiveKit path,
including the browser's own chooser. This is the host-capability pattern from
[ADR-072](../adr/ADR-072-optional-host-capabilities-in-the-shared-frontend.md);
the frontend does not branch on Electron, macOS, the app origin, or user-agent
identity.

macOS and Windows CI build and test their helpers; each complete platform bundle
embeds its helper and pinned native LiveKit runtime. The shell restricts navigation and browser
permissions at the Electron boundary, while OAuth behavior remains specified by
[FDR-023](../fdr/FDR-023-authentication-and-sessions.md).

The core model inventory is a list of stable machine-readable keys such as `config_model`, `message_model`, and `my_events_model`. Per-process metrics expose these keys via `chatto_model_info`.

| Model                            | Key files                                                                                                                                                   | Responsibility                                                                                                                                |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `ChattoCore`                     | [`core.go`](../../cli/internal/core/core.go), [`core_infrastructure.go`](../../cli/internal/core/core_infrastructure.go), [`storage.go`](../../cli/internal/core/storage.go), [`projection_wiring.go`](../../cli/internal/core/projection_wiring.go), [`core_services.go`](../../cli/internal/core/core_services.go) | Application facade and composition root; staged resource initialization, projection registration and lifecycle, API-facing operations, and only production-consumed read models, projectors, and cross-package adapters |
| NATS recovery gate              | [`nats_recovery.go`](../../cli/internal/core/nats_recovery.go), [`health.go`](../../cli/internal/http_server/health.go)                                               | Marks the replica unready across NATS continuity gaps, quarantines realtime sessions, restores volatile resources, refreshes KV watchers, waits for projection catch-up, and fails liveness after a five-minute recovery stall |
| Embedded NATS runtime           | [`nats_server.go`](../../cli/internal/embedded_nats/nats_server.go), [`server.go`](../../pkg/natsruntime/server.go), [`restore.go`](../../cli/cmd/restore.go) | `chatto run` maps Chatto-owned listener, authentication, monitoring, logging, and storage policy into the shared server lifecycle; restore uses the same lifecycle with a temporary in-process-only server |
| Runtime-unit catalogue          | [`run.go`](../../cli/cmd/run.go), [`runtimeunit.go`](../../cli/internal/runtimeunit/runtimeunit.go)                                                              | Validated composition and capped-backoff supervision of optional units under `chatto run` using the same unit implementations as standalone commands |
| `exporter.Unit`                 | [`unit.go`](../../cli/internal/exporter/unit.go)                                                                                                                 | Optional export runtime started by `[exporter].enabled` under `chatto run` or directly by its standalone command                               |
| `bleve.Unit`                    | [`unit.go`](../../cli/internal/search/bleve/unit.go), [`search_provider.go`](../../cli/cmd/search_provider.go)                                                    | Bundled message-search provider with the runtime diagnostic identity `search.BleveProvider`, started by `[search_provider].enabled` under `chatto run` or as `chatto search-provider`; opens existing EVT and encryption resources without starting `ChattoCore`, exposes status during startup replay, and joins the shared query queue only after replay is current |
| `video.Unit`                    | [`unit.go`](../../cli/internal/video/unit.go), [`asset_processing.go`](../../cli/cmd/asset_processing.go), [`asset_processing_runtime.go`](../../cli/internal/core/asset_processing_runtime.go) | Durable asset-processing worker started by `[asset_processing].enabled` under `chatto run` or explicitly as `chatto asset-processing`; `[asset_processing]` also owns its ffmpeg paths, temporary directory, and per-process concurrency; runs a private AssetProjection without starting `ChattoCore` or main-app boot mutations |
| `MyEventsModel`                  | [`my_events_model.go`](../../cli/internal/core/my_events_model.go), [`realtime_replay.go`](../../cli/internal/core/realtime_replay.go)                           | Eagerly wired `myEvents` live delivery, bounded EVT-gap planning, projection readiness, heartbeats, per-user authorization, and process-local stream counters |
| Realtime projection assembler   | [`realtime_projection.go`](../../cli/internal/connectapi/realtime_projection.go), [`realtime_projection.go`](../../cli/internal/http_server/realtime_projection.go) | Caller-authorized compacted server state and current public projection operations derived from durable/live facts without exposing EVT payloads |
| `events.EncodedEventLog`        | [`encoded_event_log.go`](../../pkg/events/encoded_event_log.go)                                                                                         | Independently versioned incubation-module boundary for opaque-byte JetStream reads and OCC-only writes, including message deduplication, atomic batches, filter-scoped guards, and stream positions; it has no Chatto event-envelope or subject-policy knowledge |
| `evtstream.Publisher`           | [`publisher.go`](../../cli/internal/evtstream/publisher.go), [`subjects.go`](../../cli/internal/evtstream/subjects.go)                                           | Chatto adapter that owns the stable EVT subject vocabulary, validates durable `corev1.Event` values, preserves their stable IDs, and protobuf-encodes/decodes them above `EncodedEventLog` |
| `events.ProjectionHandle` / `events.Projector` | [`projector.go`](../../pkg/events/projector.go), [`projector.go`](../../cli/internal/evtstream/projector.go)                             | Envelope-neutral typed projection ownership plus ordered replay, readiness, failure, snapshot, and checkpoint lifecycle; `evtstream` supplies Chatto's unchanged `corev1.Event` decoder and typed constructors |
| `events.DurableWorker`         | [`durable_worker.go`](../../pkg/events/durable_worker.go)                                                                                 | Application-neutral bounded, at-least-once execution from an application-owned JetStream pull consumer; transient fetches retry, deleted consumers return control to application lifecycle, and callers own decoding, projection barriers, idempotency, retry classification, and terminal facts |
| `ConfigModel`                    | [`config_model.go`](../../cli/internal/core/config_model.go), [`server_config_model.go`](../../cli/internal/core/server_config_model.go)                        | Sole core boundary for semantic server/user config reads and event writes, including `ConfigProjection` readiness                              |
| `NotificationPreferencesModel`   | [`notification_level.go`](../../cli/internal/core/notification_level.go)                                                                                        | Operation-level notification preference API with authZ before config preference writes                                                         |
| `MessageModel`                   | [`message_model.go`](../../cli/internal/core/message_model.go), [`messages.go`](../../cli/internal/core/messages.go)                                              | Operation-level message posting and mutation API with preflight validation, Slow Mode enforcement in preflight and room-OCC commit authorization, narrow authorization-fence plus room-OCC edits, room-scoped retractions, projection waits, atomic edit-driven echo reconciliation, read-marker side effects, and atomic author-created root-thread writes |
| `MessageSearchReadModel`         | [`message_search_read_model.go`](../../cli/internal/core/message_search_read_model.go)                                                                            | Resolves provider queries to current member-room scopes and re-authorizes thin provider hits against current room membership and message state  |
| `ReactionModel`                  | [`reaction_model.go`](../../cli/internal/core/reaction_model.go), [`reactions.go`](../../cli/internal/core/reactions.go)                                          | Sole reaction mutation boundary: actor membership and `message.react` authZ, room-aggregate OCC writes and retries, and reaction-projection readiness |
| `RoomCommandModel`               | [`room_command_model.go`](../../cli/internal/core/room_command_model.go)                                                                                         | Operation-level room lifecycle, Slow Mode configuration, membership, moderation, and DM commands with public API authorization and room-kind preconditions              |
| `RoomDirectoryReadModel`         | [`room_directory_read_model.go`](../../cli/internal/core/room_directory_read_model.go)                                                                           | Operation-level room directory and sidebar reads, viewer capability and Slow Mode deadline hydration, and directory-adjacent join commands                            |
| `RoomTimelineReadModel`          | [`room_timeline_read_model.go`](../../cli/internal/core/room_timeline_read_model.go), [`room_events.go`](../../cli/internal/core/room_events.go)                   | Operation-level room/thread timeline read API with actor membership checks, thread-root validation, and projection-backed page/window selection |
| `ReadStateModel`                 | [`read_state_model.go`](../../cli/internal/core/read_state_model.go), [`read_state_index.go`](../../cli/internal/core/read_state_index.go), [`room_unread.go`](../../cli/internal/core/room_unread.go), [`threads.go`](../../cli/internal/core/threads.go) | Operation-level room/thread read marker API plus one process-wide filtered `RUNTIME_STATE` watcher; initial-sync readiness, in-memory reads, KV OCC writes, revision barriers, and sync events |
| `ThreadFollowModel`              | [`thread_follow_model.go`](../../cli/internal/core/thread_follow_model.go), [`threads.go`](../../cli/internal/core/threads.go)                                     | Operation-level thread follow/unfollow API with actor membership checks, thread-root validation, durable follow-state events, projection readiness, and sync events |
| `RoomModel`                      | [`room_model.go`](../../cli/internal/core/room_model.go), [`pinned_messages.go`](../../cli/internal/core/pinned_messages.go)                                    | Eagerly wired room-derived projection readiness and narrow reads for room catalog, membership, layout, timeline, threads, reactions, and pinned messages; owns authorization-fenced pin mutations |
| `UserModel`                      | [`user_model.go`](../../cli/internal/core/user_model.go)                                                                                                       | Sole core owner of user profile, cold-replayed authentication, and content-key projection reads and readiness for account, identity, credential, profile, custom-status, and encryption operations |
| `InvitationModel`                | [`invitations.go`](../../cli/internal/core/invitations.go), [`invitation_projection.go`](../../cli/internal/core/invitation_projection.go)                     | Sole core owner of invite-link creation, listing, revocation, validation, compact purpose-separated token derivation, and projection readiness; redemption commits atomically with the admitted account against a whole-EVT OCC guard |
| `OAuthClientModel`               | [`oauth_clients.go`](../../cli/internal/core/oauth_clients.go), [`oauth_client_projection.go`](../../cli/internal/core/oauth_client_projection.go)             | Sole core owner of successful OAuth-client authorization records, administrative default/trusted/blocked policy, fail-closed projection-backed authorization checks, per-client active-realtime access-denial notifications on every replica, and block-triggered OAuth token cleanup |
| `UserKeyShreddingModel`          | [`user_key_shredding.go`](../../cli/internal/core/user_key_shredding.go)                                                                                       | Request-before-destruction crypto-shredding, privacy-projection barriers, synchronous idempotent completion, and shared durable recovery across replicas |
| `RBACModel`                      | [`rbac_model.go`](../../cli/internal/core/rbac_model.go)                                                                                                       | Sole core owner of RBAC projection reads and readiness for role, assignment, and permission authorization and writes |
| `MentionablesModel`              | [`mentionables_projection.go`](../../cli/internal/core/mentionables_projection.go)                                                                              | Global mention-handle namespace lookup and readiness                                                                                          |
| `PresenceModel`                  | [`presence_model.go`](../../cli/internal/core/presence_model.go), [`presence_hub.go`](../../cli/internal/core/presence_hub.go)                                    | Live presence writes plus per-process watcher, bulk-read snapshot, and fanout for presence state in `MEMORY_CACHE`                            |
| `CallModel`                      | [`call_model.go`](../../cli/internal/core/call_model.go), [`voice.go`](../../cli/internal/core/voice.go), [`lease.go`](../../cli/internal/lease/lease.go)             | Sole core owner of call-state projection reads and readiness; generation-consistent participant snapshots and call ID/E2EE access material; durable LiveKit call lifecycle/participant facts and elected LiveKit reconciliation |
| `MediaModel`                     | [`media_model.go`](../../cli/internal/core/media_model.go), [`attachments.go`](../../cli/internal/core/attachments.go)                                             | Eagerly wired attachment/media binary storage, signed asset and origin-scoped HLS URLs, transformed image cache operations                                    |
| `AssetModel`                     | [`asset_model.go`](../../cli/internal/core/asset_model.go), [`asset_cleanup.go`](../../cli/internal/core/asset_cleanup.go), [`asset_projection.go`](../../cli/internal/core/asset_projection.go) | Sole core owner of asset-projection reads and readiness; detached generation-consistent asset state; uploader-bound exclusive message attachments with asset-aggregate OCC; exact-owner deletion; processing transitions, tombstones, and shared durable physical deletion |
| `AssetUploadModel`               | [`asset_uploads.go`](../../cli/internal/core/asset_uploads.go)                                                                                                    | Eagerly wired chunked attachment upload sessions, temporary object assembly, pending-asset expiry, and process-local periodic cleanup           |
| `projectionSnapshotWorker`       | [`projection_snapshot_worker.go`](../../cli/internal/core/projection_snapshot_worker.go)                                                                          | Optional per-pass elected post-boot and daily publication of encrypted generations; a separate cluster-wide cooldown limits bounded S3 age expiry when Chatto owns lifecycle cleanup |
| `video.Service`                  | [`service.go`](../../cli/internal/video/service.go), [`processor.go`](../../cli/internal/video/processor.go)                                                   | Synchronous video/animated-GIF processing attempts: web-compatible stereo audio normalization, HLS segment packaging and upload, animated-GIF MP4 upload, and terminal asset processing events; queue and concurrency remain owned by `video.Unit` |
