# ADR-031: Room-Group-Centric ACL for Room-Scope Permissions

**Date:** 2026-05-13

## Context

The post-#330 RBAC model resolves room-scope permissions through a single hierarchy walker rooted in server-scope grants, with room-scope decisions overlaid on top via room-level allow/deny keys. The walker is uniform and tightened (see ADR-005, and the `hmans/rbac-review` work that closed self-grant escalation and dropped `admin.bypass`), but the underlying *shape* of the model produces several awkward edges:

- **Server-scope grants on `everyone` are global by default.** Room-scope perms (`message.post`, `room.join`, etc.) live on the `everyone` role at server scope and affect every room. Adjusting them globally is convenient but coarse: there is no granularity between "everyone everywhere" and "per-room override." For a multi-team server, the natural unit ("everyone on the engineering team, in engineering rooms") doesn't exist in the model.

- **No natural permission boundary for groups of rooms.** A planned **room groups** feature (which replaces the current collapsible UI groups, themselves an evolution of `RoomLayoutSection`) requires per-group access control — e.g., "Engineering" rooms accessible only to the `engineers` role. There is no container in the current model where such permissions could live. Layering room groups onto the existing model would mean stacking a second per-group tier on top of the existing server-→room overlay; better to make the group the primary container instead.

- **Implicit `everyone` constrains deny semantics.** Every authenticated user implicitly carries `everyone`, so any deny attached to `everyone` catches moderators and admins too. The hierarchy-wins rule is what makes the current model work — higher-rank grants override lower-rank denies — but this also rules out "deny-always-wins" semantics that would be useful for temporary-restriction roles (timeouts, mutes). This is unchanged by the new model; moderation actions are addressed below via user-level denies.

Chatto is at alpha. The three known production-shaped servers can absorb a `chatto reset rbac` on upgrade. This is a one-time opportunity to reshape the model before the room-groups feature lands rather than to layer over it.

A long design discussion considered alternatives — ReBAC/Zanzibar (overkill for chat's flat-ish structure), policy-as-code (incompatible with operator-configurable self-hosting), capability tokens (wrong fit for server-state-owns-everything chat). The model that best matches both the room-groups requirement and operators' actual mental model ("look at the room/category to know what's allowed there") is channel-centric ACLs as used by Discord and similar chat systems.

## Decision

Adopt a **channel-centric ACL** model for channel-room permissions with **room groups** as the primary permission container. Three permission containers, with explicit (no implicit) inheritance:

| Container | Configures | Examples |
|---|---|---|
| **Server** | Server-scope permissions only | `server.manage`, `role.manage`, `role.assign`, `admin.view-users`, `user.delete-any`, `user.delete-self` |
| **Room group** | Room-scope permissions for every channel room in the group | `message.post`, `message.react`, `room.join`, `room.manage`, `message.manage`, `message.echo` |
| **Room** | Room-scope permissions, **overriding the room group on a per-(role, permission) basis** | Same as above; only the (role, permission) pairs explicitly overridden change from the group's value, the rest inherit |

Subjects are unchanged: **roles** (with rank, RBAC-style) and **users** (for direct overrides). Every authenticated user implicitly carries `everyone`.

**DMs are out of scope for this ADR.** DM rooms are not part of any room group; their permission shape is captured separately in ADR-037. Room groups are a feature on top of channel rooms only.

This work evolves the existing `RoomLayout` / `RoomLayoutSection` storage (`proto/chatto/core/v1/models.proto`) — sections become groups. The atomic-OCC update pattern in `UpdateRoomLayout` and the live `RoomLayoutUpdatedEvent` are preserved; what changes is the section type's fields (gains `displayName`, `description`) and the disappearance of `unsorted_room_ids` (every channel room is now in a group).

### Membership and structural invariants

- **Every channel room belongs to exactly one group.** No nullable `groupID`, no "uncategorized" branch in the resolver. (DM rooms do not belong to a group.)
- **Room groups are operator-managed, not system-protected.** On first boot, one group named "Lobby" is seeded; the auto-created `announcements` and `general` channels go into it. The operator can rename, reorder, or delete this group like any other.
- **Room group deletion is rejected while rooms exist.** Operators must move all rooms out first. No "delete and reassign" cascade — the rejection is deliberate to avoid surprise.
- **Room creation requires a group.** When no room group is implied by UI context, the GraphQL API requires one explicitly. Lower-level bootstrap/import paths may still use the seed "Lobby" group while constructing first-boot state.
- **Room group membership is stored on the room record** (one `groupID` field per room).
- **Moving a room between groups requires `room.manage` in BOTH the source and target group.** The action changes the room's effective ACL overnight, so the caller must be authorized in both ends of the move.
- **Room groups are ordered.** Room group order, like room order within a group, is captured in the layout proto (same atomic-OCC pattern as today's `RoomLayout`).

### Resolution

For **server-scope** permissions: unchanged from current model. Standard hierarchy-wins RBAC walker over server-scope role grants, with user-level overrides outranking roles (Phase 1 of the current resolver).

For **DM rooms**: room groups do not apply. Reading is membership-based, starting/sending DMs uses message permissions, and the `dmBoundaryDeniedPermissions` deny-list still applies inside DM rooms.

For **channel-room-scope** permissions in room R (belonging to group G):

1. **User-level overrides**, in order: room R → group G → server (server only for permissions that carry both `ScopeServer` and `ScopeGroup` / `ScopeRoom`). First explicit decision wins.
2. **Role walk**, highest rank first. For each role:
   1. Room R's grant/deny for that role
   2. Group G's grant/deny for that role
   3. Server-scope grant/deny for that role (fallback, only for dual-scope perms)
3. **Default deny** if no decision was reached.

A small revision from the original ADR text: server scope acts as the **global default** for channel-room perms. The walker checks group state first, room state on top of that, and falls back to server only when neither tier emits a decision. This gives operators a single "global default" they can adjust once and have apply everywhere, while still letting per-group and per-room edits override locally. DMs (which aren't in any group) resolve at server scope only.

The earlier ADR text said "there is no cascade from server scope into channel-room scope." That was the initial intent; in practice it made DMs and operator-friendly defaults awkward, so we restored the cascade as the lowest-priority tier in the walk. Per-group config still wins over the server default — the ADR's headline goal ("groups are the natural permission container") is preserved; the walker just doesn't deny when nothing is configured at group or room scope.

Within the role walk, room-scope decisions override group-scope decisions *within the same role*. Across roles, hierarchy wins as today (higher rank's decision is examined first, lower-rank roles not consulted if a higher rank decided).

**The announcements pattern still uses a deny**, but now scoped to a room inside a group instead of overriding a server-scope grant. The group "Lobby" grants `message.post` to `everyone`; the `announcements` room inside it has a per-room deny for `everyone.message.post`. Moderators' grant comes through the group (no per-room override needed); the walker visits moderator first, finds the group's allow, and returns. The win over the previous model isn't "no denies" — it's that the deny is scoped, audit-visible inside its room, and doesn't compete with cross-room operator intent.

### Moderation actions

Temporary user-targeted restrictions ("mute", "timeout", "suspend") build on the existing **user-level deny** primitive, which outranks role grants. The UI exposes verbs (Mute, Timeout, Suspend with duration), not raw permission editors. Underneath, each action writes a small fixed bundle of user-level denies (server-scope, group-scope, or room-scope) with a scheduled cleanup for expiry. No new resolver concept ("restrictive role" flag etc.) is required.

### Migration

Existing servers reset RBAC on upgrade (`chatto reset rbac` already exists for related migrations). Specifically:

- A seed "Lobby" group is created.
- Existing `RoomLayoutSection`s migrate to `RoomGroup`s (id and ordering preserved; `name` becomes the group's `displayName`).
- Any rooms tracked in `unsorted_room_ids` are swept into the seed "Lobby" group.
- Groups are created with no explicit channel-room grants — the server-tier defaults cascade in via the resolver. Operators add per-group overrides only where they want to differ from the server-wide default. `SeedDefaultRoomGroupPermissions` remains available as an admin-tool affordance (a "Copy server defaults into this group" button) but no automatic path calls it.
- Server-scope perms migrate untouched.
- DM rooms and the `dmBoundaryDeniedPermissions` list are untouched; `dm.*` permissions are retired separately by ADR-037.

The three known production-shaped Chatto servers absorb this. Out-of-the-box behavior after migration matches today's defaults.

## Consequences

### Easier

- **Per-team rooms come for free.** Define a room group, restrict it to a role, every channel room in the group inherits — including rooms added later. The headline feature this ADR exists to enable.
- **Bulk operator changes scope to a group.** "Adjust how members behave in the Engineering rooms" is one group-level edit, not a per-room sweep or a global server-wide change.
- **Trace output maps to operator containers.** "Set 'Rooms' grants `message.post` to `everyone`; room `announcements` overrides with deny" is exactly what the admin UI surfaces. The walker's path matches the UI's container tree.
- **Timeout/mute is uncontroversial.** User-level deny is the primitive; moderation actions are a thin product layer on top. No new resolver concept required, no tension with group-level grants.
- **Operator mental model matches reality.** "Open the group or the room to see what's allowed there" is true. Sets are the source of truth for their rooms unless a room explicitly overrides.

### More difficult

- **Global tweaks require multi-group edits.** Today, changing a server-scope grant on `everyone` affects every room. After this change, the same effect requires editing each group (groups are independent — there is no cross-group inheritance). The admin UI must offer an "apply to all groups" affordance to make global tweaks ergonomic; under the hood it writes N keys.
- **More KV keys.** Each (group, role, perm) and (room, role, perm) override is its own key. Practical scale (low thousands) is comfortable for JetStream KV, but storage and listing costs grow linearly with groups × rooms.
- **One-time RBAC reset.** Existing servers need to migrate (`chatto reset rbac` or equivalent). Acceptable at alpha; a non-event for new deployments.
- **Room creation always needs a group.** Pre-change, a new room could be created with no group affiliation. Post-change, the API and UI must always pick a group. Drop in operator ergonomics is small but real.
- **Room-move requires two-group authorization.** Moving a room between groups needs `room.manage` in both source and target. UI must surface this clearly (preview affected users, confirmation step) and the GraphQL surface needs to reflect both checks.

### Relationship to prior ADRs

- **Supersedes ADR-005 for channel-room permissions only.** Hierarchy-wins RBAC still governs server-scope resolution; the room-scope cascade described in ADR-005 ("deny on `everyone` overridden by higher role's grant") is replaced by the room+group per-role walk. ADR-005's announcements example moves from "server-scope grant on everyone, room-scope deny on everyone" to "group-scope grant on everyone, room-scope deny on everyone" — same shape, just scoped to a group instead of cascading from the server.
- **Builds on ADR-004** (authorization at the API boundary). Core remains pure; GraphQL gates remain the enforcement layer.
- **Leaves DM room policy outside room groups.** DMs are not part of any room group; their membership-based read access, message-permission send gate, and hardcoded `dmBoundaryDeniedPermissions` list are covered by ADR-037. Room groups are a channel-rooms-only feature.
- **Compatible with ADR-037.** Removing the DM read permission does not change the group model because DM rooms never inherit group permissions.
- **Compatible with ADR-027 and ADR-030.** Server consolidation and the retirement of the space tier are preserved; this ADR introduces a *new* container (room group) below the server, not a return to two tiers.

### Out of scope for this ADR

- Custom system roles beyond owner/admin/moderator (rank is unchanged).
- Cross-group permission inheritance (groups are independent; this can be revisited if real demand emerges).
- Nested room groups (rooms belong to exactly one group; no group-of-groups).
- ReBAC / relationship-based resolution (revisit only if structural-document features appear).
- Restrictive-role flag for temporary punishment (user-level denies are the chosen primitive instead).
