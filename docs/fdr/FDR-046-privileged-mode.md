# FDR-046: Privileged Mode

**Status:** Active
**Last reviewed:** 2026-11-19

## Overview

Privileged mode keeps additional administrative and moderation permissions off
during ordinary use. A user explicitly activates these permissions for one
server session when they need them.

## Behavior

- The control appears in the current-user area of the selected server when the
  user has an assigned protected role or an active session deadline.
- Settings Overview shows a notice inside Server configuration when the mode is available but inactive. It directs the user to the shield button beside their name to activate additional permissions. The notice disappears while the mode is active.
- Activating the mode requires a confirmation. One exception: after first-run
  setup, the client activates the mode for the new owner without a
  confirmation and explains it in a welcome dialog (FDR-047).
- The dialog lists the user's assigned protected roles. The user selects the roles to activate. Direct account grants apply at their configured scopes during ordinary use and during Shield Mode.
- A protected role contributes its grants only while selected. Its denies always apply during normal permission resolution. Grants from unprotected roles, including `everyone`, apply at their configured scopes.
- Selecting `owner` activates the owner override. Without
  the mode, an owner has only the permissions of their other roles, direct
  grants, and `everyone`. Thus an owner sees, joins, and reads a restricted
  room only while the mode is active, unless ordinary grants allow it.
- The activation lasts for 15 minutes and does not extend when the user takes
  an action.
- The user can deactivate the mode until its deadline, even if all protected role assignments have been removed.
- Expiry, logout, and session revocation deactivate the mode.
- The client updates effective server permissions from the activation or
  deactivation response. It keeps its resume cursor and mounted realtime
  projection. After resume, ConnectRPC resource reads update effective room
  permissions in place before catch-up completes.
- Rapid completed changes keep realtime delivery usable when the client is
  current. A newer change replaces pending recovery and waits for the latest
  effective permissions.
- At the 15-minute deadline, the server closes each affected realtime
  connection with a reconnect instruction. The resumed subscription replaces
  effective permissions with privileged mode inactive. The server does not
  write events that it authorized for the active mode after the deadline.
- The client reads rooms and room groups again after activation,
  deactivation, and expiry. Rooms that the owner override made visible appear
  or disappear.
- A successful activation or deactivation resume from a caught-up live
  connection reuses the response's effective viewer permissions. It reads only
  missing DM profiles and reloads timelines with changed message permissions.
  It does not refresh the cached user directory. Snapshots, expiry, and
  interrupted or failed recovery use the full current-value refresh.
- An owner can stay an explicit member of a room after the mode ends but lose
  read access. Explicit memberships do not change.
- Realtime delivery evaluates each session with its own mode state. Two
  sessions of one owner with different states receive different room events.
- Notifications are not bound to one session. They use the owner's view
  without the mode.
- When one connection of a session ends the mode, the other realtime
  connections of that session reconnect within one credential check interval.
- A call connection keeps the mode state of the request that issued its
  token. Other call participants cannot see which roles the user selected. After that deadline, the next call reconciliation applies ordinary
  RBAC to the owner.
- Asset URLs that Chatto issued during the mode stay usable until their access
  tickets expire.
- The event log records successful activation and explicit deactivation
  transitions. The activation entry includes the selected role names and the fixed deadline. Automatic
  expiry does not add a second event because the deadline is already durable.
- Each connected server has independent state.
- Bot API keys keep their current direct permission behavior. Bots do not use
  privileged mode. Their owner ceiling uses current RBAC entitlement, independent
  of human session activation. A bot manager can grant permissions from the owner's protected roles even while Shield Mode is off.
- The admin entry point remains visible to entitled users while the mode is
  inactive. Protected capabilities and actions remain unavailable.
- The Moderation link and page require effective server-wide
  `room.remove-member`. Deactivation or permission loss hides the link and removes
  an open Moderation page. Direct access shows an access-denied screen.
- A user can edit or delete their own message without privileged mode. Editing
  or deleting another user's message requires active `message.manage`.

System diagnostics require the selected `owner` role.

## Design Decisions

### 1. Select protected roles

A user with effective `role.manage` can change **Require Shield Mode** on a role. The setting is fixed on for `owner` and off for `everyone`. The built-in `admin` and `moderator` roles start protected. Custom roles start unprotected unless the creator enables protection. Existing custom roles start unprotected.

Role selection lets a user moderate a room while leaving broader administration or owner access inactive. Each permission check still uses the user's current assignments and the role's current settings.

### 2. Enforce the mode on the server

**Decision:** Effective request authorization checks the authenticated session
state. The client uses the returned effective grants only as UI hints.

**Why:** A hidden client action is not an authorization boundary. Other API
clients must follow the same rule.

**Tradeoff:** The legacy activation command selects all assigned protected roles when it starts a window. A retry during that window keeps the stored selection and deadline, even if the user’s assigned roles have changed.

A role-picker retry checks the current assignments again. If a picked role has been revoked, the retry returns `PERMISSION_DENIED` when another protected role is available or `FAILED_PRECONDITION` when none are available. The session keeps its original selection and deadline. The revoked role grants no permissions while it is unassigned, and the user can still deactivate Shield Mode.

### 3. Use one fixed session window

**Decision:** One confirmation starts a fixed 15-minute window for the current
server session. Use does not extend it.

**Why:** The state is easy to understand and limits unattended authority.

**Tradeoff:** A long administration task can require another activation.

### 4. Gate the owner override

**Decision:** Privileged mode also gates the effective-owner override. Owners
keep their entitlement for discovery, delegation ceilings, and bot ceilings.

**Why:** Access to rooms that RBAC restricts is also assigned authority. An
owner must not read these rooms during ordinary use without an audited
activation. See ADR-105.

**Tradeoff:** On a fresh server, the first owner has only the `owner` role.
The announcements room denies root posts to `everyone`, so this owner must
activate the mode to post there, or receive a direct room allow or an allow through an unprotected role. MCP
tools cannot activate the mode, so they act without the owner override.

### 5. Keep runtime authority out of EVT

**Decision:** Activation is mutable runtime credential state. Minimal
activation and explicit deactivation facts are also written to EVT for audit.
These facts do not restore or change authority during replay.

**Why:** The state has the same lifecycle as the session and must disappear
with session revocation. Privileged domain actions already create their normal
audit facts.

**Tradeoff:** An EVT outage cannot prevent deactivation. If the runtime-state
change succeeds and the audit append fails, Chatto keeps the safer runtime
result and logs the audit failure.

### 6. Limit explicit permission refreshes

**Decision:** A successful resume after an explicit mode change reads rooms
and room groups and reuses viewer permissions from the mutation response.
It hydrates missing DM profiles and timelines with changed message permissions.
All other recovery paths keep the full current-value refresh.

**Why:** A permission change must not require reads for every cached user or
reload message history whose permissions did not change.

**Tradeoff:** This step keeps the reconnect. A transient change during its short
gap can remain stale until a later event or full recovery.

## Compatibility

- The viewer reports a role selection even when the list is empty. Older servers omit the selection. The client uses all-role activation only when the viewer omits the selection or the caller explicitly requests legacy activation.
- Clients that support selection send the chosen role names to `ActivatePrivilegedRoles`. Servers without this RPC reject the selection. Older replicas cannot use a selective activation to enable the owner override.
- All replicas must run the new authorization code before an operator relies
  on the gate.

## Related

- **ADRs:** ADR-040, ADR-052, ADR-079, ADR-081, ADR-087, ADR-096, ADR-105
- **FDRs:** FDR-001 (Roles & Permissions), FDR-004 (Message Editing &
  Deletion), FDR-021 (Admin Dashboard), FDR-023 (Authentication & Sessions),
  FDR-031 (Client–Server Compatibility Discovery), FDR-038 (Bot Accounts)
