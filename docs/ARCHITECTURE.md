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

Chatto is a real-time chat application with a GraphQL gateway and NATS/JetStream backend. The architecture uses **KV buckets as the source of truth** for data storage, with **event streams providing audit trails** and real-time pub/sub capabilities.

### Core Concepts

- **Server**: A deployment of Chatto, consisting of 1-n application processes connected to the same NATS system and account. ("Instance" is the older name for this concept and persists in a handful of vestigial places — the `INSTANCE*` KV bucket names and the internal `RegisteredInstance`/`isInstanceAdmin` identifiers. Treat them as a rename-in-progress.)
- **Rooms**: Communication channels on the server. Can be named (`general`) or direct messages between users; differentiated by a `kind` field (`channel` / `dm`).
- **Users**: Global to the deployment, with server membership tracked centrally and per-room membership managed in `SERVER_CONFIG`.

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
| `users`                            | List all users (server admin only).                                                    |
| `userPermissionMatrix(userId)`     | Effective allow/deny matrix for a user (admin surface; `role.manage` + outrank gate).  |
| `permissionExplanation(userId, …)` | Per-permission resolver explainer (self-inspection or admin).                          |

**Rooms** ([`query.graphqls`](../cli/internal/graph/query.graphqls), [`room.graphqls`](../cli/internal/graph/room.graphqls))

| Query                              | Description                                                                            |
| ---------------------------------- | -------------------------------------------------------------------------------------- |
| `room(roomId)`                     | Get a room by ID. Room-scoped reads (`events`, `event(eventId)`, `eventsAround`, `voiceCallToken`, `viewerCan*` flags) live as fields on the returned `Room`. |

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
| `myEvents`            | The single subscription. Multiplexes room events (messages, reactions, typing, edits, deletes, mention notifications, video processing, voice call lifecycle) and deployment-scoped events (server config, profile updates, room CRUD, room-layout changes, notifications, thread-follow sync, presence, server membership lifecycle, session termination, heartbeats) into one envelope. The membership set is tracked in real time — joining or leaving a room updates filtering immediately without reconnecting. DM-room events are additionally gated by `dm.view`. Subscribing sets the caller's presence to `ONLINE`. Only new events stream; no historical replay. |

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
| KV      | `INSTANCE`                    | Users, memberships (bucket name retained from pre-rename) |
| KV      | `INSTANCE_CONFIG`             | Server runtime configuration overrides      |
| KV      | `USER_PRESENCE`               | Presence status (memory, TTL 60s)           |
| KV      | `AUTH_TOKENS`                 | Legacy bearer auth tokens pending `RUNTIME_STATE` migration |
| KV      | `RUNTIME_STATE`               | Persisted latest-value runtime/user state, including pending notifications |
| KV      | `SERVER_CONFIG`               | Rooms (channel + DM), memberships           |
| KV      | `SERVER_RBAC`                 | Legacy RBAC seed data read by the `EVT` boot migration |
| KV      | `SERVER_RUNTIME`              | Legacy runtime state pending cleanup        |
| KV      | `SERVER_BODIES`               | Message bodies (GDPR-compliant) + standalone attachment metadata records — TODO: rename → `SERVER_CONTENT` |
| KV      | `SERVER_REACTIONS`            | Legacy emoji reactions source for boot migration only |
| KV      | `SERVER_THREADS`              | Thread metadata (reply count, participants) |
| Objects | `INSTANCE_ASSETS`             | Avatars, icons (bucket name retained from pre-rename) |
| Objects | `ASSET_CACHE`                 | Cached resized images (optional, with TTL)  |
| Objects | `SERVER_ASSETS`               | Message attachments                         |
| Stream  | `SERVER_EVENTS`               | Room/membership events                      |

See [NATS Resource Inventory](#nats-resource-inventory) for detailed key patterns and subjects.

**Important:** Event publishing is best-effort only for aggregates that
still use the legacy CRUD + audit-log pattern. If event publishing fails
there, the operation can still succeed because the KV store is the source
of truth and the event is additive.

**Exception:** Aggregates migrated to event sourcing require successful
`EVT` publishing because `EVT` is their source of truth and reads come
from in-memory projections. If event publishing fails, the write fails.
Current migrated aggregates include room membership/metadata,
room groups/layout, server config, messages/threads, reactions, and RBAC.

### Consistency Model

**Legacy CRUD aggregates:**

- Strong consistency for KV operations (source of truth)
- Read-your-writes guaranteed via immediate KV updates
- Event streams provide audit trail with best-effort delivery
- No store-mirroring problem: KV is source of truth, events are additive

**Migrated event-sourced aggregates:**

- `EVT` is the source of truth.
- Boot importers seed `EVT` from pre-ES KV/`SERVER_EVENTS` data.
- Reads come from in-memory projections rebuilt from `EVT`.
- Writes append to `EVT` only; legacy KV/stream data is not maintained as a mirror.
- Read-your-writes is provided by waiting for the local projector to reach the append sequence.

**Future (Clustered NATS - Multi-Process):**

- KV buckets remain strongly consistent (NATS JetStream R3 replication)
- Event streams continue providing audit trail and pub/sub
- Configurable retention policies on legacy `SERVER_EVENTS` only where events remain additive; `EVT` retention is effectively forever until snapshot/archival policy is designed.
- Can rebuild/migrate KV stores from current state exports (not from events)

**Benefits of the legacy CRUD approach:**

- Simple to understand and debug (CRUD operations with event logging)
- Can safely age out old events based on retention policy
- No complex event replay or projection rebuilding required
- Storage costs scale with active data, not infinite history
- Still provides full audit trail for compliance/debugging (until retention expires)

## Roles, Permissions, and Direct Messages

These sections previously described the RBAC model and DM behavior in detail. They've moved:

- **Roles, permissions, and the resolver** — see [FDR-001](fdr/FDR-001-roles-and-permissions.md) for the design and rationale, [`/.claude/rules/authorization.md`](../.claude/rules/authorization.md) for the full resolver semantics (DM boundary, user-level overrides, scope cascade), and [`/.claude/rules/admin.md`](../.claude/rules/admin.md) for the admin-side picture.
- **Permission constants and `Can*` functions** — see [`cli/internal/core/permission.go`](../cli/internal/core/permission.go) and [`cli/internal/core/can.go`](../cli/internal/core/can.go).
- **Direct Messages** — see [FDR-007](fdr/FDR-007-direct-messages.md) and [ADR-015 (DMs as a Hidden Space)](adr/ADR-015-dms-as-hidden-space.md).
- **Storage layout for RBAC and DM rooms** — captured in the [NATS Resource Inventory](#nats-resource-inventory) below alongside the rest of the KV.

## NATS Resource Inventory

### Event Types

Chatto uses a single protobuf wrapper, `corev1.Event`, for every event a user can receive — both the JetStream-stored room-scoped events and the deployment-scoped live events. The earlier two-wrapper split (`SpaceEvent` + `InstanceEvent` / live wrappers) was retired in PR #429: storage decisions (JetStream vs. NATS Core) belong to the publisher path, not the message type.

- **Wrapper fields**: `id`, `created_at`, `actor_id`
- **Concrete event**: `event` oneof; contextual fields (`spaceId`, `roomId`, etc.) live on the concrete payloads.

The oneof's field-number convention makes durability obvious at a glance:

- **`< 1000`** — persisted variants stored in JetStream. The field number is part of the on-disk wire format; do not change or reuse.
- **`>= 1000`** — live-only variants published to NATS Core. Free to reassign, modulo a single-deployment in-flight constraint.

**Proto File Organization:**

| File | Contents | Safety |
| ---- | -------- | ------ |
| `event.proto` | `Event` wrapper + the persisted event message definitions | Changing field numbers/structure affects JetStream-stored data — requires careful migration |
| `live_event.proto` | All live-only event message definitions | Safe to change freely — these are never persisted |

Both files share `package chatto.core.v1` and generate into the same Go package. The `unwrapEvent` helper in `cli/internal/graph/event_helpers.go` is the single switch from the proto oneof to a typed payload; `unwrapEventAs[T]` is the typed wrapper used by the GraphQL resolvers.

**Event Categories:**

| Category                    | Storage    | Examples                                                    | Purpose                                                        |
| --------------------------- | ---------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| JetStream-stored (room)     | Stream     | RoomCreated, MessagePosted, ReactionAdded, ReactionRemoved, UserJoinedRoom | Ordering guarantees, historical replay, projection source of truth |
| Room live-only              | NATS Core  | MessageDeleted, MessageUpdated, PresenceChanged, UserTyping | Ephemeral room notifications where another store/projection is source of truth |
| Deployment live (user/space/config) | NATS Core  | UserCreated, SpaceUpdated, ConfigUpdated, MentionNotification, NotificationCreated | Cross-tab sync, notifications, server lifecycle |

The distinction between stored and live-only events is based on how they're published (JetStream vs NATS Core). All variants share the single `corev1.Event` envelope; GraphQL exposes them through one `ServerEvent` wrapping union with the typed payloads as members of the `ServerEventType` union.

**Self-Contained Events:** Each concrete event contains all the IDs and context it needs:

- Space events contain `space_id`
- Room events contain `space_id` and `room_id`
- Membership events contain relevant IDs (`space_id` for space joins, `space_id` + `room_id` for room joins)
- Self-initiated events (e.g., `PresenceChanged`, `UserJoinedSpace`, `UserLeftSpace`) use the parent wrapper's `actor_id` instead of duplicating a `user_id` field

**Event Publishing Strategy:**

Every event eventually lands on `live.server.>` so a subscriber needs only one NATS Core subscription to see all of them:

1. **Primary Stream** (persistent):
   - `SERVER_EVENTS` (subjects `server.>`) holds room messages, thread replies, room meta lifecycle, and server-level member events. A stream-level `RePublish` config forwards every accepted message onto `live.server.>` (same suffix, new prefix). The republish fires after persistence, so a subscriber cannot observe an event that didn't durably store.
2. **Direct Live Publish** (transient/mirror):
   - Typing, legacy message edits/deletes, user/space/config notifications publish directly via NATS Core to `live.server.>` — no stream storage. EVT-backed messages and reactions also publish non-durable mirrors on `live.server.>` so the existing `myEvents` subscription path keeps one subject root while the durable source lives in EVT.

The two paths share the same subject root; leaf tokens disambiguate (`.msg.{id}`, `.meta`, `.{verb}` for republished stream events; `.reaction_added`, `.user_typing`, `.profile_updated`, etc. for direct publishes). The `myEvents` GraphQL subscription is backed by one core stream (`StreamMyEvents`) that wraps a single `ChanSubscribe("live.server.>")` plus per-event authorization. There is no per-connection JetStream consumer.

### Event Streams

| Stream                       | Wrapper          | Scope      | Description                                      |
| ---------------------------- | ---------------- | ---------- | ------------------------------------------------ |
| `SERVER_EVENTS`              | `corev1.Event`   | Server     | All JetStream-stored events for the legacy CRUD+log pattern; republishes onto `live.server.>` |
| `EVT`                 | `corev1.Event`   | Server     | Event-sourcing log ([ADR-033](adr/ADR-033-event-sourced-state-with-projections.md) / [ADR-034](adr/ADR-034-single-event-stream.md)). Subjects `evt.{aggregateType}.{aggregateId}.{eventType}`; republishes onto `live.evt.>` for future/direct consumers. Currently fed by per-aggregate boot imports ([ADR-035](adr/ADR-035-per-aggregate-phased-migration.md)); migrated aggregates include room membership/metadata, groups/layout, server config, users, messages/threads, reactions, and RBAC. |
| Live Events                  | `corev1.Event`   | Transient  | `live.server.>` is the active GraphQL subscription root. `SERVER_EVENTS` republishes into it, and migrated `EVT` aggregates publish non-durable compatibility mirrors into it. `live.evt.>` is fed by `EVT` republish but is not the active `myEvents` path during the migration window. |

**SERVER\_EVENTS subjects:**

Room events include event IDs in subjects for O(1) lookups via `GetLastMsgForSubject`. The `{kind}` segment (`channel` or `dm`) lets a single subject namespace serve both server-space rooms and DM rooms.

| Subject                                                                       | Description                                    |
| ----------------------------------------------------------------------------- | ---------------------------------------------- |
| `server.member.joined` / `.left` / `.deleted`                                 | Membership lifecycle events                    |
| `server.room.{kind}.{roomId}.msg.{eventId}`                                   | Root message posted                            |
| `server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}`             | Thread reply posted                            |
| `server.room.{kind}.{roomId}.meta`                                            | Room lifecycle + membership                    |

The event ID in message subjects enables O(1) lookup (52µs) instead of O(n) scanning. Memory overhead is ~500 bytes per unique subject, which is bounded by TTL-based retention.

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

Pattern: `live.server.{scope}.{subject}` — the single subscription root for real-time delivery. Two publishers feed it:

- `SERVER_EVENTS` RePublish (`server.>` → `live.server.>`): every accepted stream message is re-emitted onto a NATS Core subject after persistence. Subscribers don't need a JetStream consumer to receive room messages, thread replies, room meta, or server-level member events.
- Direct NATS Core publishes (`publishLiveUserEvent()`, `publishLiveDeploymentEvent()`, `publishLiveConfigEvent()`, `publishLiveRoomEvent()`, `publishLiveMemberEvent()`): transient events with no stream storage.

Subject leaf tokens never collide between the two paths — republished legacy events end in `.msg.{id}` / `.meta` / `.{member_verb}`, direct publishes use event-type tokens (`.reaction_added`, `.user_typing`, `.profile_updated`, etc.). For EVT-backed messages and reactions, live message edit/delete and reaction add/remove events are mirrors of durable `evt.room.{roomId}.message_*` / `reaction_*` events.

**Deployment-wide live events** (`live.server.{user,config}.>`):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.server.user.{userId}.created`                      | User registration completed  |
| `live.server.user.{userId}.profile_updated`              | User profile changed (broadcast) |
| `live.server.user.{userId}.user_deleted`                 | User account deleted         |
| `live.server.config.updated`                             | Server config (name/MOTD/welcome) changed |
| `live.server.config.server_updated`                      | Server branding (name/logo/banner/description) changed |
| `live.server.config.room_groups_updated`                 | Admin reordered the room sidebar / room-group layout |
| `live.server.user.{userId}.mentioned`                    | User was @mentioned          |
| `live.server.user.{userId}.dm_message`                   | New DM message received      |
| `live.server.user.{userId}.notification_created`         | New notification created     |
| `live.server.user.{userId}.notification_dismissed`       | Notification dismissed       |
| `live.server.user.{userId}.notification_level_changed`   | Viewer's server/room notification level changed |
| `live.server.user.{userId}.thread_follow_changed`        | Viewer's thread follow/unfollow toggled |
| `live.server.user.{userId}.settings_updated`             | User preferences changed     |
| `live.server.user.{userId}.room_read`                    | Room marked as read          |
| `live.server.user.{userId}.session_terminated`           | Active session revoked (logout-other-devices, account deletion) |

**Republished from `SERVER_EVENTS`** (durable, available via `live.server.>` after stream write):

| Subject                                                                       | Description                  |
| ----------------------------------------------------------------------------- | ---------------------------- |
| `live.server.room.{kind}.{roomId}.msg.{eventId}`                              | Root message posted          |
| `live.server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}`        | Thread reply posted          |
| `live.server.room.{kind}.{roomId}.meta`                                       | Room lifecycle + membership  |
| `live.server.member.joined` / `.left` / `.deleted`                            | Server-level membership      |

**Direct live publishes** (transient, never stored):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.server.room.{kind}.{roomId}.reaction_added`        | Reaction added to message    |
| `live.server.room.{kind}.{roomId}.reaction_removed`      | Reaction removed from message|
| `live.server.room.{kind}.{roomId}.message_deleted`       | Message deleted              |
| `live.server.room.{kind}.{roomId}.message_updated`       | Message edited               |
| `live.server.room.{kind}.{roomId}.user_typing`           | User typing in a room        |
| `live.server.room.{kind}.{roomId}.call_joined`           | Participant joined the LiveKit voice call |
| `live.server.room.{kind}.{roomId}.call_left`             | Participant left the LiveKit voice call |
| `live.server.room.{kind}.{roomId}.video_processed`       | Video attachment finished transcoding |

The unified `myEvents` GraphQL subscription is backed by a single core stream (`StreamMyEvents`) that combines:

- One `ChanSubscribe("live.server.>")` (covers republished stream events and direct live publishes alike) with authorization applied per event: room membership for `live.server.room.>`, `isAuthorizedForLiveEvent` for everything else.
- The PresenceHub (single per-process KV watcher on `presence.>` fanning out to all subscribers).
- An in-process heartbeat ticker (synthetic `Heartbeat` event every 25s for client-side liveness detection).

### KV Buckets (backed by streams)

| Bucket                        | Storage | Backup   | Description                                     |
| ----------------------------- | ------- | -------- | ----------------------------------------------- |
| `INSTANCE`                    | File    | Yes      | Users, memberships (bucket name retained from pre-rename) |
| `INSTANCE_CONFIG`             | File    | Yes      | Server runtime configuration overrides          |
| `RUNTIME_STATE`               | File    | Yes      | Persisted latest-value runtime/user state, including pending notifications |
| `SERVER_CONFIG`               | File    | Yes      | Rooms (channel + DM), memberships               |
| `SERVER_RBAC`                 | File    | Yes      | Legacy RBAC seed data read by the `EVT` boot migration |
| `SERVER_RUNTIME`              | File    | Yes      | Legacy runtime state pending cleanup            |
| `SERVER_BODIES`               | File    | Yes      | Message bodies (GDPR-compliant) + standalone attachment metadata records — TODO: rename → `SERVER_CONTENT` |
| `SERVER_REACTIONS`            | File    | Yes      | Legacy emoji reactions, read only by the EVT boot migration |
| `SERVER_THREADS`              | File    | Yes      | Thread metadata (reply count, participants)     |
| `AUTH_TOKENS`                 | File    | No       | Legacy bearer auth tokens pending `RUNTIME_STATE` migration |
| `USER_PRESENCE`               | Memory  | No       | User presence status (TTL 60s)                  |
| `CALL_STATE`                  | Memory  | No       | Active voice call participants, keyed `{spaceId}.{roomId}` (repopulated by LiveKit webhooks after restart) |
| `ENCRYPTION_KEYS`             | File    | **No**   | User encryption keys (excluded for security)    |
| `LINK_PREVIEW_CACHE`          | File    | No       | Cached link preview metadata (48h TTL)          |

All room data — channels and DMs alike — lives in the unified `SERVER_*` buckets. Per-space buckets (`SPACE_{spaceId}_*`) and the hidden DM space are gone after the Phase 4 migration (#354): rooms are differentiated by a `kind` segment in their KV keys (e.g. `room.channel.{roomId}` vs `room.dm.{roomId}`), and storage code never branches on `kind`.

**INSTANCE keys:**

| Key                                    | Description                                      |
| -------------------------------------- | ------------------------------------------------ |
| `user.{userId}`                        | User profile data                                |
| `user_by_login.{lowercase(login)}`     | Login-to-UserID index (case-insensitive)         |
| `auth.{userId}.password`               | Password hash (stored separately)                |
| `user.{userId}.avatar`                 | User avatar asset reference                      |
| `verified_emails.{userId}.{sha256(email)}` | One verified email per entry (proto `VerifiedEmail`) |
| `email_verification.{token}`           | Verification token with userId/email (24h TTL)   |
| `user_by_email.{sha256(email)}`        | Email-to-userId index (created on verification)  |
| `password_reset.{token}`               | Password reset token                             |
| `account_deletion.{token}`             | Account deletion confirmation token              |
| `space.{spaceId}`                      | Vestigial primary-space record (key retained from pre-rename) |
| `instance.logo`                        | Server logo asset reference (key retained from pre-rename) |
| `instance.banner`                      | Server banner asset reference (key retained from pre-rename) |
| `space_membership.{spaceId}.{userId}`  | User-server membership tracking (vestigial slot) |
| `user_preferences.{userId}`            | User display preferences (timezone, time format) |

Notes: Email verification uses SHA256 hashing for claim keys to ensure valid NATS subject characters and case-insensitive uniqueness. The claim key is created atomically when an email is verified, preventing race conditions where two users try to verify the same email. Verification tokens store userId and email in the JSON value for O(1) lookup by token.

**INSTANCE_CONFIG keys:**

| Key               | Description                                                                  |
| ----------------- | ---------------------------------------------------------------------------- |
| `config.instance` | Server configuration (proto message; key + proto name retained) — name, MOTD, welcome message |

Notes: Stores runtime configuration. Each section is a protobuf-serialized message. Server configuration (name, MOTD, welcome message) lives entirely in KV, not in chatto.toml. The TOML file is reserved for operational settings (ports, secrets, NATS config). Deleting a key reverts to defaults.

**AUTH_TOKENS keys:**

| Key       | Description                                           |
| --------- | ----------------------------------------------------- |
| `{token}` | JSON with user ID and creation time                   |

Notes: Tokens are opaque strings (`cht_AT` + 14-char NanoID). Used for `Authorization: Bearer <token>` header authentication, enabling cross-origin clients. TTL-based auto-expiry (default 90 days, configurable via `auth.token_ttl`). Excluded from backups in the legacy `AUTH_TOKENS` bucket since tokens are ephemeral credentials; ADR-036 targets durable token state for `RUNTIME_STATE` so TTL policy can be managed through the shared runtime-state bucket. Tokens are issued on login, registration, bootstrap, and OAuth callback.

**ENCRYPTION_KEYS keys:**

| Key        | Description                                          |
| ---------- | ---------------------------------------------------- |
| `{userId}` | User's 32-byte encryption key (ChaCha20-Poly1305)    |

Notes: Excluded from backups so backup archives contain only encrypted data, not the keys to decrypt it. Enables GDPR-compliant crypto-shredding: deleting a user's key renders all their messages permanently unreadable.

**SERVER\_CONFIG keys:**

Room and membership keys carry a `kind` segment (`channel` or `dm`) so listing operations can prefix-filter without loading and deserializing every record. The kind isn't a field on the `Room` proto — the storage layout is the canonical source of truth.

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
| `notification.{userId}.{notificationId}` | Pending notification record (protobuf `Notification`) for DM messages, @mentions, replies, and all-message subscriptions. Uses per-key 90-day TTL. Live sync uses `NotificationCreatedEvent` / `NotificationDismissedEvent` on `live.server.user.{userId}.*`. |

**SERVER\_RUNTIME keys:**

| Key                                    | Description                                                       |
| -------------------------------------- | ----------------------------------------------------------------- |
| `room_last_msg_at.{roomId}`            | Last message timestamp (per-room, used for sidebar sort)          |
| `video.{attachmentId}`                 | Legacy video processing state imported to EVT video manifest events at boot |
| `attachment_records.backfilled`        | Sentinel set after the `BackfillAttachmentRecords` boot migration completes; short-circuits the scan on subsequent boots. |
| `video_manifest_es.migrated`           | Sentinel set after legacy `video.*` records are imported into EVT |

These keys don't carry a kind segment — `roomId` is globally unique, so direct lookup works for DM and channel rooms alike.

**USER_PRESENCE keys:**

| Key                  | Description                               |
| -------------------- | ----------------------------------------- |
| `presence.{userId}`  | Serialized `UserPresence` proto (status)  |

Notes: Memory-based storage (not persisted). 60-second TTL with 30-second client refresh. Uses `LimitMarkerTTL` so NATS emits delete markers on TTL expiry, allowing watchers to detect offline transitions. A single per-process **PresenceHub** watches `presence.>` and fans out updates to all space subscriptions (reducing KV watcher count from O(subscriptions) to O(1)). Subscriptions filter by space membership using a lazy positive-only cache. **Multi-device support**: On disconnect, clients stop refreshing but don't explicitly delete—TTL handles expiry. This means a user stays online if any device is still connected. **Event deduplication**: Presence events are only emitted when status actually changes (online→away, etc.), not on refresh cycles. **Client-driven status**: The `updateMyPresence` mutation allows clients to set AWAY or DO_NOT_DISTURB; heartbeat refreshes use optimistic locking to preserve these statuses.

**SERVER\_BODIES keys:**

| Key                    | Description                                              |
| ---------------------- | -------------------------------------------------------- |
| `{userId}.{eventId}`   | Message body keyed by user ID and event ID               |

Notes: The compound key format `{userId}.{eventId}` enables efficient prefix-based deletion for GDPR compliance (delete all messages for a user via prefix scan). Separated from metadata for performance and operational flexibility. No `kind` segment — both IDs are globally unique NanoIDs.

**SERVER\_REACTIONS keys:**

| Key                                     | Description                                    |
| --------------------------------------- | ---------------------------------------------- |
| `{messageEventId}.{emojiName}.{userId}` | Reaction tracking (empty value = reacted; value stores nanosecond timestamp for "added at" ordering) |

Notes: Legacy source for `MigrateReactionsToES`. Current reaction writes append durable `ReactionAdded` / `ReactionRemoved` events to `evt.room.{roomId}.reaction_added` and `evt.room.{roomId}.reaction_removed`; current reaction state is derived by an in-memory projection keyed by message event ID, emoji shortcode, and actor/user ID. The bucket remains so old deployments can be imported and will be removed in the later cleanup phase.

**SERVER\_THREADS keys:**

| Key                       | Description                                              |
| ------------------------- | -------------------------------------------------------- |
| `{roomId}.{rootEventId}`  | ThreadMetadata proto (reply count, last reply, participants) |

Notes: Updated on each thread reply via optimistic locking. Tracks up to 50 participant IDs. Used for thread previews in channel view.

| Key                                          | Description                                                                              |
| -------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `{userId}.{bodyId}`                          | Marshaled `corev1.MessageBody` proto (encrypted text, attachments slice, link preview)   |

A transitional `attachment.{roomId}.{attachmentId}` key shape existed
in this bucket between #575 and #581 as a per-attachment authz index;
it was retired by the signed-locator URL scheme (ADR-032) and any
leftover entries are swept at boot by the `DropLegacyAttachmentRecords`
migration. New code should not write to `attachment.*` keys here.

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

Notes: Content-Type stored in object headers. S2 compression enabled. Legacy entity records such as server branding still reference the storage-pointer-only `DeprecatedAsset` proto until their post-migration cleanup.

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

Chatto supports on-the-fly image transformation for attachments, allowing clients to request images at specific dimensions without pre-generating all possible sizes.

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

The `Attachment` type exposes transform parameters as field arguments:

```graphql
type Attachment {
  url(width: Int, height: Int, fit: FitMode): String!
  thumbnailUrl(width: Int, height: Int, fit: FitMode): String
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

Messages use a store-then-publish pattern optimized for reliability and GDPR compliance:

**Message Identifiers:**

- **Event ID**: NanoID (e.g., `E...`) used for event identification, body storage, and lookups via O(1) subject matching
- **Body Key**: Compound key `{userId}.{eventId}` stored in `MessagePostedEvent.message_body_id`

**Write Path:**

1. Generate event with event ID
2. Construct body key as `{userId}.{eventId}` and store body in BODIES bucket
3. Publish event to room stream
4. `PublishAck.Sequence` is captured and added to the event for resolvers
5. Body exists before event is delivered - no race conditions

**Threading:**

- `in_reply_to` field stores the event ID of the parent message (empty for top-level messages)
- `in_thread` field stores the event ID of the thread root (empty for top-level messages)
- Thread subject pattern: `server.room.{kind}.{roomId}.msg.{rootEventId}.replies.{eventId}`
- Enables O(1) lookup of thread replies via wildcard pattern: `msg.*.replies.{eventId}`
- Thread metadata (reply count, participants) stored in THREADS bucket keyed by `{roomId}.{rootEventId}`

**@Mentions:**

- `@username` patterns in message body are extracted via regex (ASCII alphanumeric, underscore, hyphen)
- Usernames are resolved to user IDs; only space members are included (non-members silently ignored)
- `MessagePostedEvent.mentioned_user_ids` contains resolved user IDs
- Pending mention state is a notification record in `RUNTIME_STATE` (`notification.{userId}.{notificationId}`); sidebar orange dots derive from pending notifications, not a separate mention flag.
- Live notification published to `live.server.user.{userId}.mentioned` for toast display
- Mention notifications are dismissed when the user views the relevant room or thread, or explicitly dismisses them from the notification center.
- Self-mentions are filtered out (no notification to message author)

**GDPR Deletion:**

- Delete only removes the KV entry in BODIES bucket using the compound key
- Event remains in stream as audit record with empty body
- `GetMessageBody` returns empty string for deleted messages

### Key Patterns

- **Unified Event Subscriptions**: The `myEvents` subscription merges multiple event sources into a single stream: a JetStream ordered consumer (using `DeliverNewPolicy` for real-time delivery), NATS Core subscriptions for live-only events, and a PresenceHub subscription for presence updates.
- **Compression**: The `SERVER_EVENTS` stream uses S2 compression to reduce storage costs
- **GDPR Compliance**: Message bodies stored separately in `SERVER_BODIES` for compliant deletion while preserving audit trail
- **Unified Server Storage**: Channels and DMs share the same `SERVER_*` buckets; the `kind` segment in keys (`room.channel.*` / `room.dm.*`) disambiguates without per-space isolation
- **Out-of-Band Data Pattern**: High-volume content (message bodies) separated into dedicated `SERVER_BODIES` to avoid contention with metadata operations, enable independent scaling, and support future optimizations (compression, different storage backends)
- **Eager Server Resource Initialization**: The unified `SERVER_*` buckets (stream, KV buckets, object store) are created up-front at boot, not lazily on first use. The Phase 4 migration (#354) retired the legacy lazycache fallback that briefly accommodated the per-space storage shape.
