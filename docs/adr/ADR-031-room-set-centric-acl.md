# ADR-031: Room-Set-Centric ACL for Room-Scope Permissions

**Date:** 2026-05-13

## Context

The post-#330 RBAC model resolves room-scope permissions through a single hierarchy walker rooted in server-scope grants, with room-scope decisions overlaid on top via room-level allow/deny keys. The walker is uniform and tightened (see ADR-005, and the `hmans/rbac-review` work that closed self-grant escalation and dropped `admin.bypass`), but the underlying *shape* of the model produces several awkward edges:

- **Server-scope grants on `everyone` are global by default.** Room-scope perms (`message.post`, `room.list`, etc.) live on the `everyone` role at server scope and affect every room. Adjusting them globally is convenient but coarse: there is no granularity between "everyone everywhere" and "per-room override." For a multi-team server, the natural unit ("everyone on the engineering team, in engineering rooms") doesn't exist in the model.

- **No natural permission boundary for groups of rooms.** A planned **room sets** feature (which replaces the current collapsible UI groups, themselves an evolution of `RoomLayoutSection`) requires per-group access control — e.g., "Engineering" rooms accessible only to the `engineers` role. There is no container in the current model where such permissions could live. Layering room sets onto the existing model would mean stacking a second per-group tier on top of the existing server-→room overlay; better to make the set the primary container instead.

- **Implicit `everyone` constrains deny semantics.** Every authenticated user implicitly carries `everyone`, so any deny attached to `everyone` catches moderators and admins too. The hierarchy-wins rule is what makes the current model work — higher-rank grants override lower-rank denies — but this also rules out "deny-always-wins" semantics that would be useful for temporary-restriction roles (timeouts, mutes). This is unchanged by the new model; moderation actions are addressed below via user-level denies.

Chatto is at alpha. The three known production-shaped servers can absorb a `chatto reset rbac` on upgrade. This is a one-time opportunity to reshape the model before the room-sets feature lands rather than to layer over it.

A long design discussion considered alternatives — ReBAC/Zanzibar (overkill for chat's flat-ish structure), policy-as-code (incompatible with operator-configurable self-hosting), capability tokens (wrong fit for server-state-owns-everything chat). The model that best matches both the room-sets requirement and operators' actual mental model ("look at the room/category to know what's allowed there") is channel-centric ACLs as used by Discord and similar chat systems.

## Decision

Adopt a **channel-centric ACL** model for channel-room permissions with **room sets** as the primary permission container. Three permission containers, with explicit (no implicit) inheritance:

| Container | Configures | Examples |
|---|---|---|
| **Server** | Server-scope permissions only | `server.manage`, `role.manage`, `role.assign`, `admin.access`, `admin.view-users`, `dm.view`, `dm.write`, `user.delete-any`, `user.delete-self` |
| **Room set** | Room-scope permissions for every channel room in the set | `message.post`, `message.react`, `room.list`, `room.join`, `room.manage`, `message.edit-own/any`, `message.delete-own/any`, `message.echo`, `message.reply` |
| **Room** | Room-scope permissions, **overriding the room set on a per-(role, permission) basis** | Same as above; only the (role, permission) pairs explicitly overridden change from the set's value, the rest inherit |

Subjects are unchanged: **roles** (with rank, RBAC-style) and **users** (for direct overrides). Every authenticated user implicitly carries `everyone`.

**DMs are out of scope for this ADR.** DM rooms are not part of any room set; their permission shape (including the existing hardcoded `dmBoundaryDeniedPermissions` list in the resolver) stays as it is today. Room sets are a feature on top of channel rooms only. If we want to make DM permissions data-driven later, that's a separate concern.

This work evolves the existing `RoomLayout` / `RoomLayoutSection` storage (`proto/chatto/core/v1/models.proto`) — sections become sets. The atomic-OCC update pattern in `UpdateRoomLayout` and the live `RoomLayoutUpdatedEvent` are preserved; what changes is the section type's fields (gains `displayName`, `description`) and the disappearance of `unsorted_room_ids` (every channel room is now in a set).

### Membership and structural invariants

- **Every channel room belongs to exactly one set.** No nullable `setID`, no "uncategorized" branch in the resolver. (DM rooms do not belong to a set.)
- **Sets are operator-managed, not system-protected.** On first boot, one set named "Rooms" is seeded; the auto-created `announcements` and `general` channels go into it. The operator can rename, reorder, or delete this set like any other.
- **Set deletion is rejected while rooms exist.** Operators must move all rooms out first. No "delete and reassign" cascade — the rejection is deliberate to avoid surprise.
- **Room creation requires a set.** When no set is implied by UI context, the API requires one explicitly. There is no implicit fallback set; the seed "Rooms" set only matters at first-boot.
- **Set membership is stored on the room record** (one `setID` field per room).
- **Moving a room between sets requires `room.manage` in BOTH the source and target set.** The action changes the room's effective ACL overnight, so the caller must be authorized in both ends of the move.
- **Sets are ordered.** Set order, like room order within a set, is captured in the layout proto (same atomic-OCC pattern as today's `RoomLayout`).

### Resolution

For **server-scope** permissions: unchanged from current model. Standard hierarchy-wins RBAC walker over server-scope role grants, with user-level overrides outranking roles (Phase 1 of the current resolver).

For **DM rooms**: unchanged. The existing resolver path (membership check + `dmBoundaryDeniedPermissions` deny-list + server-scope grants for permitted actions) stays as-is. Room sets do not apply.

For **channel-room-scope** permissions in room R (belonging to set S):

1. **User-level overrides**, in order: room R → set S. First explicit decision wins.
2. **Role walk**, highest rank first. For each role:
   1. Room R's grant/deny for that role
   2. Set S's grant/deny for that role
3. **Default deny** if no decision was reached.

There is **no cascade from server scope into channel-room scope**. Server-scope grants apply only to server-scope permissions.

Within the role walk, room-scope decisions override set-scope decisions *within the same role*. Across roles, hierarchy wins as today (higher rank's decision is examined first, lower-rank roles not consulted if a higher rank decided).

**The announcements pattern still uses a deny**, but now scoped to a room inside a set instead of overriding a server-scope grant. The set "Rooms" grants `message.post` to `everyone`; the `announcements` room inside it has a per-room deny for `everyone.message.post`. Moderators' grant comes through the set (no per-room override needed); the walker visits moderator first, finds the set's allow, and returns. The win over the previous model isn't "no denies" — it's that the deny is scoped, audit-visible inside its room, and doesn't compete with cross-room operator intent.

### Moderation actions

Temporary user-targeted restrictions ("mute", "timeout", "suspend") build on the existing **user-level deny** primitive, which outranks role grants. The UI exposes verbs (Mute, Timeout, Suspend with duration), not raw permission editors. Underneath, each action writes a small fixed bundle of user-level denies (server-scope, set-scope, or room-scope) with a scheduled cleanup for expiry. No new resolver concept ("restrictive role" flag etc.) is required.

### Migration

Existing servers reset RBAC on upgrade (`chatto reset rbac` already exists for related migrations). Specifically:

- A seed "Rooms" set is created.
- Existing `RoomLayoutSection`s migrate to `RoomSet`s (id and ordering preserved; `name` becomes the set's `displayName`).
- Any rooms tracked in `unsorted_room_ids` are swept into the seed "Rooms" set.
- Each set is initialised with the current default everyone/moderator/owner/admin grants for channel-room permissions.
- Server-scope perms migrate untouched.
- DM rooms and the `dmBoundaryDeniedPermissions` list are untouched.

The three known production-shaped Chatto servers absorb this. Out-of-the-box behavior after migration matches today's defaults.

## Consequences

### Easier

- **Per-team rooms come for free.** Define a room set, restrict it to a role, every channel room in the set inherits — including rooms added later. The headline feature this ADR exists to enable.
- **Bulk operator changes scope to a set.** "Adjust how members behave in the Engineering rooms" is one set-level edit, not a per-room sweep or a global server-wide change.
- **Trace output maps to operator containers.** "Set 'Rooms' grants `message.post` to `everyone`; room `announcements` overrides with deny" is exactly what the admin UI surfaces. The walker's path matches the UI's container tree.
- **Timeout/mute is uncontroversial.** User-level deny is the primitive; moderation actions are a thin product layer on top. No new resolver concept required, no tension with set-level grants.
- **Operator mental model matches reality.** "Open the set or the room to see what's allowed there" is true. Sets are the source of truth for their rooms unless a room explicitly overrides.

### More difficult

- **Global tweaks require multi-set edits.** Today, changing a server-scope grant on `everyone` affects every room. After this change, the same effect requires editing each set (sets are independent — there is no cross-set inheritance). The admin UI must offer an "apply to all sets" affordance to make global tweaks ergonomic; under the hood it writes N keys.
- **More KV keys.** Each (set, role, perm) and (room, role, perm) override is its own key. Practical scale (low thousands) is comfortable for JetStream KV, but storage and listing costs grow linearly with sets × rooms.
- **One-time RBAC reset.** Existing servers need to migrate (`chatto reset rbac` or equivalent). Acceptable at alpha; a non-event for new deployments.
- **Room creation always needs a set.** Pre-change, a new room could be created with no group affiliation. Post-change, the API and UI must always pick a set. Drop in operator ergonomics is small but real.
- **Room-move requires two-set authorization.** Moving a room between sets needs `room.manage` in both source and target. UI must surface this clearly (preview affected users, confirmation step) and the GraphQL surface needs to reflect both checks.

### Relationship to prior ADRs

- **Supersedes ADR-005 for channel-room permissions only.** Hierarchy-wins RBAC still governs server-scope resolution; the room-scope cascade described in ADR-005 ("deny on `everyone` overridden by higher role's grant") is replaced by the room+set per-role walk. ADR-005's announcements example moves from "server-scope grant on everyone, room-scope deny on everyone" to "set-scope grant on everyone, room-scope deny on everyone" — same shape, just scoped to a set instead of cascading from the server.
- **Builds on ADR-004** (authorization at the API boundary). Core remains pure; GraphQL gates remain the enforcement layer.
- **Leaves ADR-015 (DMs as a Hidden Space) untouched.** DMs are not part of any room set; their hardcoded `dmBoundaryDeniedPermissions` list stays as today. Room sets are a channel-rooms-only feature.
- **Compatible with ADR-027 and ADR-030.** Server consolidation and the retirement of the space tier are preserved; this ADR introduces a *new* container (room set) below the server, not a return to two tiers.

### Out of scope for this ADR

- Custom system roles beyond owner/admin/moderator (rank is unchanged).
- Cross-set permission inheritance (sets are independent; this can be revisited if real demand emerges).
- Nested room sets (rooms belong to exactly one set; no set-of-sets).
- ReBAC / relationship-based resolution (revisit only if structural-document features appear).
- Restrictive-role flag for temporary punishment (user-level denies are the chosen primitive instead).
