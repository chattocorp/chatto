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

Permissions are granted through roles assigned to space members. Use `Can*` functions in `core/can.go` to check permissions.

### Resolution Algorithm

Permission resolution is a single unified walker. Position numbers run **everyone=0** at the bottom up to **owner=1000** at the top (higher number = more power). For each permission check, the resolver applies the following phases in order; the first decision wins.

**Phase 0 — DM boundary deny-list.** In DM rooms, the permissions in `dmBoundaryDeniedPermissions` (privacy/category-mismatch) are denied unconditionally regardless of any grant. See "DM Privacy Boundary" below.

**Phase 1 — User-level overrides.** Explicit grants/denies attached directly to the user (KV subject = the userID) are checked next. Room scope before server scope. **User-level decisions outrank every role grant.** Use these for:
- *Suspension*: deny a perm directly to one user → they're blocked even if their roles grant it.
- *Ad-hoc grants*: grant `message.delete-any` to one user in one room without inventing a role for it.

**Phase 2 — Role hierarchy walk.** Roles are walked in **descending position order** (highest rank first). For each role, room-scope decisions are probed before server-scope. The first allow/deny wins; lower-ranked roles aren't consulted further.

There is no "bypass" short-circuit. Owners pass permission checks because the owner role is seeded with every server-scope permission via `DefaultOwnerPermissions` (the same set as admin), not because the resolver special-cases them. Any deny configured by an operator applies uniformly — including to owners. The owner / admin distinction is enforced through rank, not through capability.

Consequences worth knowing:

- **A higher-ranked role's grant overrides a lower-ranked role's deny.** Patterns like an `#announcements` room (deny on `everyone`, grant on `moderator`) work because moderator is checked first.
- **Within a single subject, room-scope overrides server-scope.** Same-subject specificity.
- **There is no deny-always-wins floor at the role level.** An operator who wants to forbid an action across the board should deny on the highest-ranked role that should be affected — or attach a per-user deny.
- **Default-deny.** If no phase emits a decision, the result is "no decision" — treated as deny at the API boundary.

**Testing implication:** Denying a permission on `everyone` does NOT block users with higher-rank roles. To test permission denial, deny on the user's actual highest-rank role, attach a user-level deny, or deny on a higher-ranked role.

### Position numbering

| Role | Position | Notes |
|---|---|---|
| `everyone` | 0 | Implicit role every authenticated user holds |
| custom roles | 1..99 | `GetNextAvailablePosition` and `ReorderRoles` slot custom roles below moderator by default; operators can promote them via reorder |
| `moderator` | 100 | System role |
| `admin` | 900 | System role |
| `owner` | 1000 | System role; holds the same enumerated permission set as `admin`. Distinct via rank only. |

Wide gaps between system roles leave room for custom roles to be positioned at any rank without renumbering existing ones. Same-position roles resolve deterministically via stable sort + role-name secondary key. System roles can't be reordered or have their positions changed via the public API.

### DM Privacy Boundary

DM rooms use the same hierarchy walker as channels, with one extra rule: a static set of permissions is *unconditionally denied* in DM contexts regardless of role grants. See `dmBoundaryDeniedPermissions` in `permission_resolver.go`. Two reasons appear:

- **Privacy** — owners/admins/moderators cannot moderate DM contents (`message.edit-any`, `message.delete-any`, `room.manage`, `message.echo`).
- **Category mismatch** — DMs have their own listing/creation/membership APIs, so channel-style `room.create` / `member.invite` / `member.remove` don't apply.

Access *to* DM rooms is gated by participation (`requireRoomMember`). There are no `dm.*` permissions; `message.post` gates starting DMs and root DM messages, while `message.post-in-thread` gates thread replies. The deny-list only constrains what a participant can do once inside.

### Rank vs Permission: the two-step rule

RBAC has two distinct concepts that are easy to conflate:

- **Permission** — "is this role authorized to perform action X at all?" (capability gate)
- **Rank** — "does the caller outrank the specific target user?" (hierarchy invariant)

**Any mutation that targets another user requires BOTH:**

1. The relevant permission (e.g. `role.assign` for user-admin actions).
2. `OutranksUser(actor, target)` — the actor's highest role must outrank the target's.

Rank alone is **not** an authorization check. A function named `OutranksUser` answers a hierarchy question; it does not gate a capability. Conversely, a permission alone breaks the hierarchy invariant — a moderator with `admin.manage-users` should not be able to rename an owner.

Both checks together: callers use `requireUserAdminTarget` (in `graph/authz.go`) for user-admin mutations like `updateProfile` / `uploadAvatar` / `updateSettings` / `AdminMutations.updateUser`. Self-actions bypass both for identity edits, but NOT for authorization edits — see below.

**Identity edits vs. authorization edits.** A targeted user mutation falls into one of two categories, and the category determines whether self-action is allowed:

- **Identity edits** change data the user could already change about themselves (display name, login, avatar, settings). Self-action is privilege-neutral, so the gate has a self-bypass.
- **Authorization edits** change the user's permission set (`grantUserPermission` / `denyUserPermission` / `clearUserPermissionState`). Self-action would be a privilege boundary change, so the gate has NO self-bypass. The strict-outrank step fails on self by definition, so self-action is impossible by construction.

Picking the wrong helper for an authz mutation is a privilege escalation — verify the category before reusing a helper.

**Helpers:**

- `requireUserAdminTarget` — for identity/role-membership mutations (`updateProfile`, `uploadAvatar`, `updateSettings`, `AdminMutations.updateUser`, `ClearUsernameCooldown`). Requires `role.assign` AND `OutranksUser`. **Has self-bypass.**
- `requireUserPermissionTarget` — for per-user permission grants/denials (`grantUserPermission`, `denyUserPermission`, `clearUserPermissionState`). Requires `role.manage` AND `OutranksUser`. **No self-bypass.** Uses `role.manage` (not `role.assign`) because a direct user grant can attach any permission, including ones not in any role — same trust level as defining role permissions.
- `requireOutranksAuthor` — for message-content moderation (`updateMessage` / `deleteMessage` when actor != author). Combined with the permission check (`CanManageOthersMessage`) it enforces "permission AND outranks the author". Prevents a rogue moderator from editing or deleting messages from higher-ranked users. Authors editing or deleting their *own* messages do NOT go through this gate — that's always allowed, subject only to the edit window (for edits) and room membership.

**Permitted single-step uses:**

- **UI-hint resolvers** that only inform the frontend whether to show an admin affordance. `Server.viewerCanManageUser` is rank-only by design — the frontend uses it to hide buttons, not to permit operations. Backend mutations still enforce the two-step.
- Permission-only checks for non-targeted actions (e.g. `createRoom` just needs `rooms.create`; there is no target user).

**Anti-pattern (avoid):** a helper named `CanManageUser` or `CanAdminTargetUser` that internally implements only the rank check. Naming a function `Can…` implies authorization; the body must reflect that. If a function answers a hierarchy question, name it `OutranksUser`.

### Permission Constant Naming

Permission constants follow the pattern `InstPerm{Category}{Action}` (singular nouns):

| Pattern | Example | Notes |
|---------|---------|-------|
| `InstPerm{Category}{Action}` | `InstPermSpaceCreate` | Singular category |
| `InstPermAdmin{Area}{Action}` | `InstPermAdminUsersView` | Admin permissions |
**Common mistakes** (avoid these):
- `InstPermSpacesCreate` → Use `InstPermSpaceCreate` (singular)
- `InstPermAdminAccessUsersView` → Use `InstPermAdminUsersView`

The Go constants in `cli/internal/core/permissions.go` are the source of truth. Frontend TypeScript types are generated via `mise codegen-types`.

### Permission String Naming

Permission strings use **hyphens** as word separators (e.g., `message.post-in-thread`, `message.echo`, `message.manage`). Never use underscores in permission strings.

### Permission Scopes (server / group / room)

Most channel-room permissions are configurable at **three tiers** that
the resolver walks in order (room → group → server) when checking
permissions in a channel room. The first explicit allow/deny wins.

- **Server scope** — the global default. Stored on the server RBAC bucket.
  Used as-is for DM rooms (which aren't in any group) and as a fallback
  for channel rooms with no per-group / per-room override.
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
| `room.join`, `room.manage`, `message.post`, `message.post-in-thread`, `message.react`, `message.echo`, `message.manage` | `server`, `group`, `room` |

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
| `room.manage` | Edit, configure permissions on, and delete rooms |
| `room.join` | Join existing rooms |
| `message.post` | Post root messages in a room |
| `message.post-in-thread` | Post messages inside a thread |
| `message.react` | Add and remove reactions on messages |
| `message.echo` | Echo a thread reply back to the main channel |
| `message.manage` | Edit and delete *other* users' messages (subject to outranking the author). Authors editing or deleting their own messages don't need this. |
| `user.delete-any`, `user.delete-self` | Delete user accounts (server-admin / self) |
| `admin.access`, `admin.view-users`, `admin.view-system`, `admin.view-audit` | Admin panel access tiers |

## GraphQL Authorization Reference

### Queries

| Query | Auth Required | Additional Check |
|-------|---------------|------------------|
| `me` | No | Returns null if unauthenticated |
| `user(id)` | No | Public user profiles |
| `users` | Yes | Server admin only |
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
| `createRoom` | Yes | `rooms.create` |
| `joinRoom` | Yes | Space membership + `rooms.join` |
| `leaveRoom` | Yes | None |
| `postMessage` | Yes | Room membership + `message.post` (root) or `message.post-in-thread` (thread reply), + `message.echo` (if `alsoSendToChannel`) |
| `updateMessage` | Yes | Room membership + (author is allowed, subject to the edit window) OR (`message.manage` + outranks the author) |
| `deleteMessage` | Yes | Room membership + (author is allowed) OR (`message.manage` + outranks the author) |
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
| `Space.rooms` | Yes | Space membership + `rooms.browse` |
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

Default member permissions (`rooms.browse`, `rooms.create`, `rooms.join`) can be revoked from the member role. When implementing or modifying permission checks:

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
Phase 5 of #330 there is no special-case fallthrough in the permission
resolver — the config flow materialises a real `owner` role assignment:

- On email verification (registration / OAuth / admin-direct add),
  `addVerifiedEmail` checks the new email against `owners.emails` and
  auto-assigns the `owner` role if it matches. Fresh deployments work
  without a restart.
- For existing deployments, run `chatto reset rbac` after upgrading
  the binary. The command appends reset facts, re-seeds the system roles plus
  default permissions, and assigns `owner` to every user whose verified email
  matches `owners.emails`.

Owners pass every permission check through the standard hierarchy
walk (owner is position 1000, the highest rank). They have access to:

- `/admin` routes in the frontend
- `Query.admin` and `Query.users` in GraphQL
- System monitoring data (NATS stats, streams, KV buckets)
- Everything else (the owner role's grants cover all permissions)

See `admin.md` for the role / config-owner narrative.

## Attachment URL Authorization

Asset binaries are served by the HTTP handler at `/assets/attachments/{signedLocator}` (and `/assets/attachments/{signedLocator}/t/{transformPath}` for image transforms). The locator is a signed payload that carries everything the handler needs to authorize and serve — there is no separate metadata bucket to look up.

See [ADR-032](../../docs/adr/ADR-032-signed-attachment-locator-urls.md) for the full design.

**The locator IS the capability.** The signed payload carries the calling user's ID (`u`) and a Unix-second expiry (`e`) alongside the room/attachment fields. The handler trusts those claims and does not consult the session cookie or `Authorization` header. This is what lets cross-origin `<img>` tags work for remote-server attachments — neither cookies nor bearer headers reach the asset endpoint in that flow.

**Per-request flow** (`resolveLocatorAttachment` in `cli/internal/http_server/assets.go`):

1. **Signature check** — `signedurl.ParseSignedAttachmentLocator` verifies the locator's HMAC against `[core.assets].signing_secret`. Invalid → 403. Also catches forged/tampered URLs.
2. **Expiry check** — `loc.Expired(time.Now().Unix())` rejects URLs past their `ExpiresAt`. Expired → 403. Bounds the leak window.
3. **Room membership** — `RoomMembershipExists(kind, loc.UserID, loc.RoomID)` checks that the *signed user* is still a member of the room. Not a member → 403. This is what auto-revokes URLs on kick/leave.
4. **Attachment lookup** — `LookupAttachment(loc)` dispatches on the locator's source field:
   - `b` (body key) → `FindBodyAttachment(b, a)` reads the `MessageBody` and returns the matching attachment proto.
   - `v` (video-origin attachment ID) → `FindVideoOriginAttachment(v, a)` reads the `VideoProcessingState` and returns the variant or thumbnail attachment proto.
   Missing → 404.
5. Serve the binary from the proto's `Storage` field (presigned S3 redirect or NATS stream).

**The authorization model in one line:** *holding a non-expired signed URL for user X authorizes the attachment fetch as long as X is still a member of the room declared in the URL.* No per-attachment ACLs.

**URLs are per-user.** `attachmentResolver.URL` / `ThumbnailURL` (in `events.resolvers.go`) bake `auth.ForContext(ctx).Id` and `time.Now() + core.AttachmentURLTTL` into the locator at GraphQL resolve time. Two users querying the same attachment get two distinct signed URLs. We intentionally do **not** want shared/CDN-cached attachment URLs — attachments are private content.

**Properties worth knowing:**

- **The signed URL grants access standalone.** A leaked URL is usable by anyone who has it until the deadline passes *or* the signed user loses room membership, whichever comes first. We treat this as a stopgap, not a real cross-origin auth design — `AttachmentURLTTL` is deliberately short (currently **5 minutes**) so URLs really only work while a page is being rendered. A cleaner solution (service worker proxying remote-server requests with the bearer token, most likely) is a follow-up.
- **Auto-revocation still works.** Kicking a user invalidates their outstanding URLs at the next fetch (membership check fails). Deleting an attachment 404s its URLs (lookup returns nil).
- **Secret rotation invalidates every URL at once.** No key versioning today. Currently-loaded pages would 403 their attachment requests after rotation until the user re-renders; URLs are emitted on every GraphQL response, so the impact is bounded to "until next page transition." Mention in the runbook for any future rotation event.
- **Asset access bypasses the GraphQL audit layer.** No per-fetch audit log today. Pre-existing gap, not introduced by this design.

**When extending attachment auth** (e.g., per-attachment ACLs, share links, view-once semantics): the natural extension points are (a) adding fields to the locator payload, or (b) adding checks between step 3 and step 5 of the flow above. Don't reintroduce a per-attachment metadata bucket — the URL is meant to carry the policy claims.
