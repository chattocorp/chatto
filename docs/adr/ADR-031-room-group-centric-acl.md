# ADR-031: Room-Group-Centric ACL for Room-Scope Permissions

**Date:** 2026-05-13

## Context

The post-#330 RBAC model resolves room-scope permissions through a single hierarchy walker rooted in server-scope grants, with room-scope decisions overlaid on top via room-level allow/deny keys. The walker is uniform and tightened (see ADR-005, and the `hmans/rbac-review` work that closed self-grant escalation and dropped `admin.bypass`), but the underlying *shape* of the model produces several awkward edges:

- **Server-scope grants on `everyone` are global by default.** Room-scope perms (`message.post`, `room.join`, etc.) live on the `everyone` role at server scope and affect every room. Adjusting them globally is convenient but coarse: there is no granularity between "everyone everywhere" and "per-room override." For a multi-team server, the natural unit ("everyone on the engineering team, in engineering rooms") doesn't exist in the model.

- **No natural permission boundary for groups of rooms.** A planned **room groups** feature (which replaces the current collapsible UI groups, themselves an evolution of `RoomLayoutSection`) requires per-group access control — e.g., "Engineering" rooms accessible only to the `engineers` role. There is no container in the current model where such permissions could live. Layering room groups onto the existing model would mean stacking a second per-group tier on top of the existing server-→room overlay; better to make the group the primary container instead.

- **Implicit `everyone` constrained the original deny semantics.** The later
  ADR-040 resolver made an `everyone` deny catch moderators and admins too.
  ADR-052 replaces that combination rule and treats `everyone` as a scoped
  baseline.

Chatto was still early enough to reshape this model before room groups became
widely deployed. Existing permission facts remain valid; current defaults are
creation-time facts and are not reconciled or reset during startup.

A long design discussion considered alternatives — ReBAC/Zanzibar (overkill for chat's flat-ish structure), policy-as-code (incompatible with operator-configurable self-hosting), capability tokens (wrong fit for server-state-owns-everything chat). The model that best matches both the room-groups requirement and operators' actual mental model ("look at the room/category to know what's allowed there") is channel-centric ACLs as used by Discord and similar chat systems.

## Decision

Adopt a **channel-centric ACL** model for channel-room permissions with **room groups** as the shared local permission container. Three permission containers participate in nearest-scope inheritance:

| Container | Configures | Examples |
|---|---|---|
| **Server** | Server-only capabilities and broad defaults for room-capable permissions | `server.manage`, `role.manage`, `message.post`, `room.join`, `user.manage-accounts` |
| **Room group** | Room-scope permissions for every channel room in the group | `message.post`, `message.react`, `room.join`, `room.manage`, `message.manage`, `message.echo` |
| **Room** | Room-scope permissions, **overriding the room group on a per-(role, permission) basis** | Same as above; only the (role, permission) pairs explicitly overridden change from the group's value, the rest inherit |

Subjects are unchanged: **roles** and **users** (for direct overrides). Role
position controls display order, not authorization. Every authenticated user
implicitly carries `everyone`.

**DMs are out of scope for this ADR.** DM rooms are not part of any room group; their permission shape is captured separately in ADR-037. Room groups are a feature on top of channel rooms only.

This work evolves the existing `RoomLayout` / `RoomLayoutSection` storage (`proto/chatto/core/v1/models.proto`) — sections become groups. The atomic-OCC update pattern in `UpdateRoomLayout` and the live `RoomLayoutUpdatedEvent` are preserved; what changes is the section type's fields (gains `displayName`, `description`) and the disappearance of `unsorted_room_ids` (every channel room is now in a group).

### Membership and structural invariants

- **Every channel room belongs to exactly one group.** No nullable `groupID`, no "uncategorized" branch in the resolver. (DM rooms do not belong to a group.)
- **Room groups are operator-managed, not system-protected.** On first boot, one group named "Lobby" is seeded; the auto-created `announcements` and `general` channels go into it. The operator can rename, reorder, or delete this group like any other.
- **Room group deletion is rejected while rooms exist.** Operators must move all rooms out first. No "delete and reassign" cascade — the rejection is deliberate to avoid surprise.
- **Room creation requires a group.** When no room group is implied by UI context, the public room-creation API requires one explicitly. Lower-level bootstrap/import paths may still use the seed "Lobby" group while constructing first-boot state.
- **Room group membership is stored on the room record** (one `groupID` field per room).
- **Moving a room between groups requires `room.manage` in BOTH the source and target group.** The action changes the room's effective ACL overnight, so the caller must be authorized in both ends of the move.
- **Room groups are ordered.** Room group order, like room order within a group, is captured in the layout proto (same atomic-OCC pattern as today's `RoomLayout`).

### Resolution

For **server-scope** permissions: server decisions remain global defaults and
restrictions. ADR-052 combines direct-user and named-role decisions without
using role position as an authorization rank, then applies `everyone` as the
scoped baseline.

For **DM rooms**: room groups do not apply. Reading is membership-based, starting/sending DMs uses message permissions, and the `dmBoundaryDeniedPermissions` deny-list applies inside DM rooms for non-owners.

For **channel-room-scope** permissions in room R (belonging to group G):

1. For the direct user and each explicitly assigned named role, the resolver
   selects the nearest explicit decision at room R, group G, or server scope.
2. ADR-052 combines those subject decisions with deny-wins. If any named role
   or direct-user decision denies, the permission is denied; otherwise it keeps
   the most-specific allow.
3. The resolver selects the implicit `everyone` role's nearest decision as the
   scoped baseline. A named/direct allow overrides an `everyone` deny only at
   the same or a nearer scope. If nothing decides, the API boundary denies.

The earlier ADR text said "there is no cascade from server scope into
channel-room scope" and later described a first-explicit-decision walker. Both
were superseded by ADR-040's permission-only model and ADR-052's
subject-specific baseline model. The room-group
container decision remains active: room groups are still the operator-facing
place to configure permissions shared by a set of channel rooms.

**The announcements pattern uses a room-scoped deny** against
`everyone.message.post`. This blocks normal members while allowing a named role
with its own room-level posting grant. The deny is local and audit-visible
inside its room. Fresh announcements rooms pair that deny with a room-level
`admin.message.post` allow; moderators receive no announcement-specific grant.

### Moderation actions

Temporary user-targeted restrictions ("mute", "timeout", "suspend") build on the existing **user-level deny** primitive. The UI exposes verbs (Mute, Timeout, Suspend with duration), not raw permission editors. Underneath, each action writes a small fixed bundle of user-level denies (server-scope, group-scope, or room-scope) with a scheduled cleanup for expiry. No new resolver concept ("restrictive role" flag etc.) is required.

### Creation-time defaults and compatibility

- Fresh servers create a seed `Lobby` group. New groups store no permission
  decisions and inherit the server tier until an operator adds an override.
- Fresh server-role defaults are written only when the RBAC event history is
  empty. Startup never backfills a cleared or missing decision.
- Channel-room creation commits the room and its exceptional permission facts
  in one atomic EVT batch. Ordinary rooms store no default overrides.
- Fresh `announcements` rooms store an `everyone.message.post` deny and an
  `admin.message.post` allow. Existing rooms are not backfilled when defaults
  change.
- Historical permission events replay unchanged. No reset or copy-defaults
  workflow is part of the supported model.

## Consequences

### Easier

- **Per-team rooms come for free.** Define a room group, restrict it to a role, every channel room in the group inherits — including rooms added later. The headline feature this ADR exists to enable.
- **Bulk operator changes scope to a group.** "Adjust how members behave in the Engineering rooms" is one group-level edit, not a per-room sweep or a global server-wide change.
- **Trace output maps to operator containers.** "Set 'Rooms' grants `message.post` to `everyone`; room `announcements` overrides with deny" is exactly what the admin UI surfaces. The walker's path matches the UI's container tree.
- **Timeout/mute is uncontroversial.** User-level deny is the primitive; moderation actions are a thin product layer on top. No new resolver concept required, no tension with group-level grants.
- **Operator mental model matches reality.** "Open the group or the room to see what's allowed there" is true. Sets are the source of truth for their rooms unless a room explicitly overrides.

### More difficult

- **Local containment is deliberate.** A group/room `everyone` deny can contain
  less-specific administrative grants. Operators must add same-scope named
  allows for staff who should bypass that local baseline.
- **Defaults do not repair existing state.** New code defaults affect only new
  RBAC histories or newly created rooms, so operators must change existing
  deployments explicitly.
- **More EVT facts.** Each explicit group/room subject decision adds a durable
  RBAC event and projected decision, so local policy should remain intentional.
- **Room creation always needs a group.** Pre-change, a new room could be created with no group affiliation. Post-change, the API and UI must always pick a group. Drop in operator ergonomics is small but real.
- **Room-move requires two-group authorization.** Moving a room between groups needs `room.manage` in both source and target. UI must surface this clearly (preview affected users, confirmation step) and the public API surface needs to reflect both checks.

### Relationship to prior ADRs

- **Supersedes ADR-005 for channel-room containers.** ADR-040 removed role-rank
  authorization, and ADR-052 now governs subject combination at every scope.
  This ADR retains the server/group/room container model and nearest-scope walk.
- **Builds on ADR-044** (shared operation models for public API authorization). Public room/group operations enforce these checks in core operation models so ConnectRPC and future transports cannot drift.
- **Leaves DM room policy outside room groups.** DMs are not part of any room group; their membership-based read access, message-permission send gate, and hardcoded `dmBoundaryDeniedPermissions` list are covered by ADR-037. Room groups are a channel-rooms-only feature.
- **Compatible with ADR-037.** Removing the DM read permission does not change the group model because DM rooms never inherit group permissions.
- **Compatible with ADR-027 and ADR-030.** Server consolidation and the retirement of the space tier are preserved; this ADR introduces a *new* container (room group) below the server, not a return to two tiers.

### Out of scope for this ADR

- Custom system roles beyond owner/admin/moderator.
- Cross-group permission inheritance; groups independently inherit from the
  server and do not inherit from one another.
- Nested room groups (rooms belong to exactly one group; no group-of-groups).
- ReBAC / relationship-based resolution (revisit only if structural-document features appear).
- Restrictive-role flag for temporary punishment (user-level denies are the chosen primitive instead).
