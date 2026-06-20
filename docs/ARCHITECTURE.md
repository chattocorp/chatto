# Chatto Architecture

This document is the **inventory**: what currently exists in the system — streams, KV buckets, object stores, subject patterns, key shapes, GraphQL operations. It's the *what's where* reference, not the *why* one.

For *why* a particular design decision was made:

- **Cross-cutting architectural choices** (NATS as primary store, GraphQL as the API, per-user encryption, etc.) live in the [Architecture Decision Records](adr/INDEX.md).
- **Per-feature design** (Roles & Permissions, Direct Messages, Reactions, Notifications, etc.) lives in the [Feature Decision Records](fdr/INDEX.md).
- **Coding and review conventions** live in `.claude/rules/` at the repo root.

## Table of Contents

- [Overview](#overview)
  - [Core Concepts](#core-concepts)
- [NATS Authentication](#nats-authentication)
- [Architecture & APIs](#architecture--apis)
- [Core Services](#core-services)
- [Projection Inventory](#projection-inventory)
- [GraphQL API Overview](#graphql-api-overview)
  - [Queries](#queries)
  - [Mutations](#mutations)
  - [Subscriptions](#subscriptions)
  - [Admin sub-API](#admin-sub-api)
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

Chatto is a real-time chat application with a GraphQL gateway and NATS/JetStream backend. Durable domain state is event-sourced in the `EVT` stream and served from projections; `RUNTIME_STATE` holds persisted latest-value runtime state such as notifications, push subscriptions, and auth tokens.

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

**External NATS Setup:**

For connecting to an external NATS cluster:
1. Set `nats.embedded.enabled = false`
2. Set `nats.client.url` to the external NATS URL(s)
3. Set `nats.client.auth_method` plus the matching credential field (`token`, `username`/`password`, `credentials_file`, or `nkey_seed`)

## Architecture & APIs

Key files: [`cli/internal/core/core.go`](../cli/internal/core/core.go), [`cli/internal/events/`](../cli/internal/events/), [`cli/internal/http_server/metrics.go`](../cli/internal/http_server/metrics.go), [`proto/chatto/core/v1/`](../proto/chatto/core/v1/)

- **NATS**: At the core, Chatto uses a series of NATS JetStream streams, KV buckets and object storage. Data stored in these is defined as Protocol Buffers (see `proto/`).
- **Core**: The `core` package defines Chatto's domain logic and directly talks to NATS to interact with KV buckets, object stores, and the `EVT` stream. `ChattoCore` remains the compatibility facade, while smaller services own projection readiness and domain-specific write concerns.
- **GraphQL**: Client-facing API for all operations (auth, management, messaging). Subscriptions over WebSocket for real-time updates. Fields require authentication by default unless marked public in the schema; resolvers call Core methods directly and enforce operation-specific authorization before each call.
- **Metrics**: Optional Prometheus-compatible per-process metrics run on a separate internal HTTP listener configured by `[metrics]`. The endpoint exposes Go/process collectors plus Chatto readiness, GraphQL WebSocket counts, `myEvents` stream counters, NATS client counters, projection health/lag gauges, and final projection startup duration/message-count gauges once initial replay completes.
- **Web Client**: SvelteKit-based SPA that gets compiled and embedded into the Go binary. Talks to GraphQL API over HTTP/WebSocket.
- **Email**: Optional SMTP integration for transactional emails (verification, password reset). Configured via `[smtp]` in config. The `internal/email` package provides a `Mailer` that returns `ErrSMTPDisabled` when SMTP is not configured, allowing callers to handle gracefully.

## Core Services

Key files: [`cli/internal/core/core.go`](../cli/internal/core/core.go), [`cli/internal/core/*_service.go`](../cli/internal/core/), [`cli/internal/video/service.go`](../cli/internal/video/service.go)

The core runtime is process-local but must be safe under multiple Chatto replicas connected to the same NATS account. Correctness comes from JetStream/KV atomicity and projection catch-up, not in-process serialization.

`ChattoCore` keeps a small runtime service registry with stable machine-readable keys such as `config_manager` and `my_events_service`. Per-process metrics use these keys as labels; display names remain operator-facing text only.

| Service                 | Key files                                                                                                                    | Responsibility                                                                                                             |
| ----------------------- | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `ChattoCore`            | [`core.go`](../cli/internal/core/core.go)                                                                                     | Application facade, resource initialization, lifecycle, GraphQL-facing operations                                            |
| `MyEventsService`       | [`my_events_service.go`](../cli/internal/core/my_events_service.go)                                                           | `myEvents` live delivery, projection readiness, heartbeats, per-user authorization, and process-local stream counters        |
| `events.Publisher`     | [`publisher.go`](../cli/internal/events/publisher.go)                                                                        | OCC-only writes to `EVT`, including atomic batches and filter-scoped concurrency guards                                     |
| `ConfigService`         | [`config_service.go`](../cli/internal/core/config_service.go)                                                                | Semantic server/user config event writes plus `ConfigProjection` readiness                                                  |
| `ConfigManager`         | [`config_manager.go`](../cli/internal/core/config_manager.go)                                                                | Compatibility facade for server config reads/writes backed by `ConfigService`                                               |
| `RoomService`           | [`room_service.go`](../cli/internal/core/room_service.go)                                                                    | Room-derived projection readiness and narrow reads for room catalog, membership, layout, timeline, threads, reactions        |
| `UserService`           | [`user_service.go`](../cli/internal/core/user_service.go)                                                                    | User and content-key projection readiness for account/profile/encryption writes                                             |
| `RBACService`           | [`rbac_service.go`](../cli/internal/core/rbac_service.go)                                                                    | RBAC projection readiness for role, assignment, and permission writes                                                       |
| `MentionablesService`   | [`mentionables_projection.go`](../cli/internal/core/mentionables_projection.go)                                              | Global mention-handle namespace lookup and readiness                                                                        |
| `PresenceService`       | [`presence_service.go`](../cli/internal/core/presence_service.go), [`presence_hub.go`](../cli/internal/core/presence_hub.go) | Per-process watcher/fanout for live presence state in `MEMORY_CACHE`                                                       |
| `CallService`           | [`call_service.go`](../cli/internal/core/call_service.go), [`voice.go`](../cli/internal/core/voice.go), [`lease.go`](../cli/internal/lease/lease.go) | Durable LiveKit call lifecycle/participant facts, call-state projection readiness, KMS-backed E2EE key resolution, and elected LiveKit reconciliation |
| `MediaService`          | [`media_service.go`](../cli/internal/core/media_service.go), [`attachments.go`](../cli/internal/core/attachments.go)          | Attachment/media binary storage, signed asset URLs, transformed image cache operations                                      |
| `AssetService`          | [`asset_service.go`](../cli/internal/core/asset_service.go), [`asset_projection.go`](../cli/internal/core/asset_projection.go)  | Durable asset lifecycle facts, processing transitions, tombstones, derivative cleanup ordering, asset projection readiness and reads |
| `video.Service`         | [`service.go`](../cli/internal/video/service.go)                                                                             | Process-local video/animated-GIF processing; emits asset processing result events                                           |

## Projection Inventory

Key files: [`cli/internal/core/core.go`](../cli/internal/core/core.go), [`cli/internal/events/projector.go`](../cli/internal/events/projector.go), [`cli/internal/core/projection_subjects_test.go`](../cli/internal/core/projection_subjects_test.go)

Projections are in-memory read models rebuilt from `EVT`. `NewChattoCore` registers each top-level projector once with a stable machine-readable key such as `content_keys` plus a human display name such as `Content Keys`; `ChattoCore.Run` replays `evt.>` through one process-local ordered consumer, decodes each event once, dispatches it to projections whose logical subject filters match, records each projection's initial replay startup duration, waits for them to become current at boot, and writers wait for the relevant projector sequence before returning read-your-writes.

| Runtime area       | Registered projector | Consumes                                                   | Read models / primary readers                                                             |
| ------------------ | -------------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Room directory     | Room Directory       | `evt.room.>`                                               | `RoomCatalogProjection`, `RoomMembershipProjection`, `RoomBanProjection`; room/member queries and room authorization |
| Room organization  | Room Group Layout    | `evt.group.>`, `evt.layout.>`                              | `RoomGroupProjection`, `RoomLayoutProjection`; sidebar groups, sidebar links, and mixed sidebar item ordering |
| Room timeline      | Room Timeline        | `evt.room.>`                                               | Visible room timeline, latest message bodies, hidden echoes, current attachment-bearing message index, direct message-post lookup, and message asset references |
| Assets             | Assets               | `evt.asset.>`, legacy `evt.room.*.asset_*`                 | Asset creation metadata, room scope, processing manifests, derivative graph, deletion state, and legacy room-asset compatibility |
| Threads            | Threads              | `evt.room.*.thread_created`, `evt.room.*.message_posted`, `evt.room.*.message_edited`, `evt.room.*.message_retracted`, `evt.user.*.user_key_shredded` | Per-thread reply logs, summaries, participants, reply counts                               |
| Reactions          | Reactions            | `evt.room.>`                                               | Current per-message reaction sets and room-scoped snapshot OCC positions; intentionally broad so reaction writes can OCC against the room tail |
| Voice calls        | Call State           | `evt.room.>`                                               | Current LiveKit call session, participants, active room IDs, and room-scoped snapshot OCC positions |
| Server/user config | Server Config        | `evt.config.>`, selected user cleanup/preference facts     | Server config, branding refs, user preferences, notification levels, blocked usernames     |
| Users              | Users                | `evt.user.>`                                               | Account/profile/auth lookup state, verified emails, external identity links, encrypted user PII |
| Content keys       | Content Keys         | `evt.user.*.dek_generated`, `evt.user.*.user_key_shredded` | Active and shredded user DEK epochs for message bodies and user PII                        |
| RBAC               | RBAC                 | `evt.rbac.>`                                               | Roles, role order, assignments, scoped allow/deny decisions                                |
| Mentions           | Mentionables         | `evt.>`                                                    | Global mention-handle ownership across users, roles, `@all`, and `@here`                  |

Notes: registered projector keys are used by metrics and automation; registered projector names match the admin projection diagnostics. Composite projections expose nested read models, but only their parent projector is started by `ChattoCore.Run`. The shared replay fanout reduces duplicate replay delivery and protobuf decoding while keeping each projection's status, lag, failure, and read-your-writes waiters independent. Threads reports the focused logical subjects above for waits and diagnostics; non-thread room facts are skipped before `Apply`.

## GraphQL API Overview

Key files: [`cli/internal/graph/`](../cli/internal/graph/) (schemas in `*.graphqls` files, resolvers in `*.resolvers.go`)

The GraphQL API is the primary client-facing interface for Chatto. It provides queries, mutations, and a single unified subscription over HTTP and WebSocket connections. Fields require authentication by default unless explicitly marked public in the schema. Authentication accepts opaque bearer tokens first and falls back to cookie sessions when no bearer token is present. User registration, login, password reset, email verification, and external provider login flows are REST endpoints (under `/auth/...`) rather than GraphQL mutations; successful password login and registration issue both a cookie session and a bearer token. Public server metadata includes `Server.authProviders`, a list of configured external login providers with IDs, types, labels, and login URLs.

The schema is modular: each feature area lives in its own `.graphqls` file and extends the root `Query` / `Mutation` / `Subscription` types. The operations below group by user-facing area, not by source file.

### Queries

**Server & identity** ([`server.graphqls`](../cli/internal/graph/server.graphqls), [`server_rbac.graphqls`](../cli/internal/graph/server_rbac.graphqls))

| Query                                | Description                                                                                                     |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------- |
| `server`                             | Information about this Chatto server (name, branding, member counts, viewer unread notification count). Public. |
| `viewer`                             | Nullable current-user scope: authenticated identity, permissions, follows, notifications; `null` for unauthenticated callers. |

Note: there is no top-level `me` query — viewer-scoped state hangs off the `viewer` field (which is extended by several feature files, e.g. `threads.graphqls` adds `viewer.followedThreads`, `notifications.graphqls` adds `viewer.notifications` / `viewer.hasNotifications`). Notification badges use scoped `Server.viewerNotifications` and `Room.viewerNotifications` connections and read their `totalCount`.

**Users** ([`query.graphqls`](../cli/internal/graph/query.graphqls))

| Query                                   | Description                                                            |
| --------------------------------------- | ---------------------------------------------------------------------- |
| `user(userId)`                          | Authenticated lookup of a user by ID.                                  |
| `userByLogin(login)`                    | Authenticated lookup of a user by login (returns null if not found).   |
| `server.members(search, limit, offset)` | Canonical paginated member directory (authenticated users).             |

**Rooms** ([`query.graphqls`](../cli/internal/graph/query.graphqls), [`room.graphqls`](../cli/internal/graph/room.graphqls))

| Query                              | Description                                                                            |
| ---------------------------------- | -------------------------------------------------------------------------------------- |
| `server.rooms(type?)`              | List rooms visible to the caller. Channel rooms are gated by membership or `room.list`; DM rooms are membership-only. Sidebar clients use `viewerIsMember` and `viewerCanJoinRoom` on `Room` to distinguish joined, joinable, and visible-but-not-joinable channel rows. |
| `server.roomGroups`                | Ordered channel-room groups and mixed sidebar items used to render the server sidebar. Group room entries are filtered to rooms visible to the caller. |
| `room(roomId)`                     | Get a room by ID. Room-scoped reads (`members`, `events`, `event(eventId)`, `eventsAround`, `voiceCallToken`, `viewerCan*` flags, `viewerIsMember`, `viewerNotifications`) live as fields on the returned `Room`; `members(search, limit, offset)` and `viewerNotifications` are offset-paginated. `events` is the visible room timeline. Folded durable facts such as reactions are reflected in projected room reads; the web client refreshes the current room window after wake/reconnect to catch up without a full document reload. |

**RBAC tooling** ([`rbac.graphqls`](../cli/internal/graph/rbac.graphqls), [`role_permissions.graphqls`](../cli/internal/graph/role_permissions.graphqls), [`role_permission_matrix.graphqls`](../cli/internal/graph/role_permission_matrix.graphqls), [`user_permissions.graphqls`](../cli/internal/graph/user_permissions.graphqls), [`permission_inspector.graphqls`](../cli/internal/graph/permission_inspector.graphqls))

| Query                                             | Description                                                              |
| ------------------------------------------------- | ------------------------------------------------------------------------ |
| `admin.rbac.rolePermissionTierMatrix(roomId?, groupId?)` | Full role-permission matrix at server / group / room scope.       |
| `admin.rbac.rolePermissionMatrix(roleName)`       | Per-role permission matrix (`role.manage` gated).                        |
| `admin.rbac.userPermissionMatrix(userId)`         | Effective allow/deny matrix for a user (`user.manage-permissions`).       |
| `admin.rbac.permissionExplanation(userId, …)`     | Admin/tooling-only per-permission resolver explainer; no self-inspection. |

**Voice & link previews** ([`voice.graphqls`](../cli/internal/graph/voice.graphqls), [`linkpreview.graphqls`](../cli/internal/graph/linkpreview.graphqls))

| Query                       | Description                                                                |
| --------------------------- | -------------------------------------------------------------------------- |
| `activeCallRoomIds`         | Room IDs that currently have an active LiveKit voice call, read from the call-state projection. |
| `linkPreview(url)`          | Fetch (and cache) Open Graph metadata for a URL.                           |

**Admin** ([`admin.graphqls`](../cli/internal/graph/admin.graphqls))

Admin queries are nested under a single `admin: AdminQueries` field that returns `null` for unauthenticated callers. Child fields enforce concrete capability gates such as `server.manage`, `admin.view-users`, `admin.view-audit`, `role.manage`, and owner-only diagnostics. See [Admin sub-API](#admin-sub-api) below for the contents.

### Mutations

**Server settings** ([`mutation.graphqls`](../cli/internal/graph/mutation.graphqls))

| Mutation                | Description                                                |
| ----------------------- | ---------------------------------------------------------- |
| `updateServerConfig`    | Update runtime-editable server presentation/configuration fields. |
| `uploadServerLogo`      | Upload server logo.                                        |
| `deleteServerLogo`      | Delete server logo.                                        |
| `uploadServerBanner`    | Upload server banner.                                      |
| `deleteServerBanner`    | Delete server banner.                                      |

**Rooms** ([`mutation.graphqls`](../cli/internal/graph/mutation.graphqls), [`dm.graphqls`](../cli/internal/graph/dm.graphqls))

| Mutation                       | Description                                                                      |
| ------------------------------ | -------------------------------------------------------------------------------- |
| `createRoom`                   | Create a new channel room.                                                       |
| `updateRoom`                   | Update a room's name / description (`room.manage`).                              |
| `archiveRoom` / `unarchiveRoom`| Archive or restore a room (`room.manage`).                                       |
| `joinRoom` / `leaveRoom`       | Join / leave a room.                                                             |
| `banRoomMember` / `unbanRoomMember` | Create or remove a room ban (`room.ban-member`; DMs rejected; reasons required for moderation audit). Bans emit a normal leave event, maintain active ban state, and deny rejoin through ordinary join authorization. |
| `joinGroup`                    | Join every room in a group the caller has `room.join` for. Powers "Join all".    |
| `markRoomAsRead`               | Mark a room as read; records the last-seen root event ID for unread tracking.    |
| `startDM`                      | Start a DM with a participant set (returns existing room if the set matches).    |

**Messages, reactions, threads** ([`mutation.graphqls`](../cli/internal/graph/mutation.graphqls))

| Mutation                  | Description                                                                                  |
| ------------------------- | -------------------------------------------------------------------------------------------- |
| `postMessage`             | Post a message (root or thread reply; attachments additionally require `message.attach`; optional link previews / echo-to-channel).|
| `updateMessage`           | Update own message body (3-hour window); optional thread-reply echo reconciliation.           |
| `deleteMessage`           | Delete message body (GDPR crypto-shred); event stays in stream as audit trail.               |
| `deleteAttachment`        | Delete an attachment from own message.                                                       |
| `deleteLinkPreview`       | Delete a link preview from own message.                                                      |
| `addReaction` / `removeReaction` | Add or remove an emoji reaction (shortcode names).                                    |
| `sendTypingIndicator`     | Publish a transient "user is typing" live event.                                             |
| `markThreadAsRead`        | Update viewer's last-seen marker for a thread (drives unread separators).                    |
| `followThread` / `unfollowThread` | Subscribe / unsubscribe to thread reply notifications.                              |

| Room read                  | Description                                                                                  |
| -------------------------- | -------------------------------------------------------------------------------------------- |
| `Room.attachments`         | Paginated current attachment list for root messages and thread replies, authorized like `Room.events`. |

**User profile & account** ([`mutation.graphqls`](../cli/internal/graph/mutation.graphqls), [`user_preferences.graphqls`](../cli/internal/graph/user_preferences.graphqls))

| Mutation                  | Description                                                                                  |
| ------------------------- | -------------------------------------------------------------------------------------------- |
| `updateProfile`           | Update display name and/or login (login change has a 30-day cooldown).                       |
| `uploadAvatar`            | Upload avatar (resized to 256×256, WebP).                                                    |
| `deleteAvatar`            | Delete a user's avatar.                                                                      |
| `updateSettings`          | Update display preferences (timezone, time format).                                          |
| `requestAccountDeletion`  | Issue a 15-minute confirmation token for account deletion (XSS-resistant two-step).          |
| `deleteMyAccount`         | Permanently delete the authenticated user's account (GDPR crypto-shredding).                 |

**Notifications, presence, push** ([`notifications.graphqls`](../cli/internal/graph/notifications.graphqls), [`notification_level.graphqls`](../cli/internal/graph/notification_level.graphqls), [`presence.graphqls`](../cli/internal/graph/presence.graphqls), [`push.graphqls`](../cli/internal/graph/push.graphqls))

| Mutation                          | Description                                                                                  |
| --------------------------------- | -------------------------------------------------------------------------------------------- |
| `dismissNotification`             | Dismiss a single in-app notification.                                                        |
| `dismissAllNotifications`         | Dismiss every notification for the viewer (returns dismissed count).                         |
| `setServerNotificationLevel`      | Update viewer's server-wide notification level.                                              |
| `setRoomNotificationLevel`        | Update viewer's per-room notification level.                                                 |
| `updateMyPresence`                | Set caller's presence status (`OFFLINE` is implicit on disconnect, not a valid input).       |
| `subscribeToPush`                 | Register a Web Push subscription for this device.                                            |
| `unsubscribeFromPush`             | Remove a previously-registered Web Push subscription.                                        |

**Voice calls** ([`voice.graphqls`](../cli/internal/graph/voice.graphqls))

| Mutation                          | Description                                                                                  |
| --------------------------------- | -------------------------------------------------------------------------------------------- |
| `joinVoiceCall`                   | Record the caller's intent to join a LiveKit room call as a durable room fact.               |
| `leaveVoiceCall`                  | Record the caller's intent to leave a LiveKit room call as a durable room fact.              |

**Room groups** ([`room_groups.graphqls`](../cli/internal/graph/room_groups.graphqls))

| Mutation                          | Description                                                                                  |
| --------------------------------- | -------------------------------------------------------------------------------------------- |
| `createRoomGroup`                 | Create a new room group (`role.manage`).                                                     |
| `updateRoomGroup`                 | Rename / re-describe a room group.                                                           |
| `deleteRoomGroup`                 | Delete a room group (must be empty).                                                         |
| `reorderRoomGroups`               | Reorder all room groups (full list, exactly once each).                                      |
| `reorderRoomsInGroup`             | Reorder rooms within a single group.                                                         |
| `moveRoomToGroup`                 | Move a room into a different group (`room.manage` in both source and target — see ADR-031). |
| `createSidebarLink`               | Add an external link to a room group (`room.manage`).                                        |
| `updateSidebarLink`               | Rename or retarget a sidebar link (`room.manage`).                                           |
| `deleteSidebarLink`               | Remove a sidebar link from its group (`room.manage`).                                        |
| `moveSidebarLinkToGroup`          | Move a sidebar link into a different group (`room.manage` in both source and target).        |
| `reorderSidebarItemsInGroup`      | Reorder rooms and sidebar links within a single group.                                       |
| `grantGroupPermission`            | Grant a permission to a role at group scope.                                                 |
| `denyGroupPermission`             | Deny a permission to a role at group scope.                                                  |
| `clearGroupPermissionState`       | Remove both grant and denial at group scope.                                                 |

**Roles & permissions** ([`server_rbac.graphqls`](../cli/internal/graph/server_rbac.graphqls), [`server_rbac_extra.graphqls`](../cli/internal/graph/server_rbac_extra.graphqls))

| Mutation                          | Description                                                                                  |
| --------------------------------- | -------------------------------------------------------------------------------------------- |
| `createRole` / `updateRole` / `deleteRole` | CRUD for custom server roles (system roles are fixed).                              |
| `reorderRoles`                    | Reorder custom roles. System roles maintain fixed positions and are excluded.                |
| `assignRole` / `revokeRole`       | Add / remove a role assignment on a user (`role.assign`).                                    |
| `grantPermission` / `revokePermission` | Grant or revoke a permission on a role at server scope.                                 |
| `denyPermission`                  | Deny a permission on a role at server scope (clears any existing grant).                     |
| `clearPermissionState`            | Restore neutral state for a permission on a role at server scope.                            |
| `grantRoomPermission` / `denyRoomPermission` / `clearRoomPermission` | Same trio at room scope.                              |
| `grantUserPermission`             | Grant a permission directly to a user (`user.manage-permissions`).                           |
| `denyUserPermission`              | Deny a permission directly to a user (`user.manage-permissions`; any applicable deny wins).  |
| `clearUserPermissionState`        | Clear both grant and denial of a permission on a user.                                       |

**Admin** ([`admin.graphqls`](../cli/internal/graph/admin.graphqls))

Like `Query.admin`, the `admin: AdminMutations` field returns `null` for unauthenticated callers. See [Admin sub-API](#admin-sub-api) below.

### Subscriptions

| Subscription          | Description                                                                                                                                                                                                                                                                                                                                                                                                          |
| --------------------- | ---- |
| `myEvents`            | The single subscription. Multiplexes durable room/asset events from `live.evt.>` (messages, reactions, edits, retractions, room lifecycle, asset processing, voice call lifecycle/participant facts) and transient sync signals from `live.sync.>` (typing, mention notifications, video-complete pings, server config/profile/preference invalidation, notifications, thread-follow sync, presence, server membership lifecycle, session termination, heartbeats) into one GraphQL `Event` envelope. Asset lifecycle events are authorized through the room scope recorded on their `AssetCreatedEvent`. The membership set is tracked in real time — joining or leaving a room updates filtering immediately without reconnecting. DM-room events use the same membership gate as channel-room events; there is no separate DM read permission. Subscribing sets the caller's presence to `ONLINE`. The subscription is live-only: missed state is recovered by projected queries, not subscription replay cursors. |

There is no `adminAuditLogEvents` subscription — audit events arrive through `myEvents` for users with the relevant admin scope.

### Admin sub-API

`Query.admin` returns `AdminQueries`; `Mutation.admin` returns `AdminMutations`. Both return `null` when the caller is unauthenticated; individual nested fields apply their own permissions such as `server.manage`, `admin.view-system`, `admin.view-audit`, `role.manage`, or owner-only gates (see [FDR-021](fdr/FDR-021-admin-dashboard.md)). Admin operations are spread across multiple schema files but all hang off these two types.

Diagnostic fields (`admin.systemInfo`, `admin.eventLog`, `admin.eventLogEntry`, and `admin.projections`) are operator-facing inspection tools. `admin.systemInfo` is owner-only for now; `admin.projections` remains gated by `admin.view-system`. Their field names are part of the GraphQL API, but raw broker/storage strings, payload JSON, metric names, and point-in-time counts are diagnostic values rather than product-domain contracts.

| Field                                            | Type      | Description                                                                                  |
| ------------------------------------------------ | --------- | -------------------------------------------------------------------------------------------- |
| `admin.systemInfo`                               | Query     | Owner-only point-in-time operator diagnostics: connection, storage-account usage, stream/consumer state, and deployment counts. |
| `admin.serverConfig`                             | Query     | Server configuration overrides (welcome message, MOTD, blocked usernames, OG description).  |
| `admin.groupRolePermissions(groupId, roleName)`  | Query     | Explicit grants and denials for a role on a specific room group.                             |
| `admin.groupUserPermissions(groupId, userId)`    | Query     | Explicit grants and denials for a user on a specific room group.                             |
| `admin.eventLog(limit, before)`                  | Query     | Diagnostic event-log browser, newest first (`limit` default 50, max 200).                    |
| `admin.eventLogEntry(sequence)`                  | Query     | Diagnostic event-log entry lookup by sequence.                                               |
| `admin.projections`                              | Query     | Projection lag, rough memory estimates, and diagnostic metric buckets.                       |
| `admin.updateBlockedUsernames(input)`            | Mutation  | Update the newline-separated blocked-username list.                                          |
| `admin.updateUser(input)`                        | Mutation  | Update a user's login / display name (bypasses the 30-day cooldown).                         |
| `admin.clearUsernameCooldown(userId)`            | Mutation  | Manually clear a user's login change cooldown.                                               |

## Architecture Pattern: Event-Sourced Writes

### Write Path

| Type    | Resource                      | Purpose                                     |
| ------- | ----------------------------- | ------------------------------------------- |
| Stream  | `EVT`                         | Durable event-sourcing log for domain facts |
| KV      | `RUNTIME_STATE`               | Persisted latest-value runtime/user state, including pending notifications, push subscriptions, auth/workflow tokens, and wrapped app DEK records |
| KV      | `MEMORY_CACHE`                | Volatile memory-backed cache state for presence and short-lived leader leases; excluded from backups |
| KV      | `ENCRYPTION_KEYS`             | KMS key-encryption keys and per-call LiveKit E2EE keys; excluded from backups |
| Objects | `SERVER_ASSETS`               | Asset binaries (avatars, server branding, link previews, message attachments) |
| Objects | `ASSET_CACHE`                 | Optional cached image transforms with TTL    |

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
- Fresh deployments seed current invariants such as default RBAC roles and the default room group. Fresh RBAC seeds include `message.attach` for `everyone`; existing RBAC state is not silently backfilled on boot.
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

- **Roles, permissions, and the resolver** — see [FDR-001](fdr/FDR-001-roles-and-permissions.md) for the design and rationale, [`/.claude/rules/authorization.md`](../.claude/rules/authorization.md) for the full resolver semantics (DM boundary, user-level overrides, scope cascade), and [`/.claude/rules/admin.md`](../.claude/rules/admin.md) for the admin-side picture.
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
| Object store | `SERVER_ASSETS`     | File    | Yes    | Asset binaries for avatars, branding, link previews, attachments, derivatives |
| Object store | `ASSET_CACHE`       | File    | No     | Optional TTL cache for transformed image bytes                               |
| NATS Core    | `live.sync.>`       | None    | No     | Transient `corev1.LiveEvent` pubsub signals                                  |
| Republish    | `live.evt.>`        | None    | No     | Raw committed `EVT` facts republished by JetStream for server-side live delivery |

### Event Envelopes

Chatto uses `corev1.Event` as the durable EVT wrapper and `corev1.LiveEvent` as the transient NATS Core wrapper. GraphQL exposes both through one public `Event` envelope, but the protobuf wire envelopes stay separate so live-only sync signals cannot leak into the durable audit/event log shape.

- **Wrapper fields**: `id`, `created_at`, `actor_id`
- **Concrete event**: `event` oneof on the relevant wire envelope; contextual fields (`roomId`, etc.) live on the concrete payloads.

The active `Event.event` oneof variants are all durable EVT payloads, regardless of numeric tag. Transient-only pubsub signals belong in `corev1.LiveEvent`, not `corev1.Event`.

Existing `Event` oneof field numbers are part of the persisted JetStream wire format; do not renumber or reuse them.

**Proto File Organization:**

| File | Contents | Safety |
| ---- | -------- | ------ |
| `event.proto` | Durable `Event` wrapper + persisted event message definitions | Changing field numbers/structure affects JetStream-stored data — requires careful migration |
| `live_events.proto` | Transient `LiveEvent` wrapper + live-only event message definitions | Safe to change freely — these are never persisted |

Both files share `package chatto.core.v1` and generate into the same Go package. `core.EventEnvelope` is the in-process GraphQL delivery interface that can carry durable EVT, transient LiveEvent, or a heartbeat through private concrete implementations. The `unwrapEvent` helper in `cli/internal/graph/event_helpers.go` is the single switch from that delivery envelope to a typed GraphQL payload; `unwrapEventAs[T]` is the typed wrapper used by the GraphQL resolvers.

**Event Categories:**

| Category                    | Storage    | Examples                                                    | Purpose                                                        |
| --------------------------- | ---------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| JetStream-stored (room) | Stream     | RoomCreated, MessagePosted, MessageEdited, MessageRetracted, ReactionAdded, ReactionRemoved, UserJoinedRoom, CallStarted, CallParticipantJoined, CallParticipantLeft, CallEnded | Ordering guarantees, historical replay, projection source of truth |
| Room live-only              | NATS Core  | UserTyping | Ephemeral room notifications where another store/projection is source of truth |
| Deployment live (user/config) | NATS Core  | UserCreated, ServerUpdated, MentionNotification, NotificationCreated, PresenceChanged | Cross-tab sync, notifications, server lifecycle |

The distinction between stored and live-only events is explicit in the wire envelope: durable facts use `corev1.Event`, transient signals use `corev1.LiveEvent`, and GraphQL exposes both through one `Event` envelope with typed payloads as members of the `EventType` union. Room queries and server subscriptions are delivery contexts, not separate wrapper types.

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

The `myEvents` GraphQL subscription is owned by `MyEventsService` behind the `ChattoCore.StreamMyEvents` facade and subscribes to `live.sync.>` and `live.evt.>`. For deliverable raw EVT room and asset messages, it reads the republished `Nats-Sequence` header, waits for the local projections needed by authorization and follow-up resolvers, filters by the subscribing user, and then emits the GraphQL event. Asset lifecycle events resolve their room authorization through `AssetProjection`, using the room scope on `AssetCreatedEvent` and inherited parent scope for derivatives. Transient `LiveEvent` messages are adapted at this API boundary into the public GraphQL event shape. The subscription is live-only; missed state is recovered by projected reads. The bundled web client keeps its event bus subscription simple, watches server heartbeats for silent stalls, refetches server-scoped projected state after reconnect/resubscribe gaps, and refetches the current room or thread window from projections after browser wake, WebSocket reconnect, subscription end, or heartbeat-stall catch-up notifications. There is no per-connection JetStream consumer and no public subscription replay cursor.

### EVT Subject Patterns

| Stream                       | Wrapper          | Scope      | Description                                      |
| ---------------------------- | ---------------- | ---------- | ------------------------------------------------ |
| `EVT`                        | `corev1.Event`   | Server     | Event-sourcing log ([ADR-033](adr/ADR-033-event-sourced-state-with-projections.md) / [ADR-034](adr/ADR-034-single-event-stream.md)). Subjects `evt.{aggregateType}.{aggregateId}.{eventType}`; republishes onto `live.evt.>` as the raw committed-event feed. Stores room membership/metadata, groups/layout, server config, users, messages/threads, reactions, assets, RBAC, and auth workflow audit facts. |
| Live Sync                    | `corev1.LiveEvent` | Transient  | Direct NATS Core pubsub on `live.sync.>` for transient UI sync signals. `myEvents` authorizes and adapts these messages into GraphQL events; they are never projection input. |

The republished `live.evt.{aggregateType}.{aggregateId}.{eventType}` subject is an internal server-side feed; GraphQL `myEvents` waits for projections and authorization before delivering anything to clients.

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
| `live.sync.user.{userId}.profile_updated`                | User profile changed (broadcast) |
| `live.sync.user.{userId}.user_deleted`                   | User account deleted         |
| `live.sync.config.server_updated`                        | Public server profile/config changed (name/MOTD/welcome/logo/banner/description) |
| `live.sync.config.room_groups_updated`                   | Admin reordered the room sidebar / room-group layout |
| `live.sync.user.{userId}.mentioned`                      | User was @mentioned          |
| `live.sync.user.{userId}.dm_message`                     | New DM message received      |
| `live.sync.user.{userId}.notification_created`           | New notification created     |
| `live.sync.user.{userId}.notification_dismissed`         | Notification dismissed       |
| `live.sync.user.{userId}.notification_level_changed`     | Viewer's server/room notification level changed |
| `live.sync.user.{userId}.thread_follow_changed`          | Viewer's thread follow/unfollow toggled |
| `live.sync.user.{userId}.settings_updated`               | User preferences changed     |
| `live.sync.user.{userId}.room_read`                      | Room marked as read          |
| `live.sync.user.{userId}.session_terminated`             | Active session revoked (logout-other-devices, account deletion) |
| `live.sync.member.deleted`                                | Server-level membership invalidation after account deletion |
| `live.sync.room.{kind}.{roomId}.user_typing`             | User typing in a room        |

Voice call lifecycle and participant transitions are durable room EVT facts under `evt.room.{roomId}.call_started`, `evt.room.{roomId}.call_joined`, `evt.room.{roomId}.call_left`, and `evt.room.{roomId}.call_ended`, republished to `live.evt.>` for realtime subscription delivery. They drive active-call state and indicators but are hidden from normal room history timelines. Only the replica holding the `MEMORY_CACHE` lease `lease.livekit_reconciler` runs the periodic LiveKit reconciliation loop. LiveKit reconciliation appends `RECONCILIATION` facts for participant mismatches. Missing LiveKit rooms and observed empty rooms end projected calls immediately after a successful listing; pre-threshold LiveKit listing failures increment shared `MEMORY_CACHE` key `livekit.reconciliation.list_failures` and are retried on the normal reconciliation ticker, and listing failures only end projected active calls after three consecutive failed elected reconciliation cycles. A successful elected reconcile pass deletes that failure counter. `voiceCallToken` and `callParticipants` expose the active call ID so clients can ignore stale leave/end facts from previous calls in the same room. Room membership remains the authorization boundary for live delivery.

The unified `myEvents` GraphQL subscription is backed by a single core stream (`StreamMyEvents`) that combines:

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
| `notification.{userId}.{notificationId}` | Pending notification record (protobuf `Notification`) for DM messages, @mentions, replies, and all-message subscriptions. Uses per-key 90-day TTL. Live sync uses `NotificationCreatedEvent` / `NotificationDismissedEvent` on `live.sync.user.{userId}.*`. |
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
| `presence.{userId}`                        | Serialized `UserPresence` proto for the user's live status; per-key 60s TTL |
| `lease.livekit_reconciler`                 | Short-lived leader lease; only the current owner runs periodic LiveKit reconciliation |
| `livekit.reconciliation.list_failures`      | Shared consecutive LiveKit listing failure counter reset by any successful elected reconciliation pass |

Notes: Memory-based storage (not persisted, not backed up). Presence uses per-key TTL with 30-second client refresh and `LimitMarkerTTL` so NATS emits delete markers on TTL expiry. A single per-process **PresenceHub** watches `presence.>` and emits `PresenceChanged` only when a user's status changes. `Subscription.myEvents` sets the user online, and `updateMyPresence` overwrites the user's live status. On disconnect, clients do not write `OFFLINE`; they stop refreshing and TTL handles expiry. Short-lived `lease.{name}` records coordinate singleton background work across replicas without adding durable state. Active voice call participants are served from the call-state projection over durable room EVT facts and reconciled against LiveKit by the elected reconciler; per-call LiveKit E2EE keys live behind the KMS boundary in `ENCRYPTION_KEYS`, and the retired `CALL_STATE` bucket is no longer imported.

### Object Store Buckets

| Bucket                      | Description                                       |
| --------------------------- | ------------------------------------------------- |
| `ASSET_CACHE`               | Cached resized images (optional)                  |
| `SERVER_ASSETS`             | Asset binaries (avatars, server icon/banner, link previews, message attachments) |

**ASSET_CACHE keys:**

| Key                                       | Description                                  |
| ----------------------------------------- | -------------------------------------------- |
| `attachment.{attachmentId}.{paramsHash}`  | Cached WebP image at specific dimensions     |
| `server.{assetId}.{paramsHash}`           | Cached WebP transform of a server asset      |

Notes: Only created when `[core.assets.cache]` is enabled in config. Uses TTL for automatic expiration (default 7 days). Current cache entries for deleted assets are also evicted from the active prefix (`attachment` or `server`) during binary cleanup. `paramsHash` is first 16 hex chars of SHA256(`{width}x{height}_{fit}`). Animated GIFs are not cached (served directly). S2 compression enabled.

**SERVER\_ASSETS keys:**

| Key                   | Description                                     |
| --------------------- | ----------------------------------------------- |
| `{assetId}`           | User avatars, server branding images, link-preview images, original attachment files, and derivative binaries |

Notes: Asset IDs are globally unique (NanoID), so no kind segment is needed. Channel and DM assets share the same flat keyspace. Content-Type and original filename stored in object headers where available. S2 compression enabled. `MediaService` owns binary storage and serving helpers; `AssetService` owns durable lifecycle facts. Asset **metadata** (filename, dimensions, duration, storage pointer, …) is created in `AssetCreatedEvent` on `evt.asset.{assetId}.asset_created`; room scope and ownership context lives on the event (`message`, `derivative`, `user_avatar`, or `server_branding`) rather than inside `Asset`. New message bodies reference message-owned assets by ID. Processing events refer to created asset IDs and are appended under the same `evt.asset.{assetId}.*` aggregate. The asset projection also reads beta-era `evt.room.{roomId}.asset_*` facts so existing 0.1.0 histories continue to replay without a stream rewrite. Message posting asks the process-local video service to spawn video/animated-GIF processing after appending asset creation and processing-started events; there is no transient NATS Core worker subject or `video_processed` live signal. Boot recovery derives missed work from the EVT projections and calls the same local path. Video processing success records thumbnail/variant asset IDs, while each derivative binary is separately declared with `AssetCreatedEvent` and an owner pointing at the original asset. `AssetProcessingFailedEvent.failure_code` records failed/unavailable outcomes. Account deletion follows the projected message asset graph and appends `AssetDeletedEvent` for source assets and derivative children before deleting backing bytes. The asset HTTP handler doesn't look up a separate index bucket; stable asset URLs resolve metadata and room scope from `AssetProjection`, while legacy locator URLs carry the body-or-video-manifest locator in the URL itself (see "Dynamic Image Transformation" below).

### Dynamic Image Transformation

Chatto supports on-the-fly image transformation for attachments and user avatars, allowing clients to request images at specific dimensions without pre-generating all possible sizes. Public server branding images expose canonical asset URLs instead of accepting arbitrary transform dimensions.

**URL Structure:**

GraphQL attachment fields primarily return stable asset paths with a
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
the shorter `AttachmentURLTTL`. GraphQL attachment fields use the stable
`/assets/files/...` URL plus `AssetURL.expiresAt` shape.

**Transform Parameters:**

- `w` - Target width (1-2048 pixels)
- `h` - Target height (1-2048 pixels)
- `f` - Fit mode:
  - `contain` - Fit within bounds, preserve aspect ratio (may letterbox)
  - `cover` - Fill bounds, preserve aspect ratio (center-crop if needed)
  - `exact` - Stretch to exact dimensions (may distort)

**Security:**

URLs are signed with HMAC-SHA256 using a dedicated `signing_secret` (configured in `[core.assets]` section, separate from session secret). The signature covers the full path to prevent parameter tampering. Only the GraphQL API generates valid signed URLs.

**GraphQL Integration:**

The `Attachment` and `User` image fields expose transform parameters as field arguments:

```graphql
type Attachment {
  url(width: Int, height: Int, fit: FitMode): String!
  assetUrl(width: Int, height: Int, fit: FitMode): AssetURL!
  thumbnailUrl(width: Int, height: Int, fit: FitMode): String
  thumbnailAssetUrl(width: Int, height: Int, fit: FitMode): AssetURL
}

type AssetURL {
  url: String!
  expiresAt: Time!
}

type User {
  avatarUrl(width: Int, height: Int, fit: FitMode): String
}

type ServerProfile {
  logoUrl: String
  bannerUrl: String
}

enum FitMode {
  CONTAIN
  COVER
  EXACT
}
```

For `Attachment` images, `assetUrl` and `thumbnailAssetUrl` return the URL plus
the embedded access-ticket expiry so clients can refresh before lazy loads or
media startup hit an expired ticket. The string URL fields return the same
URL without the expiry. Public `ServerProfile` image
fields intentionally return canonical asset URLs without transform arguments so
anonymous server discovery cannot mint unbounded resize variants.

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

- **Event ID**: NanoID (e.g., `E...`) on the EVT envelope. This is the durable message identity used for GraphQL `Event.id`, reactions, thread metadata, message-body lookup, attachments, and projections.
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
- `MessagePostedEvent.channelEchoEventId` is a GraphQL-only derived field backed by `RoomTimelineProjection`'s echo-link index; it is not stored in the protobuf payload.
- Initial loads and cursor pagination walk the visible room timeline so thread replies, edits, retractions, reactions, asset-processing facts, and directly hidden echoes do not count as separate room timeline rows.
- Reconnect catch-up in the web client refreshes the currently viewed room window from `RoomTimelineProjection` after browser wake, WebSocket reconnect, subscription end, or heartbeat-stall catch-up notifications. When the user is at the bottom it fetches the latest page; when scrolled up it uses `eventsAround` for the visible anchor event and preserves scroll by event ID. Server-scoped stores also refetch projected state after event-bus reconnect/resubscribe gaps so notifications, unread/sidebar state, room layout, server profile/settings, and active-call indicators do not depend on replayed subscription events. `Subscription.myEvents` is live-only and does not expose a replay cursor.
- `eventsAround` uses the same visible room timeline to center jump-to-message windows on the target's visible position.
- Thread panes read the root message from `RoomTimelineProjection` and cursor-paginated replies from `ThreadProjection`. Anchored thread refreshes use `threadRepliesAround(eventId:)` to keep a visible reply in the refreshed window.

**@Mentions:**

- `@username` patterns in message body are parsed as Markdown inline mention tokens (ASCII alphanumeric, underscore, hyphen, dot), excluding code spans, code blocks, and blockquotes.
- Mention handles are resolved in the posting room; direct user handles only notify current room members, and invalid/non-member handles are silently ignored.
- `MessagePostedEvent.mentioned_user_ids` contains resolved user IDs
- Mention resolution is post-time only; later `MessageEditedEvent` facts update body content but do not add, remove, dismiss, or re-send mention notifications
- Pending mention state is a notification record in `RUNTIME_STATE` (`notification.{userId}.{notificationId}`); sidebar notification count badges derive from pending notifications, not a separate mention flag.
- Live notification published to `live.sync.user.{userId}.mentioned` for toast display
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
