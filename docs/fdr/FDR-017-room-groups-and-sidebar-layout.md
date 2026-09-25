# FDR-017: Room Groups & Sidebar Layout

**Status:** Active
**Last reviewed:** 2026-09-24

## Overview

Channel rooms are organized into **room groups** — named, ordered containers that act as both a UI grouping concept (collapsible sections in the sidebar) and the primary permission scope for room-level permissions. Every channel room belongs to exactly one group; DMs sit outside the group system entirely. Groups can also contain sidebar links: operator-managed links rendered in the same ordered sidebar section as rooms.

## Behavior

- Collapsible sidebar sections slide their content in and out, including
  drag-enabled rows and footer controls. Highlighted rows remain visible when
  required. Reduced-motion preferences disable spatial animation.

- The sidebar shows `room.list`-visible channel rooms and sidebar links grouped under their group's name in operator-defined order. Groups can be collapsed or expanded. A viewer with effective group `room.manage` also sees group actions, including when no rooms in the group are otherwise visible.
- Authorized viewers manage the layout where they use it. Group headers provide actions to create rooms and links, open group settings, or delete an empty group. Room and link rows provide their applicable settings, edit, archive, and delete actions. Server-wide room managers can create a group from a compact control after the last room group.
- A group header shows a permanent **+** button when the viewer can create a room or manage the group. It opens a small menu with **New Room** (`room.create`) and **New Link** (group `room.manage`), showing only permitted actions. The button also works when the group is collapsed. The controls and open creation menu update when group permissions change, including when privileged mode is enabled or disabled. Each action creates its entry in that group. The header context menu keeps the creation and management actions.
- Explicit drag handles let authorized viewers reorder groups and move room or link entries within or between groups. Pointer-based layouts fade each drag handle in over the leading row icon. Touch layouts keep the controls visible.
- Configured room groups, the alphabetical fallback used before a layout exists, and the Direct Messages section share the same sidebar heading, spacing, and collapse/expand interaction. This presentation does not make Direct Messages an operator-managed room group.
- When a channel room or DM becomes current, its sidebar row scrolls into view
  if the row exists. An already visible row stays in place. A closed sidebar
  stays closed; the mobile drawer is positioned for its next opening, while a
  hidden desktop sidebar waits until it opens.
- ConnectRPC `RoomDirectoryService.ListRoomGroups` exposes the same ordered sidebar structure for protobuf-first clients, filtering room entries to non-archived channel rooms visible to the viewer, preserving sidebar links, and reporting effective `room.create` and `room.manage` group capabilities in viewer state.
- Unjoined channel rooms are hidden by default in each expanded group. A compact "+ N more" row at the end of the group reveals them in their configured order. Select "Show less" to hide them again. The control appears even when only one unjoined room remains. Joined rooms, sidebar links, and the current room stay visible. The same rule applies to the alphabetical fallback. Expansion is independent for each group and resets when the sidebar is remounted. This control does not contact an external service.
- Joined channel rooms behave as normal navigation entries. Listable channel rooms the viewer has not joined yet are shown slightly faded; selecting a joinable room asks for confirmation before joining, while selecting a non-joinable room explains that access is not currently available.
- Every visible sidebar room row exposes a context menu with a final “Copy Room ID” action that writes the room's stable ID to the clipboard. Successful copies are confirmed; clipboard failures report an error. Joined rooms offer unread and leave actions where applicable; non-member rooms offer Join, disabled when the viewer lacks `room.join`. Effective room `room.manage` holders also receive a settings action for channel rooms.
- The room-layout overview remains available as a management fallback while the sidebar gains feature parity. Resource settings pages remain the place for group metadata and permission matrices.
- Group names are limited to 80 bytes; group descriptions are limited to 500 bytes.
- Every channel room belongs to exactly one group. There's no "uncategorized" branch — room creation requires a group.
- Sidebar links belong to exactly one group, carry a label and either an absolute `http`/`https` URL or a server-local path starting with `/`, and are visible to authenticated users who can see the server sidebar. The create and edit forms add `https://` when an operator enters a host name without a scheme.
- A freshly bootstrapped server has one group named "Lobby" containing the auto-created `announcements` and `general` rooms. Operators can rename it, reorder it, or replace it like any other group.
- Deleting a group is rejected while rooms or sidebar links still live in it. Operators move or delete its contents first.
- Moving a room between groups requires `room.manage` in both the source and the target group (the room's effective ACL changes overnight).
- Creating, editing, moving, deleting, or reordering sidebar links requires `room.manage` for the affected group. Moving a sidebar link between groups requires `room.manage` in both the source and target groups, matching room moves.
- Room-scope permissions (`message.post`, `room.join`, `message.react`, etc.) can be configured per group, with per-room overrides on top.
- Management-authorized group metadata reads are distinct from the user-facing room directory: `room.manage` and `role.manage` holders can load the selected group's settings even when they do not otherwise have room visibility. This read omits the group's ordered room/link entries so role permission managers do not gain private-room visibility through the settings page.

## Design Decisions

### 1. The room group is the primary permission container

**Decision:** Room-scope permissions are configured at the group level by default. A per-room override only changes the (role, permission) pairs explicitly overridden; everything else inherits from the group.
**Why:** Operators think in terms of room categories — "Engineering rooms work like X; off-topic rooms work like Y". Configuring permissions per category matches that mental model. Channel-centric ACLs (Discord-style) outperformed alternatives like ReBAC for chat's flat-ish structure. See ADR-031.
**Tradeoff:** A global tweak now requires editing every group. The admin UI surfaces an "apply to all groups" affordance to keep this ergonomic.

### 2. Per-room overrides are sparse, not full configurations

**Decision:** A room's permission config stores only the (subject, permission) pairs that differ from its group. For that same subject, a room decision replaces the group and server value; everything else inherits independently.
**Why:** Storing a full copy per room would multiply KV entries and make group-level tweaks awkward (every room would need to be touched). Sparse overrides keep the model both compact and operator-friendly.
**Tradeoff:** The permission resolver has to walk the inheritance chain (room → group → server) once per direct user or named role. Acceptable; the chain is short and cached.

### 3. Server scope cascades as a global default

**Decision:** When a subject's permission isn't decided at group or room scope, the resolver falls back to that subject's server decision. `everyone` supplies the scoped baseline: a named allow overrides an `everyone` deny only at the same or a nearer scope. This gives operators a single global default while letting a room/group baseline contain less-specific grants.
**Why:** Without server-scope cascade, every group would need a full set of grants from scratch — a worse onboarding experience and a worse story for DMs (which aren't in any group). The cascade restores a sensible default tier. See ADR-031.
**Tradeoff:** The ADR's headline "groups are the permission container" is slightly softer than it sounded — server scope still matters as a backstop. In practice operators rarely need to think about server scope unless they want a global default different from the seed.

### 4. Group deletion is non-cascading

**Decision:** A group with rooms or sidebar links in it can't be deleted. Operators must move every sidebar item out first.
**Why:** Cascading delete would be silent data loss in disguise — the operator might not realize rooms were tied to the group they're discarding. Forcing an explicit move makes the operator's intent unambiguous.
**Tradeoff:** A bit more UI work to "drain" a group before removing it. Worth it for the safety.

### 5. Moves require authorization in both ends

**Decision:** Moving a room or sidebar link from group A to group B requires `room.manage` in _both_ A and B. The UI previews affected users before confirming room moves.
**Why:** Moving across groups changes the effective permission set for everyone using the room. An admin authorized only in A shouldn't be able to dump rooms into B and grant a different audience access. Requiring both ends makes the privilege boundary symmetric.
**Tradeoff:** Operators with split responsibilities (group-of-groups admins) can't unilaterally rebalance — they need authorization on both sides. Considered correct: the operation is consequential. The write path uses a room-group projection snapshot plus `evt.group.>` OCC so concurrent moves retry from the current source group before appending the remove/add batch. User-authorized group and layout mutations validate stable request-time authorization inputs before the domain append.

### 6. Sidebar links extend the existing group aggregate

**Decision:** Sidebar links are group-owned entries persisted as durable `evt.group.{groupId}.{eventType}` facts alongside room add/remove/reorder facts.
**Why:** The sidebar already reads group membership and order from the group aggregate. Keeping external links in that aggregate gives one ordered list of sidebar items without introducing a second layout store or a parallel permission model.
**Tradeoff:** A group reorder now talks about mixed sidebar entries rather than room IDs alone. The public API keeps room-specific operations and mixed-entry operations explicit for link-aware clients.

### 7. DMs are outside the group system

**Decision:** DM rooms do not belong to any group. Membership is mandatory, and the singleton DM permission scope controls all `message.*` operations. Room and group permissions do not apply. Group concepts do not apply.
**Why:** DMs don't fit a "category of rooms" model — every DM is its own conversation. Trying to retrofit groups onto DMs would either need a synthetic "DMs" group (privilege concentration risk) or per-DM groups (meaningless). See ADR-031 and ADR-037.
**Tradeoff:** DMs use one application-wide permission tier instead of room groups or per-DM overrides. The sidebar presents them like the other collapsible navigation sections for consistency, but that visual treatment must not imply group settings or group-scoped permissions.

### 8. Sidebar visibility follows room.list, not membership

**Decision:** Channel-room sidebar entries are based on `room.list` visibility and room-group layout, while membership controls which rooms appear before the viewer selects "+ N more".
**Why:** Operators configure the sidebar through room groups. The count keeps rooms discoverable before users join them and keeps the default sidebar short.
**Tradeoff:** The sidebar can show rooms the viewer cannot enter yet. Those rows need clear affordances so discovery does not look like broken navigation.

### 9. Room directory reads are available over ConnectRPC

**Decision:** `RoomDirectoryService` is the protobuf-first read surface for room navigation: non-archived visible room lists, ordered room groups, mixed sidebar items, per-room viewer capability state, and the group join-all command.
**Why:** Clients need room/sidebar data around lifecycle commands. Keeping the directory read model in ConnectRPC lets clients render navigation and action affordances through one protobuf API surface.
**Tradeoff:** The service owns the room/sidebar visibility contract directly, so changes to room visibility must update the ConnectRPC mapping and tests.

### 10. Lightweight layout management stays in the sidebar

**Decision:** Creation, removal, and ordering actions live next to the affected room group, room, or sidebar link. Detailed metadata and permission settings remain on resource pages in the management area. The room-layout overview remains available during the transition.
**Why:** Operators can adjust the navigation structure without leaving the navigation context. The split also keeps complex forms out of the narrow sidebar.
**Tradeoff:** The permanent creation button uses space in each group header so users can find it. Drag handles still appear on hover or focus, and stay visible on touch layouts. Relative move commands preserve entries that the caller cannot see, so a filtered sidebar cannot remove hidden rooms from the authoritative layout.

### 11. Structural changes commit all authoritative facts together

**Decision:** Room creation commits the room and its initial group membership in
one atomic EVT batch. Room deletion commits the room tombstone and group removal
in one batch. Group creation and deletion commit the lifecycle fact and the
resulting global group order in one batch. Each retry rebuilds the batch from
current projections and guards every state boundary that it used.
**Why:** A process failure between separate writes can leave a room without a
group or leave a deleted group in the authoritative order. Reconciliation can
hide an incomplete state, but it cannot add the missing durable fact to EVT.
**Tradeoff:** These commands can repeat their authorization read when an input
changes during the decision. They can retry the complete command when a
concurrent room, group, or layout change advances a domain OCC boundary. See
ADR-086 and ADR-087.

## Permissions

- `room.create` — configured per group (or at server scope as a default).
- Server-scope `room.manage` — create and globally reorder room groups.
- Effective group `room.manage` — edit or delete a group and manage its sidebar entries; required in both source and target groups when moving a room or link.
- `role.manage` — configure role permission decisions at group scope without granting authority to change the group's general settings.
- `room.list` — controls whether a channel room appears in the sidebar and room directory for non-members.
- `room.join` — controls whether a non-member can join a visible channel room directly.
- All channel-room-scope permissions (`message.post`, `room.join`, etc.) are configurable per group with per-room overrides.

## Related

- **ADRs:** ADR-031 (room-group-centric ACL), ADR-037 (DM access via membership), ADR-040 (permission-only RBAC with owner override), ADR-052 (subject-specific RBAC with an everyone baseline), ADR-086 (atomic room-layout structural mutations), ADR-087 (request-time authorization with aggregate OCC), [ADR-104](../adr/ADR-104-checkpointed-client-projection-snapshots.md) (checkpointed client projections)
- **FDRs:** FDR-001 (Roles & Permissions), FDR-007 (Direct Messages), FDR-019 (Room Lifecycle)
