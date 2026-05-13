# ADR-031: Room-Set-Centric ACL for Room-Scope Permissions

**Date:** 2026-05-13

## Context

The post-#330 RBAC model resolves room-scope permissions through a single hierarchy walker rooted in server-scope grants, with room-scope decisions overlaid on top via room-level allow/deny keys. The walker is uniform and tightened (see ADR-005, and the `hmans/rbac-review` work that closed self-grant escalation and dropped `admin.bypass`), but the underlying *shape* of the model produces several awkward edges:

- **Server→room cascade forces denies for exceptions.** Room-scope perms (`message.post`, `room.list`, etc.) are granted on the `everyone` role at server scope and inherit into every room. To create an exception room (e.g. `#announcements` where only moderators post), operators must *deny* on `everyone` at room scope rather than simply not-grant. "Just don't grant" doesn't work because the server-scope grant cascades in.

- **Implicit `everyone` collides with deny-always semantics.** Every authenticated user implicitly carries `everyone`, so any deny attached to `everyone` (which is the natural place to put broad restrictions) catches moderators and admins too. The hierarchy-wins rule is what currently lets the announcements pattern function — higher-rank grants override lower-rank denies — but the same rule rules out a "deny-always-wins" semantics that would otherwise be useful for temporary-restriction roles (timeouts, mutes).

- **DM behavior is hardcoded.** A static `dmBoundaryDeniedPermissions` list in the resolver unconditionally denies certain permissions in DM rooms (`room.manage`, `message.edit-any`, `message.delete-any`, `message.echo`, `room.list`, `room.create`). This is policy expressed in code, not in data.

- **No natural permission boundary for groups of rooms.** A planned **room sets** feature (replacing the current collapsible UI groups) requires per-group access control — e.g., "Engineering" rooms accessible only to the `engineers` role. There is no container in the current model where such permissions could live. Layering room sets onto the existing cascade would create a second cascade tier on top of the first.

Chatto is at alpha. The three known production-shaped servers can absorb a `chatto reset rbac` on upgrade. This is a one-time opportunity to reshape the model before the room-sets feature lands rather than to layer over it.

A long design discussion considered alternatives — ReBAC/Zanzibar (overkill for chat's flat-ish structure), policy-as-code (incompatible with operator-configurable self-hosting), capability tokens (wrong fit for server-state-owns-everything chat). The model that best matches both the room-sets requirement and operators' actual mental model ("look at the room/category to know what's allowed there") is channel-centric ACLs as used by Discord and similar chat systems.

## Decision

Adopt a **channel-centric ACL** model for room-scope permissions with **room sets** as the primary permission container. Three permission containers, with explicit (no implicit) inheritance:

| Container | Configures | Examples |
|---|---|---|
| **Server** | Server-scope permissions only | `server.manage`, `role.manage`, `role.assign`, `admin.access`, `admin.view-users`, `dm.view`, `dm.write`, `user.delete-any`, `user.delete-self` |
| **Room set** | Room-scope permissions for every room in the set | `message.post`, `message.react`, `room.list`, `room.join`, `room.manage`, `message.edit-own/any`, `message.delete-own/any`, `message.echo`, `message.reply` |
| **Room** | Room-scope permissions, **overriding the room set on a per-permission basis** | Same as above; only specified perms override the set, the rest inherit from the set |

Subjects are unchanged: **roles** (with rank, RBAC-style) and **users** (for direct overrides). Every authenticated user implicitly carries `everyone`.

### Membership and structural invariants

- **Every room belongs to exactly one set.** No nullable `setID`, no "uncategorized" branch in the resolver.
- A built-in `default` set exists; it cannot be deleted and serves as the home for rooms not explicitly assigned to another set. New rooms default to the current UI set, falling back to `default`.
- DMs live in a system-managed `dm` set. The hardcoded `dmBoundaryDeniedPermissions` list disappears: the `dm` set simply doesn't grant `message.echo` / `room.list` / `room.create` / `room.manage` / `message.edit-any` / `message.delete-any` to anyone. The boundary becomes data, not code.
- Set membership is stored on the room record (one `setID` field per room).
- Set deletion is rejected while rooms exist; operators must move rooms out first. (Alternative: cascade to `default`. Rejected for surprise reasons.)
- Moving a room between sets is a deliberate operation gated by a permission (`room.manage` or a dedicated `room.move`), since it changes the room's effective ACL overnight.

### Resolution

For **server-scope** permissions: unchanged from current model. Standard hierarchy-wins RBAC walker over server-scope role grants, with user-level overrides outranking roles (Phase 1 of the current resolver).

For **room-scope** permissions in room R (belonging to set S):

1. **User-level overrides**, in order: room R → set S. First explicit decision wins.
2. **Role walk**, highest rank first. For each role:
   1. Room R's grant/deny for that role
   2. Set S's grant/deny for that role
3. **Default deny** if no decision was reached.

There is **no cascade from server scope into room scope** for room-scope permissions. Server-scope grants apply only to server-scope permissions.

Within the role walk, room-scope decisions override set-scope decisions *within the same role*. Across roles, hierarchy wins as today (higher rank's decision is examined first, lower-rank roles not consulted if a higher rank decided).

### Moderation actions

Temporary user-targeted restrictions ("mute", "timeout", "suspend") build on the existing **user-level deny** primitive, which outranks role grants. The UI exposes verbs (Mute, Timeout, Suspend with duration), not raw permission editors. Underneath, each action writes a small fixed bundle of user-level denies (server-scope, set-scope, or room-scope) with a scheduled cleanup for expiry. No new resolver concept ("restrictive role" flag etc.) is required.

### Migration

Existing servers reset RBAC on upgrade (`chatto reset rbac` already exists for related migrations). Specifically:

- The `default` set and `dm` set are seeded.
- Every existing room is assigned to `default` (or to `dm` for DM rooms).
- The `default` set is initialised with the current default everyone/moderator/owner/admin permissions for room-scope perms.
- Server-scope perms migrate untouched.

The three known production-shaped Chatto servers absorb this. Out-of-the-box behavior after migration matches today's defaults.

## Consequences

### Easier

- **Announcements without denies.** "Don't grant `message.post` to `everyone` in this room/set" is the literal answer. No deny key needed.
- **Per-team rooms come for free.** Define a room set, restrict it to a role, every room in the set inherits — including rooms added later.
- **DM policy is data.** Editing a single set replaces a hardcoded list in the resolver. Hypothetical future "DM with media disabled by default" becomes an admin toggle.
- **Resolver and trace output are simpler.** Two scopes to probe per role (room, set) with no fallback to server-scope grants. Trace messages map directly to UI containers operators can see.
- **Timeout/mute is uncontroversial.** User-level deny is the primitive; moderation actions are a thin product layer on top. No tension with the announcements pattern because announcements no longer uses denies.
- **Operator mental model matches reality.** "Open the room/set to see what's allowed there" is true. The room is the source of truth for that room's behavior.

### More difficult

- **Global tweaks require multi-set edits.** Today, changing a server-scope grant on `everyone` affects every room. After this change, the same effect requires editing each set (or a `default`-only edit if all sets inherit from it; they don't here — sets are independent). The admin UI must offer an "apply to all sets" affordance to make global tweaks ergonomic; under the hood it writes N keys.
- **More KV keys.** Each (set, role, perm) and (room, role, perm) override is its own key. Practical scale (low thousands) is comfortable for JetStream KV, but storage and listing costs grow linearly with sets × rooms.
- **One-time RBAC reset.** Existing servers need to migrate (`chatto reset rbac` or equivalent). Acceptable at alpha; a non-event for new deployments.
- **Set/room move semantics need product attention.** Operators moving a room between sets means an instant ACL change. UI must surface this clearly (preview affected users, confirmation step).

### Relationship to prior ADRs

- **Supersedes ADR-005 for room-scope permissions only.** Hierarchy-wins RBAC still governs server-scope resolution; the room-scope cascade described in ADR-005 ("deny on `everyone` overridden by higher role's grant") is replaced by the room+set per-role walk. ADR-005's announcements example moves from "deny on everyone, grant on moderator" to "don't grant on everyone, grant on moderator" — same effect, no deny.
- **Builds on ADR-004** (authorization at the API boundary). Core remains pure; GraphQL gates remain the enforcement layer.
- **Replaces the DM-specific scaffolding in ADR-015** in part. DMs remain "hidden" from non-participants, but the special-case `dmBoundaryDeniedPermissions` list disappears in favor of explicit `dm` set permissions.
- **Compatible with ADR-027 and ADR-030.** Server consolidation and the retirement of the space tier are preserved; this ADR introduces a *new* container (room set) below the server, not a return to two tiers.

### Out of scope for this ADR

- Custom system roles beyond owner/admin/moderator (rank is unchanged).
- Cross-set permission inheritance (sets are independent; this can be revisited if real demand emerges).
- Nested room sets (rooms belong to exactly one set; no set-of-sets).
- ReBAC / relationship-based resolution (revisit only if structural-document features appear).
- Restrictive-role flag for temporary punishment (user-level denies are the chosen primitive instead).
