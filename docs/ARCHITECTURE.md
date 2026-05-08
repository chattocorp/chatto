# Chatto Architecture

> **Note:** This document is a reference for *what* the system looks like. For *why* key decisions were made and what alternatives were considered, see the [Architecture Decision Records](adr/INDEX.md).

## Table of Contents

- [Overview](#overview)
  - [Core Concepts](#core-concepts)
- [NATS Authentication](#nats-authentication)
- [Architecture & APIs](#architecture--apis)
- [GraphQL API Overview](#graphql-api-overview)
  - [Queries](#queries)
  - [Mutations](#mutations)
  - [Subscriptions](#subscriptions)
- [Architecture Pattern: CRUD + Audit Log](#architecture-pattern-crud--audit-log)
  - [Write Path](#write-path)
  - [Consistency Model](#consistency-model)
- [Roles and Permissions](#roles-and-permissions)
  - [Permission Check Functions](#permission-check-functions)
  - [Space Permissions](#space-permissions)
  - [Instance Permissions](#instance-permissions)
- [Direct Messages (DM)](#direct-messages-dm)
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

- **Instance**: A deployment of Chatto, consisting of 1-n application processes connected to the same NATS system and account.
- **Spaces**: Logical groupings of rooms (workspaces/teams/communities). A deployment can host multiple spaces.
- **Rooms**: Channels within spaces for communication. Can be named (`general`) or direct messages between users.
- **Users**: Global to deployment, with per-space and per-room membership managed separately.

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

Key files: [`cli/internal/graph/`](cli/internal/graph/)

The GraphQL API is the primary client-facing interface for Chatto. It provides queries, mutations, and subscriptions over HTTP and WebSocket connections.

### Queries

| Query                   | Description                               |
| ----------------------- | ----------------------------------------- |
| `me`                    | Get the currently authenticated user      |
| `user(id)`              | Get a user by ID                          |
| `userByLogin(login)`    | Get a user by login name                  |
| `users`                 | List all users (instance admin only)      |
| `spaces`                | List all spaces (for discovery)           |
| `space(id)`             | Get a space by ID                         |
| `room(spaceId, roomId)` | Get a room by ID                          |
| `roomEvents(...)`       | Fetch paginated room events (default: 50) |
| `roomEvent(...)`        | Fetch a single room event by sequence     |
| `threadEvents(...)`     | Fetch thread messages (root + replies)    |
| `notifications`         | Get all notifications for current user    |
| `hasNotifications`      | Check if user has any notifications       |
| `notificationCount`     | Get count of user's notifications         |

### Mutations

| Mutation                  | Description                                             |
| ------------------------- | ------------------------------------------------------- |
| `createUser`              | Register a new user account                             |
| `createSpace`             | Create a new space                                      |
| `updateSpace`             | Update space name/description                           |
| `uploadSpaceLogo`         | Upload a logo for a space                               |
| `deleteSpaceLogo`         | Delete a space's logo                                   |
| `uploadSpaceBanner`       | Upload a banner for a space                             |
| `deleteSpaceBanner`       | Delete a space's banner                                 |
| `joinSpace`               | Join a space                                            |
| `leaveSpace`              | Leave a space                                           |
| `createRoom`              | Create a new room in a space                            |
| `joinRoom`                | Join a room                                             |
| `leaveRoom`               | Leave a room                                            |
| `markRoomAsRead`          | Mark a room as read                                     |
| `postMessage`             | Post a message (with optional attachments/thread reply) |
| `editMessage`             | Edit a message (author-only, 3-hour window)             |
| `deleteMessage`           | Delete a message body (GDPR compliance)                 |
| `deleteAttachment`        | Delete an attachment (author-only)                      |
| `addReaction`             | Add an emoji reaction                                   |
| `removeReaction`          | Remove an emoji reaction                                |
| `updateMyProfile`         | Update current user's display name                      |
| `uploadMyAvatar`          | Upload avatar (resized to 256x256, WebP)                |
| `deleteMyAvatar`          | Delete current user's avatar                            |
| `requestAccountDeletion`  | Request account deletion (generates 15-min token)       |
| `deleteMyAccount`         | Permanently delete account (GDPR crypto-shredding)      |
| `dismissNotification`     | Dismiss a single notification                           |
| `dismissAllNotifications` | Dismiss all notifications for current user              |

### Subscriptions

| Subscription                     | Description                                                                |
| -------------------------------- | -------------------------------------------------------------------------- |
| `mySpaceEvents(spaceId, status)` | All space events (messages, reactions, presence). Sets your presence.      |
| `myInstanceEvents`               | Instance events (space changes, membership, config updates, profiles, notifications) |
| `adminAuditLogEvents`            | All instance events for admin audit log (requires admin.audit.view)        |

## Architecture Pattern: CRUD + Audit Log

### Write Path

| Type    | Scope     | Resource                      | Purpose                                     |
| ------- | --------- | ----------------------------- | ------------------------------------------- |
| KV      | Instance  | `INSTANCE`                    | Users, spaces, memberships                  |
| KV      | Instance  | `INSTANCE_RBAC`               | Instance-level roles and permissions        |
| KV      | Instance  | `INSTANCE_CONFIG`             | Runtime configuration overrides             |
| KV      | Instance  | `USER_PRESENCE`               | Presence status (memory, TTL 60s)           |
| KV      | Instance  | `NOTIFICATIONS`               | User notifications (90-day TTL)             |
| KV      | Instance  | `AUTH_TOKENS`                 | Bearer auth tokens (configurable TTL)       |
| KV      | Per-space | `SPACE_{spaceId}_CONFIG`      | Rooms, memberships                          |
| KV      | Per-space | `SPACE_{spaceId}_RBAC`        | Roles, permissions, assignments             |
| KV      | Per-space | `SPACE_{spaceId}_RUNTIME`     | Read status, mention tracking               |
| KV      | Per-space | `SPACE_{spaceId}_BODIES`      | Message bodies (GDPR-compliant)             |
| KV      | Per-space | `SPACE_{spaceId}_REACTIONS`   | Emoji reactions                             |
| KV      | Per-space | `SPACE_{spaceId}_THREADS`     | Thread metadata (reply count, participants) |
| Objects | Instance  | `INSTANCE_ASSETS`             | Avatars, icons                              |
| Objects | Instance  | `ASSET_CACHE`                 | Cached resized images (optional, with TTL)  |
| Objects | Server    | `SERVER_ASSETS`               | Message attachments (primary + DM)          |
| Objects | Per-space | `SPACE_{spaceId}_ASSETS`      | Message attachments (non-primary, non-DM)   |
| Stream  | Per-space | `SPACE_{spaceId}_EVENTS`      | Room lifecycle, messages, memberships       |

See [NATS Resource Inventory](#nats-resource-inventory) for detailed key patterns and subjects.

**Important:** Event publishing is best-effort for most operations. If event publishing fails for spaces, users, or rooms, the operation still succeeds because the KV store (source of truth) was updated successfully. Event publishing failures are logged but do not block operations.

**Exception:** Message posting requires successful event publishing because messages are stored only in event streams (see Messages section below). If event publishing fails for a message, the entire post operation fails.

### Consistency Model

**Current (Single Embedded NATS):**

- Strong consistency for KV operations (source of truth)
- Read-your-writes guaranteed via immediate KV updates
- Event streams provide audit trail with best-effort delivery
- No dual-write problem: KV is source of truth, events are additive

**Future (Clustered NATS - Multi-Instance):**

- KV buckets remain strongly consistent (NATS JetStream R3 replication)
- Event streams continue providing audit trail and pub/sub
- Configurable retention policies per-space (delete old events without data loss)
- Can rebuild/migrate KV stores from current state exports (not from events)

**Benefits of This Approach:**

- Simple to understand and debug (CRUD operations with event logging)
- Can safely age out old events based on retention policy
- No complex event replay or projection rebuilding required
- Storage costs scale with active data, not infinite history
- Still provides full audit trail for compliance/debugging (until retention expires)

## Roles and Permissions

Chatto implements a data-driven roles and permissions system at two levels:

1. **Instance RBAC**: Controls instance-wide permissions (creating spaces, admin access). See `INSTANCE_RBAC` keys below.
2. **Space RBAC**: Controls per-space permissions (room management, messaging, moderation).

Both systems share a generic `rbac.Engine` that handles role CRUD, permission grants, and role assignments. The engine is configured differently for each scope (instance has implicit roles like `verified`/`everyone`; spaces have implicit `everyone` role for all members).

### Permission Resolution

Key file: [`cli/internal/core/permission_resolver.go`](cli/internal/core/permission_resolver.go)

The `PermissionResolver` uses a **deny-always-wins, instance-authority-first** model:

1. **Any DENY at any level** → Denied (deny always wins globally)
2. **Instance GRANT** → Granted (instance authority overrides lower scopes)
3. **Space GRANT** → Granted
4. **Room GRANT** → Granted
5. **Nothing** → Denied

Mental model: *"Anyone can say no. Higher authority can say yes. No one can force yes over someone else's no."*

**Instance roles only grant instance-scoped permissions** (space listing, space joining, admin access). Space-scoped permissions (room management, messaging) are governed entirely by space roles. This means spaces are self-governing — instance admins can deny permissions globally (e.g., suspend a user), but instance grants don't override space configurations.

**Membership gate**: Space-scoped permissions require space membership (except `space.join`, which non-members need to join).

### Permission Check Functions

Key files: [`cli/internal/core/can.go`](cli/internal/core/can.go), [`cli/internal/core/permissions.go`](cli/internal/core/permissions.go)

Authorization is enforced at the API boundary using `Can*` functions defined in `core/can.go`. These wrap the low-level `hasSpacePermission()` function with business-meaningful names:

| Function           | Permission Checked | Description                               |
| ------------------ | ------------------ | ----------------------------------------- |
| `CanManageSpace`   | `space.manage`     | Update space settings (name, description) |
| `CanDeleteSpace`   | `space.delete`     | Delete the space entirely                 |
| `CanManageRoles`   | `roles.manage`     | Create, update, delete roles              |
| `CanAssignRoles`   | `roles.assign`     | Assign or revoke roles to/from users      |
| `CanInviteMembers` | `members.invite`   | Invite new members to the space           |
| `CanRemoveMembers` | `members.remove`   | Remove members from the space             |
| `CanBrowseRooms`   | `rooms.browse`     | View the list of rooms in the space       |
| `CanCreateRoom`    | `rooms.create`     | Create new rooms in the space             |
| `CanManageRooms`   | `rooms.manage`     | Update or delete any room                 |
| `CanJoinRoom`      | `rooms.join`       | Join existing rooms in the space          |

Notes:
- All functions return `(bool, error)` where bool indicates permission and error indicates system failures
- DM spaces have simplified permission checks via `isDMPermissionAllowed()` (room membership is the only check)
- `SystemActorID` (`"system"`) is used for internal/bootstrap operations that bypass permission checks

### Space Permissions

**Concepts:**

- **Permissions**: Finite set of strings defined in code (e.g., `space.manage`, `room.create`, `message.post`)
- **Roles**: Named sets of permissions stored per-space (e.g., `owner`, `moderator`, `everyone`)
- **Role Assignment**: Users can have multiple roles within a space; permissions are combined (union), denials win
- **Default Roles**: Created automatically when a space is created:
  - `owner`: Full access to all space features (position 0, highest rank)
  - `moderator`: Moderation permissions — room management, member removal, message deletion (position 1)
  - `everyone`: Implicit role for all space members — default member permissions (room list/create/join, messaging)

**Storage (per-space RBAC bucket `SPACE_{spaceId}_RBAC`):**

| Key                                             | Description                                             |
| ----------------------------------------------- | ------------------------------------------------------- |
| `role.{roleName}`                               | Role metadata (name, display_name, description)         |
| `allow.{roleName}.{verb}.{objectType}.{objectId}` | Permission grant for a role                          |
| `deny.{roleName}.{verb}.{objectType}.{objectId}`  | Permission denial for a role (overrides all grants)  |
| `role_assignment.{roleName}.{userId}`           | Role assignment (empty value = assigned)                |

The `objectId` is typically `any` for space-wide permissions, or a specific room ID for room-scoped overrides.

**Available Permissions:**

| Permission        | Description                     | Default Member |
| ----------------- | ------------------------------- | -------------- |
| `space.manage`    | Update space settings           | No             |
| `space.delete`    | Delete the space                | No             |
| `roles.manage`    | Create, update, delete roles    | No             |
| `roles.assign`    | Assign roles to users           | No             |
| `members.invite`  | Invite new members to the space | No             |
| `members.remove`  | Remove members from the space   | No             |
| `rooms.browse`    | View list of rooms in space     | Yes            |
| `rooms.create`    | Create new rooms                | Yes            |
| `rooms.manage`    | Update and delete rooms         | No             |
| `rooms.join`      | Join existing rooms             | Yes            |

**Automatic Behavior:**

- Space creator is automatically assigned the `owner` role
- All space members implicitly have the `everyone` role
- Permission checks are enforced on:
  - Space operations: UpdateSpace, DeleteSpace
  - Room operations: CreateRoom, UpdateRoom, DeleteRoom, JoinRoom

### Instance Permissions

Key files: [`cli/internal/core/instance_permissions.go`](cli/internal/core/instance_permissions.go)

Instance permissions control access to instance-wide operations like creating spaces, accessing the admin panel, or managing DMs. They are defined separately from space permissions.

**Available Instance Permissions:**

| Permission            | Description                                | Default Role  |
| --------------------- | ------------------------------------------ | ------------- |
| `spaces.browse`       | View the list of spaces                    | member        |
| `spaces.join`         | Join spaces                                | verified      |
| `spaces.create`       | Create new spaces                          | verified      |
| `admin`               | Access admin panel and admin queries       | admin         |
| `admin.users.view`    | View the users page in admin               | admin         |
| `admin.users.manage`  | Edit user role assignments                 | admin         |
| `admin.spaces.view`   | View the spaces page in admin              | admin         |
| `admin.roles.view`    | View the roles page in admin               | admin         |
| `admin.roles.manage`  | Create/edit instance roles and permissions | admin         |
| `admin.system.view`   | View system and data pages in admin        | admin         |
| `admin.audit.view`    | View the audit log in admin                | admin         |
| `dms.view`            | Access DM space and read direct messages   | verified      |
| `dms.write`           | Start DM conversations and send messages   | verified      |
| `users.delete`        | Delete any user account                    | admin         |

**Instance Roles:**

| Role       | Type     | Description                                                        |
| ---------- | -------- | ------------------------------------------------------------------ |
| `admin`    | Explicit | Has all permissions implicitly. Must be explicitly assigned.       |
| `verified` | Implicit | Granted to users with a verified email. Cannot be assigned/revoked.|
| `member`   | Implicit | Granted to all authenticated users. Cannot be assigned/revoked.    |

Notes:
- Config-designated owners (`owners.emails` in chatto.toml) are matched against verified emails only
- User-specific overrides follow deny-override semantics: denials take precedence over grants
- Unverified users can only browse spaces; they must verify an email to join or create spaces

## Direct Messages (DM)

Direct messages use a special system space with ID `"DM"` that is created automatically at startup.

Key files: [`cli/internal/core/dm.go`](cli/internal/core/dm.go)

**Key Characteristics:**

- DM rooms have no names - display names are derived from participants in the UI
- Room IDs are deterministic hashes of sorted participant IDs, enabling find-or-create semantics without database queries
- Maximum 10 participants per DM conversation
- DM space has no roles - permissions are implicit based on room membership
- DM rooms are listed via dedicated `ListDMConversations` API, not the regular room browsing

**Permissions in DM Space:**

| Allowed                           | Denied                                              |
| --------------------------------- | --------------------------------------------------- |
| `rooms.join` (for FindOrCreateDM) | `space.manage`, `space.delete`                      |
|                                   | `roles.manage`, `roles.assign`                      |
|                                   | `rooms.browse` (use ListDMConversations instead)    |
|                                   | `rooms.create`, `rooms.manage` (use FindOrCreateDM) |
|                                   | `members.invite`, `members.remove`                  |

**DM Notifications:**

- Every DM message triggers a live notification to all participants except the sender
- Published to `live.instance.user.{userId}.dm_message` for toast display
- DM unread status uses standard room read tracking (no separate mention tracking)

## NATS Resource Inventory

### Event Types

Chatto uses two scope-based protobuf wrapper types for events:

- **SpaceEvent** - Wrapper for space-scoped events (both JetStream-stored and live-only)
  - Wrapper fields: `id`, `created_at`, `actor_id`
  - Contextual fields: Extracted dynamically from concrete event payloads (e.g., `spaceId`, `roomId`)
  - Concrete event: `event` (oneof containing 13 space-scoped event types)

- **InstanceEvent** - Wrapper for instance-scoped events (all live-only, no JetStream storage)
  - Wrapper fields: `id`, `created_at`, `actor_id`
  - Concrete event: `event` (oneof containing 17 instance-scoped event types)

**Proto File Organization:**

Event definitions are split across two files by scope and persistence:

| File | Contents | Safety |
| ---- | -------- | ------ |
| `event.proto` | `SpaceEvent` wrapper + 7 persisted event message definitions | Changing field numbers/structure affects JetStream-stored data — requires careful migration |
| `live_event.proto` | `InstanceEvent` wrapper + all live-only event message definitions (space + instance) | Safe to change freely — these are never persisted |

Both files share `package chatto.core.v1` and generate into the same Go package.

**Event Categories:**

| Category          | Wrapper        | Storage    | Examples                                                    | Purpose                                                        |
| ----------------- | -------------- | ---------- | ----------------------------------------------------------- | -------------------------------------------------------------- |
| JetStream-stored  | SpaceEvent     | Stream     | RoomCreated, MessagePosted, UserJoinedRoom                  | Ordering guarantees, historical replay, audit trail            |
| Space live-only   | SpaceEvent     | NATS Core  | ReactionAdded, ReactionRemoved, MessageDeleted, MessageUpdated, PresenceChanged | Ephemeral space notifications where KV bucket is source of truth |
| Instance live     | InstanceEvent  | NATS Core  | UserCreated, SpaceCreated, MentionNotification, NotificationCreated | Instance-wide notifications, user-scoped events |

The distinction between stored and live-only events is based on how they're published (JetStream vs NATS Core). Within each scope, all events use the same wrapper type — there are no separate "live" wrapper types. GraphQL mirrors this with `SpaceEvent`/`SpaceEventType` and `InstanceEvent`/`InstanceEventType`.

**Self-Contained Events:** Each concrete event contains all the IDs and context it needs:

- Space events contain `space_id`
- Room events contain `space_id` and `room_id`
- Membership events contain relevant IDs (`space_id` for space joins, `space_id` + `room_id` for room joins)
- Self-initiated events (e.g., `PresenceChanged`, `UserJoinedSpace`, `UserLeftSpace`) use the parent wrapper's `actor_id` instead of duplicating a `user_id` field

**Event Publishing Strategy:**

Events are published to two types of destinations:

1. **Primary Streams** (persistent):
   - SPACE\_{spaceId}\_EVENTS stream for space-level events (room lifecycle, messages, room membership)
2. **Live Events** (transient, NATS Core):
   - Instance-level events (user/space lifecycle) published directly to `live.instance.>` subjects
   - Space/room-level live events (reactions, typing, edits) published to `live.space.>` subjects
   - Not persisted in JetStream — fire-and-forget for real-time delivery only

### Event Streams

| Stream                       | Wrapper        | Scope      | Description                                      |
| ---------------------------- | -------------- | ---------- | ------------------------------------------------ |
| `SPACE_{spaceId}_EVENTS`     | SpaceEvent     | Per-space  | All space and room events in a unified stream    |
| Live Instance Events         | InstanceEvent  | Transient  | Instance-level events (NATS Core, direct publish)|
| Live Space/Room Events       | SpaceEvent     | Transient  | Real-time event notifications (direct publish)   |

**SPACE\_{spaceId}\_EVENTS subjects:**

Room events include event IDs in subjects for O(1) lookups via `GetLastMsgForSubject`:

| Subject                                                              | Description                                    |
| -------------------------------------------------------------------- | ---------------------------------------------- |
| `space.{spaceId}.joined`                                             | User joined space                              |
| `space.{spaceId}.left`                                               | User left space                                |
| `space.{spaceId}.member_deleted`                                     | User removed from space                        |
| `space.{spaceId}.room.{roomId}.msg.{eventId}`                        | Root message posted                            |
| `space.{spaceId}.room.{roomId}.msg.{rootEventId}.replies.{eventId}`  | Thread reply posted                            |
| `space.{spaceId}.room.{roomId}.meta`                                 | Room lifecycle + membership (created, updated, deleted, joined, left) |

The event ID in message subjects enables O(1) lookup (52µs) instead of O(n) scanning. Memory overhead is ~500 bytes per unique subject, which is bounded by TTL-based retention.

Filtering examples:

| Pattern                                                      | Description                                    |
| ------------------------------------------------------------ | ---------------------------------------------- |
| `space.{spaceId}.>`                                          | All events in the space                        |
| `space.{spaceId}.room.{roomId}.>`                            | All room events (messages + meta + threads)    |
| `space.{spaceId}.room.{roomId}.msg.>`                        | All messages (root + threads)                  |
| `space.{spaceId}.room.{roomId}.msg.*`                        | Root messages only                             |
| `space.{spaceId}.room.>`                                     | All room events across all rooms (for indexers)|
| `space.{spaceId}.room.{roomId}.msg.*.replies.>`              | All thread replies in a room                   |
| `space.{spaceId}.room.{roomId}.msg.{rootEventId}.replies.>`  | All replies in a specific thread               |
| `space.{spaceId}.room.{roomId}.msg.*.replies.{eventId}`      | Lookup a thread reply by event ID (wildcard)   |

Note: Event type (created, joined, etc.) is determined by the event payload, not the subject. Actor/user information is also in payloads, not subjects (optimized for low subject cardinality).

**User Personal Streams** (transient):

- Subject: `user.{userId}.event`
- Published via NATS Core (not JetStream) - transient, not persisted
- Receives events relevant to the user (space joins/leaves, room joins/leaves)
- Powers real-time notifications and user-centric subscriptions
- Events are dual-published: to primary stream (audit trail) and user stream (notifications)

**Live Subject Space** (transient):

Pattern: `live.{scope}.{subject}` - for real-time delivery of transient events.

Instance events are published via `publishInstanceEvent()` and space live events via `publishLiveSpaceEvent()` (both NATS Core, no JetStream storage):

**Instance-level live events** (`live.instance.>`):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.instance.user.{userId}.created`                    | User registration completed  |
| `live.instance.user.{userId}.profile_updated`            | User profile changed         |
| `live.instance.user.{userId}.user_deleted`               | User account deleted         |
| `live.instance.user.{userId}.space_created`              | Space creation               |
| `live.instance.user.{userId}.space_deleted`              | Space deletion               |
| `live.instance.user.{userId}.joined_space`               | User joined a space          |
| `live.instance.user.{userId}.left_space`                 | User left a space            |
| `live.instance.space.{spaceId}.updated`                  | Space settings changed       |
| `live.instance.config.updated`                           | Instance config changed      |
| `live.instance.user.{userId}.mentioned`                  | User was @mentioned          |
| `live.instance.user.{userId}.dm_message`                 | New DM message received      |
| `live.instance.user.{userId}.notification_created`       | New notification created     |
| `live.instance.user.{userId}.notification_dismissed`     | Notification dismissed       |
| `live.instance.user.{userId}.settings_updated`           | User preferences changed     |
| `live.instance.user.{userId}.room_read`                  | Room marked as read          |
| `live.instance.space.{spaceId}.new_message`              | New message in space         |

**Space/room-level live events** (`live.space.>`):

| Subject                                                  | Description                  |
| -------------------------------------------------------- | ---------------------------- |
| `live.space.{spaceId}.member_deleted`                    | User removed from space      |
| `live.space.{spaceId}.room.{roomId}.reaction_added`      | Reaction added to message    |
| `live.space.{spaceId}.room.{roomId}.reaction_removed`    | Reaction removed from message|
| `live.space.{spaceId}.room.{roomId}.message_deleted`     | Message deleted              |
| `live.space.{spaceId}.room.{roomId}.message_updated`     | Message edited               |
| `live.space.{spaceId}.presence_changed`                  | User presence changed        |

All live events bypass JetStream entirely — KV buckets are the source of truth. Space live events are delivered through the unified `mySpaceEvents` subscription alongside JetStream-stored events. The subscription handler merges:
- JetStream consumer for stored events (messages, room lifecycle)
- NATS Core subscription to `live.space.{spaceId}.*` for space-level live events (member_deleted)
- NATS Core subscription to `live.space.{spaceId}.room.>` for room-level live events (reactions, edits, deletes)
- PresenceHub subscription for presence updates (single per-process KV watcher fans out to all space subscriptions, filtered to space members)

### KV Buckets (backed by streams)

| Bucket                        | Scope     | Storage | Backup   | Description                                     |
| ----------------------------- | --------- | ------- | -------- | ----------------------------------------------- |
| `INSTANCE`                    | Instance  | File    | Yes      | Users, spaces, memberships                      |
| `INSTANCE_RBAC`               | Instance  | File    | Yes      | Instance-level roles and permissions            |
| `INSTANCE_CONFIG`             | Instance  | File    | Yes      | Runtime configuration overrides                 |
| `SERVER_CONFIG`               | Server    | File    | Yes      | Rooms (channel + DM), memberships               |
| `SERVER_RBAC`                 | Server    | File    | Yes      | Roles, permissions, assignments                 |
| `SERVER_RUNTIME`              | Server    | File    | Yes      | Read state, mention tracking                    |
| `SERVER_BODIES`               | Server    | File    | Yes      | Message bodies (GDPR-compliant)                 |
| `SERVER_REACTIONS`            | Server    | File    | Yes      | Emoji reactions on messages                     |
| `SERVER_THREADS`              | Server    | File    | Yes      | Thread metadata (reply count, participants)     |
| `NOTIFICATIONS`               | Instance  | File    | Yes      | User notifications (90-day TTL)                 |
| `AUTH_TOKENS`                 | Instance  | File    | No       | Bearer auth tokens (configurable TTL, default 90d) |
| `USER_PRESENCE`               | Instance  | Memory  | No       | User presence status (TTL 60s)                  |
| `ENCRYPTION_KEYS`             | Instance  | File    | **No**   | User encryption keys (excluded for security)    |
| `SPACE_{spaceId}_BODIES`      | Per-space | File    | Yes      | Message bodies (GDPR-compliant)                 |
| `SPACE_{spaceId}_REACTIONS`   | Per-space | File    | Yes      | Emoji reactions on messages                     |
| `SPACE_{spaceId}_THREADS`     | Per-space | File    | Yes      | Thread metadata (reply count, participants)     |
| `LINK_PREVIEW_CACHE`          | Instance  | File    | No       | Cached link preview metadata (48h TTL)          |

**Migration note (#330 / ADR-027 phase 4a–4c, 4e):** the primary space's CONFIG/RBAC/RUNTIME/BODIES/REACTIONS/THREADS data and the DM system space's CONFIG/RUNTIME/BODIES/REACTIONS/THREADS data were folded into `SERVER_*` (4a–4c). Per-space attachment object stores (`SPACE_{primary}_ASSETS`, `SPACE_DM_ASSETS`) were folded into `SERVER_ASSETS` in phase 4e. The legacy `SPACE_{primary}_*` and `SPACE_DM_*` buckets/object stores are still present (no-deletes rule) but dormant — reads route to `SERVER_*`. A future cleanup pass will retire them.

**INSTANCE keys:**

| Key                                    | Description                                      |
| -------------------------------------- | ------------------------------------------------ |
| `user.{userId}`                        | User profile data                                |
| `user_by_login.{lowercase(login)}`     | Login-to-UserID index (case-insensitive)         |
| `auth.{userId}.password`               | Password hash (stored separately)                |
| `user.{userId}.avatar`                 | User avatar asset reference                      |
| `user.{userId}.verified_emails`        | List of verified emails (JSON array)             |
| `email_verification.{token}`           | Verification token with userId/email (24h TTL)   |
| `user_by_email.{sha256(email)}`        | Email-to-userId index (created on verification)  |
| `password_reset.{token}`               | Password reset token                             |
| `account_deletion.{token}`             | Account deletion confirmation token              |
| `space.{spaceId}`                      | Space configurations                             |
| `space.{spaceId}.logo`                 | Space logo asset reference                       |
| `space.{spaceId}.banner`               | Space banner asset reference                     |
| `space_membership.{spaceId}.{userId}`  | User-space membership tracking                   |
| `user_preferences.{userId}`            | User display preferences (timezone, time format) |
| `migration_lock`                       | Schema-migration mutex (per-key TTL'd, owner ID as value) — see #330 phase 4 |
| `migration.phase4a_complete`           | Phase 4a (primary CONFIG/RBAC/RUNTIME → `SERVER_*`) completion marker |
| `migration.phase4b_complete`           | Phase 4b (DM merge + kind-prefixed keys) completion marker |
| `migration.phase4c_complete`           | Phase 4c (per-message KVs: BODIES/REACTIONS/THREADS → `SERVER_*`) completion marker |
| `migration.phase4e_complete`           | Phase 4e (per-space attachment object stores → `SERVER_ASSETS`) completion marker |

Notes: Email verification uses SHA256 hashing for claim keys to ensure valid NATS subject characters and case-insensitive uniqueness. The claim key is created atomically when an email is verified, preventing race conditions where two users try to verify the same email. Verification tokens store userId and email in the JSON value for O(1) lookup by token.

**INSTANCE_CONFIG keys:**

| Key               | Description                                                                  |
| ----------------- | ---------------------------------------------------------------------------- |
| `config.instance` | Instance configuration (InstanceConfig proto) - name, MOTD, welcome message  |

Notes: Stores runtime configuration. Each section is a protobuf-serialized message. Instance configuration (name, MOTD, welcome message) lives entirely in KV, not in chatto.toml. The TOML file is reserved for operational settings (ports, secrets, NATS config). Deleting a key reverts to defaults.

**INSTANCE_RBAC keys:**

| Key                                              | Description                                         |
| ------------------------------------------------ | --------------------------------------------------- |
| `first_admin_assigned`                           | Bootstrap marker with first admin's user ID         |
| `role.{roleName}`                                | Role metadata (admin, verified, member)             |
| `role_permission.{roleName}.{permission}`        | Role permission grant (empty value = granted)       |
| `role_assignment.{roleName}.{userId}`            | Role assignment (implicit roles not stored)         |
| `user_permission.{userId}.{permission}`          | User-specific permission grant                      |
| `user_permission_denied.{userId}.{permission}`   | User-specific permission denial (overrides grants)  |

**Instance Roles:**

| Role       | Type     | Default Permissions                                              |
| ---------- | -------- | ---------------------------------------------------------------- |
| `admin`    | Explicit | All permissions (implicit)                                       |
| `verified` | Implicit | `spaces.browse`, `spaces.join`, `spaces.create`, `dms.view`, `dms.write` |
| `member`   | Implicit | `spaces.browse`                                                  |

Notes:
- The `admin` role has all permissions implicitly and must be explicitly assigned
- The `verified` role is implicit for users with at least one verified email (cannot be explicitly assigned/revoked)
- The `member` role is implicit for all authenticated users (no assignments stored)
- Config-designated owners (`owners.emails`) are matched against verified emails only (unverified emails are ignored)
- User-specific overrides follow deny-override semantics: denials take precedence over grants and role permissions
- Unverified users can only browse spaces; they must verify an email to join or create spaces

**NOTIFICATIONS keys:**

| Key                          | Description                                       |
| ---------------------------- | ------------------------------------------------- |
| `{userId}.{notificationId}`  | Notification record (protobuf Notification)       |

Notes: 90-day TTL for automatic cleanup. Notifications are created for DM messages, @mentions, and thread replies. Supports real-time sync via `NotificationCreatedEvent` and `NotificationDismissedEvent` published to `live.instance.user.{userId}.*`.

**AUTH_TOKENS keys:**

| Key       | Description                                           |
| --------- | ----------------------------------------------------- |
| `{token}` | JSON with user ID and creation time                   |

Notes: Tokens are opaque strings (`cht_AT` + 14-char NanoID). Used for `Authorization: Bearer <token>` header authentication, enabling cross-origin clients. TTL-based auto-expiry (default 90 days, configurable via `auth.token_ttl`). Excluded from backups since tokens are ephemeral credentials. Tokens are issued on login, registration, bootstrap, and OAuth callback.

**ENCRYPTION_KEYS keys:**

| Key        | Description                                          |
| ---------- | ---------------------------------------------------- |
| `{userId}` | User's 32-byte encryption key (ChaCha20-Poly1305)    |

Notes: Excluded from backups so backup archives contain only encrypted data, not the keys to decrypt it. Enables GDPR-compliant crypto-shredding: deleting a user's key renders all their messages permanently unreadable.

**SERVER\_CONFIG keys:**

Room and membership keys carry a `kind` segment (`channel` or `dm`) so listing operations can prefix-filter without loading and deserializing every record. The kind isn't a field on the `Room` proto — the storage layout is the canonical source of truth.

| Key                                                  | Description                                      |
| ---------------------------------------------------- | ------------------------------------------------ |
| `room.channel.{roomId}`                              | Channel-style room                               |
| `room.dm.{roomId}`                                   | Direct-message room                              |
| `room_name_index.{lowercaseName}`                    | Atomic name claim → room ID. Channels only; DMs have empty names. Enforces case-insensitive uniqueness without a read-then-write race. |
| `room_membership.channel.{roomId}.{userId}`          | Channel membership (room-first ordering matches `room.{kind}.{X}`)  |
| `room_membership.dm.{roomId}.{userId}`               | DM membership                                    |
| `role.{roleName}`                                    | Role metadata (name, display_name, description)  |
| `role_permission.{roleName}.{permission}`            | Permission grant (empty value = granted)         |
| `role_assignment.{roleName}.{userId}`                | Role assignment (empty value = assigned)         |
| `user_permission.{userId}.{permission}`              | User-specific permission grant (overrides role perms)   |
| `user_permission_denied.{userId}.{permission}`       | User-specific permission denial (overrides all grants)  |

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

Same shape as the legacy `SPACE_{spaceId}_RBAC` keys (`role.*`, `role_permission.*`, `role_assignment.*`, `user_permission.*`, `user_permission_denied.*`). Held in `SERVER_RBAC` post-#330 phase 4a; pre-4a primary data was copied verbatim.

**SERVER\_RUNTIME keys:**

| Key                                    | Description                                                       |
| -------------------------------------- | ----------------------------------------------------------------- |
| `room_read_event.{userId}.{roomId}`    | Last-read root message event ID (UTF-8 string, ~14 bytes). Empty value = "joined but no specific event read yet" (e.g. joined an empty room). Missing key triggers a one-time lazy init to the room's current last event ("caught up at first read post-deploy"). The legacy `room_read_status.*` keys (8-byte uint64 sequences) are orphaned and ignored. |
| `room_mention_status.{userId}.{roomId}`| Unread mention indicator (boolean — key presence means unread)    |
| `room_last_msg_at.{roomId}`            | Last message timestamp (per-room, used for sidebar sort)          |
| `video.{attachmentId}`                 | Video processing state for an attachment                          |

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

Notes: Emoji stored as name (e.g., "thumbsup") for NATS KV key compatibility. Separated for load isolation (high-volume). Events are live-only (not stored in JetStream). KV bucket is source of truth. Keyed by event ID (not the volatile JetStream sequence) so reactions survive any future stream re-publishing.

**SERVER\_THREADS keys:**

| Key                       | Description                                              |
| ------------------------- | -------------------------------------------------------- |
| `{roomId}.{rootEventId}`  | ThreadMetadata proto (reply count, last reply, participants) |

Notes: Updated on each thread reply via optimistic locking. Tracks up to 50 participant IDs. Used for thread previews in channel view.

### Object Store Buckets

| Bucket                      | Scope     | Description                                       |
| --------------------------- | --------- | ------------------------------------------------- |
| `INSTANCE_ASSETS`           | Instance  | User avatars, space icons                         |
| `ASSET_CACHE`               | Instance  | Cached resized images (optional)                  |
| `SERVER_ASSETS`             | Server    | Message attachments (primary + DM)                |
| `SPACE_{spaceId}_ASSETS`    | Per-space | Message attachments (non-primary, non-DM spaces)  |

**INSTANCE_ASSETS keys:**

| Key          | Description                         |
| ------------ | ----------------------------------- |
| `{assetId}`  | User avatars, space icons, etc.     |

Notes: Content-Type stored in object headers. S2 compression enabled. Assets referenced by `Asset` proto in entity records (e.g., `User.Avatar`).

**ASSET_CACHE keys:**

| Key                                    | Description                                  |
| -------------------------------------- | -------------------------------------------- |
| `{spaceId}.{attachmentId}.{paramsHash}`| Cached WebP image at specific dimensions     |

Notes: Only created when `[core.assets.cache]` is enabled in config. Uses TTL for automatic expiration (default 7 days). `paramsHash` is first 16 hex chars of SHA256(`{width}x{height}_{fit}`). Animated GIFs are not cached (served directly). S2 compression enabled.

**SERVER\_ASSETS keys (primary + DM, post phase 4e):**

| Key                   | Description                                     |
| --------------------- | ----------------------------------------------- |
| `{attachmentId}`      | Original attachment files (images, videos, etc.)|
| `{attachmentId}_thumb`| WebP thumbnails (256px max dimension)           |

Notes: Same key shape as the legacy per-space `ASSETS` stores — attachment IDs are globally unique, so no kind segment is needed. Content-Type and original filename stored in object headers. S2 compression enabled. Attachment metadata stored in `MessageBody` proto in `SERVER_BODIES`.

**SPACE\_{spaceId}\_ASSETS keys (non-primary, non-DM spaces only):**

| Key                   | Description                                     |
| --------------------- | ----------------------------------------------- |
| `{attachmentId}`      | Original attachment files (images, videos, etc.)|
| `{attachmentId}_thumb`| WebP thumbnails (256px max dimension)           |

Notes: Same shape as `SERVER_ASSETS`. Content-Type and original filename stored in object headers. S2 compression enabled.

### Dynamic Image Transformation

Chatto supports on-the-fly image transformation for attachments, allowing clients to request images at specific dimensions without pre-generating all possible sizes.

**URL Structure:**

```
/assets/space/{spaceId}/attachments/{attachmentId}/t/{signedPath}
```

Where `{signedPath}` is: `{base64params}.{signature}`

- `{base64params}` - Base64URL-encoded JSON: `{"w":640,"h":512,"f":"contain"}`
- `{signature}` - Truncated HMAC-SHA256 (32 hex chars) of `{spaceId}/{attachmentId}/{base64params}`

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
- Thread subject pattern: `space.{spaceId}.room.{roomId}.msg.{rootEventId}.replies.{eventId}`
- Enables O(1) lookup of thread replies via wildcard pattern: `msg.*.replies.{eventId}`
- Thread metadata (reply count, participants) stored in THREADS bucket keyed by `{roomId}.{rootEventId}`

**@Mentions:**

- `@username` patterns in message body are extracted via regex (ASCII alphanumeric, underscore, hyphen)
- Usernames are resolved to user IDs; only space members are included (non-members silently ignored)
- `MessagePostedEvent.mentioned_user_ids` contains resolved user IDs
- Mention status stored in RUNTIME bucket (`room_mention_status.{userId}.{roomId}`)
- Live notification published to `live.instance.user.{userId}.mentioned` for toast display
- Mention indicator cleared when user calls `markRoomAsRead`
- Self-mentions are filtered out (no notification to message author)

**GDPR Deletion:**

- Delete only removes the KV entry in BODIES bucket using the compound key
- Event remains in stream as audit record with empty body
- `GetMessageBody` returns empty string for deleted messages

### Key Patterns

- **Unified Event Subscriptions**: The `mySpaceEvents` subscription merges multiple event sources into a single stream: a JetStream ordered consumer (using `DeliverNewPolicy` for real-time delivery), NATS Core subscriptions for live-only events, and a PresenceHub subscription for presence updates.
- **Compression**: Space and room event streams use S2 compression to reduce storage costs
- **GDPR Compliance**: Message bodies stored separately in BODIES buckets for compliant deletion while preserving audit trail
- **Per-Space Isolation**: Rooms, memberships use per-space metadata buckets; message bodies use per-space BODIES buckets
- **Out-of-Band Data Pattern**: High-volume content (message bodies) separated into dedicated BODIES buckets to avoid contention with metadata operations, enable independent scaling, and support future optimizations (compression, different storage backends)
- **Eager Space Resource Initialization**: All per-space resources (stream, 5 KV buckets, object store) are created at space creation time via `createSpaceResources()`. This ensures predictable behavior and avoids first-use latency. The cache `getOrCreate()` methods still use `CreateOrUpdate` for backward compatibility with spaces created before this pattern was introduced.
