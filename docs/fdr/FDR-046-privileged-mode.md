# FDR-046: Privileged Mode

**Status:** Active
**Last reviewed:** 2026-10-07

## Overview

Privileged mode keeps additional administrative and moderation permissions off
during ordinary use. A user explicitly activates these permissions for one
server session when they need them.

## Behavior

- The control appears in the current-user area of the selected server when the
  user has an elevation-required permission entitlement. This includes owner
  entitlement and explicit allows at any scope. A deny can still prevent an
  explicit allow from becoming effective.
- Activating the mode requires a confirmation.
- The mode activates all elevation-required permissions that the user is
  entitled to use. It does not activate a role and does not add a grant.
- For an effective owner, the mode also activates the owner override. Without
  the mode, an owner has only the permissions of their other roles, direct
  grants, and `everyone`. Thus an owner sees, joins, and reads a restricted
  room only while the mode is active, unless ordinary grants allow it.
- The activation lasts for 15 minutes and does not extend when the user takes
  an action.
- The user can deactivate the mode immediately.
- Expiry, logout, and session revocation deactivate the mode.
- The client updates effective server permissions from the activation or
  deactivation response. It keeps its connection, resume cursor, and mounted
  state. The server acknowledges the adopted session mode on that connection;
  ConnectRPC reads then update effective room permissions and call visibility.
- At the 15-minute deadline, the server adopts inactive mode on each affected
  realtime connection and acknowledges it without closing the socket. The
  server does not write events authorized for active mode after the deadline.
- The client reads rooms and room groups again after activation,
  deactivation, and expiry. Rooms that the owner override made visible appear
  or disappear.
- A successful activation or deactivation on a live connection reuses the
  response's effective viewer permissions. It reads only
  missing DM profiles and reloads timelines with changed message permissions.
  It does not refresh the cached user directory. Expiry and other tabs read
  viewer permissions too. Snapshots, lost acknowledgements, and interrupted
  or failed recovery use the full current-value refresh.
- An owner can stay an explicit member of a room after the mode ends but lose
  read access. Explicit memberships do not change.
- Realtime delivery evaluates each session with its own mode state. Two
  sessions of one owner with different states receive different room events.
- Notifications are not bound to one session. They use the owner's view
  without the mode.
- When one connection of a session ends the mode, the other realtime
  connections adopt the change through runtime-state notifications. Independent
  sessions of the same user keep their own mode. The periodic credential check
  remains a fallback.
- A call connection keeps the mode state of the request that issued its
  token. After that deadline, the next call reconciliation applies ordinary
  RBAC to the owner.
- Asset URLs that Chatto issued during the mode stay usable until their access
  tickets expire.
- The event log records successful activation and explicit deactivation
  transitions. The activation entry includes the fixed deadline. Automatic
  expiry does not add a second event because the deadline is already durable.
- Each connected server has independent state.
- Bot API keys keep their current direct permission behavior. Bots do not use
  privileged mode. Their owner ceiling uses current RBAC entitlement, independent
  of human session activation. Granting an elevation-required permission to a
  bot requires the acting human to have it active at the target scope. Clearing
  a grant does not require activation of that permission.
- The admin entry point remains visible to entitled users while the mode is
  inactive. Protected capabilities and actions remain unavailable.
- The Moderation link and page require effective server-wide
  `room.remove-member`. Deactivation or permission loss hides the link and removes
  an open Moderation page. Direct access shows an access-denied screen.
- A user can edit or delete their own message without privileged mode. Editing
  or deleting another user's message requires active `message.manage`.

## Elevation-Required Permissions

The initial catalog requires privileged mode for these permissions:

- `server.manage` and `server.manage-neighbors`
- `room.create`, `room.manage`, and `room.remove-member`
- `message.manage`
- `role.manage` and `role.assign`
- `admin.view-users` and `admin.view-audit`
- `user.invite`, `user.delete-any`, `user.manage-accounts`, and
  `user.manage-permissions`
- `bot.manage`
- owner-only system diagnostics

Ordinary room listing, joining, reading, posting, thread posting, attachments,
reactions, message echo, `user.delete-self`, and `bot.create` remain active for
users who have them through roles or grants. An owner who has them only through
the owner override needs active privileged mode.

## Design Decisions

### 1. Activate permissions, not roles

**Decision:** The server classifies permissions that require activation.

**Why:** Custom roles and direct user grants can provide the same authority as
the built-in owner, admin, and moderator roles.

**Tradeoff:** The catalog must classify each new dangerous permission.

### 2. Enforce the mode on the server

**Decision:** Effective request authorization checks the authenticated session
state. The client uses the returned effective grants only as UI hints.

**Why:** A hidden client action is not an authorization boundary. Other API
clients must follow the same rule.

**Tradeoff:** Old clients cannot use elevated permissions on a new server.

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
activate the mode to post there, or get the `admin` role or a room allow. MCP
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

**Decision:** A mode change keeps the existing socket. The server refreshes its
session authority, recovers durable events, and acknowledges the adopted mode.
The client reads rooms, room groups, and active calls, and reuses viewer
permissions from an explicit mutation response.
It hydrates missing DM profiles and timelines with changed message permissions.
All other recovery paths keep the full current-value refresh.

**Why:** A permission change must not require reads for every cached user or
reload message history whose permissions did not change.

**Tradeoff:** Durable events committed during the internal subscription handoff
are replayed. Transient presence or typing changes during that short gap can
remain stale until a later update. A failed handoff uses reconnect recovery.

## Compatibility

- A new client with an old server does not show the control.
- An old client with a new server cannot activate elevated permissions.
- All replicas must run the new authorization code before an operator relies
  on the gate.

## Related

- **ADRs:** ADR-040, ADR-052, ADR-079, ADR-081, ADR-087, ADR-096, ADR-105
- **FDRs:** FDR-001 (Roles & Permissions), FDR-004 (Message Editing &
  Deletion), FDR-021 (Admin Dashboard), FDR-023 (Authentication & Sessions),
  FDR-031 (Client–Server Compatibility Discovery), FDR-038 (Bot Accounts)
