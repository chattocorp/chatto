# Authorization Model

This document describes the authorization requirements and policies for Chatto's GraphQL API.

## Core Principles

1. **Users are bound to a server** - All users exist within a single Chatto server
2. **Spaces are discoverable** - Users can browse all spaces for discovery purposes. To make a server fully private, place it behind a reverse-proxy auth layer or disable the `Query.spaces` / `Query.space(id)` resolvers via a future configuration flag — the GraphQL gateway alone cannot enforce private-server discovery.
3. **Room access requires space membership** - Users must join a space before accessing its rooms
4. **Message access requires room membership** - Users can only read/write messages in rooms they've joined
5. **User profiles are public** - Basic user info (id, login, displayName, avatar) is visible to all authenticated users
6. **Membership info is private** - Users can only see their own space/room memberships

## Authorization Architecture

Authorization is enforced at the **API boundary**, not in core:

| Layer | Responsibility |
|-------|----------------|
| **GraphQL** | User-facing API. Checks authorization via `Can*` functions before calling core. |
| **Core** | Pure business logic. Assumes caller is authorized. Documents requirements in comments. |
| **NATS** | Extension/internal API. Trusted context, calls core directly. |

**Why this design:**
- Core functions are reusable from trusted contexts (NATS handlers, background jobs)
- No redundant permission checks when core calls other core functions
- Clear separation: GraphQL handles user authorization, core handles business logic
- Audit logging can be added orthogonally without coupling to authorization

## Permission System

Permissions are granted through roles and direct per-user overrides. Use `Can*` functions in `core/can.go` to check permissions.

### Resolution Algorithm

Chatto uses permission-only RBAC with a non-lockout owner override. See ADR-040.

Resolution order:

1. **Effective-owner override.** A user with the durable `owner` role, or a verified email matching `owners.emails`, receives every known RBAC permission.
2. **DM boundary deny-list for non-owners.** In DM rooms, privacy/category-mismatch permissions are denied regardless of grants for everyone else.
3. **Deny-wins for non-owners.** Any applicable user or role deny blocks the permission.
4. **Allow if any allow applies.** If no deny applies, any applicable user or role allow grants the permission.
5. **Default deny.** If nothing applies, the API treats the result as denied.

Applicable decisions include direct user decisions and all roles assigned to the user, including implicit `everyone`. For room checks, room, group, and server scopes all contribute. Server-scope room/message decisions therefore work as broad defaults/global overrides, while room and group decisions are local exceptions.

### Role Positions

Role `position` is ordering/display metadata and legacy event compatibility. It is not an authorization rank.

| Role | Position | Notes |
|---|---|---|
| `everyone` | 0 | Implicit role every authenticated user holds |
| custom roles | 1..99 | Operators can reorder these for display |
| `moderator` | 100 | System role |
| `admin` | 900 | System role |
| `owner` | 1000 | System role; effective owners also include verified `owners.emails` matches |

### DM Privacy Boundary

DM rooms use the same permission resolver with one extra rule: a static set of permissions is denied in DM contexts regardless of role grants for non-owners. See `dmBoundaryDeniedPermissions` in `permission_resolver.go`.

- **Privacy** — owners/admins/moderators cannot moderate DM contents (`message.manage`, `room.manage`, `room.ban-member`, `message.echo`).
- **Category mismatch** — DMs have fixed membership APIs, so channel-style `room.create` / `room.ban-member` do not apply.

Access to DM rooms is gated by participation (`requireRoomMember`). There are no `dm.*` permissions; `message.post` gates starting DMs and root DM messages, while `message.post-in-thread` gates thread replies.

### Targeted Operations

Targeted mutations are permission-gated, not rank-gated:

- `requireUserAdminTarget` — self is allowed; cross-user identity/role membership actions require `role.assign`.
- `requireUserPermissionTarget` — direct per-user permission grant/deny/clear requires `user.manage-permissions`.
- Message moderation of another user's message requires `message.manage` in the room. Authors can edit/delete their own messages without `message.manage`.
- Room bans require `room.ban-member` in the room. DM rooms reject bans.

Owners are protected from lockout by the effective-owner override, not by target-rank checks.

### Permission Constant Naming

Permission constants follow the pattern `InstPerm{Category}{Action}` (singular nouns):

| Pattern | Example | Notes |
|---------|---------|-------|
| `InstPerm{Category}{Action}` | `InstPermSpaceCreate` | Singular category |
| `InstPermAdmin{Area}{Action}` | `InstPermAdminUsersView` | Admin permissions |
**Common mistakes** (avoid these):
- `InstPermSpacesCreate` → Use `InstPermSpaceCreate` (singular)

The Go constants in `cli/internal/core/permissions.go` are the source of truth. Frontend TypeScript types are generated via `mise codegen-types`.

### Permission String Naming

Permission strings use **hyphens** as word separators (e.g., `message.post-in-thread`, `message.echo`, `message.manage`). Never use underscores in permission strings.

### Permission Scopes (server / group / room)

Most channel-room permissions are configurable at **three tiers**. The
resolver collects all applicable decisions from server, group, and room scope:
any deny wins for non-owners, otherwise any allow grants the permission.

- **Server scope** — the broad default. Stored on the server RBAC bucket.
  Used as-is for DM rooms (which aren't in any group) and as a fallback
  for channel rooms unless a per-group / per-room deny narrows it.
- **Group scope** — per-room-group config (ADR-031). Stored against a
  group ID. Overrides server-scope when present.
- **Room scope** — per-room override. Stored against a room ID.
  Overrides both group-scope and server-scope when present.

A permission's `Scopes` field declares which tiers it can be configured
at. Examples after the message-perms consolidation:

| Permission | Scopes |
|------------|--------|
| `server.manage`, `role.manage`, `role.assign`, `admin.*`, `user.*` | `server` only |
| `room.create` | `server`, `group` (no per-room — you can't create a room inside a room) |
| `room.join`, `room.manage`, `room.ban-member`, `message.post`, `message.post-in-thread`, `message.react`, `message.echo`, `message.manage` | `server`, `group`, `room` |

### Permission Naming

Permission strings must have exactly two dot-separated segments:
`{objectType}.{verb}`. The RBAC key code derives `objectType` and `verb`
directly from that shape and panics on nested dots. For compound actions,
keep the object type as the first segment and use dashes in the verb:

- Good: `room.ban-member`, `message.post-in-thread`, `admin.view-users`
- Bad: `room.member.remove`, `message.thread.post`

Moderator actions that affect another user's membership should use explicit
moderation events, not overloaded join/leave events. In particular,
`UserJoinedRoomEvent` and `UserLeftRoomEvent` are actor-only membership facts;
the actor is the user who joined or left. A moderator ban/removal must use a
dedicated event such as `RoomMemberBannedEvent` for audit and moderation state.
If the action should be visible as a normal membership transition, also emit a
normal actor-only join/leave event for the affected user; never add a target
user ID to join/leave events.

When adding a permission, update `cli/internal/core/permission.go`,
`frontend/src/lib/permissions.ts`, the relevant FDR/ADR docs, and tests
covering permission scope / DM boundary behavior. Then regenerate the Go
and frontend mirrors (`mise codegen-cli`, `mise codegen-types`, and
`mise codegen-frontend` as applicable).

`CanCreateRoom(userID, kind, groupID)` takes an optional group context:
when `groupID` is non-empty the check uses the group→server walk; with
no group it uses pure server-scope. This lets operators grant a role
the ability to create rooms only in specific groups.

### Built-in Permissions

| Permission | Description |
|------------|-------------|
| `server.manage` | Update server settings (name, description, logo) |
| `role.manage` | Create/edit/delete roles and their permission grants |
| `role.assign` | Assign roles to users |
| `room.create` | Create new rooms in a group |
| `room.manage` | Edit, configure permissions on, and delete channel rooms |
| `room.ban-member` | Ban members from channel rooms |
| `room.join` | Join existing rooms |
| `message.post` | Post root messages in a room |
| `message.post-in-thread` | Post messages inside a thread |
| `message.react` | Add and remove reactions on messages |
| `message.echo` | Echo a thread reply back to the main channel |
| `message.manage` | Edit and delete *other* users' messages. Authors editing or deleting their own messages don't need this. |
| `user.manage-permissions` | Edit direct per-user permission overrides |
| `user.delete-any`, `user.delete-self` | Delete user accounts (server-admin / self) |
| `admin.view-users`, `admin.view-system`, `admin.view-audit` | Admin panel sub-view access tiers |

## GraphQL Authorization Reference

### Queries

| Query | Auth Required | Additional Check |
|-------|---------------|------------------|
| `me` | No | Returns null if unauthenticated |
| `user(userId)` / `userByLogin(login)` | Yes | Member profiles are public to authenticated users |
| `server.members(...)` | Yes | Server member directory visible to authenticated users |
| `spaces` | No | Discovery - lists all spaces |
| `space(id)` | No | Discovery - view any space |
| `room(spaceId, roomId)` | Yes | Room membership required |
| `roomEvents(...)` | Yes | Room membership required |
| `roomEvent(...)` | Yes | Room membership required |
| `admin` | Yes | Server admin only |

### Mutations

| Mutation | Auth Required | Additional Check |
|----------|---------------|------------------|
| `createUser` | No | Self-registration |
| `createSpace` | Yes | None (anyone can create) |
| `updateSpace` | Yes | `space.manage` |
| `joinSpace` | Yes | None |
| `leaveSpace` | Yes | None |
| `createRoom` | Yes | `room.create` |
| `joinRoom` | Yes | Space membership + `room.join` |
| `leaveRoom` | Yes | None |
| `banRoomMember` | Yes | Channel rooms only; `room.ban-member` |
| `postMessage` | Yes | Room membership + `message.post` (root) or `message.post-in-thread` (thread reply), + `message.echo` (if `alsoSendToChannel`) |
| `updateMessage` | Yes | Room membership + (author is allowed, subject to the edit window) OR `message.manage` |
| `deleteMessage` | Yes | Room membership + (author is allowed) OR `message.manage` |
| `markRoomAsRead` | Yes | Room membership |
| `addReaction` | Yes | Room membership |
| `removeReaction` | Yes | Room membership |
| `updateMyPresence` | Yes | None (sets caller's own presence) |

### Subscriptions

| Subscription | Auth Required | Additional Check |
|--------------|---------------|------------------|
| `myEvents` | Yes | None at gateway; per-event scoping is enforced inside the resolver (room membership for room events, target-user filtering for private user events, etc.) |

### Field Resolvers

| Field | Auth Required | Additional Check |
|-------|---------------|------------------|
| `Space.rooms` | Yes | Space membership + `room.list` |
| `Space.memberCount` | No | Public count |
| `Space.roomCount` | No | Public count |
| `Space.assetCount` | No | Public count |
| `Room.members` | Yes | Room membership |
| `Room.hasUnread` | No | Returns false if unauthenticated |
| `User.spaces` | Yes | Self only (`caller.Id == obj.Id`) |
| `User.rooms` | Yes | Self only (`caller.Id == obj.Id`) |
| `User.avatarURL` | No | Public |
| `User.presenceStatus` | No | Public |

## Implementation Patterns

### GraphQL Resolver with Permission Check
```go
func (r *mutationResolver) CreateRoom(ctx context.Context, input model.CreateRoomInput) (*Room, error) {
    user, err := requireAuth(ctx)
    if err != nil {
        return nil, err
    }

    // Check permission at GraphQL layer
    can, err := r.core.CanCreateRoom(ctx, user.Id, input.SpaceID)
    if err != nil {
        return nil, err
    }
    if !can {
        return nil, core.ErrPermissionDenied
    }

    // Core function assumes caller is authorized
    return r.core.CreateRoom(ctx, user.Id, input.SpaceID, input.Name, input.Desc)
}
```

### Core Function (no authorization check)
```go
// CreateRoom creates a new room in a space.
// Authorization: Caller must verify CanCreateRoom before calling.
func (c *ChattoCore) CreateRoom(ctx context.Context, actorID, spaceID, name, desc string) (*Room, error) {
    // Business logic only - no permission check here
}
```

### Authentication Helpers (in graph/authz.go)
```go
user, err := requireAuth(ctx)           // Returns authenticated user or error
user, err := requireSpaceMember(ctx, r.core, spaceID)  // + space membership
user, err := requireRoomMember(ctx, r.core, spaceID, roomID)  // + room membership
```

### Self-Only Access Check
```go
if caller.Id != obj.Id {
    return nil, fmt.Errorf("access denied: cannot view other users' data")
}
```

## Customizable Permissions

Default member permissions (`room.list`, `room.join`, `message.post`, `message.post-in-thread`, `message.react`, `message.echo`, and `user.delete-self`) can be denied or cleared from the `everyone` role. When implementing or modifying permission checks:

1. **Always use RBAC resolution** - Never hardcode permission grants based on role names or "default" lists
2. **Test both grant and revoke** - Permissions must work when granted AND when revoked
3. **Follow the server RBAC pattern** - Use the `Can*` helpers or permission resolver to check projected RBAC state

**Anti-pattern (avoid):**
```go
// BAD: Hardcoded bypass that ignores actual role permissions
if isMember && isDefaultPermission(perm) {
    return true, nil  // Bypasses RBAC resolution!
}
```

**Correct pattern:**
```go
// GOOD: Ask the permission resolver / Can helper for the actual decision
canPost, err := core.CanPostMessage(ctx, userID)
if err != nil {
    return false, err
}
return canPost, nil
```

## Server Owner via Config

Owners can be designated via `owners.emails` in `chatto.toml`. After
the RBAC simplification, this is both a runtime effective-owner path and a
best-effort materialized role assignment:

- On email verification (registration / OAuth / admin-direct add),
  `addVerifiedEmail` checks the new email against `owners.emails` and
  auto-assigns the `owner` role if it matches. Fresh deployments work
  without a restart.
- If the durable role assignment is missing, the verified config email still
  grants effective owner access.

Owners pass every known permission check through the effective-owner override.
They have access to:

- `/admin` routes in the frontend
- `Query.admin` in GraphQL; member-directory reads use authenticated `Server.members`
- System monitoring data (NATS stats, streams, KV buckets)
- Everything else (owners are virtual all-allow subjects)

See `admin.md` for the role / config-owner narrative.

## OAuth Client Authorization

Chatto's `/oauth/authorize` endpoint is for compatible Chatto clients that need
opaque bearer tokens for cross-origin server connections.

- Redirect URIs must match the server's configured `webserver.url`, an explicit
  `webserver.oauth_redirect_origins` entry, an exact `webserver.allowed_origins`
  entry, or a loopback development origin. Wildcard CORS
  (`allowed_origins = ["*"]`) is not OAuth redirect trust.
- `oauth_redirect_origins = ["*"]` is an OAuth-specific temporary escape hatch
  for controlled alpha deployments. It allows any otherwise valid HTTPS
  redirect origin, so prefer exact origins for production.
- The first authorization for a trusted redirect origin must show a consent
  screen. Approval is remembered per user + canonical origin through durable
  `EVT` facts; denial is recorded for audit but does not grant consent.
- Do not add an operator-managed OAuth client registry or require `client_id`
  for this flow. Any version-compatible Chatto client should be able to connect
  to any compatible Chatto server once the origin is trusted and the user
  consents.
- Do not persist full redirect URIs in `EVT`. OAuth consent facts may store
  the canonical redirect origin in plaintext so users can later recognize and
  manage approved client addresses.

## Attachment URL Authorization

Attachment binaries are primarily served by the HTTP handler at `/assets/files/{assetId}` (and `/assets/files/{assetId}/image/{width}x{height}/{fit}` for image transforms). Browser-facing GraphQL fields append a per-user `access` ticket query parameter and expose its expiry through `AssetURL { url, expiresAt }`. The stable path identifies the binary; the ticket authorizes direct browser/standalone-client loads.

See [ADR-032](../../docs/adr/ADR-032-signed-attachment-locator-urls.md) for the full design.

**The access ticket IS the browser capability.** The signed ticket carries the asset ID (`a`), calling user's ID (`u`), Unix-second expiry (`e`), and transform parameters when applicable. The handler trusts those signed claims and does not require cookies or bearer headers when an `access` ticket is present. This is what lets cross-origin `<img>` tags work for remote-server attachments — neither cookies nor bearer headers reach the asset endpoint in that flow.

**Per-request flow** (`resolveStableAttachment` in `cli/internal/http_server/assets.go`):

1. **Ticket or request-user resolution** — `resolveStableAssetViewerID` accepts either a signed `access` ticket or, for same-origin/API clients fetching the original binary, the request cookie/bearer user. Image derivatives require a ticket so transform parameters stay bounded by GraphQL-minted URLs.
2. **Signature / expiry / transform check** — `signedurl.ParseSignedAssetAccessTicket` verifies HMAC, expiry, asset ID, and transform parameters. Invalid, expired, or mismatched → 403.
3. **Asset declaration lookup** — `RoomTimeline.AssetCreation(assetID)` and `AssetRoomID(assetID)` resolve the asset and its room scope from projected EVT facts. Missing → 404/403.
4. **Room membership** — `RoomMembershipExists(kind, userID, roomID)` checks that the resolved user is still a member of the room. Not a member → 403. This is what auto-revokes URLs on kick/leave.
5. Serve the binary from the declared asset storage (presigned S3 redirect or NATS stream).

**The authorization model in one line:** *holding a non-expired access ticket for user X authorizes the asset fetch as long as X is still a member of the asset's room.* No per-attachment ACLs.

**URLs are per-user.** `attachmentResolver.AssetURL` / `ThumbnailAssetURL`, `videoProcessing.thumbnailAssetUrl`, and `videoVariant.assetUrl` (in `events.resolvers.go`) bake `callerID(ctx)` and `time.Now() + core.AssetAccessTicketTTL` into the access ticket at GraphQL resolve time. Two users querying the same attachment get two distinct signed URLs. We intentionally do **not** want shared/CDN-cached attachment URLs — attachments are private content.

**Properties worth knowing:**

- **The access ticket grants access standalone.** A leaked URL is usable by anyone who has it until the deadline passes *or* the signed user loses room membership, whichever comes first. `AssetAccessTicketTTL` is currently **1 hour**; browser clients refresh `AssetURL` fields before expiry and after load failures.
- **The browser app hides tickets behind virtual URLs when possible.** Once the Service Worker controls the page, frontend render helpers turn stable `/assets/files/...` URLs into same-origin `/__chatto/assets/{serverId}/...` URLs and register the real ticketed target with the worker. The copied DOM URL is not a bearer credential; it only resolves inside a Chatto client with the matching server registration and credentials. The worker may keep full successful responses in its private asset cache, which is acceptable because the user has already received those bytes.
- **Auto-revocation still works.** Kicking a user invalidates their outstanding URLs at the next fetch (membership check fails). Deleting an attachment 404s its URLs (lookup returns nil).
- **Secret rotation invalidates every URL at once.** No key versioning today. Currently-loaded pages would 403 their attachment requests after rotation until the user re-renders; URLs are emitted on every GraphQL response, so the impact is bounded to "until next page transition." Mention in the runbook for any future rotation event.
- **Asset access bypasses the GraphQL audit layer.** No per-fetch audit log today. Pre-existing gap, not introduced by this design.
- **Legacy locator URLs still exist.** `/assets/attachments/{signedLocator}` and transform variants remain supported for compatibility/internal fallback. They use `AttachmentURLTTL` (currently **5 minutes**) and `resolveLocatorAttachment`, but new GraphQL attachment fields should prefer the stable `/assets/files/...` + `access` ticket shape.

**When extending attachment auth** (e.g., per-attachment ACLs, share links, view-once semantics): the natural extension points are (a) adding scoped fields to the access ticket when the browser must carry the claim, or (b) adding checks between asset declaration lookup and serving the binary. Don't reintroduce a per-attachment metadata bucket — the asset event and URL credential are meant to carry the policy claims.
