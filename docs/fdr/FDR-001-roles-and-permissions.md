# FDR-001: Roles & Permissions (RBAC)

**Status:** Active
**Last reviewed:** 2026-08-28

## Overview

Chatto controls who can do what through role-based access control. Every authenticated human user holds one or more roles; each role grants or denies specific permissions. Permissions can also be overridden per room-group and per room, giving operators fine-grained control without inventing parallel role systems. Bot accounts are the deliberate exception: they use an explicit direct-permission allowlist bounded by their human owner's current authority (FDR-038).

## Behavior

- Every authenticated human user belongs to the implicit `everyone` role and may additionally hold one or more named roles. Bots inherit neither `everyone` nor named-role permissions.
- The system roles are `owner`, `admin`, `moderator`, `everyone`. Role position controls ordering/display and legacy event compatibility; it is not an authorization rank.
- A role grants or denies named permissions like `message.post`, `room.create`, `admin.view-users`.
- A permission identifier has two or more non-empty dot-separated components.
  New identifiers use `<domain>.<capability>[.<narrower-capability>...]`.
  Each registered dotted prefix is a broader permission that includes its
  descendants.
- Permission grants/denies can be configured at three scopes: per-server, per room-group, and per room. Each direct user or named role contributes its nearest decision; denies win across those explicit subjects. The implicit `everyone` role supplies the scoped baseline, and an allow overrides its deny only at the same or a nearer scope.
- Permissions gate capabilities and channel-room message access. Channel-room
  membership is necessary for message reads. `message.read` supplies broad
  read authority and includes `message.read.interactions`, which
  supplies authority for related threads only. DM membership authorizes DM
  reads. `message.post` separately
  gates root-message posting and permits human users to start DMs. Bot accounts
  cannot start DMs regardless of their permissions.
- Server admins can drag-and-drop to reorder custom roles. System role positions are fixed for ordering consistency.
- Custom role display names are limited to 80 bytes; descriptions are limited to 500 bytes.
- Owners are always granted all permissions. An effective owner has the durable `owner` role; verified users listed in `owners.emails` in `chatto.toml` are materialized into that role at boot or through retryable durable work after verification.
- `admin` and every other non-owner role confer only their explicit permission decisions; they have no role-name-based authority.
- Owner permissions are virtual rather than persisted defaults: fresh servers do not seed editable owner permission rows, and the admin UI shows owner permissions as read-only green checks.
- RBAC editor and inspection APIs are exposed through ConnectRPC admin services. Admin entry is authenticated, and individual operations keep narrower gates such as `role.manage`, `role.assign`, `user.manage-accounts`, `user.manage-permissions`, or `room.manage`.
- Delegated role assignment is bounded by the assigner's own authority. A non-owner may assign a role only when they effectively possess every permission that role explicitly allows at the same scope, and may revoke it only when they have authority over all of its explicit allow and deny decisions. Only an effective owner may assign or revoke the `owner` role.
- Default permissions are creation-time state: fresh server defaults are seeded only into an empty RBAC stream, and channel-room defaults are committed atomically with room creation. Startup does not backfill missing or cleared decisions.
- Roles have a `pingable` setting that controls whether `@role` pings notify assigned room members. Fresh servers seed `moderator` as pingable and leave `owner`, `admin`, and `everyone` unpingable.
- User-initiated RBAC writes carry the authenticated user's ID as the event actor. Synthetic `system` actors are reserved for bootstrap, seeding, migrations, and other non-user maintenance.
- Losing effective room visibility through membership, room-group layout, or
  RBAC removes inaccessible notification occurrences. A durable visibility
  boundary prevents activity queued before that loss from becoming visible if
  access is quickly regained.

## Design Decisions

### 1. Flat, single-tier role layout

**Decision:** One server-wide role layer. No separate "instance roles" vs "space roles".
**Why:** The earlier two-tier split duplicated concepts and made permission resolution unpredictable. Collapsing into one tier with per-room-group / per-room overrides gives equivalent flexibility with one mental model. See ADR-027 and ADR-030.
**Tradeoff:** Operators who liked per-space role ownership now configure that through room-group overrides instead.

### 2. Named subjects with an `everyone` baseline

**Decision:** For non-owner human users, select the nearest room/group/server decision independently for the direct user and every explicitly assigned named role. Denies win across those decisions. Select `everyone`'s nearest decision as the scoped baseline; a direct-user or named-role allow overrides an `everyone` deny only at the same or a nearer scope. If nothing applies, the result is denied at the API boundary. Bots instead use only explicit direct-user allows, further bounded by their owner's current effective permissions.
**Why:** Operators can express an allowlist by denying the `everyone` baseline and granting a named role, while a named restriction role such as `suspended` still reliably denies. Role position remains irrelevant to authorization. See ADR-052.
**Tradeoff:** An `everyone` deny can be overridden deliberately at its own scope or a nearer one. A restriction role's deny beats other subjects' grants, but a nearer allow configured on that same role replaces its broader deny. Direct-user decisions follow the same nearest-scope rule. ADR-052 records the compatibility audit.

### 3. Three permission scopes (server / group / room)

**Decision:** For each subject, room checks use the nearest decision at room, group, or server scope. Server-scope message and room permissions act as broad defaults; room/group decisions are local overrides for that same subject. Fresh dev/bootstrap servers grant ordinary member capabilities such as `room.list`, `room.join`, `message.read`, `message.post`, `message.post-in-thread`, `message.attach`, `message.react`, and `message.echo` to `everyone` at server scope. The effective `message.read` allow includes `message.read.interactions`; bootstrap does not store a second grant. Fresh servers do not grant `room.create` to `everyone`. Admins get explicit server-tier administrative and `room.*` defaults plus `message.manage`, while ordinary content participation continues to come from `everyone`. Moderators get server-tier `message.manage` and `room.ban-member`.
**Why:** Operators want both "system-wide policy" and "this one channel works differently" without modelling separate role systems. See ADR-031 and ADR-052.
**Tradeoff:** Scope precedence is per subject, not global: one role's room allow does not erase a different named role's deny.

### 4. Owners are effective-owner overrides

**Decision:** Owners are always granted all permissions. Owner role permission rows are not seeded on fresh servers and are not editable through the RBAC UI/API.
**Why:** Instance owners must not be able to lock themselves out through unusual role or per-user permission configuration. See ADR-040.
**Tradeoff:** RBAC cannot be used to restrict owners, and owner permissions appear as virtual read-only allows rather than stored permission decisions. Restricting owner access requires changing ownership configuration or account state.

### 5. Config-designated owners converge on the durable role

**Decision:** `owners.emails` is materialized as durable `owner` role assignments. Existing verified matches are repaired at boot; a new matching verification is processed by a retryable durable worker and waits for that source fact before returning. Permission checks use only the durable role, and the role cannot be revoked while the matching verified email remains configured.
**Why:** One durable representation keeps live authorization, event-time visibility, and recovery behavior consistent. A transient role append failure remains pending for redelivery instead of creating a live-only owner that notification cleanup cannot recognize.
**Tradeoff:** A transient materialization failure can delay completion of email verification. Removing an email from `owners.emails` does not automatically revoke an already materialized owner role, because the server cannot distinguish config-created assignments from manual ones; operators may revoke it after updating configuration.

### 6. Target-user mutations are permission-gated and role assignment is bounded

**Decision:** Mutations that target another user require concrete permissions, not actor-vs-target rank checks. Role assignment uses `role.assign`, but a non-owner may assign only roles whose explicit allows they themselves effectively hold at each exact scope. Revocation is also bounded by every explicit allow and deny on the role, because removing a deny can restore authority. The `owner` role remains owner-only; `admin` has no implicit authority outside its explicit permissions. Account lifecycle and recovery operations use `user.manage-accounts`; direct user permission overrides use `user.manage-permissions`; room bans use `room.ban-member`.
**Why:** Concrete permissions are easier to audit and explain than a role-rank hierarchy, while bounding `role.assign` prevents delegated role managers from granting authority they do not possess or removing restrictions they cannot control.
**Tradeoff:** A delegated assigner may need the target role's underlying permissions even when they only administer membership. Owners remain the recovery path, and old replicas can enforce the earlier unbounded rule during a rolling upgrade until they are replaced.

### 7. RBAC state is event-sourced

**Decision:** Role definitions, role order, assignments, and explicit permission decisions are durable events, with reads served from an in-memory RBAC projection.
**Why:** This aligns RBAC with Chatto's current event-sourced architecture and makes authorization reads rebuildable from the deployment event log. See ADR-033 and ADR-035.
**Tradeoff:** Writes must append events and wait for local projection catch-up before returning, so mutation paths need optimistic concurrency handling instead of direct state writes. Changes that can affect authority advance a narrow durable authorization fence. Message posts and edits check that fence without advancing it, so a concurrent classified authorization change reruns their complete decision without coupling unrelated message traffic together. Reactions and message retractions deliberately use request-time authorization plus room OCC, so a cross-aggregate revocation is eventually consistent for an already in-flight mutation.

User-triggered RBAC events are audit facts as well as state facts, so their event envelope actor is the user who performed the operation. Core APIs still accept `SystemActorID` for trusted non-user paths such as bootstrapping default roles and permissions.

### 8. Permission-decision events carry typed scope and subject

**Decision:** Permission grant/deny/clear events store `scope` as `{kind, id}` (`SERVER`, `GROUP`, `ROOM`) and `subject` as `{kind, id}` (`ROLE`, `USER`).
**Why:** The old flattened fields made role/user permission subjects indistinguishable and relied on string conventions for scope. The typed shape freezes the domain model before beta and prevents future role IDs from colliding with user IDs.
**Tradeoff:** Event constructors do a little more validation, and compatibility readers for older persisted event shapes have to infer subject kind from legacy wire fields.

### 9. Defaults are one-time initialization, not startup policy

**Decision:** Apply the current server default set only when the durable RBAC stream is empty. New groups and ordinary rooms store no default decisions. Commit a channel room and any exceptional default decisions in one atomic EVT batch: fresh announcements rooms deny `message.post` to `everyone` and allow it for `admin`. Do not inspect, copy, reset, or reconcile existing permission state during startup.
**Why:** Absence is a meaningful RBAC state. Reapplying code defaults on every startup makes an operator's explicit clear indistinguishable from incomplete bootstrap state.
**Tradeoff:** Adding a new code default does not grant it to existing servers or rooms automatically. Older replicas in a rolling deployment still use their historical non-atomic room-creation path until they are replaced.

### 10. Permission names define inclusion

**Decision:** Use `<domain>.<capability>[.<narrower-capability>...]` for new
permission identifiers. Use a hyphen only inside one component. Prefer no more
than three components unless the product needs a deeper capability tree. Each
registered dotted prefix includes its descendants. The resolver follows these
ancestors transitively. `message.read` therefore includes
`message.read.interactions`. The child does not include the parent. A child
deny cannot restrict an effective parent allow, and a parent deny cannot
restrict a separate child allow. Inclusion changes effective authorization
only and does not store an additional grant. Catalog validation rejects a
nested permission without its immediate parent and parent-child pairs with
different categories or scopes.
**Why:** The three-component name shows that interaction reads are a narrower
part of message reads. One naming rule keeps authorization, explanations,
delegation limits, and the admin UI consistent without separate relationship
metadata.
**Tradeoff:** A permission name is an authorization contract. A capability
that does not belong below an existing permission must use a different name.

## Permissions

The full permission catalog is in `cli/internal/core/permission.go`. Key permissions that gate RBAC management itself:

- `role.manage` — configure role definitions and the permissions attached to them.
- `role.assign` — assign or revoke roles, bounded for non-owners by the target role's explicit scoped permission decisions.
- `user.manage-accounts` — create users, edit account identity, reset passwords, attach verified emails, and clear login cooldowns.
- `user.manage-permissions` — edit direct per-user permission overrides.
- `admin.view-users`, `admin.view-audit` — gate specific admin UI sub-views; admin UI entry is derived from concrete capabilities rather than a standalone `admin.access` permission. System diagnostics are owner-only and exposed through a viewer capability, not through grantable RBAC.
- `message.read` — read message content and message-specific metadata in
  channel rooms. Fresh servers grant this to `everyone` at server scope.
  Existing servers are not backfilled or reconciled, so operators must add any
  wanted grants during upgrade. DM membership authorizes DM reads without this
  permission.
- `message.read.interactions` — read only channel-room threads that the account
  started or where another account directly mentioned it. A relationship gives
  access to the complete thread. An effective `message.read` allow includes
  this permission. Fresh servers store only the `message.read` grant for
  `everyone`. Existing servers are not backfilled or reconciled. DM membership
  authorizes DM reads without this permission.
- `message.post` — post root messages in rooms and let human users start DMs.
  Bot accounts cannot start DMs. Fresh servers grant this permission to
  `everyone` at server scope. Fresh announcement rooms replace that baseline
  with a room-level `everyone` deny and a room-level `admin` allow. Moderators
  and other named roles need their own room-level posting grant.
- `message.attach` — attach files to new messages. Fresh servers grant this to `everyone` at server scope; existing servers are not automatically backfilled after upgrade, so operators may need to grant it manually if uploads should remain enabled.
- `room.manage` — edit/configure/delete channel rooms.
- `room.ban-member` — ban members from channel rooms. DM membership is not managed through this permission.

## Related

- **ADRs:** ADR-027 (instance/space consolidation), ADR-030 (space tier retirement), ADR-031 (room-group-centric ACL), ADR-033 (event-sourced state), ADR-035 (per-aggregate migration), ADR-037 (DM access via membership), ADR-040 (permission-only RBAC with owner override and name-derived permission inclusion), ADR-042 (protobuf-first public API), ADR-044 (ConnectRPC service conventions), ADR-052 (subject-specific RBAC with an everyone baseline), ADR-076 (notification occurrences), ADR-077 (persistent notification list), ADR-080 (explicit message-read permissions), ADR-082 (derived thread interactions)
- **FDRs:** Every FDR that mentions a permission depends on this one; see also FDR-012 (Notifications), FDR-038 (Bot Accounts), and FDR-039 (Message Access & Interactions).
