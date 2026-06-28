# Chatto Architecture

This document is the **inventory**: what currently exists in the system — streams, KV buckets, object stores, subject patterns, key shapes, and API operations. It's the *what's where* reference, not the *why* one.

For *why* a particular design decision was made:

- **Cross-cutting architectural choices** (NATS as primary store, API strategy, per-user encryption, etc.) live in the [Architecture Decision Records](adr/INDEX.md).
- **Per-feature design** (Roles & Permissions, Direct Messages, Reactions, Notifications, etc.) lives in the [Feature Decision Records](fdr/INDEX.md).
- **Coding and review conventions** live in the repo root [`AGENTS.md`](../AGENTS.md) and directory-specific `AGENTS.md` files.

## Table of Contents

- [Overview](#overview)
  - [Core Concepts](#core-concepts)
- [NATS Authentication](#nats-authentication)
- [Architecture & APIs](#architecture--apis)
- [Core Models](#core-models)
- [Projection Inventory](#projection-inventory)
- [ConnectRPC API Overview](#connectrpc-api-overview)
  - [Endpoint Inventory](#endpoint-inventory)
- [Realtime WebSocket API Overview](#realtime-websocket-api-overview)
- [Architecture Pattern: Event-Sourced Writes](#architecture-pattern-event-sourced-writes)
  - [Write Path](#write-path)
  - [Consistency Model](#consistency-model)
- [NATS Resource Inventory](#nats-resource-inventory)
  - [Current Resources](#current-resources)
  - [Event Envelopes](#event-envelopes)
  - [EVT Subject Patterns](#evt-subject-patterns)
  - [Durable EVT Event Inventory](#durable-evt-event-inventory)
  - [Transient Live Subjects](#transient-live-subjects)
  - [KV Buckets (backed by streams)](#kv-buckets-backed-by-streams)
  - [Object Store Buckets](#object-store-buckets)
  - [Dynamic Image Transformation](#dynamic-image-transformation)
  - [Messages](#messages)
  - [Key Patterns](#key-patterns)

## Overview

Chatto is a real-time chat application with a ConnectRPC public API, a protobuf realtime WebSocket, and a NATS/JetStream backend. Durable domain state is event-sourced in the `EVT` stream and served from projections; `RUNTIME_STATE` holds persisted latest-value runtime state such as notifications, push subscriptions, and auth tokens.

### Core Concepts

This document uses the canonical terms from the [glossary](GLOSSARY.md), especially **Server**, **Room**, **DM**, **Event**, **Projection**, **Subject**, and **Live Event**. The rest of this document focuses on where those concepts live in the current runtime architecture.

## NATS Authentication

Chatto supports embedded NATS for single-process/self-hosted installs and external NATS for clustered deployments. Embedded NATS is configured under `[nats.embedded]`; when the embedded TCP listener is enabled, `ReadConfig` derives matching `[nats.client]` defaults for CLI/admin commands. External NATS connections are configured explicitly via `[nats.client]`.

| Method        | Config                                  | Description                                                       |
| ------------- | --------------------------------------- | ----------------------------------------------------------------- |
| `token`       | `token` / `nats.embedded.auth_token`    | Default for embedded NATS and simple external deployments.        |
| `userpass`    | `username`, `password`                  | Simple username/password authentication.                          |
| `credentials` | `credentials_file`                      | JWT authentication via standard `.creds` file for external NATS.  |
| `nkey`        | `nkey_seed`                             | NKey seed auth for external NATS deployments that use NKeys.      |
| `none`        | -                                       | No authentication; only acceptable on trusted private networks.   |

**Embedded NATS Setup:**

When using embedded NATS (default), `chatto init` generates:
- `chatto.toml` with `[nats.embedded]`, in-process NATS enabled, JetStream data directory, and generated `auth_token`
- A commented `nats.embedded.port` example; uncommenting the port enables a local TCP listener and derived `[nats.client]` connection defaults for CLI/admin commands
- Standalone NATS-client tools such as `chatto exporter` require either external NATS or the embedded TCP listener. Single-process installs can instead opt into `[exporter].enabled = true` so `chatto run` starts the exporter in-process over its existing NATS connection.

**External NATS Setup:**

For connecting to an external NATS cluster:
1. Set `nats.embedded.enabled = false`
2. Set `nats.client.url` to the external NATS URL(s)
3. Set `nats.client.auth_method` plus the matching credential field (`token`, `username`/`password`, `credentials_file`, or `nkey_seed`)

## Architecture & APIs

Key files: [`cli/internal/core/core.go`](../cli/internal/core/core.go), [`cli/internal/events/`](../cli/internal/events/), [`cli/internal/runtimeunit/`](../cli/internal/runtimeunit/), [`cli/internal/connectapi/`](../cli/internal/connectapi/), [`cli/internal/http_server/connect.go`](../cli/internal/http_server/connect.go), [`cli/internal/http_server/metrics.go`](../cli/internal/http_server/metrics.go), [`cli/internal/exporter/`](../cli/internal/exporter/), [`proto/chatto/core/v1/`](../proto/chatto/core/v1/), [`proto/chatto/api/v1/`](../proto/chatto/api/v1/)

- **NATS**: At the core, Chatto uses a series of NATS JetStream streams, KV buckets and object storage. Data stored in these is defined as Protocol Buffers (see `proto/`).
- **Core**: The `core` package defines Chatto's domain logic and directly talks to NATS to interact with KV buckets, object stores, and the `EVT` stream. `ChattoCore` remains the compatibility facade, while smaller models own projection readiness and domain-specific write concerns.
- **Runtime Units**: Optional long-running processes that share Chatto configuration, logging, NATS, and JetStream access implement the `runtimeunit.Unit` interface. A unit can run as a standalone `chatto <unit>` command over `[nats.client]`, or be embedded under `chatto run` when its config section enables that mode. Only `chatto run` starts embedded NATS and boots `ChattoCore`; observer/projection/worker units must open the resources they need without using core boot paths unless they explicitly need the main application runtime.
- **ConnectRPC**: Protobuf-first public API mounted under `/api/connect`. Public API protos live in `proto/chatto/api/v1/` and generate Go server/client bindings under `cli/internal/pb/chatto/api/v1/` plus TypeScript bindings under `apps/frontend/src/lib/pb/chatto/api/v1/`. Service implementations live in `cli/internal/connectapi/`; the HTTP server only mounts them and injects authentication context. The bundled web client uses the current API for public server profile metadata, server profile/security/branding administration, self-service profile/avatar/preferences/account-deletion commands, text/link-preview/attachment message posting, composer link-preview fetches, message-link preview cards, message edits/deletes, attachment/link-preview removal, room attachment listing and signed URL refresh, typing indicators, presence self-reporting, reaction mutations, custom status writes, role catalog reads and role CRUD, permission matrix reads and role/user permission writes, room notification override mutations, room directory/sidebar reads, admin room layout reads/writes, admin system diagnostics, admin event-log reads, admin user identity/cooldown commands, channel room lifecycle and membership commands, DM start actions, Universal changes, room-ban reads/commands, main-room historical timeline pages, and thread historical timeline pages.
- **Realtime WebSocket**: Protobuf WebSocket protocol mounted at `/api/realtime`. Clients send and receive binary `chatto.api.v1.Realtime*` frames generated from `proto/chatto/api/v1/realtime.proto`. The endpoint authenticates with a bearer token in the initial hello frame or existing cookie auth, then delegates delivery to `core.StreamMyEvents` so room membership, RBAC, projection readiness, live membership changes, and session termination behavior stay centralized. The first version is live-only and does not expose acknowledgement or replay cursors; reconnect catch-up comes from projected reads.
- **Metrics & Diagnostics**: Optional Prometheus-compatible per-process metrics run on a separate internal HTTP listener configured by `[metrics]`. The endpoint exposes Go/process collectors plus Chatto readiness, realtime stream counters, NATS client counters, projection health/lag gauges, and final projection startup duration/message-count gauges once initial replay completes. Deployment-wide Chatto instance metrics are exposed by the separate `chatto exporter` command, or by `chatto run` when `[exporter].enabled = true`; the exporter reads existing `EVT` and `MEMORY_CACHE` resources without running core boot mutations and can cache S3 bucket-size scans. Go pprof debug endpoints are available on the per-process metrics listener under `/debug/pprof/` only when `[metrics].pprof` / `CHATTO_METRICS_PPROF` is enabled. `[diagnostics].startup_cpu_profile` / `CHATTO_DIAGNOSTICS_STARTUP_CPU_PROFILE` writes a process-startup CPU profile through core boot for local replay benchmarking and operator troubleshooting.
- **Web Client**: SvelteKit-based SPA that gets compiled and embedded into the Go binary. Talks to ConnectRPC for protobuf-first API surfaces, uses `/api/realtime` for live updates, and still carries a small local render DTO compatibility layer while component models are moved to protobuf-native names. Presence self-reporting is Connect-based; the realtime event bus sends a bearer token in the protobuf hello frame when available and recovers missed live state with projected reads.
- **Email**: Optional SMTP integration for transactional emails (verification, password reset). Configured via `[smtp]` in config. The `internal/email` package provides a `Mailer` that returns `ErrSMTPDisabled` when SMTP is not configured, allowing callers to handle gracefully.

## Core Models

Key files: [`cli/internal/core/core.go`](../cli/internal/core/core.go), [`cli/internal/core/*_model.go`](../cli/internal/core/), [`cli/internal/video/service.go`](../cli/internal/video/service.go)

The core runtime is process-local but must be safe under multiple Chatto replicas connected to the same NATS account. Correctness comes from JetStream/KV atomicity and projection catch-up, not in-process serialization.

`ChattoCore` keeps a core model inventory with stable machine-readable keys such as `config_manager`, `message_model`, and `my_events_model`. Per-process metrics expose these keys via `chatto_model_info`; `chatto_service_info` remains a deprecated compatibility alias that emits the previous `*_service` label values. Display names remain operator-facing text only.

| Model                            | Key files                                                                                                                                                   | Responsibility                                                                                                                                |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `ChattoCore`                     | [`core.go`](../cli/internal/core/core.go)                                                                                                                    | Application facade, resource initialization, lifecycle, API-facing operations                                                                  |
| `MyEventsModel`                  | [`my_events_model.go`](../cli/internal/core/my_events_model.go)                                                                                              | `myEvents` live delivery, projection readiness, heartbeats, per-user authorization, and process-local stream counters                          |
| `events.Publisher`              | [`publisher.go`](../cli/internal/events/publisher.go)                                                                                                       | OCC-only writes to `EVT`, including atomic batches and filter-scoped concurrency guards                                                        |
| `ConfigModel`                    | [`config_model.go`](../cli/internal/core/config_model.go)                                                                                                   | Semantic server/user config event writes plus `ConfigProjection` readiness                                                                     |
| `ConfigManager`                  | [`config_manager.go`](../cli/internal/core/config_manager.go)                                                                                               | Compatibility facade for server config reads/writes backed by `ConfigModel`                                                                   |
| `NotificationPreferencesModel`   | [`notification_level.go`](../cli/internal/core/notification_level.go)                                                                                        | Operation-level notification preference API with authZ before config preference writes                                                         |
| `MessageModel`                   | [`message_model.go`](../cli/internal/core/message_model.go), [`messages.go`](../cli/internal/core/messages.go)                                              | Operation-level message posting API with room/thread authZ, large-mention confirmation, and read-marker side effects                           |
| `ReactionModel`                  | [`reaction_model.go`](../cli/internal/core/reaction_model.go), [`reactions.go`](../cli/internal/core/reactions.go)                                          | Operation-level reaction mutation API with actor membership and `message.react` authZ before room-aggregate reaction writes                    |
| `RoomTimelineReadModel`          | [`room_timeline_read_model.go`](../cli/internal/core/room_timeline_read_model.go), [`room_events.go`](../cli/internal/core/room_events.go)                   | Operation-level room/thread timeline read API with actor membership checks, thread-root validation, and projection-backed page/window selection |
| `ReadStateModel`                 | [`read_state_model.go`](../cli/internal/core/read_state_model.go), [`room_unread.go`](../cli/internal/core/room_unread.go)                                   | Operation-level room/thread read marker API with actor membership checks, anchor validation, advance-only marker writes, and sync events       |
| `ThreadFollowModel`              | [`thread_follow_model.go`](../cli/internal/core/thread_follow_model.go), [`threads.go`](../cli/internal/core/threads.go)                                     | Operation-level thread follow/unfollow API with actor membership checks, thread-root validation, runtime follow markers, and sync events       |
| `RoomModel`                      | [`room_model.go`](../cli/internal/core/room_model.go)                                                                                                       | Room-derived projection readiness and narrow reads for room catalog, membership, layout, timeline, threads, reactions                          |
| `UserModel`                      | [`user_model.go`](../cli/internal/core/user_model.go)                                                                                                       | User and content-key projection readiness for account/profile/custom-status/encryption writes                                                  |
| `RBACModel`                      | [`rbac_model.go`](../cli/internal/core/rbac_model.go)                                                                                                       | RBAC projection readiness for role, assignment, and permission writes                                                                          |
| `MentionablesModel`              | [`mentionables_projection.go`](../cli/internal/core/mentionables_projection.go)                                                                              | Global mention-handle namespace lookup and readiness                                                                                          |
| `PresenceModel`                  | [`presence_model.go`](../cli/internal/core/presence_model.go), [`presence_hub.go`](../cli/internal/core/presence_hub.go)                                    | Live presence writes plus per-process watcher/fanout for presence state in `MEMORY_CACHE`                                                     |
| `CallModel`                      | [`call_model.go`](../cli/internal/core/call_model.go), [`voice.go`](../cli/internal/core/voice.go), [`lease.go`](../cli/internal/lease/lease.go)             | Durable LiveKit call lifecycle/participant facts, call-state projection readiness, KMS-backed E2EE key resolution, and elected LiveKit reconciliation |
| `MediaModel`                     | [`media_model.go`](../cli/internal/core/media_model.go), [`attachments.go`](../cli/internal/core/attachments.go)                                             | Attachment/media binary storage, signed asset URLs, transformed image cache operations                                                         |
| `AssetModel`                     | [`asset_model.go`](../cli/internal/core/asset_model.go), [`asset_projection.go`](../cli/internal/core/asset_projection.go)                                   | Durable asset lifecycle facts, processing transitions, tombstones, derivative cleanup ordering, asset projection readiness and reads            |
| `video.Service`                  | [`service.go`](../cli/internal/video/service.go)                                                                                                            | Process-local video/animated-GIF processing; emits asset processing result events                                                              |

## Projection Inventory

Key files: [`cli/internal/core/core.go`](../cli/internal/core/core.go), [`cli/internal/events/projector.go`](../cli/internal/events/projector.go), [`cli/internal/core/projection_subjects_test.go`](../cli/internal/core/projection_subjects_test.go)

Projections are in-memory read models rebuilt from `EVT`. `NewChattoCore` registers each top-level projector once with a stable machine-readable key such as `content_keys` plus a human display name such as `Content Keys`; `ChattoCore.Run` replays `evt.>` through one process-local ordered consumer, decodes each event once, dispatches it to projections whose logical subject filters match, records each projection's initial replay startup duration, waits for them to become current at boot, and writers wait for the relevant projector sequence before returning read-your-writes. The projector framework owns JetStream message handling and passes stable stream sequence numbers into `Projection.Apply`; projection implementations do not inspect consumer sequence numbers or raw JetStream metadata.

| Runtime area       | Registered projector | Consumes                                                   | Read models / primary readers                                                             |
| ------------------ | -------------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Room directory     | Room Directory       | `evt.room.>`                                               | `RoomCatalogProjection`, `RoomMembershipProjection`, `RoomBanProjection`; room/member queries, room authorization, and Universal-room effective membership |
| Room organization  | Room Group Layout    | `evt.group.>`, `evt.layout.>`                              | `RoomGroupProjection`, `RoomLayoutProjection`; sidebar groups, sidebar links, and mixed sidebar item ordering |
| Room timeline      | Room Timeline        | `evt.room.>`                                               | Visible room timeline, latest message bodies, hidden echoes, current attachment-bearing message index, direct message-post lookup, and message asset references |
| Assets             | Assets               | `evt.asset.>`, legacy `evt.room.*.asset_*`                 | Asset creation metadata, room scope, processing manifests, derivative graph, deletion state, and legacy room-asset compatibility |
| Threads            | Threads              | `evt.room.*.thread_created`, `evt.room.*.message_posted`, `evt.room.*.message_edited`, `evt.room.*.message_retracted`, `evt.user.*.user_key_shredded` | Per-thread reply logs, summaries, participants, reply counts                               |
| Reactions          | Reactions            | `evt.room.>`                                               | Current per-message reaction sets and room-scoped snapshot OCC positions; intentionally broad so reaction writes can OCC against the room tail |
| Voice calls        | Call State           | `evt.room.>`                                               | Current LiveKit call session, participants, active room IDs, and room-scoped snapshot OCC positions |
| Server/user config | Server Config        | `evt.config.>`, selected user cleanup/preference facts     | Server config, branding refs, user preferences, notification levels, blocked usernames     |
| Users              | Users                | `evt.user.>`                                               | Account/profile/custom-status/auth lookup state, verified emails, external identity links, encrypted user PII |
| Content keys       | Content Keys         | `evt.user.*.dek_generated`, `evt.user.*.user_key_shredded` | Active and shredded user DEK epochs for message bodies and user PII                        |
| RBAC               | RBAC                 | `evt.rbac.>`                                               | Roles, role order, assignments, scoped allow/deny decisions                                |
| Mentions           | Mentionables         | `evt.>`                                                    | Global mention-handle ownership across users, roles, `@all`, and `@here`                  |

Notes: registered projector keys are used by metrics and automation; registered projector names match the admin projection diagnostics. Composite projections expose nested read models, but only their parent projector is started by `ChattoCore.Run`. The shared replay fanout reduces duplicate replay delivery and protobuf decoding while keeping each projection's status, lag, failure, and read-your-writes waiters independent. `Subjects()` is the logical consumption and readiness contract; optional replay subjects are only the physical consumer filter. Focused logical filters are appropriate for stable derived indexes such as Threads, while broad filters remain intentional for projections whose snapshots expose room-tail OCC positions, such as Reactions and Call State. Threads reports the focused logical subjects above for waits and diagnostics; non-thread room facts are skipped before `Apply`.

## ConnectRPC API Overview

Key files: [`proto/chatto/api/v1/`](../proto/chatto/api/v1/), [`cli/internal/connectapi/`](../cli/internal/connectapi/), [`cli/internal/connectapi/room_timeline_assembler.go`](../cli/internal/connectapi/room_timeline_assembler.go), [`cli/internal/http_server/connect.go`](../cli/internal/http_server/connect.go), [`cli/internal/pb/chatto/api/v1/`](../cli/internal/pb/chatto/api/v1/), [`apps/frontend/src/lib/api/`](../apps/frontend/src/lib/api/), [`apps/frontend/src/lib/state/userSummaries.svelte.ts`](../apps/frontend/src/lib/state/userSummaries.svelte.ts)

The ConnectRPC API is the protobuf-first public API mounted under `/api/connect`. Generated service paths are prefixed by that mount, for example `/api/connect/chatto.api.v1.ServerService/GetServer`. `chatto.api.v1` is the broad base API for integrations and the bundled web client; app-specific API should be exceptional rather than the default home for frontend-used features. `cli/internal/connectapi` owns service implementations and API/core protobuf mapping; `cli/internal/http_server/connect.go` owns Gin route mounting and existing bearer-token-then-cookie auth injection. Registered Connect handlers carry an explicit auth policy: `ServerService.GetServer` is public, while private services are wrapped with `connectrpc.com/authn` HTTP middleware. The HTTP edge resolves the full user through existing auth mechanisms, stores a narrow `connectapi.Caller` in Connect authn context, and private Connect handlers pass only the actor ID into core operation models. Those models own room membership authorization, RBAC/permission editor authorization, reaction/thread/root/anchor validation, and lower-level core helper calls. Public URL generation prefers the configured `webserver.url` origin; when it is unset, ConnectRPC and `/api/server` fall back only to the direct request TLS state and host, and do not implicitly trust forwarded proxy headers. The ConnectRPC layer keeps protobuf request/response shaping, shared offset pagination with `PageRequest`/`PageInfo`, schema validation, opaque timeline cursor parsing/formatting, hydrated public timeline DTOs, and Connect status mapping.

Authenticated private Connect requests are rejected by auth middleware before protobuf decoding and schema validation. Once authenticated, `connectrpc.com/validate` enforces public API protovalidate rules before service methods run; missing required request IDs and invalid notification enum values return `InvalidArgument` at the generated handler layer. Public API protobufs are intentionally separate from persisted `corev1` EVT protobufs. `ServerInfoState` uses `ServerService.GetServer` for public profile metadata. `ServerStateService` exposes authenticated server profile/runtime state, server settings update commands, server branding upload/delete commands, and security-sensitive blocked-username reads/writes. Settings and branding updates require `server.manage`; text config writes through `ConfigManager.UpdateServerConfigFunc`, branding writes delegate to the existing core server-branding upload/set/delete pipeline, and both publish the `ServerUpdated` live event. Server branding uploads use a service-specific Connect read limit derived from the configured asset upload size, while blocked-username updates remain admin-only config facts without a member-visible live event. `AccountService` exposes self-service profile, avatar, display-preference, and account-deletion commands over ConnectRPC. Avatar uploads use an account-specific Connect read limit derived from the configured asset upload size, then delegate to the existing core avatar upload/set pipeline so validation, resizing, asset lifecycle facts, and cleanup behavior stay shared. `UserService.GetUser` and `GetUserByLogin` expose authenticated public profile lookup by ID/login, including presence, custom status, avatar URL, creation time, deleted marker, and roles; missing singular users return `NOT_FOUND`. `BatchGetUsers` exposes lightweight ID-keyed `UserSummary` rows for render caches and omits missing IDs without failing the batch. Timeline includes use the same `UserSummary` shape, while notifications and voice-call participant lists use `UserPresenceSummary` when live presence/custom status is needed. The web client keeps a per-server `UserSummaryCache` keyed by `{serverId, userId}`: timeline includes prime the cache, and cache misses can be loaded through `BatchGetUsers` without one request per user. `AdminDiagnosticsService.GetSystemInfo` exposes owner-only operator diagnostics over ConnectRPC: NATS connection metadata, JetStream account/stream/consumer state, aggregate server counts, and projection health/memory estimates. `AdminEventLogService` exposes audit-gated EVT inspection over ConnectRPC: newest-first event pages, bounded filtered scans, event type suggestions, and single-sequence payload reads that return `NOT_FOUND` when the requested sequence does not exist. `AdminUserManagementService` exposes server-admin member list/detail reads, role assignment/revocation, and admin user identity and username-cooldown commands over ConnectRPC. Member reads delegate to core `ListAdminMembers` / `GetAdminMemberDetails`: any authenticated caller can read public member/role fields, verified-email fields require self or `admin.view-users`, and another user's login-cooldown timestamp is visible only to server admins. Role assignment commands delegate to core `AdminAssignServerRole` / `AdminRevokeServerRole`, which require `role.assign` and prevent revoking the caller's own `owner` or `admin` role. Identity commands delegate to core `AdminUpdateUser` / `AdminClearLoginChangeCooldown`, where self-targets pass and other targets require `role.assign`. `RoleService` exposes role catalog, role detail, CRUD, and custom role reordering; role writes delegate to core admin methods so `role.manage` authorization stays centralized.

`MessageService` is the message-write Connect path for text/link-preview/attachment posts, message edits/deletes, attachment/link-preview removal, and live-only typing indicators. `PostMessage` accepts uploaded attachment bytes and existing room-scoped attachment asset IDs; uploaded bytes are preflighted through the core `MessageModel` authorization path before writing asset lifecycle facts, then posted through the same core message model so room/thread authorization, author/moderator RBAC, edit-window checks, echo-to-channel checks, durable message facts, and post-send read-marker side effects do not drift. `AttachmentService` exposes room attachment list pages and signed attachment URL refresh over ConnectRPC; both paths delegate room membership checks to core helpers before returning attachment rows, transformed thumbnails, video thumbnails, and variant URLs. `LinkPreviewService.FetchLinkPreview` is the authenticated composer metadata path; it delegates to core link-preview fetch/cache logic and returns nullable preview metadata plus a transformed preview-image URL when an image asset is cached. `PresenceService.ReportPresence` is the authenticated self-reporting path for live `ONLINE`, `AWAY`, and `DO_NOT_DISTURB` presence; `OFFLINE` is inferred from TTL expiry and is not accepted as a report. Its `user_selected` flag marks explicit user choices so automatic reports from other clients cannot overwrite manually selected Away/DND. `ReactionService.AddReaction` and `RemoveReaction` are the reaction-write Connect path used by the web client. `RoomService` exposes channel room lifecycle and membership commands over ConnectRPC: create, update, archive, unarchive, Universal changes, join, leave, start DM, list active room bans, ban, and unban. The bundled web client uses these RPCs for channel creation, DM start actions, room-directory join/leave actions, server-admin room metadata/archive/Universal changes, and room-ban list/unban moderation. The handlers keep protobuf request validation and Connect status mapping at the transport boundary, while the core room/DM models own operation authorization such as `room.create`, scoped `room.manage`, `room.join`, `message.post` for DM starts, and `room.ban-member`. `RoomDirectoryService` exposes room navigation reads over ConnectRPC: non-archived visible channel rooms, active DM rooms, ordered room groups, mixed sidebar items, per-room viewer state, single-room refresh, and the group join-all command. It delegates visibility, viewer-state capability flags, and join-all filtering to the core `RoomDirectoryReadModel`, which preserves the visibility contract: non-archived channel rooms are shown through membership or `room.list`, DM rooms are membership-only and empty DMs are omitted, archived rooms stay directly refreshable through `GetRoom`, and archived or hidden room entries are dropped from group/sidebar results. The web client also uses `RoomDirectoryService.JoinGroup` for directory "join all" actions. `AdminRoomLayoutService` exposes the admin room layout editor over ConnectRPC: visible admin layout reads, room-group CRUD/reorder, room moves, sidebar-link CRUD/moves, and mixed sidebar item reorders. Its reads preserve the caller-visibility contract, while mutations mirror the previous resolver gates: room-group metadata/order requires `role.manage`, and room/sidebar-link group placement requires group-scoped `room.manage` in the affected source and/or target groups. `UserStatusService.SetCustomStatus` and `ClearCustomStatus` are authenticated self-service profile mutations that delegate to the core user model, which writes durable user aggregate EVT facts. `NotificationPreferencesService.SetRoomNotificationLevel` is the authenticated/authorized preference mutation path: Connect requests use the shared bearer-token-then-cookie auth resolution, then delegate to the core `NotificationPreferencesModel`, which enforces channel-room membership before writing the room notification preference. `RoomTimelineService.GetRoomEvents`, `GetRoomEventsAround`, `ResolveMessageLinkTarget`, `GetThreadEvents`, and `GetThreadEventsAround` are the historical read paths: Connect authenticates and parses opaque cursors, accepts legacy `seq:` cursors as a compatibility input, delegates membership checks, permalink target resolution, and projection-backed room/thread page selection to `RoomTimelineReadModel`, then hands projected rows to `roomTimelineAssembler` for public DTO hydration. The assembler owns renderable protobuf timeline rows with users, message bodies, reactions, thread metadata, link previews, and attachment URLs so browser clients avoid resolver-style N+1 fetching and future public read surfaces can reuse the same mapping; the web client uses `ResolveMessageLinkTarget` for `/m/{messageId}` routes that may point at thread-only replies and reuses `GetRoomEventsAround` for pasted Chatto message-link preview cards. `ReadStateService` and `ThreadService` carry timeline-adjacent paths through core `ReadStateModel` and `ThreadFollowModel`: marking rooms/threads read, listing viewer followed threads with hydrated root-message render data, and following/unfollowing threads.

### Endpoint Inventory

ConnectRPC unary RPCs are mounted at these HTTP paths. The service and method names come from `proto/chatto/api/v1/*.proto`; the final route is `connectapi.Prefix` (`/api/connect`) plus the generated Connect service path.

| Endpoint                                                                                     | Service                          | RPC                             | Auth / authorization                                                            | Description                                                                                   |
| -------------------------------------------------------------------------------------------- | -------------------------------- | ------------------------------- | ------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `/api/connect/chatto.api.v1.ServerService/GetServer`                                         | `ServerService`                  | `GetServer`                     | Public                                                                          | Public server metadata: name, version, auth methods/providers, registration state, OAuth URL. |
| `/api/connect/chatto.api.v1.ServerStateService/GetServerState`                               | `ServerStateService`             | `GetServerState`                | Authenticated.                                                                  | Authenticated server profile/runtime state and viewer server capability flags.                |
| `/api/connect/chatto.api.v1.ServerStateService/UpdateServerConfig`                           | `ServerStateService`             | `UpdateServerConfig`            | Authenticated; requires `server.manage`.                                        | Update runtime-editable server profile settings and publish a server-updated live event.      |
| `/api/connect/chatto.api.v1.ServerStateService/UploadServerLogo`                             | `ServerStateService`             | `UploadServerLogo`              | Authenticated; requires `server.manage`.                                        | Upload, resize, store, and set the public server logo.                                       |
| `/api/connect/chatto.api.v1.ServerStateService/DeleteServerLogo`                             | `ServerStateService`             | `DeleteServerLogo`              | Authenticated; requires `server.manage`.                                        | Clear the public server logo.                                                                 |
| `/api/connect/chatto.api.v1.ServerStateService/UploadServerBanner`                           | `ServerStateService`             | `UploadServerBanner`            | Authenticated; requires `server.manage`.                                        | Upload, resize, store, and set the public server banner.                                     |
| `/api/connect/chatto.api.v1.ServerStateService/DeleteServerBanner`                           | `ServerStateService`             | `DeleteServerBanner`            | Authenticated; requires `server.manage`.                                        | Clear the public server banner.                                                               |
| `/api/connect/chatto.api.v1.ServerStateService/GetServerSecurityConfig`                      | `ServerStateService`             | `GetServerSecurityConfig`       | Authenticated; requires `server.manage`.                                        | Read security-sensitive server configuration such as blocked usernames.                       |
| `/api/connect/chatto.api.v1.ServerStateService/UpdateBlockedUsernames`                       | `ServerStateService`             | `UpdateBlockedUsernames`        | Authenticated; requires `server.manage`.                                        | Update the newline-separated blocked-username list.                                          |
| `/api/connect/chatto.api.v1.AccountService/UpdateProfile`                                    | `AccountService`                 | `UpdateProfile`                 | Authenticated self-service.                                                     | Update the caller's display name and/or login.                                                |
| `/api/connect/chatto.api.v1.AccountService/UploadAvatar`                                     | `AccountService`                 | `UploadAvatar`                  | Authenticated self-service.                                                     | Upload, resize, store, and set the caller's avatar image.                                     |
| `/api/connect/chatto.api.v1.AccountService/DeleteAvatar`                                     | `AccountService`                 | `DeleteAvatar`                  | Authenticated self-service.                                                     | Delete the caller's avatar; idempotent when no avatar is set.                                 |
| `/api/connect/chatto.api.v1.AccountService/UpdateSettings`                                   | `AccountService`                 | `UpdateSettings`                | Authenticated self-service.                                                     | Update the caller's display preferences.                                                      |
| `/api/connect/chatto.api.v1.AccountService/RequestAccountDeletion`                           | `AccountService`                 | `RequestAccountDeletion`        | Authenticated self-service.                                                     | Issue a short-lived self-deletion confirmation token.                                         |
| `/api/connect/chatto.api.v1.AccountService/DeleteMyAccount`                                  | `AccountService`                 | `DeleteMyAccount`               | Authenticated self-service with confirmation token.                              | Permanently delete the authenticated account.                                                 |
| `/api/connect/chatto.api.v1.AdminDiagnosticsService/GetSystemInfo`                           | `AdminDiagnosticsService`        | `GetSystemInfo`                 | Authenticated; owner-only.                                                     | Read operator diagnostics for NATS, JetStream, server counts, and projections.                |
| `/api/connect/chatto.api.v1.AdminEventLogService/ListEvents`                                 | `AdminEventLogService`           | `ListEvents`                    | Authenticated; requires `admin.view-audit`.                                    | Read newest-first EVT diagnostic pages with optional bounded filters.                         |
| `/api/connect/chatto.api.v1.AdminEventLogService/ListEventTypes`                             | `AdminEventLogService`           | `ListEventTypes`                | Authenticated; requires `admin.view-audit`.                                    | List durable event type labels usable for event-log filtering.                                |
| `/api/connect/chatto.api.v1.AdminEventLogService/GetEvent`                                   | `AdminEventLogService`           | `GetEvent`                      | Authenticated; requires `admin.view-audit`.                                    | Read one diagnostic EVT entry by stream sequence, including JSON payload.                     |
| `/api/connect/chatto.api.v1.AdminUserManagementService/ListMembers`                           | `AdminUserManagementService`     | `ListMembers`                  | Authenticated.                                                                 | List server members for the admin members screen.                                            |
| `/api/connect/chatto.api.v1.AdminUserManagementService/GetMember`                             | `AdminUserManagementService`     | `GetMember`                    | Authenticated.                                                                 | Read one server member plus role and permission metadata for admin details.                   |
| `/api/connect/chatto.api.v1.AdminUserManagementService/AssignRole`                            | `AdminUserManagementService`     | `AssignRole`                   | Authenticated; requires `role.assign`.                                         | Assign a server role to a user.                                                              |
| `/api/connect/chatto.api.v1.AdminUserManagementService/RevokeRole`                            | `AdminUserManagementService`     | `RevokeRole`                   | Authenticated; requires `role.assign`; cannot revoke caller's own owner/admin role. | Revoke a server role from a user.                                                        |
| `/api/connect/chatto.api.v1.AdminUserManagementService/UpdateUser`                           | `AdminUserManagementService`     | `UpdateUser`                    | Authenticated; self-target or requires `role.assign`.                          | Update a user's login and/or display name as an admin action.                                 |
| `/api/connect/chatto.api.v1.AdminUserManagementService/ClearUsernameCooldown`                 | `AdminUserManagementService`     | `ClearUsernameCooldown`         | Authenticated; self-target or requires `role.assign`.                          | Clear a user's self-service username-change cooldown.                                        |
| `/api/connect/chatto.api.v1.RoleService/ListRoles`                                            | `RoleService`                    | `ListRoles`                     | Authenticated.                                                                  | List the role catalog and viewer role-management capabilities.                               |
| `/api/connect/chatto.api.v1.RoleService/GetRole`                                              | `RoleService`                    | `GetRole`                       | Authenticated; role roster is included only with `role.assign`.                 | Read one role plus optional assigned-user roster for admin role details.                      |
| `/api/connect/chatto.api.v1.RoleService/CreateRole`                                           | `RoleService`                    | `CreateRole`                    | Authenticated; requires `role.manage`.                                         | Create a custom server role.                                                                 |
| `/api/connect/chatto.api.v1.RoleService/UpdateRole`                                           | `RoleService`                    | `UpdateRole`                    | Authenticated; requires `role.manage`.                                         | Update role display name, description, and optional pingability.                              |
| `/api/connect/chatto.api.v1.RoleService/DeleteRole`                                           | `RoleService`                    | `DeleteRole`                    | Authenticated; requires `role.manage`; system roles cannot be deleted.          | Delete a custom server role.                                                                 |
| `/api/connect/chatto.api.v1.RoleService/ReorderRoles`                                         | `RoleService`                    | `ReorderRoles`                  | Authenticated; requires `role.manage`; list must contain every custom role exactly once. | Replace custom server role ordering while system roles keep fixed positions.          |
| `/api/connect/chatto.api.v1.PermissionService/GetRolePermissionTierMatrix`                     | `PermissionService`              | `GetRolePermissionTierMatrix`   | Authenticated; requires `role.manage`, except room scope also allows scoped `room.manage`. | Read server/group/room role permission matrix data for the permissions editor.          |
| `/api/connect/chatto.api.v1.PermissionService/GetRolePermissionMatrix`                         | `PermissionService`              | `GetRolePermissionMatrix`       | Authenticated; requires `role.manage`.                                         | Read one role's permission matrix across server, group, and room scopes.                      |
| `/api/connect/chatto.api.v1.PermissionService/GetUserPermissionMatrix`                         | `PermissionService`              | `GetUserPermissionMatrix`       | Authenticated; requires `user.manage-permissions`.                             | Read one user's direct override and effective permission matrix.                              |
| `/api/connect/chatto.api.v1.PermissionService/ExplainPermissions`                              | `PermissionService`              | `ExplainPermissions`            | Authenticated; requires `role.manage`; self-inspection is rejected.             | Explain permission resolution for another user at server or room scope.                       |
| `/api/connect/chatto.api.v1.PermissionService/SetRolePermission`                               | `PermissionService`              | `SetRolePermission`             | Authenticated; requires `role.manage`, except room scope also allows scoped `room.manage`; owner role is immutable. | Set or clear one role permission decision.                                      |
| `/api/connect/chatto.api.v1.PermissionService/RevokeRolePermissionGrant`                       | `PermissionService`              | `RevokeRolePermissionGrant`     | Authenticated; requires `role.manage`; owner role is immutable.                 | Remove only one server-scope allow grant from a role, preserving any explicit denial.          |
| `/api/connect/chatto.api.v1.PermissionService/SetUserPermission`                               | `PermissionService`              | `SetUserPermission`             | Authenticated; requires `user.manage-permissions`.                             | Set or clear one direct user permission decision.                                             |
| `/api/connect/chatto.api.v1.MessageService/PostMessage`                                      | `MessageService`                 | `PostMessage`                   | Authenticated; requires room membership and post/thread/attach/echo permissions as applicable. | Post a root message or thread reply with text, link preview, uploaded attachment bytes, or existing attachment asset IDs, returning either a renderable event or large-mention confirmation challenge. |
| `/api/connect/chatto.api.v1.MessageService/UpdateMessage`                                    | `MessageService`                 | `UpdateMessage`                 | Authenticated; requires room membership; non-authors require `message.manage`; echo-state changes are author-only and may require `message.echo` + `message.post`. | Edit a message body and optionally reconcile a thread reply's channel echo state.             |
| `/api/connect/chatto.api.v1.MessageService/DeleteMessage`                                    | `MessageService`                 | `DeleteMessage`                 | Authenticated; requires room membership; non-authors require `message.manage`.  | Retract a message body while keeping the durable audit fact.                                  |
| `/api/connect/chatto.api.v1.MessageService/DeleteAttachment`                                 | `MessageService`                 | `DeleteAttachment`              | Authenticated; requires room membership and message authorship.                 | Remove one attachment from the author's message and delete its asset best-effort.             |
| `/api/connect/chatto.api.v1.MessageService/DeleteLinkPreview`                                | `MessageService`                 | `DeleteLinkPreview`             | Authenticated; requires room membership and message authorship.                 | Remove the accepted link preview from the author's message.                                   |
| `/api/connect/chatto.api.v1.MessageService/SendTypingIndicator`                              | `MessageService`                 | `SendTypingIndicator`           | Authenticated; requires room membership.                                        | Publish a transient room/thread typing indicator; no posting permission is required.          |
| `/api/connect/chatto.api.v1.AttachmentService/ListRoomAttachments`                           | `AttachmentService`              | `ListRoomAttachments`           | Authenticated; requires room membership.                                        | List current attachment-bearing room messages for the room files/media panel.                 |
| `/api/connect/chatto.api.v1.AttachmentService/RefreshMessageAttachmentUrls`                   | `AttachmentService`              | `RefreshMessageAttachmentUrls`  | Authenticated; requires room membership and a current message body.             | Refresh signed attachment, thumbnail, video thumbnail, and video variant URLs for a message.  |
| `/api/connect/chatto.api.v1.LinkPreviewService/FetchLinkPreview`                             | `LinkPreviewService`             | `FetchLinkPreview`              | Authenticated.                                                                  | Fetch and cache Open Graph metadata for a composer URL, returning nullable preview metadata.  |
| `/api/connect/chatto.api.v1.PresenceService/ReportPresence`                                  | `PresenceService`                | `ReportPresence`                | Authenticated self-service.                                                     | Refresh the caller's live presence as online, away, or do-not-disturb; `user_selected` protects explicit Away/DND from automatic overwrites. |
| `/api/connect/chatto.api.v1.UserService/GetUser`                                             | `UserService`                    | `GetUser`                       | Authenticated.                                                                  | Read one public user profile by stable ID, including presence, custom status, avatar URL, and roles. |
| `/api/connect/chatto.api.v1.UserService/GetUserByLogin`                                      | `UserService`                    | `GetUserByLogin`                | Authenticated; returns `NOT_FOUND` when not found.                              | Read one public user profile by login identifier.                                             |
| `/api/connect/chatto.api.v1.UserService/BatchGetUsers`                                       | `UserService`                    | `BatchGetUsers`                 | Authenticated; unknown IDs are omitted.                                         | Read lightweight user summaries in one request for frontend cache misses.                    |
| `/api/connect/chatto.api.v1.UserStatusService/SetCustomStatus`                               | `UserStatusService`              | `SetCustomStatus`               | Authenticated self-service.                                                     | Write the current user's custom status through durable user EVT.                               |
| `/api/connect/chatto.api.v1.UserStatusService/ClearCustomStatus`                             | `UserStatusService`              | `ClearCustomStatus`             | Authenticated self-service.                                                     | Clear the current user's custom status through durable user EVT.                               |
| `/api/connect/chatto.api.v1.NotificationPreferencesService/GetRoomNotificationPreference`     | `NotificationPreferencesService` | `GetRoomNotificationPreference` | Authenticated; requires channel-room membership.                                | Read the viewer's room notification preference and effective level.                            |
| `/api/connect/chatto.api.v1.NotificationPreferencesService/SetRoomNotificationLevel`          | `NotificationPreferencesService` | `SetRoomNotificationLevel`      | Authenticated; requires channel-room membership.                                | Write the viewer's room notification level through `NotificationPreferencesService`.           |
| `/api/connect/chatto.api.v1.ReadStateService/MarkRoomAsRead`                                 | `ReadStateService`               | `MarkRoomAsRead`                | Authenticated; requires room membership.                                        | Advance the viewer's room read marker, optionally anchored to a root message event ID.         |
| `/api/connect/chatto.api.v1.ReadStateService/MarkThreadAsRead`                               | `ReadStateService`               | `MarkThreadAsRead`              | Authenticated; requires room membership and a valid non-echo thread root event. | Advance the viewer's thread read marker, optionally anchored to a message in that thread.      |
| `/api/connect/chatto.api.v1.ReactionService/AddReaction`                                     | `ReactionService`                | `AddReaction`                   | Authenticated; requires room membership and `message.react`.                    | Add the viewer's reaction to a message event; returns false if it already existed.             |
| `/api/connect/chatto.api.v1.ReactionService/RemoveReaction`                                  | `ReactionService`                | `RemoveReaction`                | Authenticated; requires room membership and `message.react`.                    | Remove the viewer's reaction from a message event; returns false if it did not exist.          |
| `/api/connect/chatto.api.v1.RoomService/CreateRoom`                                          | `RoomService`                    | `CreateRoom`                    | Authenticated; requires `room.create` in the target group.                      | Create a channel room in a room group, optionally with Universal enabled.                      |
| `/api/connect/chatto.api.v1.RoomService/UpdateRoom`                                          | `RoomService`                    | `UpdateRoom`                    | Authenticated; requires room-management capability.                             | Update a room's name and description.                                                         |
| `/api/connect/chatto.api.v1.RoomService/ArchiveRoom`                                         | `RoomService`                    | `ArchiveRoom`                   | Authenticated; requires room-management capability.                             | Archive a room so it is hidden from active room lists.                                        |
| `/api/connect/chatto.api.v1.RoomService/UnarchiveRoom`                                       | `RoomService`                    | `UnarchiveRoom`                 | Authenticated; requires room-management capability.                             | Restore an archived room to active room lists.                                                |
| `/api/connect/chatto.api.v1.RoomService/SetRoomUniversal`                                    | `RoomService`                    | `SetRoomUniversal`              | Authenticated; requires room-management capability; channel rooms only.         | Change whether a channel grants effective membership to eligible server members.              |
| `/api/connect/chatto.api.v1.RoomService/JoinRoom`                                            | `RoomService`                    | `JoinRoom`                      | Authenticated; requires `room.join` for the target room.                        | Join the target room as the current user.                                                     |
| `/api/connect/chatto.api.v1.RoomService/StartDM`                                             | `RoomService`                    | `StartDM`                       | Authenticated; requires DM start capability via `message.post` not being denied. | Start or fetch a direct-message room for the current user and participant set.                |
| `/api/connect/chatto.api.v1.RoomService/LeaveRoom`                                           | `RoomService`                    | `LeaveRoom`                     | Authenticated; rejected for DM and Universal rooms.                             | Leave the target room as the current user.                                                    |
| `/api/connect/chatto.api.v1.RoomService/ListRoomBans`                                        | `RoomService`                    | `ListRoomBans`                  | Authenticated; requires `room.ban-member`.                                      | List active room bans, optionally filtered to one channel room.                               |
| `/api/connect/chatto.api.v1.RoomService/BanRoomMember`                                       | `RoomService`                    | `BanRoomMember`                 | Authenticated; requires `room.ban-member`; channel rooms only.                  | Ban a current room member with a required reason and optional expiry.                         |
| `/api/connect/chatto.api.v1.RoomService/UnbanRoomMember`                                     | `RoomService`                    | `UnbanRoomMember`               | Authenticated; requires `room.ban-member`; channel rooms only.                  | Remove an active room ban with a required reason.                                             |
| `/api/connect/chatto.api.v1.RoomDirectoryService/ListRooms`                                  | `RoomDirectoryService`           | `ListRooms`                     | Authenticated; non-archived channel visibility follows membership or `room.list`; DMs require membership. | List rooms visible to the viewer with per-room viewer state and capabilities.                 |
| `/api/connect/chatto.api.v1.RoomDirectoryService/ListRoomGroups`                             | `RoomDirectoryService`           | `ListRoomGroups`                | Authenticated; group room entries are filtered to non-archived visible channel rooms. | List ordered room groups with visible room entries and sidebar links.                         |
| `/api/connect/chatto.api.v1.RoomDirectoryService/GetRoom`                                    | `RoomDirectoryService`           | `GetRoom`                       | Authenticated; requires channel visibility or DM membership.                    | Refresh one visible room by ID with viewer state and capabilities.                            |
| `/api/connect/chatto.api.v1.RoomDirectoryService/JoinGroup`                                  | `RoomDirectoryService`           | `JoinGroup`                     | Authenticated; joins only unarchived rooms where `room.join` allows the viewer. | Join every joinable room in a group, skipping already-joined and non-joinable rooms.          |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/ListAdminRoomLayout`                      | `AdminRoomLayoutService`         | `ListAdminRoomLayout`           | Authenticated; rooms are filtered to caller-visible channel rooms.              | List ordered admin room groups with room metadata and mixed sidebar items.                    |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/CreateRoomGroup`                          | `AdminRoomLayoutService`         | `CreateRoomGroup`               | Authenticated; requires `role.manage`.                                         | Create a room group.                                                                          |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/UpdateRoomGroup`                          | `AdminRoomLayoutService`         | `UpdateRoomGroup`               | Authenticated; requires `role.manage`.                                         | Update room group metadata.                                                                   |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/DeleteRoomGroup`                          | `AdminRoomLayoutService`         | `DeleteRoomGroup`               | Authenticated; requires `role.manage`; group must be empty.                    | Delete an empty room group.                                                                   |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/ReorderRoomGroups`                        | `AdminRoomLayoutService`         | `ReorderRoomGroups`             | Authenticated; requires `role.manage`; IDs must be a full permutation.         | Replace the global room group order.                                                          |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/MoveRoomToGroup`                          | `AdminRoomLayoutService`         | `MoveRoomToGroup`               | Authenticated; requires group-scoped `room.manage` in source and target groups. | Move a channel room to another group with source-change protection.                           |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/ReorderSidebarItemsInGroup`               | `AdminRoomLayoutService`         | `ReorderSidebarItemsInGroup`    | Authenticated; requires group-scoped `room.manage`; entries must match current set. | Replace mixed room/sidebar-link order inside a group.                                    |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/CreateSidebarLink`                        | `AdminRoomLayoutService`         | `CreateSidebarLink`             | Authenticated; requires group-scoped `room.manage` in the target group.        | Create a sidebar link in a room group.                                                        |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/UpdateSidebarLink`                        | `AdminRoomLayoutService`         | `UpdateSidebarLink`             | Authenticated; requires group-scoped `room.manage` in the link's current group. | Update sidebar link label or URL.                                                             |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/DeleteSidebarLink`                        | `AdminRoomLayoutService`         | `DeleteSidebarLink`             | Authenticated; requires group-scoped `room.manage` in the link's current group. | Delete a sidebar link.                                                                        |
| `/api/connect/chatto.api.v1.AdminRoomLayoutService/MoveSidebarLinkToGroup`                   | `AdminRoomLayoutService`         | `MoveSidebarLinkToGroup`        | Authenticated; requires group-scoped `room.manage` in source and target groups. | Move a sidebar link to another room group.                                                    |
| `/api/connect/chatto.api.v1.RoomTimelineService/GetRoomEvents`                                | `RoomTimelineService`            | `GetRoomEvents`                 | Authenticated; requires room membership.                                        | Latest/before/after main-room timeline page with hydrated render data.                         |
| `/api/connect/chatto.api.v1.RoomTimelineService/GetRoomEventsAround`                          | `RoomTimelineService`            | `GetRoomEventsAround`           | Authenticated; requires room membership.                                        | Main-room timeline window centered on a visible target event.                                  |
| `/api/connect/chatto.api.v1.RoomTimelineService/ResolveMessageLinkTarget`                     | `RoomTimelineService`            | `ResolveMessageLinkTarget`      | Authenticated; requires room membership.                                        | Resolve a message permalink target, including thread-only replies, to room or thread context. |
| `/api/connect/chatto.api.v1.RoomTimelineService/GetThreadEvents`                              | `RoomTimelineService`            | `GetThreadEvents`               | Authenticated; requires room membership and a valid non-echo thread root event. | Thread root plus latest/before/after reply page with hydrated render data.                     |
| `/api/connect/chatto.api.v1.RoomTimelineService/GetThreadEventsAround`                        | `RoomTimelineService`            | `GetThreadEventsAround`         | Authenticated; requires room membership and a valid non-echo thread root event. | Thread root plus reply window centered on a root or reply anchor event.                        |
| `/api/connect/chatto.api.v1.ThreadService/ListFollowedThreads`                                | `ThreadService`                  | `ListFollowedThreads`           | Authenticated.                                                                  | List viewer followed threads with room names, unread metadata, root-message render data, and user includes. |
| `/api/connect/chatto.api.v1.ThreadService/FollowThread`                                      | `ThreadService`                  | `FollowThread`                  | Authenticated; requires room membership and a valid non-echo thread root event. | Mark the viewer as following a thread and emit the user-scoped follow-state sync event.        |
| `/api/connect/chatto.api.v1.ThreadService/UnfollowThread`                                    | `ThreadService`                  | `UnfollowThread`                | Authenticated; requires room membership and a valid non-echo thread root event. | Remove the viewer's thread follow marker and emit the user-scoped follow-state sync event.     |

## Realtime WebSocket API Overview

Key files: [`proto/chatto/api/v1/realtime.proto`](../proto/chatto/api/v1/realtime.proto), [`cli/internal/http_server/realtime.go`](../cli/internal/http_server/realtime.go), [`cli/internal/core/my_events_model.go`](../cli/internal/core/my_events_model.go)

The protobuf realtime API is mounted at `GET /api/realtime` and upgrades to a binary WebSocket protocol. The first client frame must be `RealtimeClientFrame.hello`; the server accepts protocol version 1, authenticates either the hello bearer token or the existing cookie session, and replies with `RealtimeServerFrame.hello`. The second client frame must be `subscribe_events`, after which the server sends `subscribed` and starts forwarding authorized `RealtimeEventEnvelope` frames plus application-level heartbeats. Clients can send `ping` frames and receive `pong`. The v1 protocol is live-only and does not expose acknowledgement frames, resume requests, or event cursors.

The HTTP handler does not implement independent room/RBAC filtering. After authentication it calls `core.StreamMyEventsWithOptions` with presence self-reporting disabled, then maps the already-authorized core envelope into public `chatto.api.v1` realtime frames. This keeps room membership, DM privacy, server/user/config event gates, projection readiness, live membership changes, slow-consumer shutdown, and session termination behavior shared with `core.StreamMyEvents`. Realtime events are public API signals, not raw persisted `corev1.Event` or `corev1.LiveEvent` payloads.

Clients recover missed state with projected reads, the same projected-read catch-up model. There is no per-connection JetStream consumer and no public subscription replay cursor.

| Endpoint        | Frame schema                          | Auth / authorization                                                                 | Description |
| --------------- | ------------------------------------- | ------------------------------------------------------------------------------------ | ----------- |
| `/api/realtime` | `chatto.api.v1.Realtime*` binary protobuf frames | Bearer token in hello or cookie auth; delivery delegated to `StreamMyEvents` for per-event authorization. | Live-only authenticated event stream for messages, reactions, typing, presence, rooms, notifications, read state, server/user profile invalidation, and session termination. |

## Architecture Pattern: Event-Sourced Writes

### Write Path

| Type    | Resource                      | Purpose                                     |
| ------- | ----------------------------- | ------------------------------------------- |
| Stream  | `EVT`                         | Durable event-sourcing log for domain facts |
| KV      | `RUNTIME_STATE`               | Persisted latest-value runtime/user state, including pending notifications, push subscriptions, auth/workflow tokens, and wrapped app DEK records |
| KV      | `MEMORY_CACHE`                | Volatile memory-backed cache state for presence and short-lived leader leases; excluded from backups |
| KV      | `ENCRYPTION_KEYS`             | KMS key-encryption keys and per-call LiveKit E2EE keys; excluded from backups |
| Objects | `SERVER_ASSETS` or S3 bucket  | Persisted asset binaries                    |
| Objects | `ASSET_CACHE`                 | Optional cached image transforms with TTL   |

See [NATS Resource Inventory](#nats-resource-inventory) for detailed key patterns and subjects.

`EVT` publishing is mandatory for event-sourced domain facts because `EVT`
is the source of truth and reads come from in-memory projections. If event
publishing fails, the write fails. Current aggregates include room
membership/metadata, room groups/layout, server config, users,
messages/threads, reactions, voice call participation, assets, RBAC, and
auth workflow audit facts.

### Consistency Model

**Latest-value KV/runtime state:**

- Strong consistency for KV operations
- Read-your-writes guaranteed via immediate KV updates
- Per-key TTLs are used for expiring records such as notifications and auth/workflow tokens
- These records are operational state, not durable domain history

**Event-sourced aggregates:**

- `EVT` is the source of truth.
- Fresh deployments seed current invariants such as default RBAC roles, the default room group, and default channel rooms. The seeded `#announcements` room is Universal; `#general` is a normal channel room. Fresh RBAC seeds include `message.attach` for `everyone`; existing RBAC state is not silently backfilled on boot.
- Reads come from in-memory projections rebuilt from `EVT`.
- Room timeline reads use `RoomTimelineProjection`'s visible per-room timeline for initial loads, forward/backward pagination, and around-message windows; `Room.attachments` uses the projection's current attachment-bearing message index so it does not decrypt unrelated message bodies. Folded room facts such as edits, retractions, reactions, and thread replies are handled by derived indexes or sibling projections instead of being retained in the per-room timeline slice. Asset lifecycle facts live in `AssetProjection`, which also consumes legacy beta `evt.room.{roomId}.asset_*` facts. Live `Subscription.myEvents` delivery reads the committed EVT feed, waits for projection readiness, and emits authorized events without exposing folded facts as standalone timeline rows in `Room.events`.
- Writes append to `EVT` only for durable domain facts; legacy KV/stream data is not maintained as a mirror.
- Mutations whose decision comes from a projection use a snapshot that carries both derived state and the applied stream sequence for the same OCC subject/filter. On conflict, writers wait for the owning projection to the latest matching tail and retry from a fresh snapshot.
- Read-your-writes is provided by waiting for the local projector to reach the append sequence.

- KV buckets remain strongly consistent (NATS JetStream R3 replication)
- `EVT` provides durable audit/history and projection replay; transient live events provide UI sync.
- `EVT` retention is effectively forever until snapshot/archival policy is designed.
- `RUNTIME_STATE` can be rebuilt only from current operational exports or fresh user action, not from `EVT`, by design.

## Roles, Permissions, and Direct Messages

These sections previously described the RBAC model and DM behavior in detail. They've moved:

- **Roles, permissions, and the resolver** — see [FDR-001](fdr/FDR-001-roles-and-permissions.md) for the design and rationale, and [`cli/AGENTS.md`](../cli/AGENTS.md) for resolver semantics (DM boundary, user-level overrides, scope cascade) and the admin-side picture.
- **Permission constants and `Can*` functions** — see [`cli/internal/core/permission.go`](../cli/internal/core/permission.go) and [`cli/internal/core/can.go`](../cli/internal/core/can.go).
- **Direct Messages** — see [FDR-007](fdr/FDR-007-direct-messages.md) and [ADR-037 (DM Access via Membership)](adr/ADR-037-dm-access-via-membership.md).
- **Storage layout for RBAC and DM rooms** — captured in the [NATS Resource Inventory](#nats-resource-inventory) below.

## NATS Resource Inventory

Key files: [`cli/internal/core/core.go`](../cli/internal/core/core.go), [`cli/internal/events/subjects.go`](../cli/internal/events/subjects.go), [`proto/chatto/core/v1/event.proto`](../proto/chatto/core/v1/event.proto), [`cli/internal/core/subjects/subjects.go`](../cli/internal/core/subjects/subjects.go)

### Current Resources

| Type         | Name                | Storage | Backup | Description                                                                 |
| ------------ | ------------------- | ------- | ------ | --------------------------------------------------------------------------- |
| Stream       | `EVT`               | File    | Yes    | Event-sourcing log for durable `corev1.Event` facts on `evt.>`              |
| KV bucket    | `RUNTIME_STATE`     | File    | Yes    | Persisted latest-value runtime state, auth/session tokens, notifications, wrapped app DEKs |
| KV bucket    | `MEMORY_CACHE`      | Memory  | No     | Volatile presence and short-lived leader leases                             |
| KV bucket    | `ENCRYPTION_KEYS`   | File    | No     | KMS key-encryption keys and per-call LiveKit E2EE keys; excluded from backups |
| Object store | `SERVER_ASSETS`     | File    | Yes    | Default/legacy NATS-backed persisted asset binaries                         |
| Object store | `ASSET_CACHE`       | File    | No     | Optional TTL cache for transformed image bytes                               |
| NATS Core    | `live.sync.>`       | None    | No     | Transient `corev1.LiveEvent` pubsub signals                                  |
| Republish    | `live.evt.>`        | None    | No     | Raw committed `EVT` facts republished by JetStream for server-side live delivery |

### Event Envelopes

Chatto uses `corev1.Event` as the durable EVT wrapper and `corev1.LiveEvent` as the transient NATS Core wrapper. The realtime API maps both through public protobuf frames, while the protobuf wire envelopes stay separate so live-only sync signals cannot leak into the durable audit/event log shape.

- **Wrapper fields**: `id`, `created_at`, `actor_id`
- **Concrete event**: `event` oneof on the relevant wire envelope; contextual fields (`roomId`, etc.) live on the concrete payloads.

The active `Event.event` oneof variants are all durable EVT payloads, regardless of numeric tag. Transient-only pubsub signals belong in `corev1.LiveEvent`, not `corev1.Event`.

Existing `Event` oneof field numbers are part of the persisted JetStream wire format; do not renumber or reuse them.

**Proto File Organization:**

| File | Contents | Safety |
| ---- | -------- | ------ |
| `event.proto` | Durable `Event` wrapper + persisted event message definitions | Changing field numbers/structure affects JetStream-stored data — requires careful migration |
| `live_events.proto` | Transient `LiveEvent` wrapper + live-only event message definitions | Safe to change freely — these are never persisted |

Both files share `package chatto.core.v1` and generate into the same Go package. `core.EventEnvelope` is the in-process realtime delivery interface that can carry durable EVT, transient LiveEvent, or a heartbeat through private concrete implementations.

**Event Categories:**

| Category                    | Storage    | Examples                                                    | Purpose                                                        |
| --------------------------- | ---------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| JetStream-stored (room) | Stream     | RoomCreated, RoomUniversalChanged, MessagePosted, MessageEdited, MessageRetracted, ReactionAdded, ReactionRemoved, UserJoinedRoom, CallStarted, CallParticipantJoined, CallParticipantLeft, CallEnded | Ordering guarantees, historical replay, projection source of truth |
| Room live-only              | NATS Core  | UserTyping | Ephemeral room notifications where another store/projection is source of truth |
| Deployment live (user/config) | NATS Core  | UserCreated, ServerUpdated, MentionNotification, NotificationCreated, PresenceChanged | Cross-tab sync, notifications, server lifecycle |

The distinction between stored and live-only events is explicit in the wire envelope: durable facts use `corev1.Event`, transient signals use `corev1.LiveEvent`. Room queries and server subscriptions are delivery contexts, not separate wrapper types.

**Self-Contained Events:** Each concrete event contains all the IDs and context it needs:

- Room events contain `room_id`.
- Membership events contain relevant IDs (`room_id` for room joins/leaves).
- Self-initiated events (e.g., `PresenceChanged`) use the parent wrapper's `actor_id` instead of duplicating a `user_id` field.

**Event Publishing Strategy:**

User-facing live delivery is built from two internal NATS Core subject roots:

1. **Primary Stream** (persistent):
   - `EVT` (subjects `evt.>`) holds event-sourced domain state. Its stream-level `RePublish` config forwards every committed event once onto `live.evt.>`. This is a raw committed-event feed, not a client contract.
2. **Direct Live Publish** (transient):
   - Transient UI sync signals publish as `corev1.LiveEvent` via NATS Core to `live.sync.>` — no stream storage.

`MyEventsModel` is owned behind the `ChattoCore.StreamMyEvents` facade and subscribes to `live.sync.>` and `live.evt.>`. For deliverable raw EVT room and asset messages, it reads the republished `Nats-Sequence` header, waits for the local projections needed by authorization and follow-up resolvers, filters by the subscribing user, and then emits the authorized event. Asset lifecycle events resolve their room authorization through `AssetProjection`, using the room scope on `AssetCreatedEvent` and inherited parent scope for derivatives. Transient `LiveEvent` messages are adapted at this API boundary into public protobuf `/api/realtime` frames. Both surfaces are live-only; missed state is recovered by projected reads. The bundled web client opens `/api/realtime`, watches server heartbeats for silent stalls, refetches server-scoped projected state after reconnect gaps, and refetches the current room or thread window from projections after browser wake, WebSocket reconnect, socket end, or heartbeat-stall catch-up notifications. There is no per-connection JetStream consumer and no public subscription replay cursor.

### EVT Subject Patterns

| Stream                       | Wrapper          | Scope      | Description                                      |
| ---------------------------- | ---------------- | ---------- | ------------------------------------------------ |
| `EVT`                        | `corev1.Event`   | Server     | Event-sourcing log ([ADR-033](adr/ADR-033-event-sourced-state-with-projections.md) / [ADR-034](adr/ADR-034-single-event-stream.md)). Subjects `evt.{aggregateType}.{aggregateId}.{eventType}`; republishes onto `live.evt.>` as the raw committed-event feed. Stores room membership/metadata, groups/layout, server config, users, messages/threads, reactions, assets, RBAC, and auth workflow audit facts. |
| Live Sync                    | `corev1.LiveEvent` | Transient  | Direct NATS Core pubsub on `live.sync.>` for transient UI sync signals. `StreamMyEvents` authorizes and adapts these messages into realtime events; they are never projection input. |

The republished `live.evt.{aggregateType}.{aggregateId}.{eventType}` subject is an internal server-side feed; `StreamMyEvents` waits for projections and authorization before delivering anything to clients.

| Pattern                                          | Description                                                                     |
| ------------------------------------------------ | ------------------------------------------------------------------------------- |
| `evt.>`                                          | All durable event-sourced facts                                                 |
| `evt.room.>`                                     | All room aggregate facts                                                        |
| `evt.room.{roomId}.{eventType}`                  | One room aggregate fact                                                         |
| `evt.room.*.{eventType}`                         | One room event type across all rooms                                            |
| `evt.asset.>`                                    | All asset aggregate facts                                                       |
| `evt.asset.{assetId}.{eventType}`                | One asset aggregate fact                                                        |
| `evt.asset.*.{eventType}`                        | One asset event type across all assets                                          |
| `evt.config.>`                                   | Dynamic server/user configuration and preferences                               |
| `evt.config.{subject}.{eventType}`               | Config fact for `server`, a user ID, or another configurable subject            |
| `evt.group.{groupId}.{eventType}`                | Room group metadata and group-owned sidebar item ordering/membership facts      |
| `evt.layout.default.{eventType}`                 | Singleton sidebar group ordering facts                                          |
| `evt.user.{userId}.{eventType}`                  | User/account/profile/auth lookup facts and user-scoped auth audit facts         |
| `evt.user.*.{eventType}`                         | One user event type across all users                                            |
| `evt.rbac.{server\|scopeId}.{eventType}`         | Server-level RBAC or scoped RBAC decision facts for a room/group ID             |
| `evt.auth.server.{eventType}`                    | Server-wide auth audit facts before a user aggregate exists                     |
| `live.evt.>`                                     | JetStream republish of committed `EVT` facts                                    |

The aggregate ID is intentionally part of the subject; actor/user and detailed context stay in the protobuf payload. Asset subjects are keyed by asset ID, while room scope lives in `AssetCreatedEvent` and is resolved by `AssetProjection`. Cross-event-type invariants use wildcard OCC filters such as `evt.room.>`, `evt.asset.>`, or `evt.rbac.>`.

### Durable EVT Event Inventory

| Subject pattern                                              | Protobuf event message                              |
| ------------------------------------------------------------ | --------------------------------------------------- |
| `evt.room.{roomId}.room_created`                             | `RoomCreatedEvent`                                  |
| `evt.room.{roomId}.room_updated`                             | `RoomUpdatedEvent`                                  |
| `evt.room.{roomId}.room_archived`                            | `RoomArchivedEvent`                                 |
| `evt.room.{roomId}.room_unarchived`                          | `RoomUnarchivedEvent`                               |
| `evt.room.{roomId}.room_universal_changed`                   | `RoomUniversalChangedEvent`                         |
| `evt.room.{roomId}.room_deleted`                             | `RoomDeletedEvent`                                  |
| `evt.room.{roomId}.user_joined`                              | `UserJoinedRoomEvent`                               |
| `evt.room.{roomId}.user_left`                                | `UserLeftRoomEvent`                                 |
| `evt.room.{roomId}.call_started`                             | `CallStartedEvent`                                  |
| `evt.room.{roomId}.call_joined`                              | `CallParticipantJoinedEvent`                        |
| `evt.room.{roomId}.call_left`                                | `CallParticipantLeftEvent`                          |
| `evt.room.{roomId}.call_ended`                               | `CallEndedEvent`                                    |
| `evt.room.{roomId}.room_member_banned`                       | `RoomMemberBannedEvent`                             |
| `evt.room.{roomId}.room_member_unbanned`                     | `RoomMemberUnbannedEvent`                           |
| `evt.room.{roomId}.message_body`                             | `MessageBodyEvent`                                  |
| `evt.room.{roomId}.message_posted`                           | `MessagePostedEvent`                                |
| `evt.room.{roomId}.message_edited`                           | `MessageEditedEvent`                                |
| `evt.room.{roomId}.message_retracted`                        | `MessageRetractedEvent`                             |
| `evt.room.{roomId}.thread_created`                           | `ThreadCreatedEvent`                                |
| `evt.room.{roomId}.reaction_added`                           | `ReactionAddedEvent`                                |
| `evt.room.{roomId}.reaction_removed`                         | `ReactionRemovedEvent`                              |
| `evt.asset.{assetId}.asset_created`                          | `AssetCreatedEvent`                                 |
| `evt.asset.{assetId}.asset_processing_started`               | `AssetProcessingStartedEvent`                       |
| `evt.asset.{assetId}.asset_processing_succeeded`             | `AssetProcessingSucceededEvent`                     |
| `evt.asset.{assetId}.asset_processing_failed`                | `AssetProcessingFailedEvent`                        |
| `evt.asset.{assetId}.asset_deleted`                          | `AssetDeletedEvent`                                 |
| `evt.config.{subject}.server_name_changed`                   | `ServerNameChangedEvent`                            |
| `evt.config.{subject}.server_description_changed`            | `ServerDescriptionChangedEvent`                     |
| `evt.config.{subject}.server_welcome_message_changed`        | `ServerWelcomeMessageChangedEvent`                  |
| `evt.config.{subject}.server_motd_changed`                   | `ServerMotdChangedEvent`                            |
| `evt.config.{subject}.server_blocked_usernames_changed`      | `ServerBlockedUsernamesChangedEvent`                |
| `evt.config.{subject}.server_logo_set`                       | `ServerLogoSetEvent`                                |
| `evt.config.{subject}.server_logo_cleared`                   | `ServerLogoClearedEvent`                            |
| `evt.config.{subject}.server_banner_set`                     | `ServerBannerSetEvent`                              |
| `evt.config.{subject}.server_banner_cleared`                 | `ServerBannerClearedEvent`                          |
| `evt.config.{subject}.user_timezone_changed`                 | `UserTimezoneChangedEvent`                          |
| `evt.config.{subject}.user_timezone_cleared`                 | `UserTimezoneClearedEvent`                          |
| `evt.config.{subject}.user_time_format_changed`              | `UserTimeFormatChangedEvent`                        |
| `evt.config.{subject}.user_time_format_cleared`              | `UserTimeFormatClearedEvent`                        |
| `evt.config.{subject}.user_server_notification_level_set`    | `UserServerNotificationLevelSetEvent`               |
| `evt.config.{subject}.user_server_notification_level_cleared` | `UserServerNotificationLevelClearedEvent`          |
| `evt.config.{subject}.user_room_notification_level_set`      | `UserRoomNotificationLevelSetEvent`                 |
| `evt.config.{subject}.user_room_notification_level_cleared`  | `UserRoomNotificationLevelClearedEvent`             |
| `evt.group.{groupId}.group_created`                         | `RoomGroupCreatedEvent`                             |
| `evt.group.{groupId}.group_updated`                         | `RoomGroupUpdatedEvent`                             |
| `evt.group.{groupId}.group_deleted`                         | `RoomGroupDeletedEvent`                             |
| `evt.group.{groupId}.room_added`                            | `RoomAddedToGroupEvent`                             |
| `evt.group.{groupId}.room_removed`                          | `RoomRemovedFromGroupEvent`                         |
| `evt.group.{groupId}.rooms_reordered`                       | `RoomsInGroupReorderedEvent`                        |
| `evt.group.{groupId}.sidebar_link_added`                    | `SidebarLinkAddedToGroupEvent`                      |
| `evt.group.{groupId}.sidebar_link_updated`                  | `SidebarLinkUpdatedEvent`                           |
| `evt.group.{groupId}.sidebar_link_removed`                  | `SidebarLinkRemovedFromGroupEvent`                  |
| `evt.group.{groupId}.sidebar_entries_reordered`             | `SidebarGroupEntriesReorderedEvent`                 |
| `evt.layout.default.groups_reordered`                        | `RoomGroupsReorderedEvent`                          |
| `evt.user.{userId}.account_created`                         | `UserAccountCreatedEvent`                           |
| `evt.user.{userId}.login_changed`                           | `UserLoginChangedEvent`                             |
| `evt.user.{userId}.display_name_changed`                    | `UserDisplayNameChangedEvent`                       |
| `evt.user.{userId}.avatar_set`                              | `UserAvatarSetEvent`                                |
| `evt.user.{userId}.avatar_cleared`                          | `UserAvatarClearedEvent`                            |
| `evt.user.{userId}.custom_status_set`                       | `UserCustomStatusSetEvent`                          |
| `evt.user.{userId}.custom_status_cleared`                   | `UserCustomStatusClearedEvent`                      |
| `evt.user.{userId}.verified_email_added`                    | `UserVerifiedEmailAddedEvent`                       |
| `evt.user.{userId}.password_hash_changed`                   | `UserPasswordHashChangedEvent`                      |
| `evt.user.{userId}.oidc_subject_linked`                     | `UserOIDCSubjectLinkedEvent` (legacy replay)        |
| `evt.user.{userId}.external_identity_linked`                | `UserExternalIdentityLinkedEvent`                   |
| `evt.user.{userId}.server_preferences_changed`              | `UserServerPreferencesChangedEvent`                 |
| `evt.user.{userId}.login_cooldown_started`                  | `UserLoginCooldownStartedEvent`                     |
| `evt.user.{userId}.login_cooldown_cleared`                  | `UserLoginCooldownClearedEvent`                     |
| `evt.user.{userId}.account_deleted`                         | `UserAccountDeletedEvent`                           |
| `evt.user.{userId}.user_key_shredded`                       | `UserKeyShreddedEvent`                              |
| `evt.user.{userId}.dek_generated`                           | `UserDEKGeneratedEvent`                             |
| `evt.user.{userId}.email_verification_code_issued`          | `EmailVerificationCodeIssuedEvent`                  |
| `evt.user.{userId}.password_reset_link_issued`              | `PasswordResetLinkIssuedEvent`                      |
| `evt.user.{userId}.account_deletion_confirmation_issued`    | `AccountDeletionConfirmationIssuedEvent`            |
| `evt.user.{userId}.password_reset_completed`                | `PasswordResetCompletedEvent`                       |
| `evt.user.{userId}.login_succeeded`                         | `LoginSucceededEvent`                               |
| `evt.user.{userId}.logout_succeeded`                        | `LogoutSucceededEvent`                              |
| `evt.user.{userId}.auth_code_issued`                        | `AuthCodeIssuedEvent`                               |
| `evt.user.{userId}.auth_code_exchange_succeeded`            | `AuthCodeExchangeSucceededEvent`                    |
| `evt.user.{userId}.auth_code_exchange_failed`               | `AuthCodeExchangeFailedEvent`                       |
| `evt.user.{userId}.bearer_token_issued`                     | `BearerTokenIssuedEvent`                            |
| `evt.user.{userId}.bearer_token_revoked`                    | `BearerTokenRevokedEvent`                           |
| `evt.user.{userId}.oauth_consent_granted`                   | `OAuthConsentGrantedEvent`                          |
| `evt.user.{userId}.oauth_consent_denied`                    | `OAuthConsentDeniedEvent`                           |
| `evt.rbac.{server\|scopeId}.role_created`                   | `RbacRoleCreatedEvent`                             |
| `evt.rbac.{server\|scopeId}.role_display_name_changed`      | `RbacRoleDisplayNameChangedEvent`                  |
| `evt.rbac.{server\|scopeId}.role_description_changed`       | `RbacRoleDescriptionChangedEvent`                  |
| `evt.rbac.{server\|scopeId}.role_pingable_changed`          | `RbacRolePingableChangedEvent`                     |
| `evt.rbac.{server\|scopeId}.role_deleted`                   | `RbacRoleDeletedEvent`                             |
| `evt.rbac.{server\|scopeId}.roles_reordered`                | `RbacRolesReorderedEvent`                          |
| `evt.rbac.{server\|scopeId}.role_assigned`                  | `RbacRoleAssignedEvent`                            |
| `evt.rbac.{server\|scopeId}.role_revoked`                   | `RbacRoleRevokedEvent`                             |
| `evt.rbac.{server\|scopeId}.permission_granted`             | `RbacPermissionGrantedEvent`                       |
| `evt.rbac.{server\|scopeId}.permission_denied`              | `RbacPermissionDeniedEvent`                        |
| `evt.rbac.{server\|scopeId}.permission_cleared`             | `RbacPermissionClearedEvent`                       |
| `evt.auth.server.registration_verification_code_issued`    | `RegistrationVerificationCodeIssuedEvent`           |
| `evt.auth.server.login_failed`                             | `LoginFailedEvent`                                  |

Notes: Subject suffixes are stable NATS event tokens defined in [`cli/internal/events/subjects.go`](../cli/internal/events/subjects.go). Protobuf message types are the concrete `corev1.Event` oneof payloads defined in [`proto/chatto/core/v1/event.proto`](../proto/chatto/core/v1/event.proto) and sibling `*_events.proto` files. The current asset write path uses `evt.asset.{assetId}.*`; `AssetProjection` also consumes beta-era `evt.room.{roomId}.asset_*` histories for replay compatibility.

### Transient Live Subjects

Transient sync signals use `corev1.LiveEvent` and are published directly on NATS Core. They are not persisted and are not projection input.

Patterns: `live.sync.>` for transient `LiveEvent` pubsub and `live.evt.>` for raw EVT committed facts. `myEvents` consumes both roots server-side:

- Direct NATS Core publishes (`publishLiveEvent()`): transient `corev1.LiveEvent` messages on `live.sync.>` with no stream storage.
- `EVT` RePublish (`evt.>` → `live.evt.>`): every committed event-sourced fact is re-emitted once by JetStream. Chatto replicas must wait for local projection readiness and authorize before exposing deliverable room or asset events to clients.

`SERVER_EVENTS` no longer has a `RePublish` live path and runtime code no longer writes legacy `server.>` mirrors. Historical `SERVER_EVENTS` streams may still appear in old backups, but current boot and live-delivery paths do not read or import them.

**Transient live sync events** (`live.sync.{user,config,room}.>`):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.sync.user.{userId}.created`                        | User registration completed  |
| `live.sync.user.{userId}.profile_updated`                | User profile changed (broadcast for login/display/avatar updates; custom status set/clear is delivered from `live.evt.>`) |
| `live.sync.user.{userId}.user_deleted`                   | User account deleted         |
| `live.sync.config.server_updated`                        | Public server profile/config changed (name/MOTD/welcome/logo/banner/description) |
| `live.sync.config.room_groups_updated`                   | Admin reordered the room sidebar / room-group layout |
| `live.sync.user.{userId}.mentioned`                      | User was @mentioned (legacy attention signal; suppressed during DND) |
| `live.sync.user.{userId}.dm_message`                     | New DM message received (legacy attention signal; suppressed during DND) |
| `live.sync.user.{userId}.notification_created`           | New notification created; may be marked silent for DND alert suppression |
| `live.sync.user.{userId}.notification_dismissed`         | Notification dismissed       |
| `live.sync.user.{userId}.notification_level_changed`     | Viewer's server/room notification level changed |
| `live.sync.user.{userId}.thread_follow_changed`          | Viewer's thread follow/unfollow toggled |
| `live.sync.user.{userId}.settings_updated`               | User preferences changed     |
| `live.sync.user.{userId}.room_read`                      | Room marked as read          |
| `live.sync.user.{userId}.session_terminated`             | Active session revoked (logout-other-devices, account deletion) |
| `live.sync.member.deleted`                                | Server-level membership invalidation after account deletion |
| `live.sync.room.{kind}.{roomId}.user_typing`             | User typing in a room        |

Voice call lifecycle and participant transitions are durable room EVT facts under `evt.room.{roomId}.call_started`, `evt.room.{roomId}.call_joined`, `evt.room.{roomId}.call_left`, and `evt.room.{roomId}.call_ended`, republished to `live.evt.>` for realtime subscription delivery. They drive active-call state and indicators but are hidden from normal room history timelines. LiveKit room names include the active Chatto call ID suffix so LiveKit participant and room-finished observations are applied only to the matching call session. Only the replica holding the `MEMORY_CACHE` lease `lease.livekit_reconciler` runs the periodic LiveKit reconciliation loop. LiveKit reconciliation appends `RECONCILIATION` facts for participant mismatches in the matching call session. Missing LiveKit rooms and observed empty rooms end projected calls immediately after a successful listing; per-room LiveKit `not_found` responses while listing participants are treated as that room being gone/empty so other rooms can still reconcile. Pre-threshold LiveKit listing failures increment shared `MEMORY_CACHE` key `livekit.reconciliation.list_failures` and are retried on the normal reconciliation ticker, and listing failures only end projected active calls after three consecutive failed elected reconciliation cycles. A successful elected reconcile pass deletes that failure counter. `voiceCallToken` and `callParticipants` expose the active call ID so clients can ignore stale leave/end facts from previous calls in the same room. Room membership remains the authorization boundary for live delivery.

The `/api/realtime` WebSocket is backed by the single core stream `StreamMyEvents`, which combines:

- One `ChanSubscribe("live.sync.>")` for transient `LiveEvent` messages, and one `ChanSubscribe("live.evt.>")` for raw committed EVT facts. Authorization is applied per event: room membership for room subjects, asset room membership for asset subjects, `isAuthorizedForLiveEvent` for user/config/member subjects, and projection readiness before deliverable `live.evt.>` events.
- Live-only subscription delivery. Missed state after reconnect is recovered from projected reads: server-scoped stores refetch their current projections after event-bus gaps, and the visible room/thread refetches its current message window. Transient sync and presence signals remain live-only.
- The PresenceHub (single per-process KV watcher on `presence.>` fanning out per-user status changes to all subscribers).
- An in-process heartbeat ticker (synthetic `Heartbeat` event every 25s for client-side liveness detection).

### KV Buckets (backed by streams)

| Bucket                        | Storage | Backup   | Description                                     |
| ----------------------------- | ------- | -------- | ----------------------------------------------- |
| `RUNTIME_STATE`               | File    | Yes      | Persisted latest-value runtime/user state, including pending notifications, push subscriptions, auth/workflow tokens, and wrapped app DEK records |
| `MEMORY_CACHE`                | Memory  | No       | Volatile cache state; presence keyed `presence.{userId}` and short-lived leader leases keyed `lease.{name}` with per-key TTLs |
| `ENCRYPTION_KEYS`             | File    | **No**   | KMS KEKs and LiveKit per-call E2EE keys (excluded for security); app-owned wrapped DEKs live in `RUNTIME_STATE` |

**ENCRYPTION_KEYS keys:**

| Key                   | Description                       |
| --------------------- | --------------------------------- |
| `kek.{keyRef}`        | Protobuf `UserKeyEncryptionKey` per-user KEK record addressed by opaque KMS key ref |
| `call.e2ee.{callId}`  | Protobuf `UserKeyEncryptionKey` record containing the raw LiveKit E2EE key for one active call; referenced by `CallStartedEvent.e2ee_key_ref` and shredded when `CallEndedEvent` commits |

Notes: Excluded from backups so backup archives do not contain the KEKs needed to unwrap protected content or the per-call media keys needed to decrypt captured LiveKit media. Chatto core uses the in-process `internal/kms` boundary for KEK creation, DEK wrap/unwrap, call-key lookup, and key shredding. App-owned wrapped DEK records live in `RUNTIME_STATE` under `dek.{contentKeyRef}`.

**RUNTIME\_STATE keys:**

`RUNTIME_STATE` is the persisted home for latest-value runtime state that
survives restart but is not content/domain history. See
[ADR-036](adr/ADR-036-runtime-state-kv-boundary.md).

| Key                                    | Description                                                       |
| -------------------------------------- | ----------------------------------------------------------------- |
| `read.room.{userId}.{roomId}`          | Last-read root message event ID (UTF-8 string, ~14 bytes). Empty value = "joined but no specific event read yet" (e.g. joined an empty room). Missing key triggers a one-time lazy init to the room's current last event. |
| `read.thread.{userId}.{roomId}.{threadRootEventId}` | Latest thread message event ID the user has seen. |
| `notification.{userId}.{notificationId}` | Pending notification record (protobuf `Notification`) for DM messages, @mentions, replies, and all-message subscriptions. Uses per-key 90-day TTL. Live sync uses `NotificationCreatedEvent` / `NotificationDismissedEvent` on `live.sync.user.{userId}.*`; DND keeps the record but marks creation sync silent and skips push delivery. |
| `push_subscription.{userId}.{endpointHash}` | Web Push subscription record (protobuf `PushSubscription`) for a user's browser/device. The endpoint hash keeps multiple devices per user while deduplicating the same browser subscription. |
| `email_otp.{hmac(subject)}.{hmac(code)}` | Shared registration and email-verification OTP code JSON. Registration values carry normalized email; authenticated email-verification values carry user ID and email. The subject hash scopes registration by email and authenticated verification by user/email, the code hash verifies the submitted six-digit code, and the raw code is never stored. Uses per-key 15-minute TTL. |
| `email_otp.{hmac(subject)}.challenge` | Shared OTP challenge JSON with failed-attempt and issued-code counters. Wrong-code attempts update this record revision-safely, five wrong guesses exhaust the challenge until TTL, and at most ten codes can be issued for one challenge window. Uses per-key 15-minute TTL. |
| `registration_completion.{hmac}` | Registration completion token JSON created after code verification. Uses per-key 15-minute TTL. |
| `password_reset.{hmac}` | Password reset token JSON. Uses per-key 1-hour TTL. |
| `account_deletion_token.{hmac}` | Account deletion confirmation token JSON. Uses per-key 15-minute TTL. |
| `session.{hmac}` | Opaque bearer auth token JSON with the user auth generation it was issued against. Uses per-key `auth.token_ttl` (default 90 days); successful validation refreshes the key with a new per-key TTL for sliding-window expiry. Password resets, password changes, and account deletion revoke all older bearer tokens by advancing the user's auth generation through durable user events; scans of `session.*` delete matching records as cleanup. |
| `grant.{hmac}` | OAuth authorization code JSON with the user auth generation it was issued against. Uses per-key 5-minute TTL and is deleted on exchange attempt. |
| `link_preview.{urlHash}` | Cached link preview metadata (protobuf `CachedLinkPreview`) keyed by SHA-256 of the normalized URL. Successful previews use per-key 24-hour TTL; failed fetches use per-key 1-hour TTL. |
| `dek.{contentKeyRef}` | Wrapped purpose-scoped app DEK record (protobuf `UserDataEncryptionKey`). No TTL; shredded on account deletion. |

Token HMAC keys are derived with `[core].secret_key` and the token family as a domain separator. Backups include `RUNTIME_STATE`, so sessions and pending links survive restore only when the same `core.secret_key` is kept; backup archives do not contain raw bearer tokens or raw link/code values. Backups also include wrapped app DEK records, but those records cannot decrypt content without the KEKs in `ENCRYPTION_KEYS` or an external KMS.

**MEMORY_CACHE keys:**

| Key                                        | Description                                      |
| ------------------------------------------ | ------------------------------------------------ |
| `presence.{userId}`                        | Serialized `UserPresence` proto for the user's live status and manual-selection flag; per-key 60s TTL |
| `lease.livekit_reconciler`                 | Short-lived leader lease; only the current owner runs periodic LiveKit reconciliation |
| `livekit.reconciliation.list_failures`      | Shared consecutive LiveKit listing failure counter reset by any successful elected reconciliation pass |

Notes: Memory-based storage (not persisted, not backed up). Presence uses per-key TTL with 30-second client refresh and `LimitMarkerTTL` so NATS emits delete markers on TTL expiry. A single per-process **PresenceHub** watches `presence.>` and emits `PresenceChanged` only when a user's status changes. Clients refresh live status through ConnectRPC `PresenceService.ReportPresence`. On disconnect or "look offline", clients do not write `OFFLINE`; they stop refreshing and TTL handles expiry. Short-lived `lease.{name}` records coordinate singleton background work across replicas without adding durable state. Active voice call participants are served from the call-state projection over durable room EVT facts and reconciled against LiveKit by the elected reconciler; per-call LiveKit E2EE keys live behind the KMS boundary in `ENCRYPTION_KEYS`, and the retired `CALL_STATE` bucket is no longer imported.

### Object Store Buckets

| Bucket                      | Description                                       |
| --------------------------- | ------------------------------------------------- |
| `ASSET_CACHE`               | Cached resized images (optional)                  |
| `SERVER_ASSETS`             | NATS-backed persisted asset binaries              |

**ASSET_CACHE keys:**

| Key                                       | Description                                  |
| ----------------------------------------- | -------------------------------------------- |
| `attachment.{attachmentId}.{paramsHash}`  | Cached WebP image at specific dimensions     |
| `server.{assetId}.{paramsHash}`           | Cached WebP transform of a server asset      |

Notes: Only created when `[core.assets.cache]` is enabled in config. Uses TTL for automatic expiration (default 7 days). Current cache entries for deleted assets are also evicted from the active prefix (`attachment` or `server`) during binary cleanup. `paramsHash` is first 16 hex chars of SHA256(`{width}x{height}_{fit}`). Animated GIFs are not cached (served directly). S2 compression enabled.

**SERVER\_ASSETS keys:**

| Key         | Description                                    |
| ----------- | ---------------------------------------------- |
| `{assetId}` | Persisted asset binary by ID when stored in NATS |

**S3 asset keys:**

| Key                     | Description                                                  |
| ----------------------- | ------------------------------------------------------------ |
| `attachments/{assetId}` | Message attachment originals and derivative binaries         |
| `instance/{assetId}`    | Server-scoped assets: user avatars, server branding images, and link-preview images |

Notes: Asset IDs are globally unique (NanoID), so NATS-backed assets do not need a kind segment. S3 stores logical, prefix-free keys; any configured `path_prefix` is applied only when talking to the S3 backend. Content-Type and original filename stored in object headers where available. S2 compression enabled for `SERVER_ASSETS`. `MediaModel` owns binary storage and serving helpers; `AssetModel` owns durable lifecycle facts. Asset **metadata** (filename, dimensions, duration, storage pointer, …) is created in `AssetCreatedEvent` on `evt.asset.{assetId}.asset_created`; room scope and ownership context lives on the event (`message`, `derivative`, `user_avatar`, or `server_branding`) rather than inside `Asset`. New message bodies reference message-owned assets by ID. Link preview images are server-scoped persisted assets and are embedded in message bodies as `LinkPreview.image_asset` (`AssetRecord`) so the body records whether the image lives in S3 or `SERVER_ASSETS`; `image_asset_id` remains for compatibility with clients and older stored previews. Processing events refer to created asset IDs and are appended under the same `evt.asset.{assetId}.*` aggregate. The asset projection also reads beta-era `evt.room.{roomId}.asset_*` facts so existing 0.1.0 histories continue to replay without a stream rewrite. Message posting asks the process-local video service to spawn video/animated-GIF processing after appending asset creation and processing-started events; there is no transient NATS Core worker subject or `video_processed` live signal. Boot recovery derives missed work from the EVT projections and calls the same local path. Video processing success records thumbnail/variant asset IDs, while each derivative binary is separately declared with `AssetCreatedEvent` and an owner pointing at the original asset. `AssetProcessingFailedEvent.failure_code` records failed/unavailable outcomes. Account deletion follows the projected message asset graph and appends `AssetDeletedEvent` for source assets and derivative children before deleting backing bytes. The asset HTTP handler doesn't look up a separate index bucket; stable asset URLs resolve metadata and room scope from `AssetProjection`, while legacy locator URLs carry the body-or-video-manifest locator in the URL itself (see "Dynamic Image Transformation" below).

### Dynamic Image Transformation

Chatto supports on-the-fly image transformation for attachments and user avatars, allowing clients to request images at specific dimensions without pre-generating all possible sizes. Public server branding images expose canonical asset URLs instead of accepting arbitrary transform dimensions.

**URL Structure:**

ConnectRPC attachment fields primarily return stable asset paths with a
per-user `access` ticket query parameter. Originals:

```
/assets/files/{assetId}?access={base64payload}.{hexHMAC}
```

Image transforms use stable dimensions in the path and bind those same
parameters into the access ticket:

```
/assets/files/{assetId}/image/{width}x{height}/{fit}?access={base64payload}.{hexHMAC}
```

Where:

- `{assetId}` — the declared `AssetCreatedEvent.asset.id`
- `{base64payload}` — base64url-encoded JSON `{a, u, e, w?, h?, f?}` (asset id, signed user id, Unix-second expiry, optional transform)
- `{hexHMAC}` — first 16 bytes of HMAC-SHA256 of `{base64payload}` (32 hex chars)

The HMAC uses `[core.assets].signing_secret`. The HTTP handler verifies the
ticket signature, expiry, asset ID, and transform parameters, then resolves the
asset and room scope from `AssetProjection`. Every request checks that the
signed user is still a member of the asset's room
before serving the binary.

Internal fallback locator URLs use this shape:

```
/assets/attachments/{base64payload}.{hexHMAC}
/assets/attachments/{base64payload}.{hexHMAC}/t/{base64params}.{signature}
```

Those locator payloads carry room/source/attachment/user/expiry claims and use
the shorter `AttachmentURLTTL`. ConnectRPC attachment fields use the stable
`/assets/files/...` URL plus `AssetURL.expiresAt` shape.

**Transform Parameters:**

- `w` - Target width (1-2048 pixels)
- `h` - Target height (1-2048 pixels)
- `f` - Fit mode:
  - `contain` - Fit within bounds, preserve aspect ratio (may letterbox)
  - `cover` - Fill bounds, preserve aspect ratio (center-crop if needed)
  - `exact` - Stretch to exact dimensions (may distort)

**Security:**

URLs are signed with HMAC-SHA256 using a dedicated `signing_secret` (configured in `[core.assets]` section, separate from session secret). The signature covers the full path to prevent parameter tampering. ConnectRPC room timeline responses and `AttachmentService` responses generate valid signed URLs.

**ConnectRPC Integration:**

`RoomTimelineService`, `MessageService.PostMessage`, and `AttachmentService` return `AssetURL`-style objects for attachment URLs and thumbnails, including the embedded access-ticket expiry so clients can refresh before lazy loads or media startup hit an expired ticket. Public server profile image fields intentionally return canonical asset URLs without arbitrary transform arguments so anonymous server discovery cannot mint unbounded resize variants.

**Caching:**

Transformed images are generated on-demand. Public server assets can be cached
aggressively; authenticated attachment URLs are cacheable only as private
browser responses because their access ticket is per-user:

- Stable attachment originals and derivatives: `Cache-Control: private, max-age=3600`
- Server-scoped public assets and signed server transforms: public immutable/cacheable responses
- `ETag` based on asset ID and transform parameters
- Optional `ASSET_CACHE` object-store entries can cache resized bytes server-side

**Output Format:**

All transformed images are encoded as WebP for optimal compression and quality.

### Messages

Messages are persisted as durable `EVT` facts. Public timeline facts (`MessagePostedEvent`, `MessageEditedEvent`, `MessageRetractedEvent`) are bodyless; encrypted bodies live in private `MessageBodyEvent` payload facts on `evt.room.{roomId}.message_body` and are not delivered through live user subscriptions. Message bodies use the compact ADR-007 v2 envelope: XChaCha20-Poly1305 encrypts the body with the author's active message-body DEK epoch. AAD binds the message event ID, body event ID, room ID, author ID, and epoch. Per-user wrapped DEKs live in app-owned `RUNTIME_STATE` records; user EVT records only their purpose, epoch, content-key ref, wrapping algorithm, opaque wrapping key ref, and provider metadata for future KMS implementations. Durable user PII fields use a separate user-PII DEK epoch with user-event-specific AAD.

**Message Identifiers:**

- **Event ID**: NanoID (e.g., `E...`) on the EVT envelope. This is the durable message identity used for reactions, thread metadata, message-body lookup, attachments, and projections.
- **Payload**: `MessagePostedEvent` is payload-only. It carries room/thread/echo fields, but not an event ID, message-body ID alias, or embedded body.

**Write Path:**

1. Generate an EVT envelope with event ID
2. Generate a private body event ID and encrypt the body with the author's active message-body DEK epoch
3. Atomically append `MessageBodyEvent` before the bodyless `MessagePostedEvent`
4. Wait for local projections to reach the public message append sequence before serving read-your-writes

**Threading:**

- `in_reply_to` field stores the event ID of the parent message (empty for top-level messages)
- `in_thread` field stores the event ID of the thread root (empty for top-level messages)
- Thread replies are ordinary `MessagePostedEvent` facts on `evt.room.{roomId}.message_posted` with `in_thread` set to the root event ID.
- Thread replies can be echoed to the room timeline at post time or during the author's edit window. Echoes are separate `MessagePostedEvent` facts with `echo_of_event_id`; removing an edit-time echo appends a normal `MessageRetractedEvent` for the echo artifact.
- Thread reply lists are cursor-paginated; reply counts, participants, followed-thread pages, and last-reply timestamps are derived from the `ThreadProjection`.

**Read Path:**

- Room-level message history is served from `RoomTimelineProjection`, which keeps the visible room event log plus derived indexes for latest body state, hidden echoes, message asset references, and direct message-post lookup. Asset metadata, processing manifests, and derivative graphs are served from `AssetProjection`.
- Initial loads and cursor pagination walk the visible room timeline so thread replies, edits, retractions, reactions, asset-processing facts, and directly hidden echoes do not count as separate room timeline rows.
- Reconnect catch-up in the web client refreshes the currently viewed room window from `RoomTimelineProjection` after browser wake, realtime WebSocket reconnect, socket end, or heartbeat-stall catch-up notifications. When the user is at the bottom it fetches the latest page; when scrolled up it uses `eventsAround` for the visible anchor event and preserves scroll by event ID. Server-scoped stores also refetch projected state after event-bus reconnect gaps so notifications, unread/sidebar state, room layout, server profile/settings, and active-call indicators do not depend on replayed realtime events. `/api/realtime` v1 and `Subscription.myEvents` are live-only and do not expose a replay cursor.
- `eventsAround` uses the same visible room timeline to center jump-to-message windows on the target's visible position.
- Thread panes read the root message from `RoomTimelineProjection` and cursor-paginated replies from `ThreadProjection`. Anchored thread refreshes use `threadRepliesAround(eventId:)` to keep a visible reply in the refreshed window.

**@Mentions:**

- `@username` patterns in message body are parsed as Markdown inline mention tokens (ASCII alphanumeric, underscore, hyphen, dot), excluding code spans, code blocks, and blockquotes.
- Mention handles are resolved in the posting room; direct user handles only notify current room members, and invalid/non-member handles are silently ignored.
- `MessagePostedEvent.mentioned_user_ids` contains resolved user IDs
- Mention resolution is post-time only; later `MessageEditedEvent` facts update body content but do not add, remove, dismiss, or re-send mention notifications
- Pending mention state is a notification record in `RUNTIME_STATE` (`notification.{userId}.{notificationId}`); sidebar notification count badges derive from pending notifications, not a separate mention flag.
- Non-DND mentions also publish a legacy attention event to `live.sync.user.{userId}.mentioned`; the persisted notification still syncs through `notification_created`.
- Mention notifications are dismissed when the user views the relevant room or thread, or explicitly dismisses them from the notification center.
- Self-mentions are filtered out (no notification to message author)

**GDPR Deletion:**

- Delete appends `MessageRetractedEvent` to `EVT`; projections tombstone the message body before rendering.
- Edit appends a new private `MessageBodyEvent` before a bodyless public `MessageEditedEvent`; obsolete body payload events are securely deleted best-effort after projection catch-up.
- Attachment bytes are deleted from backing object storage best-effort and corresponding asset deletion facts are appended.

### Key Patterns

- **Unified Event Subscriptions**: The `myEvents` subscription merges EVT republish (`live.evt.>`), transient sync (`live.sync.>`), and PresenceHub updates into one authorized user stream.
- **Compression**: The `EVT` stream uses S2 compression to reduce storage costs
- **GDPR Compliance**: Message bodies are encrypted per author; deletion is represented by EVT retraction/shred facts and projections refuse to render shredded or retracted content.
- **Unified Event-Sourced Rooms**: Channels and DMs share `evt.room.{roomId}.>` subjects and room projections.
- **Current Resource Initialization**: Current resources are created up front at boot by `newStorage`.
