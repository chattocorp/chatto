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
- [GraphQL API Overview](#graphql-api-overview)
  - [Queries](#queries)
  - [Mutations](#mutations)
  - [Subscriptions](#subscriptions)
  - [Admin sub-API](#admin-sub-api)
- [Architecture Pattern: CRUD + Audit Log](#architecture-pattern-crud--audit-log)
  - [Write Path](#write-path)
  - [Consistency Model](#consistency-model)
- [NATS Resource Inventory](#nats-resource-inventory)
  - [Event Types](#event-types)
  - [Event Streams](#event-streams)
  - [KV Buckets (backed by streams)](#kv-buckets-backed-by-streams)
  - [Object Store Buckets](#object-store-buckets)
  - [Dynamic Image Transformation](#dynamic-image-transformation)
  - [Messages](#messages)
  - [Key Patterns](#key-patterns)

## Overview

Chatto is a real-time chat application with a GraphQL gateway and NATS/JetStream backend. Durable domain state is event-sourced in the `EVT` stream and served from projections; `RUNTIME_STATE` holds persisted latest-value runtime state such as notifications, push subscriptions, and auth tokens. Legacy KV buckets and `SERVER_EVENTS` are opened only when present so boot importers can seed `EVT` from pre-ES deployments.

### Core Concepts

- **Server**: A deployment of Chatto, consisting of 1-n application processes connected to the same NATS system and account. ("Instance" is the older name for this concept and persists in a handful of vestigial places — the `INSTANCE*` KV bucket names and the internal `RegisteredInstance`/`isInstanceAdmin` identifiers. Treat them as a rename-in-progress.)
- **Rooms**: Communication channels on the server. Can be named (`general`) or direct messages between users; differentiated by a `kind` field (`channel` / `dm`).
- **Users**: Global to the deployment, with account/profile state and per-room membership projected from `EVT`.

## NATS Authentication

Chatto supports multiple methods for authenticating with NATS, configured via `[nats.client]` in `chatto.toml`:

| Method        | Config             | Description                                                      |
| ------------- | ------------------ | ---------------------------------------------------------------- |
| `nkey`        | `nkey_seed`        | Default for embedded NATS. Uses Ed25519 keypairs.                |
| `userpass`    | `username`, `password` | Simple username/password authentication.                      |
| `credentials` | `credentials_file` | JWT authentication via standard `.creds` file (for external NATS). |
| `none`        | -                  | No authentication (for trusted networks only).                   |

**Embedded NATS Setup:**

When using embedded NATS (default), `chatto init` generates:
- `chatto.toml` with NKey seed in `[nats.client]`
- `nats-server.conf` with the corresponding public key in `authorization.users`

The `nats-server.conf` file is auto-generated on first startup if missing. Users can edit it to add clustering, TLS, or additional authorization rules.

**External NATS Setup:**

For connecting to an external NATS cluster with JWT authentication:
1. Set `nats.embedded.enabled = false`
2. Set `nats.client.auth_method = "credentials"`
3. Set `nats.client.credentials_file = "path/to/your.creds"`

## Architecture & APIs

Key files: [`cli/internal/core/core.go`](cli/internal/core/core.go)

- **NATS**: At the core, Chatto uses a series of NATS JetStream streams, KV buckets and object storage. Data stored in these is defined as Protocol Buffers (see `proto/`).
- **Core**: The `core` package defines Chatto's domain logic and directly talks to NATS to interact with KV buckets and streams. It provides a ChattoCore struct with methods for all operations (spaces, users, rooms, messages, memberships).
- **GraphQL**: Client-facing API for all operations (auth, management, messaging). Subscriptions over WebSocket for real-time updates. GraphQL resolvers call Core methods directly, performing authentication and authorization before each call.
- **Web Client**: SvelteKit-based SPA that gets compiled and embedded into the Go binary. Talks to GraphQL API over HTTP/WebSocket.
- **Email**: Optional SMTP integration for transactional emails (verification, password reset). Configured via `[smtp]` in config. The `internal/email` package provides a `Mailer` that returns `ErrSMTPDisabled` when SMTP is not configured, allowing callers to handle gracefully.

## GraphQL API Overview

Key files: [`cli/internal/graph/`](cli/internal/graph/) (schemas in `*.graphqls` files, resolvers in `*.resolvers.go`)

The GraphQL API is the primary client-facing interface for Chatto. It provides queries, mutations, and a single unified subscription over HTTP and WebSocket connections. Authentication is cookie-session-based; user registration, login, password reset, email verification, and OAuth flows are REST endpoints (under `/auth/...`) rather than GraphQL mutations.

The schema is modular: each feature area lives in its own `.graphqls` file and extends the root `Query` / `Mutation` / `Subscription` types. The operations below group by user-facing area, not by source file.

### Queries

**Server & identity** ([`server.graphqls`](../cli/internal/graph/server.graphqls), [`server_rbac.graphqls`](../cli/internal/graph/server_rbac.graphqls))

| Query                                | Description                                                                    |
| ------------------------------------ | ------------------------------------------------------------------------------ |
| `server`                             | Information about this Chatto server (name, branding, member counts). Public. |
| `viewer`                             | Current authenticated user's identity, permissions, follows, notifications.    |

Note: there is no top-level `me` query — viewer-scoped state hangs off the `viewer` field (which is extended by several feature files, e.g. `threads.graphqls` adds `viewer.followedThreads`, `notifications.graphqls` adds `viewer.notifications` / `viewer.hasNotifications`).

**Users** ([`query.graphqls`](../cli/internal/graph/query.graphqls), [`user_permissions.graphqls`](../cli/internal/graph/user_permissions.graphqls), [`permission_inspector.graphqls`](../cli/internal/graph/permission_inspector.graphqls))

| Query                              | Description                                                                            |
| ---------------------------------- | -------------------------------------------------------------------------------------- |
| `user(userId)`                     | Get a user by ID.                                                                      |
| `userByLogin(login)`               | Get a user by login (returns null if not found).                                       |
| `users(search, limit, offset)`     | Paginated user directory (server admin only).                                          |
| `userPermissionMatrix(userId)`     | Effective allow/deny matrix for a user (admin surface; `role.manage` + outrank gate).  |
| `permissionExplanation(userId, …)` | Per-permission resolver explainer (self-inspection or admin).                          |

**Rooms** ([`query.graphqls`](../cli/internal/graph/query.graphqls), [`room.graphqls`](../cli/internal/graph/room.graphqls))

| Query                              | Description                                                                            |
| ---------------------------------- | -------------------------------------------------------------------------------------- |
| `room(roomId)`                     | Get a room by ID. Room-scoped reads (`members`, `events`, `event(eventId)`, `eventsAround`, `voiceCallToken`, `viewerCan*` flags) live as fields on the returned `Room`; `members` is offset-paginated. |

**RBAC introspection** ([`role_permissions.graphqls`](../cli/internal/graph/role_permissions.graphqls), [`role_permission_matrix.graphqls`](../cli/internal/graph/role_permission_matrix.graphqls))

| Query                                       | Description                                                              |
| ------------------------------------------- | ------------------------------------------------------------------------ |
| `rolePermissions(roleName, roomId?)`        | A role's grants/denials across every applicable tier.                    |
| `tierRoles(roomId?, groupId?)`              | Full permission matrix at server / group / room scope.                   |
| `rolePermissionMatrix(roleName)`            | Per-role permission matrix (`role.manage` gated).                        |

**Voice & link previews** ([`voice.graphqls`](../cli/internal/graph/voice.graphqls), [`linkpreview.graphqls`](../cli/internal/graph/linkpreview.graphqls))

| Query                       | Description                                                                |
| --------------------------- | -------------------------------------------------------------------------- |
| `activeCallRoomIds`         | Room IDs that currently have an active LiveKit voice call.                 |
| `linkPreview(url)`          | Fetch (and cache) Open Graph metadata for a URL.                           |

**Admin** ([`admin.graphqls`](../cli/internal/graph/admin.graphqls))

Admin queries are nested under a single `admin: AdminQueries` field that returns `null` for non-admins — so one auth gate covers the whole sub-surface. See [Admin sub-API](#admin-sub-api) below for the contents.

### Mutations

**Server settings** ([`mutation.graphqls`](../cli/internal/graph/mutation.graphqls))

| Mutation                | Description                                                |
| ----------------------- | ---------------------------------------------------------- |
| `updateServer`          | Update server name / description.                          |
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
| `joinGroup`                    | Join every room in a group the caller has `room.join` for. Powers "Join all".    |
| `markRoomAsRead`               | Mark a room as read; records the last-seen root event ID for unread tracking.    |
| `startDM`                      | Start a DM with a participant set (returns existing room if the set matches).    |

**Messages, reactions, threads** ([`mutation.graphqls`](../cli/internal/graph/mutation.graphqls))

| Mutation                  | Description                                                                                  |
| ------------------------- | -------------------------------------------------------------------------------------------- |
| `postMessage`             | Post a message (root or thread reply; optional attachments / link previews / echo-to-channel).|
| `updateMessage`           | Update own message body (3-hour window).                                                     |
| `deleteMessage`           | Delete message body (GDPR crypto-shred); event stays in stream as audit trail.               |
| `deleteAttachment`        | Delete an attachment from own message.                                                       |
| `deleteLinkPreview`       | Delete a link preview from own message.                                                      |
| `addReaction` / `removeReaction` | Add or remove an emoji reaction (shortcode names).                                    |
| `sendTypingIndicator`     | Publish a transient "user is typing" live event.                                             |
| `markThreadAsRead`        | Update viewer's last-seen marker for a thread (drives unread separators).                    |
| `followThread` / `unfollowThread` | Subscribe / unsubscribe to thread reply notifications.                              |

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

**Room groups** ([`room_groups.graphqls`](../cli/internal/graph/room_groups.graphqls))

| Mutation                          | Description                                                                                  |
| --------------------------------- | -------------------------------------------------------------------------------------------- |
| `createRoomGroup`                 | Create a new room group (`role.manage`).                                                     |
| `updateRoomGroup`                 | Rename / re-describe a room group.                                                           |
| `deleteRoomGroup`                 | Delete a room group (must be empty).                                                         |
| `reorderRoomGroups`               | Reorder all room groups (full list, exactly once each).                                      |
| `reorderRoomsInGroup`             | Reorder rooms within a single group.                                                         |
| `moveRoomToSet`                   | Move a room into a different group (`room.manage` in both source and target — see ADR-031). |
| `grantGroupPermission`            | Grant a permission to a role at group scope (overrides server defaults).                     |
| `denyGroupPermission`             | Deny a permission to a role at group scope.                                                  |
| `clearGroupPermissionState`       | Remove both grant and denial at group scope.                                                 |

**Roles & permissions** ([`server_rbac.graphqls`](../cli/internal/graph/server_rbac.graphqls), [`server_rbac_extra.graphqls`](../cli/internal/graph/server_rbac_extra.graphqls))

| Mutation                          | Description                                                                                  |
| --------------------------------- | -------------------------------------------------------------------------------------------- |
| `createRole` / `updateRole` / `deleteRole` | CRUD for custom server roles (system roles are fixed).                              |
| `reorderRoles`                    | Reorder custom roles. System roles maintain fixed positions and are excluded.                |
| `assignRole` / `revokeRole`       | Add / remove a role assignment on a user (`role.assign` + outrank target).                   |
| `grantPermission` / `revokePermission` | Grant or revoke a permission on a role at server scope.                                 |
| `denyPermission`                  | Deny a permission on a role at server scope (clears any existing grant).                     |
| `clearPermissionState`            | Restore neutral state for a permission on a role at server scope.                            |
| `grantRoomPermission` / `denyRoomPermission` / `clearRoomPermission` | Same trio at room scope.                              |
| `grantUserPermission`             | Grant a permission directly to a user (beats role decisions; no self-action).                |
| `denyUserPermission`              | Deny a permission directly to a user (beats role grants; no self-action).                    |
| `clearUserPermissionState`        | Clear both grant and denial of a permission on a user.                                       |

**Admin** ([`admin.graphqls`](../cli/internal/graph/admin.graphqls))

Like `Query.admin`, the `admin: AdminMutations` field returns `null` for non-admins. See [Admin sub-API](#admin-sub-api) below.

### Subscriptions

| Subscription          | Description                                                                                                                                                                                                                                                                                                                                                                                                          |
| --------------------- | ---- |
| `myEvents`            | The single subscription. Multiplexes durable room events from `live.evt.>` (messages, reactions, edits, retractions, room lifecycle, asset processing) and transient sync signals from `live.sync.>` (typing, mention notifications, video-complete pings, voice call lifecycle, server config/profile/preference invalidation, notifications, thread-follow sync, presence, server membership lifecycle, session termination, heartbeats) into one GraphQL `Event` envelope. The membership set is tracked in real time — joining or leaving a room updates filtering immediately without reconnecting. DM-room events use the same membership gate as channel-room events; there is no separate DM read permission. Subscribing sets the caller's presence to `ONLINE`. Only new events stream; no historical replay. |

There is no `adminAuditLogEvents` subscription — audit events arrive through `myEvents` for users with the relevant admin scope.

### Admin sub-API

`Query.admin` returns `AdminQueries`; `Mutation.admin` returns `AdminMutations`. Both return `null` when the caller lacks admin access, so the nested fields don't need individual auth checks (see [FDR-021](fdr/FDR-021-admin-dashboard.md)). Admin operations are spread across multiple schema files but all hang off these two types.

| Field                                            | Type      | Description                                                                                  |
| ------------------------------------------------ | --------- | -------------------------------------------------------------------------------------------- |
| `admin.systemInfo`                               | Query     | Aggregate operational metrics: NATS connection + JetStream account usage totals.            |
| `admin.serverConfig`                             | Query     | Server configuration overrides (welcome message, MOTD, blocked usernames, OG description).  |
| `admin.serverPermissions`                        | Query     | List every available server permission identifier (catalog).                                 |
| `admin.groupRolePermissions(groupId, roleName)`  | Query     | Explicit grants and denials for a role on a specific room group.                             |
| `admin.groupUserPermissions(groupId, userId)`    | Query     | Explicit grants and denials for a user on a specific room group.                             |
| `admin.updateServerConfig(input)`                | Mutation  | Update server configuration.                                                                 |
| `admin.resetServerConfig`                        | Mutation  | Reset server configuration to defaults.                                                      |
| `admin.updateUser(input)`                        | Mutation  | Update a user's login / display name (bypasses the 30-day cooldown).                         |
| `admin.clearUsernameCooldown(userId)`            | Mutation  | Manually clear a user's login change cooldown.                                               |

## Architecture Pattern: CRUD + Audit Log Moving to Event Sourcing

### Write Path

| Type    | Resource                      | Purpose                                     |
| ------- | ----------------------------- | ------------------------------------------- |
| KV      | `RUNTIME_STATE`               | Persisted latest-value runtime/user state, including pending notifications, push subscriptions, and auth/workflow tokens |
| KV      | `MEMORY_CACHE`                | Volatile memory-backed cache state, including presence and active voice calls; excluded from backups |
| Objects | `INSTANCE_ASSETS`             | Avatars, icons (bucket name retained from pre-rename) |
| Objects | `ASSET_CACHE`                 | Cached resized images (optional, with TTL)  |
| Objects | `SERVER_ASSETS`               | Message attachments                         |
| Legacy import KV | `INSTANCE`          | Users, verified emails, branding, display preferences, and push subscriptions from pre-ES deployments |
| Legacy import KV | `INSTANCE_CONFIG`   | Server configuration from pre-ES deployments |
| Legacy import KV | `SERVER_CONFIG`     | Rooms, memberships, room sets/layout, and notification preferences from pre-ES deployments |
| Legacy import KV | `SERVER_RBAC`       | RBAC seed data from pre-ES deployments |
| Legacy import KV | `SERVER_RUNTIME`    | Read markers, thread follows, migration sentinels, and video processing state from pre-ES deployments |
| Legacy import KV | `SERVER_BODIES`     | Pre-ES message bodies and attachment metadata |
| Legacy import KV | `SERVER_REACTIONS`  | Emoji reactions from pre-ES deployments |
| Stream  | `SERVER_EVENTS`               | Legacy room event import source; no new runtime writes |

See [NATS Resource Inventory](#nats-resource-inventory) for detailed key patterns and subjects.

`EVT` publishing is mandatory for event-sourced domain facts because `EVT`
is the source of truth and reads come from in-memory projections. If event
publishing fails, the write fails. Current migrated aggregates include room
membership/metadata, room groups/layout, server config, users,
messages/threads, reactions, RBAC, and auth workflow audit facts.

### Consistency Model

**Latest-value KV/runtime state:**

- Strong consistency for KV operations
- Read-your-writes guaranteed via immediate KV updates
- Per-key TTLs are used for expiring records such as notifications and auth/workflow tokens
- These records are operational state, not durable domain history

**Migrated event-sourced aggregates:**

- `EVT` is the source of truth.
- Boot importers seed `EVT` from pre-ES KV/`SERVER_EVENTS` data when those resources exist.
- Reads come from in-memory projections rebuilt from `EVT`.
- Room timeline reads use `RoomTimelineProjection`'s derived visible-room index for initial loads, forward/backward pagination, and around-message windows. The raw room log still preserves folded facts such as edits, retractions, reactions, assets, and thread replies; visible readers skip or fold those facts before serving the room timeline.
- Writes append to `EVT` only; legacy KV/stream data is not maintained as a mirror.
- Read-your-writes is provided by waiting for the local projector to reach the append sequence.

**Future (Clustered NATS - Multi-Process):**

- KV buckets remain strongly consistent (NATS JetStream R3 replication)
- `EVT` provides durable audit/history and projection replay; transient live events provide UI sync.
- `EVT` retention is effectively forever until snapshot/archival policy is designed.
- `RUNTIME_STATE` can be rebuilt only from current operational exports or fresh user action, not from `EVT`, by design.

## Roles, Permissions, and Direct Messages

These sections previously described the RBAC model and DM behavior in detail. They've moved:

- **Roles, permissions, and the resolver** — see [FDR-001](fdr/FDR-001-roles-and-permissions.md) for the design and rationale, [`/.claude/rules/authorization.md`](../.claude/rules/authorization.md) for the full resolver semantics (DM boundary, user-level overrides, scope cascade), and [`/.claude/rules/admin.md`](../.claude/rules/admin.md) for the admin-side picture.
- **Permission constants and `Can*` functions** — see [`cli/internal/core/permission.go`](../cli/internal/core/permission.go) and [`cli/internal/core/can.go`](../cli/internal/core/can.go).
- **Direct Messages** — see [FDR-007](fdr/FDR-007-direct-messages.md) and [ADR-037 (DM Access via Membership)](adr/ADR-037-dm-access-via-membership.md).
- **Storage layout for RBAC and DM rooms** — captured in the [NATS Resource Inventory](#nats-resource-inventory) below alongside the rest of the KV.

## NATS Resource Inventory

### Event Types

Chatto uses `corev1.Event` as the durable EVT wrapper and `corev1.LiveEvent` as the transient NATS Core wrapper. GraphQL exposes both through one public `Event` envelope, but the protobuf wire envelopes stay separate so live-only sync signals cannot leak into the durable audit/event log shape.

- **Wrapper fields**: `id`, `created_at`, `actor_id`
- **Concrete event**: `event` oneof on the relevant wire envelope; contextual fields (`roomId`, etc.) live on the concrete payloads.

The oneof's field-number convention makes durability obvious at a glance:

- **`< 1000`** — persisted variants stored in JetStream. The field number is part of the on-disk wire format; do not change or reuse.
- **`>= 1000`** — retired legacy live-only `Event` tags, except frozen durable reaction tags `1050` / `1051`. New transient payloads belong in `LiveEvent`; new durable facts should use an intentional low-numbered block.

**Proto File Organization:**

| File | Contents | Safety |
| ---- | -------- | ------ |
| `event.proto` | Durable `Event` wrapper + persisted event message definitions | Changing field numbers/structure affects JetStream-stored data — requires careful migration |
| `live_events.proto` | Transient `LiveEvent` wrapper + live-only event message definitions | Safe to change freely — these are never persisted |

Both files share `package chatto.core.v1` and generate into the same Go package. `core.EventEnvelope` is the in-process GraphQL delivery interface that can carry durable EVT, transient LiveEvent, or a heartbeat through private concrete implementations. The `unwrapEvent` helper in `cli/internal/graph/event_helpers.go` is the single switch from that delivery envelope to a typed GraphQL payload; `unwrapEventAs[T]` is the typed wrapper used by the GraphQL resolvers.

**Event Categories:**

| Category                    | Storage    | Examples                                                    | Purpose                                                        |
| --------------------------- | ---------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| JetStream-stored (room)     | Stream     | RoomCreated, MessagePosted, MessageEdited, MessageRetracted, ReactionAdded, ReactionRemoved, UserJoinedRoom | Ordering guarantees, historical replay, projection source of truth |
| Room live-only              | NATS Core  | UserTyping, VideoProcessingCompleted, CallParticipantJoined, CallParticipantLeft | Ephemeral room notifications where another store/projection is source of truth |
| Deployment live (user/config) | NATS Core  | UserCreated, ServerUpdated, ConfigUpdated, MentionNotification, NotificationCreated, PresenceChanged | Cross-tab sync, notifications, server lifecycle |

The distinction between stored and live-only events is explicit in the wire envelope: durable facts use `corev1.Event`, transient signals use `corev1.LiveEvent`, and GraphQL exposes both through one `Event` envelope with typed payloads as members of the `EventType` union. Room queries and server subscriptions are delivery contexts, not separate wrapper types.

**Self-Contained Events:** Each concrete event contains all the IDs and context it needs:

- Room events contain `room_id`.
- Membership events contain relevant IDs (`room_id` for room joins/leaves).
- Self-initiated events (e.g., `PresenceChanged`) use the parent wrapper's `actor_id` instead of duplicating a `user_id` field.

**Event Publishing Strategy:**

User-facing live delivery is built from two internal NATS Core subject roots:

1. **Primary Stream** (persistent):
   - `SERVER_EVENTS` (subjects `server.>`) holds pre-ES room messages, thread replies, room meta lifecycle, and server-level member events for migration/import tooling. Runtime mutations no longer write it, and it no longer participates in live delivery.
   - `EVT` (subjects `evt.>`) holds event-sourced domain state. Its stream-level `RePublish` config forwards every committed event once onto `live.evt.>`. This is a raw committed-event feed, not a client contract.
2. **Direct Live Publish** (transient):
   - Transient UI sync signals publish as `corev1.LiveEvent` via NATS Core to `live.sync.>` — no stream storage.

The `myEvents` GraphQL subscription is backed by one core stream (`StreamMyEvents`) that subscribes to `live.sync.>` and `live.evt.>`. For deliverable raw EVT room messages, it reads the republished `Nats-Sequence` header, waits for the local projections needed by authorization and follow-up resolvers, filters by the subscribing user, and then emits the GraphQL event. Transient `LiveEvent` messages are adapted at this API boundary into the public GraphQL event shape. There is no per-connection JetStream consumer.

### Event Streams

| Stream                       | Wrapper          | Scope      | Description                                      |
| ---------------------------- | ---------------- | ---------- | ------------------------------------------------ |
| `SERVER_EVENTS`              | `corev1.Event`   | Server     | Pre-ES room/member event log retained for boot imports and inspection; no new runtime writes and no live delivery. |
| `EVT`                        | `corev1.Event`   | Server     | Event-sourcing log ([ADR-033](adr/ADR-033-event-sourced-state-with-projections.md) / [ADR-034](adr/ADR-034-single-event-stream.md)). Subjects `evt.{aggregateType}.{aggregateId}.{eventType}`; republishes onto `live.evt.>` as the raw committed-event feed. Currently fed by per-aggregate boot imports ([ADR-035](adr/ADR-035-per-aggregate-phased-migration.md)); migrated aggregates include room membership/metadata, groups/layout, server config, users, messages/threads, reactions, RBAC, and auth workflow audit facts. |
| Live Sync                    | `corev1.LiveEvent` | Transient  | Direct NATS Core pubsub on `live.sync.>` for transient UI sync signals. `myEvents` authorizes and adapts these messages into GraphQL events; they are never projection input. |

**SERVER\_EVENTS subjects (legacy import source):**

Pre-ES room events included event IDs in subjects for O(1) lookups via `GetLastMsgForSubject`. Current runtime reads use `EVT` projections instead; these subjects are documented so migrations and debugging tools can interpret historical data. The `{kind}` segment (`channel` or `dm`) lets a single subject namespace serve both server-space rooms and DM rooms.

| Subject                                                                       | Description                                    |
| ----------------------------------------------------------------------------- | ---------------------------------------------- |
| `server.member.joined` / `.left` / `.deleted`                                 | Membership lifecycle events                    |
| `server.room.{kind}.{roomId}.msg.{eventId}`                                   | Root message posted                            |
| `server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}`             | Thread reply posted                            |
| `server.room.{kind}.{roomId}.meta`                                            | Room lifecycle + membership                    |

The old event ID in message subjects enabled O(1) lookup (52µs) instead of O(n) scanning. Runtime message lookup now comes from `RoomTimelineProjection` rebuilt from `EVT`.

Filtering examples:

| Pattern                                                              | Description                                    |
| -------------------------------------------------------------------- | ---------------------------------------------- |
| `server.>`                                                           | All events                                     |
| `server.room.{kind}.{roomId}.>`                                      | All events in a room (messages + meta + threads) |
| `server.room.{kind}.{roomId}.msg.>`                                  | All messages (root + threads)                  |
| `server.room.{kind}.{roomId}.msg.*`                                  | Root messages only                             |
| `server.room.{kind}.>`                                               | All events of one kind (channels or DMs)       |
| `server.room.{kind}.{roomId}.msg.*.replies.>`                        | All thread replies in a room                   |
| `server.room.{kind}.{roomId}.msg.{rootEventId}.replies.>`            | All replies in a specific thread               |
| `server.room.{kind}.{roomId}.msg.*.replies.{eventId}`                | Lookup a thread reply by event ID              |

Note: Event type (created, joined, etc.) is determined by the event payload, not the subject. Actor/user information is also in payloads, not subjects (optimized for low subject cardinality).

**User Personal Streams** (transient):

- Subject: `user.{userId}.event`
- Published via NATS Core (not JetStream) - transient, not persisted
- Receives events relevant to the user (space joins/leaves, room joins/leaves)
- Powers real-time notifications and user-centric subscriptions
- Events are dual-published: to primary stream (audit trail) and user stream (notifications)

**Live Subject Space**:

Patterns: `live.sync.>` for transient `LiveEvent` pubsub and `live.evt.>` for raw EVT committed facts. `myEvents` consumes both roots server-side:

- Direct NATS Core publishes (`publishLiveEvent()`): transient `corev1.LiveEvent` messages on `live.sync.>` with no stream storage.
- `EVT` RePublish (`evt.>` → `live.evt.>`): every committed event-sourced fact is re-emitted once by JetStream. Chatto replicas must wait for local projection readiness and authorize before exposing deliverable room events to clients.

`SERVER_EVENTS` no longer has a `RePublish` live path and runtime code no longer writes legacy `server.>` mirrors. Remaining use is boot-import/read-only inspection of pre-ES data.

**Transient live sync events** (`live.sync.{user,config,room}.>`):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.sync.user.{userId}.created`                        | User registration completed  |
| `live.sync.user.{userId}.profile_updated`                | User profile changed (broadcast) |
| `live.sync.user.{userId}.user_deleted`                   | User account deleted         |
| `live.sync.config.updated`                               | Server config (name/MOTD/welcome) changed |
| `live.sync.config.server_updated`                        | Server branding (name/logo/banner/description) changed |
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
| `live.sync.room.{kind}.{roomId}.call_joined`             | Participant joined the LiveKit voice call |
| `live.sync.room.{kind}.{roomId}.call_left`               | Participant left the LiveKit voice call |
| `live.sync.room.{kind}.{roomId}.video_processed`         | Video attachment finished transcoding |

The unified `myEvents` GraphQL subscription is backed by a single core stream (`StreamMyEvents`) that combines:

- One `ChanSubscribe("live.sync.>")` for transient `LiveEvent` messages, and one `ChanSubscribe("live.evt.>")` for raw committed EVT facts. Authorization is applied per event: room membership for room subjects, `isAuthorizedForLiveEvent` for user/config/member subjects, and projection readiness before deliverable `live.evt.>` events.
- The PresenceHub (single per-process KV watcher on `presence.>` fanning out per-user status changes to all subscribers).
- An in-process heartbeat ticker (synthetic `Heartbeat` event every 25s for client-side liveness detection).

### KV Buckets (backed by streams)

| Bucket                        | Storage | Backup   | Description                                     |
| ----------------------------- | ------- | -------- | ----------------------------------------------- |
| `RUNTIME_STATE`               | File    | Yes      | Persisted latest-value runtime/user state, including pending notifications, push subscriptions, and auth/workflow tokens |
| `MEMORY_CACHE`                | Memory  | No       | Volatile cache state; presence keyed `presence.{userId}` with per-key TTL, active voice calls keyed `call.{spaceId}.{roomId}` |
| `ENCRYPTION_KEYS`             | File    | **No**   | KMS KEKs and app-owned wrapped DEK records (excluded for security) |
| `LINK_PREVIEW_CACHE`          | File    | No       | Legacy retired standalone link-preview cache; current entries live in `RUNTIME_STATE` |
| `USER_PRESENCE`               | Memory  | No       | Legacy retired presence bucket; not provisioned on fresh boot |
| `CALL_STATE`                  | Memory  | No       | Legacy retired active-call bucket; copied best-effort into `MEMORY_CACHE` on boot if present, not provisioned on fresh boot |
| `INSTANCE`                    | File    | Yes      | Legacy import source for users, verified emails, branding, display preferences, and push subscriptions; not provisioned on fresh boot |
| `INSTANCE_CONFIG`             | File    | Yes      | Legacy server configuration import source; not provisioned on fresh boot |
| `SERVER_CONFIG`               | File    | Yes      | Legacy rooms, memberships, room sets/layout, and notification preferences import source; not provisioned on fresh boot |
| `SERVER_RBAC`                 | File    | Yes      | Legacy RBAC seed data import source; not provisioned on fresh boot |
| `SERVER_RUNTIME`              | File    | Yes      | Legacy read markers, thread follows, migration sentinels, and video processing state import source; not provisioned on fresh boot |
| `SERVER_BODIES`               | File    | Yes      | Legacy message bodies and attachment metadata import source; not provisioned on fresh boot |
| `SERVER_REACTIONS`            | File    | Yes      | Legacy emoji reactions import source; not provisioned on fresh boot |

Fresh deployments create only current resources (`EVT`, projections' consumers, `RUNTIME_STATE`, live/runtime buckets, and object stores). Legacy KV buckets are opened opportunistically when present so boot importers can copy pre-ES state into `EVT` or `RUNTIME_STATE`; the application does not create or maintain them as mirrors.

Pre-ES room data — channels and DMs alike — lived in the unified `SERVER_*` buckets. Per-space buckets (`SPACE_{spaceId}_*`) and the old hidden-DM-space storage model are gone after the Phase 4 migration (#354): rooms were differentiated by a `kind` segment in their KV keys (e.g. `room.channel.{roomId}` vs `room.dm.{roomId}`). Current room state is projected from `EVT`.

**INSTANCE keys:**

| Key                                    | Description                                      |
| -------------------------------------- | ------------------------------------------------ |
| `user.{userId}`                        | User profile data                                |
| `user_by_login.{lowercase(login)}`     | Login-to-UserID index (case-insensitive)         |
| `auth.{userId}.password`               | Password hash (stored separately)                |
| `user.{userId}.avatar`                 | User avatar asset reference                      |
| `verified_emails.{userId}.{sha256(email)}` | One verified email per entry (proto `VerifiedEmail`) |
| `user_by_email.{sha256(email)}`        | Email-to-userId index (created on verification)  |
| `space.{spaceId}`                      | Vestigial primary-space record (key retained from pre-rename) |
| `instance.logo`                        | Legacy server logo asset reference, imported into EVT config events |
| `instance.banner`                      | Legacy server banner asset reference, imported into EVT config events |
| `space_membership.{spaceId}.{userId}`  | User-server membership tracking (vestigial slot) |
| `user_preferences.{userId}`            | User display preferences (timezone, time format) |

Notes: `INSTANCE` is legacy import-only. Current user/account/profile state is projected from `EVT`; legacy KV user records are imported into encrypted durable user events for login, display name, and verified email payloads using the user's active user-PII DEK epoch. Cookie-session records plus verification, registration, password-reset, account-deletion, bearer-session, and OAuth authorization-code token verifiers live in `RUNTIME_STATE` under HMAC-derived keys. Email verification claim facts use hashed email identifiers in `EVT` to preserve case-insensitive uniqueness without storing raw email values in audit events.

**RUNTIME_STATE auth/session keys:**

| Key                                           | Description |
| --------------------------------------------- | ----------- |
| `cookie_session.{userId}.{sessionHmac}`       | Server-side embedded-SPA cookie session record (proto `CookieSession`) with per-key TTL |
| `session.{hmac}`                              | Opaque bearer-token verifier with per-key TTL |
| `grant.{hmac}`                                | OAuth authorization-code verifier with 5-minute per-key TTL |
| `registration.{hmac}`                         | Email-first registration token verifier |
| `email_verification.{hmac}`                   | Email verification token verifier |
| `password_reset.{hmac}`                       | Password reset token verifier |
| `account_deletion_token.{hmac}`               | Account deletion confirmation token verifier |

**EVT auth audit subjects:**

| Subject                                                                  | Description |
| ------------------------------------------------------------------------ | ----------- |
| `evt.auth.server.registration_link_issued`                               | Registration link issued before a user exists. |
| `evt.auth.server.login_failed`                                           | Failed password login attempt, with hashed identifier only. |
| `evt.user.{userId}.email_verification_link_issued`                       | Email verification link issued. |
| `evt.user.{userId}.password_reset_link_issued`                           | Password reset link issued. |
| `evt.user.{userId}.account_deletion_confirmation_issued`                 | Account deletion confirmation token issued. |
| `evt.user.{userId}.password_reset_completed`                             | Password reset completed. |
| `evt.user.{userId}.login_succeeded` / `.logout_succeeded`                | Cookie-session login/logout completed. |
| `evt.user.{userId}.auth_code_issued`                                     | OAuth authorization code issued, with hashed redirect URI. |
| `evt.user.{userId}.auth_code_exchange_succeeded`                         | OAuth authorization code exchange completed. |
| `evt.user.{userId}.auth_code_exchange_failed`                            | Known OAuth authorization code exchange failed after code lookup. |
| `evt.user.{userId}.bearer_token_issued` / `.bearer_token_revoked`         | Opaque bearer token issued or explicitly revoked. |

These audit payloads include only safe request metadata: capped user agent and an HMAC-SHA256 IP hash when request metadata is available. Raw tokens, links, passwords, auth codes, raw IP addresses, and raw email/login identifiers are not persisted in EVT audit payloads.

**INSTANCE_CONFIG keys:**

| Key               | Description                                                                  |
| ----------------- | ---------------------------------------------------------------------------- |
| `config.instance` | Legacy server configuration import source (proto message; key + proto name retained) |

Notes: Server configuration now lives in EVT config events and is served from the in-memory config projection. This bucket is retained as a boot-import source for pre-ES deployments and is not created on fresh boot. The TOML file remains reserved for operational settings (ports, secrets, NATS config).

**ENCRYPTION_KEYS keys:**

| Key             | Description                       |
| --------------- | --------------------------------- |
| `kek.{keyRef}`  | Protobuf `UserKeyEncryptionKey` per-user KEK record addressed by opaque KMS key ref; legacy raw 32-byte compatibility is also accepted |
| `dek.{keyRef}`  | Protobuf `UserDataEncryptionKey` wrapped purpose-scoped DEK record addressed by opaque app content-key ref |
| `user.{userId}` | Raw 32-byte legacy direct-key message-body compatibility path only |

Notes: Excluded from backups so backup archives contain only encrypted data, not the keys to decrypt it. Chatto core uses the in-process `internal/kms` wrapper boundary for KEK creation, DEK wrap/unwrap, and KEK shredding. New built-in KMS writes store KEKs as protobuf `UserKeyEncryptionKey` records under `kek.*` refs, while raw 32-byte `kek.*` records remain readable for compatibility with older exports. New message bodies and durable user PII store wrapped, purpose-scoped DEK epochs as app-owned protobuf `UserDataEncryptionKey` records under `dek.*` refs; the user EVT stream stores `UserDEKGeneratedEvent` audit facts with the purpose, epoch, content-key ref, wrapping algorithm, opaque wrapping key ref, and provider metadata. Legacy bodies use the local raw `user.{userId}` KEK directly only for compatibility. Enables GDPR-compliant crypto-shredding: shredding a user's recorded content-key refs or wrapping-key refs renders their encrypted content permanently unreadable.

**SERVER\_CONFIG keys:**

Room and membership keys in this legacy import bucket carry a `kind` segment (`channel` or `dm`) so listing operations can prefix-filter without loading and deserializing every record. Current room and membership state is projected from `EVT`; `SERVER_CONFIG` is not created on fresh boot.

| Key                                                  | Description                                      |
| ---------------------------------------------------- | ------------------------------------------------ |
| `room.channel.{roomId}`                              | Channel-style room. The Room proto carries `set_id` referencing its `RoomSet` (ADR-031). |
| `room.dm.{roomId}`                                   | Direct-message room (no `set_id` — DMs aren't part of any room set). |
| `room_name_index.{lowercaseName}`                    | Atomic name claim → room ID. Channels only; DMs have empty names. Enforces case-insensitive uniqueness without a read-then-write race. |
| `room_membership.channel.{roomId}.{userId}`          | Channel membership (room-first ordering matches `room.{kind}.{X}`)  |
| `room_membership.dm.{roomId}.{userId}`               | DM membership                                    |
| `room_layout`                                        | Single proto holding the ordered list of `RoomSet`s (and each set's ordered room IDs). Updated atomically with OCC. |

Useful filter patterns:

| Pattern                                              | Matches                                          |
| ---------------------------------------------------- | ------------------------------------------------ |
| `room.channel.*`                                     | All channel rooms                                |
| `room.dm.*`                                          | All DM rooms                                     |
| `room.*.*`                                           | All rooms regardless of kind                     |
| `room_membership.{kind}.{roomId}.*`                  | All members of one room (pure prefix)            |
| `room_membership.{kind}.*.{userId}`                  | A user's memberships of one kind (server-side wildcard) |
| `room_membership.{kind}.>`                           | All memberships of one kind                      |

**SERVER\_RBAC keys:**

RBAC is event-sourced under `evt.rbac.>`. Role CRUD/reorder, role assignment,
direct user overrides, and server-scoped permission decisions use
`evt.rbac.server.*`; room and group scoped decisions use
`evt.rbac.{roomId}.*` and `evt.rbac.{groupId}.*` with the Chatto entity ID
directly as the aggregate ID. All RBAC writes share `evt.rbac.>` as their OCC
domain, so those subject partitions are descriptive labels, not independent
consistency boundaries. Permission checks, admin role/permission reads,
permission inspector traces, and hierarchy/outrank checks read from the
in-memory RBAC projection.

`SERVER_RBAC` is retained only as legacy import evidence until the aggregate
cleanup phase. The RBAC boot importer reads historical `role.*`, `member.*`,
`allow.*`, `deny.*`, `group_allow.*`, `group_deny.*`, `room_allow.*`, and
`room_deny.*` keys into `EVT` using OCC so repeated boots skip an already seeded
RBAC subject family.

**RUNTIME\_STATE keys:**

`RUNTIME_STATE` is the persisted home for latest-value runtime state that
survives restart but is not content/domain history. See
[ADR-036](adr/ADR-036-runtime-state-kv-boundary.md).

| Key                                    | Description                                                       |
| -------------------------------------- | ----------------------------------------------------------------- |
| `read.room.{userId}.{roomId}`          | Last-read root message event ID (UTF-8 string, ~14 bytes). Empty value = "joined but no specific event read yet" (e.g. joined an empty room). Missing key triggers a one-time lazy init to the room's current last event ("caught up at first read post-deploy"). Legacy `SERVER_RUNTIME` `room_read_event.*` keys are copied here at boot; older `room_read_status.*` sequence keys are orphaned and ignored. |
| `read.thread.{userId}.{roomId}.{threadRootEventId}` | Latest thread message event ID the user has seen. Values copied from legacy `thread_last_opened.*` may be 8-byte UnixNano timestamps until rewritten by a new read action. |
| `notification.{userId}.{notificationId}` | Pending notification record (protobuf `Notification`) for DM messages, @mentions, replies, and all-message subscriptions. Uses per-key 90-day TTL. Live sync uses `NotificationCreatedEvent` / `NotificationDismissedEvent` on `live.sync.user.{userId}.*`. |
| `push_subscription.{userId}.{endpointHash}` | Web Push subscription record (protobuf `PushSubscription`) for a user's browser/device. Legacy `INSTANCE` keys are copied here at boot; the endpoint hash keeps multiple devices per user while deduplicating the same browser subscription. |
| `registration.{hmac}` | Email-first registration token JSON. Uses per-key 24-hour TTL. |
| `email_verification.{hmac}` | Email verification token JSON with user ID and email. Uses per-key 24-hour TTL. |
| `password_reset.{hmac}` | Password reset token JSON. Uses per-key 1-hour TTL. |
| `account_deletion_token.{hmac}` | Account deletion confirmation token JSON. Uses per-key 15-minute TTL. |
| `session.{hmac}` | Opaque bearer auth token JSON. Uses per-key `auth.token_ttl` (default 90 days); successful validation refreshes the key with a new per-key TTL for sliding-window expiry. |
| `grant.{hmac}` | OAuth authorization code JSON. Uses per-key 5-minute TTL and is deleted on exchange attempt. |
| `link_preview.{urlHash}` | Cached link preview metadata (protobuf `CachedLinkPreview`) keyed by SHA-256 of the normalized URL. Successful previews use per-key 24-hour TTL; failed fetches use per-key 1-hour TTL. |

Token HMAC keys are derived with `[core].secret_key` and the token family as a domain separator. Backups include `RUNTIME_STATE`, so sessions and pending links survive restore only when the same `core.secret_key` is kept; backup archives do not contain raw bearer tokens or raw link/code values.

**SERVER\_RUNTIME keys:**

| Key                                    | Description                                                       |
| -------------------------------------- | ----------------------------------------------------------------- |
| `room_last_msg_at.{roomId}`            | Last message timestamp (per-room, used for sidebar sort)          |
| `video.{attachmentId}`                 | Legacy video processing state imported to EVT video manifest events at boot |
| `attachment_records.backfilled`        | Sentinel set after the `BackfillAttachmentRecords` boot migration completes; short-circuits the scan on subsequent boots. |
| `video_manifest_es.migrated`           | Sentinel set after legacy `video.*` records are imported into EVT |

These keys don't carry a kind segment — `roomId` is globally unique, so direct lookup works for DM and channel rooms alike.

**MEMORY_CACHE keys:**

| Key                                        | Description                                      |
| ------------------------------------------ | ------------------------------------------------ |
| `presence.{userId}`                        | Serialized `UserPresence` proto for the user's live status; per-key 60s TTL |
| `call.{spaceId}.{roomId}`                  | JSON active voice call participant list          |

Notes: Memory-based storage (not persisted, not backed up). Presence uses per-key TTL with 30-second client refresh and `LimitMarkerTTL` so NATS emits delete markers on TTL expiry. A single per-process **PresenceHub** watches `presence.>` and emits `PresenceChanged` only when a user's status changes. `Subscription.myEvents` sets the user online, and `updateMyPresence` overwrites the user's live status. On disconnect, clients do not write `OFFLINE`; they stop refreshing and TTL handles expiry. Voice call state is also volatile and is repopulated by LiveKit webhooks; legacy `CALL_STATE` entries are copied into `MEMORY_CACHE` on boot during the storage rename.

**SERVER\_BODIES keys:**

| Key                    | Description                                              |
| ---------------------- | -------------------------------------------------------- |
| `{userId}.{eventId}`   | Message body keyed by user ID and event ID               |
| `{userId}.{bodyId}`    | Older naming for the same legacy body record shape       |

Notes: The compound key format `{userId}.{eventId}` enables efficient prefix-based deletion for GDPR compliance (delete all messages for a user via prefix scan). Separated from metadata for performance and operational flexibility. No `kind` segment — both IDs are globally unique NanoIDs.

A transitional `attachment.{roomId}.{attachmentId}` key shape existed
in this bucket between #575 and #581 as a per-attachment authz index;
it was retired by the signed-locator URL scheme (ADR-032) and any
leftover entries are swept at boot by the `DropLegacyAttachmentRecords`
migration. New code should not write to `attachment.*` keys here.

**SERVER\_REACTIONS keys:**

| Key                                     | Description                                    |
| --------------------------------------- | ---------------------------------------------- |
| `{messageEventId}.{emojiName}.{userId}` | Reaction tracking (empty value = reacted; value stores nanosecond timestamp for "added at" ordering) |

Notes: Legacy source for `MigrateReactionsToES`. Current reaction writes append durable `ReactionAdded` / `ReactionRemoved` events to `evt.room.{roomId}.reaction_added` and `evt.room.{roomId}.reaction_removed`; current reaction state is derived by an in-memory projection keyed by message event ID, emoji shortcode, and actor/user ID. The bucket remains so old deployments can be imported and will be removed in the later cleanup phase.

### Object Store Buckets

| Bucket                      | Description                                       |
| --------------------------- | ------------------------------------------------- |
| `INSTANCE_ASSETS`           | User avatars, server icon/banner (bucket name retained from pre-rename) |
| `ASSET_CACHE`               | Cached resized images (optional)                  |
| `SERVER_ASSETS`             | Message attachments                               |

**INSTANCE_ASSETS keys:**

| Key          | Description                         |
| ------------ | ----------------------------------- |
| `{assetId}`  | User avatars, space icons, etc.     |

Notes: Content-Type stored in object headers. S2 compression enabled. Server logo and banner bytes remain here when backed by the NATS object store; the current logo/banner pointers are imported into and updated through EVT config events.

**ASSET_CACHE keys:**

| Key                                       | Description                                  |
| ----------------------------------------- | -------------------------------------------- |
| `attachment.{attachmentId}.{paramsHash}`  | Cached WebP image at specific dimensions     |
| `server.{assetId}.{paramsHash}`           | Cached WebP transform of a server asset      |

Notes: Only created when `[core.assets.cache]` is enabled in config. Uses TTL for automatic expiration (default 7 days). `paramsHash` is first 16 hex chars of SHA256(`{width}x{height}_{fit}`). Animated GIFs are not cached (served directly). S2 compression enabled. Pre-ADR-030-Phase-4 entries written under a `{server|DM}.…` prefix are no longer looked up after the kind-less URL switchover and age out via TTL.

**SERVER\_ASSETS keys (primary + DM, post phase 4e):**

| Key                   | Description                                     |
| --------------------- | ----------------------------------------------- |
| `{attachmentId}`      | Original attachment files (images, videos, etc.)|
| `{attachmentId}_thumb`| WebP thumbnails (256px max dimension)           |

Notes: Asset IDs are globally unique (NanoID), so no kind segment is needed. Channel and DM assets share the same flat keyspace. Content-Type and original filename stored in object headers. S2 compression enabled. Asset **metadata** (filename, dimensions, duration, storage pointer, …) is created in `AssetCreatedEvent`; ownership context lives on the event (`message`, `derivative`, `user_avatar`) rather than inside `Asset`. Future room or server asset owners should add explicit owner branches when those features start emitting asset events. Message-owned assets are also embedded as `Attachment` protos inside the owning `MessageBody` for message rendering and signed URL back-pointers. Processing events refer to created asset IDs. Message posting asks the process-local video service to spawn video/animated-GIF processing after appending asset creation and processing-started events; there is no transient NATS Core worker subject. Boot recovery derives missed work from the EVT projection and calls the same local path. Video processing success records thumbnail/variant asset IDs, while each derivative binary is separately declared with `AssetCreatedEvent` and an owner pointing at the original asset. `AssetProcessingFailedEvent.failure_code` records failed/unavailable outcomes. Account deletion follows the projected message asset graph and appends `AssetDeletedEvent` for source assets and derivative children before deleting backing bytes. Boot migrations backfill asset creation events from legacy message attachments and import legacy `SERVER_RUNTIME video.*` records into asset creation and processing events. The asset HTTP handler doesn't look up a separate index bucket; the body-or-video-manifest locator travels in the URL itself as a signed locator (see "Dynamic Image Transformation" below).

### Dynamic Image Transformation

Chatto supports on-the-fly image transformation for attachments, user avatars, and server branding images, allowing clients to request images at specific dimensions without pre-generating all possible sizes.

**URL Structure:**

Attachment URLs encode everything the HTTP handler needs (roomId for
authz; bodyKey or videoOriginId for source-of-truth lookup;
attachmentId) into a signed locator path segment. Originals:

```
/assets/attachments/{base64payload}.{hexHMAC}
```

Transforms append the standard signed-transform-path component:

```
/assets/attachments/{base64payload}.{hexHMAC}/t/{base64params}.{signature}
```

Where:

- `{base64payload}` — base64url-encoded JSON `{r, b?, v?, a}` (room id; exactly one of bodyKey or videoOriginId; attachment id)
- `{hexHMAC}` — first 16 bytes of HMAC-SHA256 of `{base64payload}` (32 hex chars)
- `{base64params}` — base64url-encoded JSON `{"w":640,"h":512,"f":"contain"}`
- `{signature}` — first 16 bytes of HMAC-SHA256 of `attachment/{locator}/{base64params}` (32 hex chars)

Both HMACs use the same `[core.assets].signing_secret`. The HTTP handler
verifies the locator signature, then resolves the source proto by
fetching `MessageBody` / `AssetCreatedEvent` state (for body attachments) or the projected
`AssetProcessingSucceededEvent.video` manifest (for variants/thumbnails) — no
separate index bucket lookup is needed.

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

The `Attachment`, `User`, and `ServerConfig` image fields expose transform parameters as field arguments:

```graphql
type Attachment {
  url(width: Int, height: Int, fit: FitMode): String!
  thumbnailUrl(width: Int, height: Int, fit: FitMode): String
}

type User {
  avatarUrl(width: Int, height: Int, fit: FitMode): String
}

type ServerConfig {
  logoUrl(width: Int, height: Int, fit: FitMode): String
  bannerUrl(width: Int, height: Int, fit: FitMode): String
}

enum FitMode {
  CONTAIN
  COVER
  EXACT
}
```

When arguments are provided, the resolver returns a signed transform URL. Without arguments, the original/default thumbnail URL is returned for backward compatibility.

**Caching:**

Transformed images are generated on-demand with aggressive HTTP caching:

- `Cache-Control: public, max-age=31536000, immutable` (1 year)
- `ETag` based on attachment ID and transform parameters
- No server-side caching; relies on CDN/proxy caching

**Output Format:**

All transformed images are encoded as WebP for optimal compression and quality.

### Messages

Messages are persisted as durable `EVT` facts. Public timeline facts (`MessagePostedEvent`, `MessageEditedEvent`, `MessageRetractedEvent`) are bodyless on new writes; encrypted bodies live in private `MessageBodyEvent` payload facts on `evt.room.{roomId}.message_body` and are not delivered through live user subscriptions. New bodies use the compact ADR-007 v2 envelope: XChaCha20-Poly1305 encrypts the body with the author's active message-body DEK epoch. AAD binds the message event ID, body event ID, room ID, author ID, and epoch. Per-user wrapped DEKs live in app-owned `ENCRYPTION_KEYS` records; user EVT records only their purpose, epoch, content-key ref, wrapping algorithm, opaque wrapping key ref, and provider metadata for future KMS implementations. New durable user PII fields use a separate user-PII DEK epoch with user-event-specific AAD. The older `SERVER_EVENTS` + `SERVER_BODIES` store-then-publish shape is retained only as import evidence and for legacy backup restores.

**Message Identifiers:**

- **Event ID**: NanoID (e.g., `E...`) on the EVT envelope. This is the durable message identity used for GraphQL `Event.id`, reactions, thread metadata, message-body lookup, attachments, and projections.
- **Payload**: `MessagePostedEvent` is payload-only. It carries room/thread/echo fields, but not an event ID or message-body ID alias. Its `body` field is legacy decode-only.
- **Legacy import**: older `SERVER_EVENTS` records may contain an unknown-field `message_body_id` pointing at the legacy `{userId}.{eventId}` `SERVER_BODIES` key. The ES importer uses that only while copying legacy state into `MessageBodyEvent` + bodyless `MessagePostedEvent` EVT facts.

**Write Path:**

1. Generate an EVT envelope with event ID
2. Generate a private body event ID and encrypt the body with the author's active message-body DEK epoch
3. Atomically append `MessageBodyEvent` before the bodyless `MessagePostedEvent`
4. Wait for local projections to reach the public message append sequence before serving read-your-writes

**Threading:**

- `in_reply_to` field stores the event ID of the parent message (empty for top-level messages)
- `in_thread` field stores the event ID of the thread root (empty for top-level messages)
- Thread replies are ordinary `MessagePostedEvent` facts on `evt.room.{roomId}.message_posted` with `in_thread` set to the root event ID.
- Thread reply lists, reply counts, participants, followed-thread pages, and last-reply timestamps are derived from the `ThreadProjection`.

**Read Path:**

- Room-level message history is served from `RoomTimelineProjection`, which keeps the raw room event log plus derived indexes for latest body state, hidden echoes, assets, and room-visible timeline entries.
- Initial loads and cursor pagination walk the derived visible-room index so thread replies, edits, retractions, reactions, asset-processing facts, and directly hidden echoes do not count as separate room timeline rows.
- `eventsAround` uses the same visible-room index to center jump-to-message windows on the target's visible position.
- Thread panes read the root message from `RoomTimelineProjection` and replies from `ThreadProjection`.

**@Mentions:**

- `@username` patterns in message body are extracted via regex (ASCII alphanumeric, underscore, hyphen)
- Usernames are resolved to user IDs; only server members are included (non-members silently ignored)
- `MessagePostedEvent.mentioned_user_ids` contains resolved user IDs
- Mention resolution is post-time only; later `MessageEditedEvent` facts update body content but do not add, remove, dismiss, or re-send mention notifications
- Pending mention state is a notification record in `RUNTIME_STATE` (`notification.{userId}.{notificationId}`); sidebar orange dots derive from pending notifications, not a separate mention flag.
- Live notification published to `live.sync.user.{userId}.mentioned` for toast display
- Mention notifications are dismissed when the user views the relevant room or thread, or explicitly dismisses them from the notification center.
- Self-mentions are filtered out (no notification to message author)

**GDPR Deletion:**

- Delete appends `MessageRetractedEvent` to `EVT`; projections tombstone the message body before rendering.
- Edit appends a new private `MessageBodyEvent` before a bodyless public `MessageEditedEvent`; obsolete body payload events are securely deleted best-effort after projection catch-up.
- Attachment bytes are deleted from backing object storage best-effort and corresponding asset deletion facts are appended.

### Key Patterns

- **Unified Event Subscriptions**: The `myEvents` subscription merges EVT republish (`live.evt.>`), transient sync (`live.sync.>`), and PresenceHub updates into one authorized user stream.
- **Compression**: The `EVT` and legacy `SERVER_EVENTS` streams use S2 compression to reduce storage costs
- **GDPR Compliance**: Message bodies are encrypted per author; deletion is represented by EVT retraction/shred facts and projections refuse to render shredded or retracted content.
- **Unified Event-Sourced Rooms**: Channels and DMs share `evt.room.{roomId}.>` subjects and room projections; legacy `SERVER_*` buckets are import-only.
- **Legacy Body Store**: `SERVER_BODIES` is retained for pre-ES imports and legacy backup restores; new message bodies are encrypted into private `MessageBodyEvent` payloads and projected from `EVT`.
- **Current Resource Initialization**: Current resources are created up front at boot. Legacy import resources (`INSTANCE`, `INSTANCE_CONFIG`, `SERVER_CONFIG`, `SERVER_RBAC`, `SERVER_RUNTIME`, `SERVER_BODIES`, `SERVER_REACTIONS`, `SERVER_EVENTS`) are opened only if they already exist.
