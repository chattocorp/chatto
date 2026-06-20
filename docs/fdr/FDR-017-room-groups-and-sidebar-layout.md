# FDR-017: Room Groups & Sidebar Layout

**Status:** Active
**Last reviewed:** 2026-06-18

## Overview

Channel rooms are organized into **room groups** — named, ordered containers that act as both a UI grouping concept (collapsible sections in the sidebar) and the primary permission scope for room-level permissions. Every channel room belongs to exactly one group; DMs sit outside the group system entirely. Groups can also contain sidebar links: operator-managed links rendered in the same ordered sidebar section as rooms.

## Behavior

- The sidebar shows `room.list`-visible channel rooms and sidebar links grouped under their group's name in operator-defined order. Groups can be collapsed/expanded.
- Joined channel rooms behave as normal navigation entries. Listable channel rooms the viewer has not joined yet are shown slightly faded; selecting a joinable room asks for confirmation before joining, while selecting a non-joinable room explains that access is not currently available.
- Server admins can create, rename, reorder, and delete groups via the admin UI.
- Group names are limited to 80 bytes; group descriptions are limited to 500 bytes.
- Every channel room belongs to exactly one group. There's no "uncategorized" branch — room creation requires a group.
- Sidebar links belong to exactly one group, carry a label and either an absolute `http`/`https` URL or a server-local path starting with `/`, and are visible to authenticated users who can see the server sidebar.
- A freshly bootstrapped server has one group named "Lobby" containing the auto-created `announcements` and `general` rooms. Operators can rename it, reorder it, or replace it like any other group.
- Deleting a group is rejected while rooms or sidebar links still live in it. Operators move or delete its contents first.
- Moving a room between groups requires `room.manage` in both the source and the target group (the room's effective ACL changes overnight).
- Creating, editing, moving, deleting, or reordering sidebar links requires `room.manage` for the affected group. Moving a sidebar link between groups requires `room.manage` in both the source and target groups, matching room moves.
- Room-scope permissions (`message.post`, `room.join`, `message.react`, etc.) can be configured per group, with per-room overrides on top.

## Design Decisions

### 1. The room group is the primary permission container

**Decision:** Room-scope permissions are configured at the group level by default. A per-room override only changes the (role, permission) pairs explicitly overridden; everything else inherits from the group.
**Why:** Operators think in terms of room categories — "Engineering rooms work like X; off-topic rooms work like Y". Configuring permissions per category matches that mental model. Channel-centric ACLs (Discord-style) outperformed alternatives like ReBAC for chat's flat-ish structure. See ADR-031.
**Tradeoff:** A global tweak now requires editing every group. The admin UI surfaces an "apply to all groups" affordance to keep this ergonomic.

### 2. Per-room overrides are sparse, not full configurations

**Decision:** A room's permission config stores only the (role, permission) pairs that differ from its group. The rest are inherited.
**Why:** Storing a full copy per room would multiply KV entries and make group-level tweaks awkward (every room would need to be touched). Sparse overrides keep the model both compact and operator-friendly.
**Tradeoff:** The permission resolver has to walk the inheritance chain (room → group → server) for every check. Acceptable; the chain is short and cached.

### 3. Server scope cascades as a global default

**Decision:** When a permission isn't decided at group or room scope, the resolver falls back to the server-scope grant. This gives operators a single "global default" to adjust once.
**Why:** Without server-scope cascade, every group would need a full set of grants from scratch — a worse onboarding experience and a worse story for DMs (which aren't in any group). The cascade restores a sensible default tier. See ADR-031.
**Tradeoff:** The ADR's headline "groups are the permission container" is slightly softer than it sounded — server scope still matters as a backstop. In practice operators rarely need to think about server scope unless they want a global default different from the seed.

### 4. Group deletion is non-cascading

**Decision:** A group with rooms or sidebar links in it can't be deleted. Operators must move every sidebar item out first.
**Why:** Cascading delete would be silent data loss in disguise — the operator might not realize rooms were tied to the group they're discarding. Forcing an explicit move makes the operator's intent unambiguous.
**Tradeoff:** A bit more UI work to "drain" a group before removing it. Worth it for the safety.

### 5. Moves require authorization in both ends

**Decision:** Moving a room or sidebar link from group A to group B requires `room.manage` in *both* A and B. The UI previews affected users before confirming room moves.
**Why:** Moving across groups changes the effective permission set for everyone using the room. An admin authorized only in A shouldn't be able to dump rooms into B and grant a different audience access. Requiring both ends makes the privilege boundary symmetric.
**Tradeoff:** Operators with split responsibilities (group-of-groups admins) can't unilaterally rebalance — they need authorization on both sides. Considered correct: the operation is consequential. The write path uses a room-group projection snapshot plus `evt.group.>` OCC so concurrent moves retry from the current source group before appending the remove/add batch.

### 6. Sidebar links extend the existing group aggregate

**Decision:** Sidebar links are group-owned entries persisted as durable `evt.group.{groupId}.{eventType}` facts alongside room add/remove/reorder facts.
**Why:** The sidebar already reads group membership and order from the group aggregate. Keeping external links in that aggregate gives one ordered list of sidebar items without introducing a second layout store or a parallel permission model.
**Tradeoff:** A group reorder now talks about mixed sidebar entries rather than room IDs alone. The GraphQL API keeps the existing room-specific operations for compatibility and adds mixed-entry operations for link-aware clients.

### 7. DMs are outside the group system

**Decision:** DM rooms don't belong to any group. Reading is governed by DM room membership; sending and starting DMs use message permissions; the hardcoded `dmBoundaryDeniedPermissions` list still prevents channel-style moderation inside DMs. Group concepts don't apply.
**Why:** DMs don't fit a "category of rooms" model — every DM is its own conversation. Trying to retrofit groups onto DMs would either need a synthetic "DMs" group (privilege concentration risk) or per-DM groups (meaningless). See ADR-031 and ADR-037.
**Tradeoff:** DMs keep a small policy branch outside the room-group model. That branch is about DM privacy and creation, not about read visibility.

### 8. Sidebar visibility follows room.list, not membership

**Decision:** Channel-room sidebar entries are based on `room.list` visibility and room-group layout, while membership only changes the row's presentation and action.
**Why:** Operators configure the sidebar through room groups. Showing all listable rooms makes that layout the user's map of the server, and lets users discover rooms before joining them.
**Tradeoff:** The sidebar can show rooms the viewer cannot enter yet. Those rows need clear affordances so discovery does not look like broken navigation.

## Permissions

- `room.create` — configured per group (or at server scope as a default).
- `room.manage` — required in both source and target groups when moving a room.
- `room.list` — controls whether a channel room appears in the sidebar and room directory for non-members.
- `room.join` — controls whether a non-member can join a visible channel room directly.
- All channel-room-scope permissions (`message.post`, `room.join`, etc.) are configurable per group with per-room overrides.

## Related

- **ADRs:** ADR-031 (room-group-centric ACL), ADR-037 (DM access via membership), ADR-040 (permission-only RBAC with owner override)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-007 (Direct Messages), FDR-019 (Room Lifecycle)
