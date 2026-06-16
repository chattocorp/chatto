# FDR-001: Roles & Permissions (RBAC)

**Status:** Active
**Last reviewed:** 2026-06-16

## Overview

Chatto controls who can do what through role-based access control. Every authenticated user holds one or more server roles; each role grants or denies specific permissions. Permissions can also be overridden per room-group and per room, giving operators fine-grained control without inventing parallel role systems.

## Behavior

- Every authenticated user belongs to the implicit `everyone` role and may additionally hold one or more named server roles.
- The system roles are `owner`, `admin`, `moderator`, `everyone`. Role position controls ordering/display and legacy event compatibility; it is not an authorization rank.
- A role grants or denies named permissions like `message.post`, `room.create`, `admin.view-users`.
- Permission grants/denies can be configured at three scopes: per-server (global override/default), per room-group, and per room. For non-owners, any applicable deny wins; otherwise any applicable allow grants access.
- Permissions gate capabilities, not every form of visibility. For example, DM read access comes from room membership, while `message.post` gates starting DMs and sending root DM messages.
- Server admins can drag-and-drop to reorder custom roles. System role positions are fixed for ordering consistency.
- Custom role display names are limited to 80 bytes; descriptions are limited to 500 bytes.
- Owners are always granted all permissions. An effective owner is either assigned the durable `owner` role or has a verified email listed in `owners.emails` in `chatto.toml`.
- Owner permissions are virtual rather than persisted defaults: fresh servers do not seed editable owner permission rows, and the admin UI shows owner permissions as read-only green checks.
- GraphQL RBAC editor and inspection queries live under `Query.admin.rbac`. `Query.admin` is an authenticated namespace; the RBAC fields keep their narrower gates such as `role.manage` or `room.manage`.
- Roles have a `pingable` setting that controls whether `@role` pings notify assigned room members. Fresh servers seed `moderator` as pingable and leave `owner`, `admin`, and `everyone` unpingable.
- User-initiated RBAC writes carry the authenticated user's ID as the event actor. Synthetic `system` actors are reserved for bootstrap, seeding, resets, migrations, and other non-user maintenance.

## Design Decisions

### 1. Flat, single-tier role layout

**Decision:** One server-wide role layer. No separate "instance roles" vs "space roles".
**Why:** The earlier two-tier split duplicated concepts and made permission resolution unpredictable. Collapsing into one tier with per-room-group / per-room overrides gives equivalent flexibility with one mental model. See ADR-027 and ADR-030.
**Tradeoff:** Operators who liked per-space role ownership now configure that through room-group overrides instead.

### 2. Deny-wins resolution for non-owners

**Decision:** For non-owners, any applicable deny wins. If there is no deny, any applicable allow grants the permission. If nothing applies, the result is denied at the API boundary.
**Why:** Deny-wins is simple, supports future restriction roles such as `suspended`, and avoids making role position part of authorization. See ADR-040.
**Tradeoff:** An `everyone` deny really applies to every non-owner because all authenticated users carry `everyone`. The built-in announcements room therefore uses a room-level `everyone` deny for `message.post`, which blocks root posts for all non-owner users in that room.

### 3. Three permission scopes (server / group / room)

**Decision:** Room checks consider room, group, and server-scope decisions. Server-scope message and room permissions act as broad defaults; room/group decisions are local exceptions. Fresh dev/bootstrap servers grant ordinary member capabilities such as `room.list`, `room.join`, `message.post`, `message.post-in-thread`, `message.react`, and `message.echo` to `everyone` at server scope. They do not grant `room.create` to `everyone`. Admins get server-tier `room.*` defaults plus `message.manage`; moderators get server-tier `message.manage` and `room.ban-member`.
**Why:** Operators want both "system-wide policy" and "this one channel works differently" without modelling separate role systems. See ADR-031 and ADR-040.
**Tradeoff:** A broad server-scope deny blocks the permission everywhere for affected non-owner users. That is intentional, but operators should prefer room/group denies for local restrictions.

### 4. Owners are effective-owner overrides

**Decision:** Owners are always granted all permissions. Owner role permission rows are not seeded on fresh servers and are not editable through the RBAC UI/API.
**Why:** Instance owners must not be able to lock themselves out through unusual role or per-user permission configuration. See ADR-040.
**Tradeoff:** RBAC cannot be used to restrict owners, and owner permissions appear as virtual read-only allows rather than stored permission decisions. Restricting owner access requires changing ownership configuration or account state.

### 5. Config-designated owners remain effective even without a durable role

**Decision:** `owners.emails` is checked at permission time for verified users and also materialized as an `owner` role assignment where possible.
**Why:** The config is the emergency recovery path. Even if the durable `owner` role is removed, a verified configured owner remains able to recover access.
**Tradeoff:** Removing an email from `owners.emails` now matters at the next permission check; durable owner role assignments may still need separate cleanup.

### 6. Target-user mutations are permission-gated

**Decision:** Mutations that target another user require concrete permissions, not actor-vs-target rank checks. Role assignment uses `role.assign`; direct user permission overrides use `user.manage-permissions`; room bans use `room.ban-member`.
**Why:** The single-server model no longer needs rank hierarchy to protect separate spaces. Concrete permissions are easier to audit and explain.
**Tradeoff:** Permissions must be granted thoughtfully: a user with `role.assign` can assign roles to any target, and a user with `room.ban-member` can ban any non-owner-protected room member.

### 7. RBAC state is event-sourced

**Decision:** Role definitions, role order, assignments, and explicit permission decisions are durable events, with reads served from an in-memory RBAC projection.
**Why:** This aligns RBAC with Chatto's current event-sourced architecture and makes authorization reads rebuildable from the deployment event log. See ADR-033 and ADR-035.
**Tradeoff:** Writes must append events and wait for local projection catch-up before returning, so mutation paths need optimistic concurrency handling instead of direct state writes.

User-triggered RBAC events are audit facts as well as state facts, so their event envelope actor is the user who performed the operation. Core APIs still accept `SystemActorID` for trusted non-user paths such as bootstrapping default roles and permissions.

### 8. Permission-decision events carry typed scope and subject

**Decision:** Permission grant/deny/clear events store `scope` as `{kind, id}` (`SERVER`, `GROUP`, `ROOM`) and `subject` as `{kind, id}` (`ROLE`, `USER`).
**Why:** The old flattened fields made role/user permission subjects indistinguishable and relied on string conventions for scope. The typed shape freezes the domain model before beta and prevents future role IDs from colliding with user IDs.
**Tradeoff:** Event constructors do a little more validation, and compatibility readers for older persisted event shapes have to infer subject kind from legacy wire fields.

## Permissions

The full permission catalog is in `cli/internal/core/permission.go`. Key permissions that gate RBAC management itself:

- `role.manage` — create, edit, delete roles and the permissions attached to them.
- `role.assign` — assign roles to users.
- `user.manage-permissions` — edit direct per-user permission overrides.
- `admin.view-users`, `admin.view-system`, `admin.view-audit` — gate specific admin UI sub-views; admin UI entry is derived from concrete capabilities rather than a standalone `admin.access` permission.
- `message.post` — post root messages in rooms and start DMs. Fresh servers grant this to `everyone` at server scope; announcement rooms add a room-level `everyone` deny.
- `room.manage` — edit/configure/delete channel rooms.
- `room.ban-member` — ban members from channel rooms. DM membership is not managed through this permission.

## Related

- **ADRs:** ADR-004 (authorization at API boundary), ADR-027 (instance/space consolidation), ADR-030 (space tier retirement), ADR-031 (room-group-centric ACL), ADR-033 (event-sourced state), ADR-035 (per-aggregate migration), ADR-037 (DM access via membership), ADR-040 (permission-only RBAC with owner override)
- **FDRs:** Every FDR that mentions a permission depends on this one.
