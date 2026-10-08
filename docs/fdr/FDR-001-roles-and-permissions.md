# FDR-001: Roles & Permissions (RBAC)

**Status:** Active
**Last reviewed:** 2026-10-08

## Overview

Chatto controls who can do what through role-based access control. Every authenticated human user holds one or more roles; each role, `everyone` included, only grants specific permissions; only single users can also deny them. Settings apply at server, room-group, and room scope, giving operators fine-grained control without inventing parallel role systems. Bot accounts are the deliberate exception: they use an explicit direct-permission allowlist bounded by their human owner's current authority (FDR-038).

Role metadata and member role lists update through realtime events. Display
changes do not reload the full client. Permission changes refresh affected data
in place for every user, including non-owners. The server shell and permitted
pages stay visible and mounted, so filters, drafts, and scroll positions remain
intact. Input is blocked during the authority check. Revoked access removes
the affected data or page; failed checks clear only the data they could not
reauthorize. Older reads and mutation results cannot restore that data.
Unrelated users keep their data.
A successful role creation or deletion can
still navigate after this cleanup, provided the user has not left the page or
changed session. This separates privacy cleanup from request completion
(ADR-062).

## Account Room Membership

Role and account permission matrices show only channels that are not archived.
Server, DM, and group columns remain available, including empty groups.
Archiving preserves permissions and membership. Unarchiving restores the channel
column with its existing permissions on the next matrix fetch.

The account permission matrix has a **Joined** row for human and bot accounts.
A confirmation dialog explains that the membership change takes effect
immediately after confirmation. Cancel leaves membership unchanged.
Configured permission grants stay unchanged.
`user.manage-accounts` or `room.manage` for the room can add an account without
its `room.join` permission. This lets a room manager invite a user who cannot
join independently. These permissions also authorize removal after join
permission is lost. Archived channels have no column in this matrix.

Bot owners and human bot managers can manage their bots without `room.manage`.
Without an account or room management override, adding a bot requires its
effective `room.join`, including its owner's ceiling. Suspensions and archived rooms
prevent adding. Universal membership remains automatic; DMs are excluded.
The event log records the acting manager and target account for each change,
including management overrides. Membership does not grant message permissions.
See [FDR-038](FDR-038-bot-accounts.md).

## Permission Matrices

Each matrix cell shows the setting that the subject stores and the result
that the subject gets. The server calculates the result with the same resolver
as authorization (ADR-116), so the cell and the permission check always agree.

- A role cell shows what a member gets who holds only that role, together
  with the allows of `everyone`.
- An account cell shows what that account gets. When the account has privileged
  mode available, the cell also tells which permissions it gets only in
  privileged mode. A room ban shows `room.join` as denied.
- A role cell, `everyone` included, changes between allow and no setting.
  No role cell offers deny. Account cells can also deny.
- The server tells which cells the viewer can change. A cell for a permission
  that the viewer does not have at that scope is read-only, and its title
  tells why. Owners see no locked cells.
- The server permissions page warns that its settings apply to every room and
  room group.

## Access Summary

The settings pages of a channel room and of a room group show who can find and
join the room, or the rooms of the group:

- everyone can find and join it;
- everyone can join it, but it does not show in the room list for everyone;
- only members of the listed roles can join it;
- only owners and the members that an operator adds can open it. This
  statement is a warning, and it tells how to open the room.

`AdminPermissionService.GetAccessSummary` supplies the summary. It resolves
`room.list` and `room.join` for `everyone` and for a member who holds only one
named role, with the same resolver as authorization. It leaves out settings on
single users. It needs `role.manage`, or `room.manage` at the room or group.
Explicit room members, and owners in privileged mode, have access whatever the
summary says.

## Permission Help

Each row of a role or account permission matrix has an information button after
the permission name. The button opens a dialog. In narrow touch windows, the
dialog opens as a bottom sheet, so touch users can read the help too. The dialog
shows:

- the short description and a longer explanation of the permission;
- the category and the scopes where the permission can be set;
- whether the permission needs privileged mode;
- the permissions that it includes, and the permissions that include it.

A related permission is a button that shows the help for that permission. The
**Joined** row has its own dialog that explains membership. A permission from a
newer server shows only its identifier and category.

The frontend copies scopes, inclusions, and privileged-mode requirements from
the backend permission catalog. Update both catalogs together.

## Behavior

- Role and account permission reads use bounded scope pages or an exact scope
  filter. Each scope includes its applicable decisions, with inheritance from
  broader scopes even when those scopes are outside the page. The editors load
  more columns at the horizontal scroll edge. This bounds each calculation
  while allowing large servers to expose their full configuration.

- Role details and assigned members are separate reads. Member lists load in
  bounded pages so large roles do not require every user profile at once.
  Roster access requires `role.assign`, not access to the full administrative
  user directory. Only explicit assignments appear; `everyone` has no roster.
  Pages reflect current assignments, so concurrent changes can shift offsets.

- Every authenticated human user belongs to the implicit `everyone` role and may additionally hold one or more named roles. Bots inherit neither `everyone` nor named-role permissions.
- The system roles are `owner`, `admin`, `moderator`, `everyone`. Role order is the administrative rank (Design Decision 11). It never changes whether a permission is allowed.
- A role grants named permissions like `message.post`, `room.create`, `admin.view-users`.
- A permission identifier is an opaque, stable value. Current identifiers use
  punctuation to help developers recognize them, but punctuation does not
  define authority. The permission catalog defines inclusion explicitly.
- Permission grants and denies can be configured at Server, Direct messages,
  Room group, and Room scope. Channel checks use Room, Room group, then Server.
  DM checks use Direct messages, then Server. Roles, `everyone` included, can
  only grant; single users can also deny (Design Decision 2). A Server-scope
  allow reaches every room.
- Roles are the normal way to give permissions. Direct per-user decisions are
  for rare exceptions: the admin UI does not show them on role pages, and they
  give no rank (Design Decision 11). Documentation recommends roles first.
- Permissions gate capabilities and channel-room message access. Channel-room
  membership is necessary for message reads. `message.read` supplies broad
  read authority and includes `message.read-interactions`, which
  supplies authority for related threads only. The same read rules apply in
  DMs after the membership check. `message.post` separately
  gates root-message posting and permits human users to start DMs. Bot accounts
  cannot start DMs regardless of their permissions.
- Role managers create, edit, and order roles on the **Roles** page. Each role has General and Permissions tabs, and a Members tab for people who may assign roles. `everyone` has no Members tab. The name `new` is reserved for the create page. Earlier role addresses under the Permissions page redirect to it.
- Role managers change the role order by dragging roles on the **Roles** page. `owner` stays at the top and `everyone` at the bottom; `admin`, `moderator`, and custom roles share one order. A new role starts lowest. A holder of `role.manage` can move every role except `owner` and `everyone`, also above their own highest role. Each drag saves one move: the role goes directly above another role, or lowest. A role that someone else creates or deletes at the same time does not make the move fail. There is no fixed limit on the number of roles.
- An account ranks at its highest role. An account without roles ranks with `everyone`. Owners rank above every role.
- A non-owner can act on another account only when they rank strictly above it. This applies to password, profile, avatar, login cooldown, deletion, roles, direct permissions, room removal, suspension lifts, membership removal, and bot management. Message moderation and adding a member to a room do not depend on rank.
- Custom role display names are limited to 80 bytes; descriptions are limited to 500 bytes.
- Owners are always entitled to all permissions. The owner override becomes effective only while the human session has privileged mode active. Without it, an owner resolves through direct grants, other roles, and `everyone` like any other user. An effective owner has the durable `owner` role; verified users listed in `owners.emails` in `chatto.toml` are materialized into that role at boot or through retryable durable work after verification.
- `admin` and every other non-owner role confer only their explicit permission decisions; they have no role-name-based authority.
- Owner permissions are virtual rather than persisted defaults: fresh servers do not seed editable owner permission rows, and the admin UI shows owner permissions as read-only green checks.
- RBAC editor and inspection APIs are exposed through ConnectRPC admin services. Admin entry is authenticated, and individual operations keep narrower gates such as `role.manage`, `role.assign`, `user.manage-accounts`, `user.manage-permissions`, or `room.manage`.
- Delegated role assignment is bounded by the assigner's own authority and rank. A non-owner may assign a role only when it ranks below them. They must also effectively possess every permission that the role explicitly allows at the same scope. Revoking the role needs the same permissions. Only an effective owner may assign or revoke the `owner` role.
- Permission editing is bounded by the editor's own authority and rank. To
  set or clear one role decision, or to set, deny, or clear one direct-user
  decision, a non-owner must
  effectively have that permission at the decision's scope. A holder of
  `role.manage` may change every role except `owner`; other editors, such as
  room and room-group managers, need the role to rank below them. A user must
  rank below the editor unless it is their own account. `everyone` ranks
  below every account. To delete a role, a non-owner needs `role.manage` and
  every permission that the role allows. Bot decisions keep the bot
  rules in FDR-038.
- Default permissions are creation-time state: fresh server defaults are seeded only into an empty RBAC stream, and the seeded rooms' defaults are committed atomically with room creation. New servers, rooms, and room groups start closed (Design Decision 9). Startup does not backfill missing or cleared decisions, except for the one-time upgrade grants in Design Decision 9.
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

### 2. Roles only grant; a deny on the user decides

**Decision:** For non-owner human users, an allow of an including permission allows the included permission. Otherwise, a deny as the user's own nearest room/group/server setting (or Direct messages/server setting) decides. Otherwise, any allow of the user, of a role, or of `everyone` at any applicable scope allows. If nothing applies, the result is denied at the API boundary. All roles, `everyone` included, can only grant: the API rejects every role deny with `INVALID_ARGUMENT`, and stored role denies have no effect. Bots instead use only explicit direct-user allows, further bounded by their owner's current RBAC entitlement.
**Why:** Two rules explain every result: a user deny, else any allow. Giving a role never removes access. Scope order matters only for the user's own settings. Operators express a private room by omission: they do not allow room access for `everyone` at a scope that reaches the room, and they allow it for a role at the room or its room group. Role position does not affect resolution; it is only the administrative rank (Design Decision 11). See ADR-116.
**Tradeoff:** A Server-scope allow, of any subject, reaches every room, so a private room needs closed Server-scope defaults (Design Decision 9). Restriction roles such as `suspended` no longer work; until account suspension exists, operators deny permissions on the user. On upgrade, every role deny, `everyone` denies included, stops having an effect, so a room that was private only through an `everyone` deny opens to everyone who has the Server-scope allows. There is no migration. The server logs a warning at startup with the number of ignored role denies and the IDs of the affected rooms and room groups.

### 3. Four permission scopes

**Decision:** For each subject, channel checks use the nearest decision at Room,
Room group, or Server scope. DM checks use the nearest decision at Direct
messages or Server scope. All `message.*` permissions apply to Direct messages.
No `room.*` permission applies there. Fresh servers allow the DM message and
call permissions for `everyone` at Direct messages scope, so these allows do
not reach rooms. Bots need an explicit
direct-user allow at the applicable scope. The effective `message.read` allow
includes `message.read-interactions`; bootstrap does not store a second grant.
**Why:** Operators need system-wide defaults, channel overrides, and a DM-only
policy for integrations without one permission object for each DM. See ADR-031,
ADR-052, and ADR-091.
**Tradeoff:** Scope precedence applies only to the user's own settings. Between subjects, any applicable allow counts, so a broad allow cannot be narrowed for a role, only for single users.

### 4. Owners are effective-owner overrides

**Decision:** Owners are always entitled to all permissions. Owner role permission rows are not seeded on fresh servers and are not editable through the RBAC UI/API. Privileged mode gates the complete owner override at request time (ADR-105).
**Why:** Instance owners must not be able to lock themselves out through unusual role or per-user permission configuration. Activation depends only on entitlement, so the gate does not lock owners out. See ADR-040.
**Tradeoff:** RBAC cannot be used to restrict owners, and owner permissions appear as virtual read-only allows rather than stored permission decisions. Restricting owner access requires changing ownership configuration or account state.

### 5. Config-designated owners converge on the durable role

**Decision:** `owners.emails` is materialized as durable `owner` role assignments. Existing verified matches are repaired at boot; a new matching verification is processed by a retryable durable worker and waits for that source fact before returning. Permission checks use only the durable role, and the role cannot be revoked while the matching verified email remains configured.
**Why:** One durable representation keeps live authorization, current
notification visibility, and recovery behavior consistent. A transient role
append failure remains pending for redelivery instead of creating a live-only
owner that notification cleanup cannot recognize.
**Tradeoff:** A transient materialization failure can delay completion of email verification. Removing an email from `owners.emails` does not automatically revoke an already materialized owner role, because the server cannot distinguish config-created assignments from manual ones; operators may revoke it after updating configuration.

### 6. Target-user mutations need a permission, a higher rank, and bounded authority

**Decision:** Mutations that target another account require a concrete permission and a higher rank (Design Decision 11). Role assignment uses `role.assign`. A non-owner may assign only roles that rank below them. They must effectively hold each explicit allow of the role at its exact scope. Revocation needs the same permissions. Permission editing uses the same bound. A non-owner may set or clear a role decision, or set, deny, or clear a direct-user decision, only for a permission that they effectively hold at that scope. To delete a role, they must hold every permission that the role allows. The `owner` role remains owner-only; `admin` has no implicit authority outside its explicit permissions. Account lifecycle and recovery operations use `user.manage-accounts`; direct user permission overrides use `user.manage-permissions`; moderated room removal uses `room.remove-member`.
**Why:** Without the bounds, `role.manage` or `user.manage-permissions` alone would let a holder grant themselves every permission, which would also make the `role.assign` bound ineffective. The bound covers denies and clears because removing a restriction can restore authority. The rank protects accounts at or above the actor, which an authority bound alone cannot do. See ADR-115.
**Tradeoff:** A delegated assigner or editor may need the underlying permissions and a role above the accounts that they manage. An actor can still give their authority to a second account below them; the event log records who made each change. Owners remain the recovery path, and old replicas can enforce the earlier unbounded rules during a rolling upgrade until they are replaced.

### 7. RBAC state is event-sourced

**Decision:** Role definitions, role order, assignments, and explicit permission decisions are durable events, with reads served from an in-memory RBAC projection.
**Why:** This aligns RBAC with Chatto's current event-sourced architecture and makes authorization reads rebuildable from the deployment event log. See ADR-033 and ADR-035.
**Tradeoff:** Writes must append events and wait for local projection catch-up before returning, so mutation paths need optimistic concurrency handling instead of direct state writes. Authorization-sensitive commands validate stable request-time RBAC, room-group, and user inputs. Domain events use OCC on the aggregate or filter that owns the command invariant. A cross-aggregate revocation that occurs after the final authorization validation can overlap an already-authorized command.

User-triggered RBAC events are audit facts as well as state facts, so their event envelope actor is the user who performed the operation. Core APIs still accept `SystemActorID` for trusted non-user paths such as bootstrapping default roles and permissions.

### 8. Permission-decision events carry typed scope and subject

**Decision:** Permission grant/deny/clear events store `scope` as `{kind, id}` (`SERVER`, `GROUP`, `ROOM`) and `subject` as `{kind, id}` (`ROLE`, `USER`).
**Why:** The old flattened fields made role/user permission subjects indistinguishable and relied on string conventions for scope. The typed shape freezes the domain model before beta and prevents future role IDs from colliding with user IDs.
**Tradeoff:** Event constructors do a little more validation, and compatibility readers for older persisted event shapes have to infer subject kind from legacy wire fields.

### 9. Defaults are one-time initialization, not startup policy

**Decision:** Apply the current server default set only when the durable RBAC stream is empty. New servers start closed, because a Server-scope allow reaches every room:

- `everyone` gets only `user.delete-self` at Server scope.
- `everyone` gets `message.read`, `message.post`, `message.attach`,
  `message.react`, `message.echo`, and the `call.*` permissions at Direct
  messages scope.
- `admin` gets administrative permissions at Server scope, but not `room.list`
  or `room.join`. Admins reach rooms like other members, or with `room.manage`
  in privileged mode. `moderator` gets `message.manage` and
  `room.remove-member`.
- The seeded `#general` room allows `everyone` `room.list`, `room.join`,
  `message.read`, `message.post`, `message.attach`, `message.react`,
  `message.echo`, and the `call.*` permissions at Room scope.
- The seeded universal `#announcements` room allows `everyone` `room.list`,
  `room.join`, `message.read`, `message.react`, and `message.post-in-thread`
  at Room scope, and allows `admin` `message.post` there.
- The seeded `Lobby` room group has no `everyone` allows.

Commit a seeded room and its default decisions in one atomic EVT batch. Every
other new room and room group stores no default decision, so it starts closed
until an operator opens it. Upgraded servers keep their stored settings. Do not
reset existing permission state during startup. A new permission that
gates an existing capability gets a one-time upgrade grant (ADR-113):

- Initialize each missing server-level `everyone` call permission once. Any
  historical grant, deny, or clear of that decision at Server or Direct
  messages scope prevents it. New servers seed the Direct messages decisions,
  so they get no Server-scope call grant.
- Upgrade a 0.4 log once as a set. Allow `message.read` for `everyone`, and
  copy each current `room.ban-member` decision to `room.remove-member`. Any
  historical decision for a permission introduced in 0.5 prevents the whole
  set. Capabilities that are new in 0.5, such as bots and invite links, get
  no upgrade grant.

These grants preserve existing capabilities without undoing an operator's
later decision.
**Why:** Absence is a meaningful RBAC state. Reapplying code defaults on every startup makes an operator's explicit clear indistinguishable from incomplete bootstrap state.
**Tradeoff:** Operators must open each new room and room group; the access summary warns about closed rooms. Upgraded servers keep their Server-scope `everyone` allows, so their rooms stay open unless the operator removes them. Apart from the explicit upgrade grants above, adding a new code default does not grant it to existing servers or rooms automatically. Changes of meaning, such as `room.manage` for room-group managers, remain manual review items. Older replicas in a rolling deployment still use their historical non-atomic room-creation path until they are replaced.

### 10. The permission catalog defines inclusion

**Decision:** Treat permission identifiers as opaque, stable values. Use
`domain.capability` or `domain.capability-with-qualifier` for current names.
Define each inclusion directly in the permission catalog. The catalog states
that `message.read` includes `message.read-interactions`. The narrow permission
does not include the broad permission. A narrow deny cannot restrict an
effective broad allow, and a broad deny cannot restrict a separate narrow
allow. Inclusion changes effective authorization only and does not store an
additional grant. Catalog validation rejects unknown targets, self-inclusion,
and relationships with incompatible categories or scopes. Public APIs and EVT
facts use the stable identifier.
**Why:** Authorization must not change because a developer chose punctuation
for a new identifier. Explicit metadata keeps inclusion reviewable while stable
identifiers preserve persisted facts and integrations.
**Tradeoff:** The backend and frontend catalogs must keep their explicit
relationships in sync. Tests cover the current relationship.

### 11. Role order is an administrative rank

**Decision:** Role order ranks accounts for administration only. `owner` is fixed at the top and `everyone` at the bottom; the other roles share one order, and a new role starts lowest. An account ranks at its highest role; owners outrank every role, and `everyone` ranks below every account. A non-owner may act only on accounts that rank strictly below them and assign only roles that rank strictly below their highest role. A holder of `role.manage` may edit, delete, and move every role except `owner`; other editors of role decisions, such as room managers, may edit only roles below their highest role. Direct user decisions do not affect rank. Bots hold no roles and rank like their owner.
**Why:** Delegated administration needs to protect accounts above the actor, and features need a user's highest role. Discord, Matrix, and Zulip use the same model. Keeping rank out of permission resolution keeps the resolution rules (Design Decision 2) intact. See ADR-115.
**Tradeoff:** Two accounts with the same highest role cannot act on each other. A user with administrative direct permissions but no matching role ranks low. A holder of `role.manage` can restrict higher accounts by clearing allows of `everyone` or of roles that those accounts hold, within the permissions that they hold. A holder of `role.manage` can change every role except `owner` and move their own role higher, so give `role.manage` only to administrators. Older replicas ignore role moves and lowest placement during a rolling upgrade.

## Permissions

- `call.start`, `call.join`, `call.voice`, `call.camera`, and `call.screenshare`
  separate starting, joining, and publishing media. Room membership remains
  mandatory. These permissions apply to rooms and the shared Direct messages
  scope. See [FDR-016](FDR-016-voice-calls.md).

The full permission catalog is in `cli/internal/core/permission.go`. Key permissions that gate RBAC management itself:

- `role.manage` — configure role definitions and the permissions attached to them, bounded for non-owners by their own scoped authority.
- `role.assign` — assign or revoke roles, bounded for non-owners by the target role's explicit scoped permission decisions.
- `user.manage-accounts` — create users, edit account identity, reset passwords, attach verified emails, clear login cooldowns, and bypass the holder's own login cooldown.
- `user.manage-permissions` — edit direct per-user permission overrides of other users, bounded for non-owners by their own scoped authority.
- `admin.view-users`, `admin.view-audit` — gate specific admin UI sub-views; admin UI entry is derived from concrete capabilities rather than a standalone `admin.access` permission. System diagnostics are owner-only and exposed through a viewer capability, not through grantable RBAC.
- `message.read` — read message content and message-specific metadata in
  channel rooms and DMs. Fresh servers grant this to `everyone` at Direct
  messages scope and in the seeded rooms. Upgraded 0.4 servers receive a
  Server-scope grant for `everyone` once. Startup does not reconcile it after
  that.
- `message.read-interactions` — read only threads that the account
  started or where another account directly mentioned it. A relationship gives
  access to the complete thread. An effective `message.read` allow includes
  this permission. Fresh and upgraded servers store only the `message.read`
  grants for `everyone`.
- `message.post` — post root messages and thread replies, and let human users start DMs.
  Includes `message.post-in-thread` and `message.post-in-interactions`. A narrow
  deny cannot restrict an effective broad allow.
  Bot accounts cannot start DMs. Fresh servers grant this permission to
  `everyone` at Direct messages scope and in the seeded `#general` room. In
  the seeded `#announcements` room, only `admin` has a room-level allow.
  Moderators and other named roles need their own posting grant there.
- `message.post-in-thread` — reply in any readable thread where room policy permits it.
- `message.post-in-interactions` — reply only in readable threads with an interaction relationship.
- `message.attach` — attach files to new messages. Fresh servers grant this to `everyone` at Direct messages scope and in the seeded `#general` room; existing servers are not automatically backfilled after upgrade, so operators may need to grant it manually if uploads should remain enabled.
- `room.manage` — edit/configure/delete channel rooms.
- `room.remove-member` — remove current channel-room members with an optional suspension. DM membership is not managed through this permission. Upgraded 0.4 servers copy each `room.ban-member` decision to this permission once.

## Related

- **ADRs:** ADR-027 (instance/space consolidation), ADR-030 (space tier retirement), ADR-031 (room-group-centric ACL), ADR-033 (event-sourced state), ADR-035 (per-aggregate migration), ADR-037 (DM access via membership), ADR-040 (permission-only RBAC with owner override and explicit catalog inclusion), ADR-042 (protobuf-first public API), ADR-044 (ConnectRPC service conventions), ADR-052 (subject-specific RBAC with an everyone baseline), ADR-076 (notification occurrences), ADR-077 (persistent notification list), ADR-080 (explicit message-read permissions), ADR-082 (derived thread interactions), ADR-087 (request-time authorization with aggregate OCC), ADR-096 (session-scoped privileged mode), ADR-105 (privileged mode gates the owner override), ADR-113 (one-time upgrade grants for new permissions), ADR-115 (role hierarchy for administration), ADR-116 (roles only grant permissions)
- **FDRs:** Every FDR that mentions a permission depends on this one; see also FDR-012 (Notifications), FDR-038 (Bot Accounts), FDR-039 (Message Access & Interactions), and FDR-046 (Privileged Mode).
