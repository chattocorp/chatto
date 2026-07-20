# FDR-001: Roles & Permissions (RBAC)

**Status:** Active
**Last reviewed:** 2026-07-20

## Overview

Chatto controls who can do what through role-based access control. Every authenticated user holds one or more roles; each role grants or denies specific permissions. Permissions can also be overridden per room-group and per room, giving operators fine-grained control without inventing parallel role systems.

## Behavior

- Every authenticated user belongs to the implicit `everyone` role and may additionally hold one or more named roles.
- The system roles are `owner`, `admin`, `moderator`, `everyone`. Role position controls ordering/display and legacy event compatibility; it is not an authorization rank.
- A role grants or denies named permissions like `message.post`, `room.create`, `admin.view-users`.
- Permission grants/denies can be configured at three scopes: per-server, per room-group, and per room. Each direct user or named role contributes its nearest decision; denies win across those explicit subjects. The implicit `everyone` role supplies the scoped baseline, and an allow overrides its deny only at the same or a nearer scope.
- Permissions gate capabilities, not every form of visibility. For example, DM read access comes from room membership, while `message.post` gates starting DMs and sending root DM messages.
- Server admins can drag-and-drop to reorder custom roles. System role positions are fixed for ordering consistency.
- Custom role display names are limited to 80 bytes; descriptions are limited to 500 bytes.
- Owners are always granted all permissions. An effective owner is either assigned the durable `owner` role or has a verified email listed in `owners.emails` in `chatto.toml`.
- `admin` and every other non-owner role confer only their explicit permission decisions; they have no role-name-based authority.
- Owner permissions are virtual rather than persisted defaults: fresh servers do not seed editable owner permission rows, and the admin UI shows owner permissions as read-only green checks.
- RBAC editor and inspection APIs are exposed through ConnectRPC admin services. Admin entry is authenticated, and individual operations keep narrower gates such as `role.manage`, `role.assign`, `user.manage-accounts`, `user.manage-permissions`, or `room.manage`.
- Delegated role assignment is bounded by the assigner's own authority. A non-owner may assign a role only when they effectively possess every permission that role explicitly allows at the same scope, and may revoke it only when they have authority over all of its explicit allow and deny decisions. Only an effective owner may assign or revoke the `owner` role.
- Default permissions are creation-time state: fresh server defaults are seeded only into an empty RBAC stream, and channel-room defaults are committed atomically with room creation. Startup does not backfill missing or cleared decisions.
- Roles have a `pingable` setting that controls whether `@role` pings notify assigned room members. Fresh servers seed `moderator` as pingable and leave `owner`, `admin`, and `everyone` unpingable.
- User-initiated RBAC writes carry the authenticated user's ID as the event actor. Synthetic `system` actors are reserved for bootstrap, seeding, migrations, and other non-user maintenance.

## Design Decisions

### 1. Flat, single-tier role layout

**Decision:** One server-wide role layer. No separate "instance roles" vs "space roles".
**Why:** The earlier two-tier split duplicated concepts and made permission resolution unpredictable. Collapsing into one tier with per-room-group / per-room overrides gives equivalent flexibility with one mental model. See ADR-027 and ADR-030.
**Tradeoff:** Operators who liked per-space role ownership now configure that through room-group overrides instead.

### 2. Named subjects with an `everyone` baseline

**Decision:** For non-owners, select the nearest room/group/server decision independently for the direct user and every explicitly assigned named role. Denies win across those decisions. Select `everyone`'s nearest decision as the scoped baseline; a direct-user or named-role allow overrides an `everyone` deny only at the same or a nearer scope. If nothing applies, the result is denied at the API boundary.
**Why:** Operators can express an allowlist by denying the `everyone` baseline and granting a named role, while a named restriction role such as `suspended` still reliably denies. Role position remains irrelevant to authorization. See ADR-052.
**Tradeoff:** An `everyone` deny can be overridden deliberately at its own scope or a nearer one. A restriction role's deny beats other subjects' grants, but a nearer allow configured on that same role replaces its broader deny. Direct-user decisions follow the same nearest-scope rule. ADR-052 records the compatibility audit.

### 3. Three permission scopes (server / group / room)

**Decision:** For each subject, room checks use the nearest decision at room, group, or server scope. Server-scope message and room permissions act as broad defaults; room/group decisions are local overrides for that same subject. Fresh dev/bootstrap servers grant ordinary member capabilities such as `room.list`, `room.join`, `message.post`, `message.post-in-thread`, `message.attach`, `message.react`, and `message.echo` to `everyone` at server scope. They do not grant `room.create` to `everyone`. Admins get explicit server-tier administrative and `room.*` defaults plus `message.manage`, while ordinary content participation continues to come from `everyone`. Moderators get server-tier `message.manage` and `room.ban-member`.
**Why:** Operators want both "system-wide policy" and "this one channel works differently" without modelling separate role systems. See ADR-031 and ADR-052.
**Tradeoff:** Scope precedence is per subject, not global: one role's room allow does not erase a different named role's deny.

### 4. Owners are effective-owner overrides

**Decision:** Owners are always granted all permissions. Owner role permission rows are not seeded on fresh servers and are not editable through the RBAC UI/API.
**Why:** Instance owners must not be able to lock themselves out through unusual role or per-user permission configuration. See ADR-040.
**Tradeoff:** RBAC cannot be used to restrict owners, and owner permissions appear as virtual read-only allows rather than stored permission decisions. Restricting owner access requires changing ownership configuration or account state.

### 5. Config-designated owners remain effective even without a durable role

**Decision:** `owners.emails` is checked at permission time for verified users and also materialized as an `owner` role assignment where possible.
**Why:** The config is the emergency recovery path. Even if the durable `owner` role is removed, a verified configured owner remains able to recover access.
**Tradeoff:** Removing an email from `owners.emails` now matters at the next permission check; durable owner role assignments may still need separate cleanup.

### 6. Target-user mutations are permission-gated and role assignment is bounded

**Decision:** Mutations that target another user require concrete permissions, not actor-vs-target rank checks. Role assignment uses `role.assign`, but a non-owner may assign only roles whose explicit allows they themselves effectively hold at each exact scope. Revocation is also bounded by every explicit allow and deny on the role, because removing a deny can restore authority. The `owner` role remains owner-only; `admin` has no implicit authority outside its explicit permissions. Account lifecycle and recovery operations use `user.manage-accounts`; direct user permission overrides use `user.manage-permissions`; room bans use `room.ban-member`.
**Why:** Concrete permissions are easier to audit and explain than a role-rank hierarchy, while bounding `role.assign` prevents delegated role managers from granting authority they do not possess or removing restrictions they cannot control.
**Tradeoff:** A delegated assigner may need the target role's underlying permissions even when they only administer membership. Owners remain the recovery path, and old replicas can enforce the earlier unbounded rule during a rolling upgrade until they are replaced.

### 7. RBAC state is event-sourced

**Decision:** Role definitions, role order, assignments, and explicit permission decisions are durable events, with reads served from an in-memory RBAC projection.
**Why:** This aligns RBAC with Chatto's current event-sourced architecture and makes authorization reads rebuildable from the deployment event log. See ADR-033 and ADR-035.
**Tradeoff:** Writes must append events and wait for local projection catch-up before returning, so mutation paths need optimistic concurrency handling instead of direct state writes. Authorization-sensitive writes also use a narrow durable authorization fence: changes that can affect authority advance it atomically with their domain facts, and a conflicting write reruns authorization. This adds one empty fence fact to those batches but avoids contending with ordinary chat traffic.

User-triggered RBAC events are audit facts as well as state facts, so their event envelope actor is the user who performed the operation. Core APIs still accept `SystemActorID` for trusted non-user paths such as bootstrapping default roles and permissions.

### 8. Permission-decision events carry typed scope and subject

**Decision:** Permission grant/deny/clear events store `scope` as `{kind, id}` (`SERVER`, `GROUP`, `ROOM`) and `subject` as `{kind, id}` (`ROLE`, `USER`).
**Why:** The old flattened fields made role/user permission subjects indistinguishable and relied on string conventions for scope. The typed shape freezes the domain model before beta and prevents future role IDs from colliding with user IDs.
**Tradeoff:** Event constructors do a little more validation, and compatibility readers for older persisted event shapes have to infer subject kind from legacy wire fields.

### 9. Defaults are one-time initialization, not startup policy

**Decision:** Apply the current server default set only when the durable RBAC stream is empty. New groups and ordinary rooms store no default decisions. Commit a channel room and any exceptional default decisions in one atomic EVT batch: fresh announcements rooms deny `message.post` to `everyone` and allow it for `admin`. Do not inspect, copy, reset, or reconcile existing permission state during startup.
**Why:** Absence is a meaningful RBAC state. Reapplying code defaults on every startup makes an operator's explicit clear indistinguishable from incomplete bootstrap state.
**Tradeoff:** Adding a new code default does not grant it to existing servers or rooms automatically. Older replicas in a rolling deployment still use their historical non-atomic room-creation path until they are replaced.

## Permissions

The full permission catalog is in `cli/internal/core/permission.go`. Key permissions that gate RBAC management itself:

- `role.manage` — configure role definitions and the permissions attached to them.
- `role.assign` — assign or revoke roles, bounded for non-owners by the target role's explicit scoped permission decisions.
- `user.manage-accounts` — create users, edit account identity, reset passwords, attach verified emails, and clear login cooldowns.
- `user.manage-permissions` — edit direct per-user permission overrides.
- `admin.view-users`, `admin.view-audit` — gate specific admin UI sub-views; admin UI entry is derived from concrete capabilities rather than a standalone `admin.access` permission. System diagnostics are owner-only and exposed through a viewer capability, not through grantable RBAC.
- `message.post` — post root messages in rooms and start DMs. Fresh servers grant this to `everyone` at server scope; fresh announcement rooms replace that baseline with a room-level `everyone` deny and a room-level `admin` allow. Moderators and other named roles need their own room-level posting grant.
- `message.attach` — attach files to new messages. Fresh servers grant this to `everyone` at server scope; existing servers are not automatically backfilled after upgrade, so operators may need to grant it manually if uploads should remain enabled.
- `room.manage` — edit/configure/delete channel rooms.
- `room.ban-member` — ban members from channel rooms. DM membership is not managed through this permission.

## Related

- **ADRs:** ADR-004 (authorization at API boundary), ADR-027 (instance/space consolidation), ADR-030 (space tier retirement), ADR-031 (room-group-centric ACL), ADR-033 (event-sourced state), ADR-035 (per-aggregate migration), ADR-037 (DM access via membership), ADR-040 (permission-only RBAC with owner override), ADR-052 (subject-specific RBAC with an everyone baseline)
- **FDRs:** Every FDR that mentions a permission depends on this one.
