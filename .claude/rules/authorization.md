# Authorization Model

This document describes the authorization requirements and policies for Chatto's GraphQL API.

## Core Principles

1. **Users are bound to an instance** - All users exist within a single Chatto instance
2. **Spaces are discoverable** - Users can browse all spaces for discovery purposes. To make an instance fully private, place it behind a reverse-proxy auth layer or disable the `Query.spaces` / `Query.space(id)` resolvers via a future configuration flag — the GraphQL gateway alone cannot enforce private-instance discovery.
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

### Hierarchy-Wins Resolution

Permission resolution follows role hierarchy order (lower position = higher rank):

1. Get user's roles sorted by position (lower = higher rank)
2. For each role in order, check for explicit grant or deny
3. First explicit decision found wins

This enables patterns like:
- `#announcements` rooms where `everyone` is denied `message.post` but `owner/admin/moderator` can still post (higher rank checked first), while everyone retains `message.post-in-thread` to discuss in threads
- Instance admin not being blocked by an `everyone` denial

**Testing implication:** Denying a permission on the `everyone` role does NOT block users with higher-rank roles (like `admin`). To test permission denial, deny on the user's actual highest-rank role or a role with equal/higher rank.

### Permission Constant Naming

Permission constants follow the pattern `InstPerm{Category}{Action}` (singular nouns):

| Pattern | Example | Notes |
|---------|---------|-------|
| `InstPerm{Category}{Action}` | `InstPermSpaceCreate` | Singular category |
| `InstPermAdmin{Area}{Action}` | `InstPermAdminUsersView` | Admin permissions |
| `InstPermDM{Action}` | `InstPermDMWrite` | DM permissions |

**Common mistakes** (avoid these):
- `InstPermSpacesCreate` → Use `InstPermSpaceCreate` (singular)
- `InstPermDMsWrite` → Use `InstPermDMWrite` (no plural 's')
- `InstPermAdminAccessUsersView` → Use `InstPermAdminUsersView`

The Go constants in `cli/internal/core/permissions.go` are the source of truth. Frontend TypeScript types are generated via `mise codegen-types`.

### Permission String Naming

Permission strings use **hyphens** as word separators (e.g., `message.post-in-thread`, `message.edit-own`, `message.reply-in-thread`). Never use underscores in permission strings.

### Built-in Permissions

| Permission | Description |
|------------|-------------|
| `space.manage` | Update space settings (name, description) |
| `space.delete` | Delete the space |
| `roles.manage` | Create/edit/delete roles |
| `roles.assign` | Assign roles to users |
| `members.invite` | Invite new members |
| `members.remove` | Remove members from space |
| `rooms.browse` | View list of rooms in space |
| `rooms.create` | Create new rooms |
| `rooms.manage` | Update/delete any room |
| `rooms.join` | Join existing rooms |

## GraphQL Authorization Reference

### Queries

| Query | Auth Required | Additional Check |
|-------|---------------|------------------|
| `me` | No | Returns null if unauthenticated |
| `user(id)` | No | Public user profiles |
| `users` | Yes | Instance admin only |
| `spaces` | No | Discovery - lists all spaces |
| `space(id)` | No | Discovery - view any space |
| `room(spaceId, roomId)` | Yes | Room membership required |
| `roomEvents(...)` | Yes | Room membership required |
| `roomEvent(...)` | Yes | Room membership required |
| `admin` | Yes | Instance admin only |

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
| `postMessage` | Yes | Room membership + `message.post` (root) or `message.post-in-thread` (thread reply), + `message.reply` (if `inReplyTo` in room) or `message.reply-in-thread` (if `inReplyTo` in thread), + `message.echo` (if `alsoSendToChannel`) |
| `markRoomAsRead` | Yes | Room membership |
| `addReaction` | Yes | Room membership |
| `removeReaction` | Yes | Room membership |
| `deleteMessage` | Yes | Room membership + message ownership |
| `updateMyPresence` | Yes | None (sets caller's own presence) |

### Subscriptions

| Subscription | Auth Required | Additional Check |
|--------------|---------------|------------------|
| `mySpaceEvents(spaceId)` | Yes | Space membership |
| `mySpaceLiveEvents(spaceId)` | Yes | Space membership |
| `myInstanceEvents` | Yes | None (user's own events) |
| `presenceUpdates(spaceId)` | Yes | Space membership |

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

1. **Always use the RBAC engine** - Never hardcode permission grants based on role names or "default" lists
2. **Test both grant and revoke** - Permissions must work when granted AND when revoked
3. **Follow the instance RBAC pattern** - Use `engine.RoleHasPermission(ctx, RoleMember, permStr)` to check actual KV state

**Anti-pattern (avoid):**
```go
// BAD: Hardcoded bypass that ignores actual role permissions
if isMember && isDefaultPermission(perm) {
    return true, nil  // Bypasses RBAC engine!
}
```

**Correct pattern:**
```go
// GOOD: Always check actual role permissions via RBAC engine
if isMember {
    hasPerm, err := engine.RoleHasPermission(ctx, RoleMember, string(perm))
    if hasPerm {
        return true, nil
    }
}
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
  the binary. The command wipes `SERVER_RBAC`, re-seeds the system
  roles plus default permissions, and assigns `owner` to every user
  whose verified email matches `owners.emails`.

Owners pass every permission check through the standard hierarchy
walk (owner is rank 0). They have access to:

- `/admin` routes in the frontend
- `Query.admin` and `Query.users` in GraphQL
- System monitoring data (NATS stats, streams, KV buckets)
- Everything else (the owner role's grants cover all permissions)

See `admin.md` for the role / config-owner narrative.
