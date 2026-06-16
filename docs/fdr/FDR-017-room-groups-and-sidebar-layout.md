# FDR-017: Room Groups & Sidebar Layout

**Status:** Active
**Last reviewed:** 2026-06-15

## Overview

Channel rooms are organized into **room groups** — named, ordered containers that act as both a UI grouping concept (collapsible sections in the sidebar) and the primary permission scope for room-level permissions. Every channel room belongs to exactly one group; DMs sit outside the group system entirely.

## Behavior

- The sidebar shows rooms grouped under their group's name in operator-defined order. Groups can be collapsed/expanded.
- Server admins can create, rename, reorder, and delete groups via the admin UI.
- Group names are limited to 80 bytes; group descriptions are limited to 500 bytes.
- Every channel room belongs to exactly one group. There's no "uncategorized" branch — room creation requires a group.
- A freshly bootstrapped server has one group named "Lobby" containing the auto-created `announcements` and `general` rooms. Operators can rename it, reorder it, or replace it like any other group.
- Deleting a group is rejected while rooms still live in it. Operators move rooms out first.
- Moving a room between groups requires `room.manage` in both the source and the target group (the room's effective ACL changes overnight).
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

**Decision:** A group with rooms in it can't be deleted. Operators must move every room out first.
**Why:** Cascading delete would be silent data loss in disguise — the operator might not realize rooms were tied to the group they're discarding. Forcing an explicit move makes the operator's intent unambiguous.
**Tradeoff:** A bit more UI work to "drain" a group before removing it. Worth it for the safety.

### 5. Moves require authorization in both ends

**Decision:** Moving a room from group A to group B requires `room.manage` in *both* A and B. The UI previews affected users before confirming.
**Why:** Moving across groups changes the effective permission set for everyone using the room. An admin authorized only in A shouldn't be able to dump rooms into B and grant a different audience access. Requiring both ends makes the privilege boundary symmetric.
**Tradeoff:** Operators with split responsibilities (group-of-groups admins) can't unilaterally rebalance — they need authorization on both sides. Considered correct: the operation is consequential. The write path uses a room-group projection snapshot plus `evt.group.>` OCC so concurrent moves retry from the current source group before appending the remove/add batch.

### 6. DMs are outside the group system

**Decision:** DM rooms don't belong to any group. Reading is governed by DM room membership; sending and starting DMs use message permissions; the hardcoded `dmBoundaryDeniedPermissions` list still prevents channel-style moderation inside DMs. Group concepts don't apply.
**Why:** DMs don't fit a "category of rooms" model — every DM is its own conversation. Trying to retrofit groups onto DMs would either need a synthetic "DMs" group (privilege concentration risk) or per-DM groups (meaningless). See ADR-031 and ADR-037.
**Tradeoff:** DMs keep a small policy branch outside the room-group model. That branch is about DM privacy and creation, not about read visibility.

## Permissions

- `room.create` — configured per group (or at server scope as a default).
- `room.manage` — required in both source and target groups when moving a room.
- All channel-room-scope permissions (`message.post`, `room.join`, etc.) are configurable per group with per-room overrides.

## Related

- **ADRs:** ADR-031 (room-group-centric ACL), ADR-037 (DM access via membership), ADR-040 (permission-only RBAC with owner override)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-007 (Direct Messages), FDR-019 (Room Lifecycle)
