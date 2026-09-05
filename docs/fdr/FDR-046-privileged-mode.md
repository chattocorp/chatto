# FDR-046: Privileged Mode

**Status:** Active
**Last reviewed:** 2026-09-05

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
- The activation lasts for 15 minutes and does not extend when the user takes
  an action.
- The user can deactivate the mode immediately.
- Expiry, logout, and session revocation deactivate the mode.
- The client updates effective server permissions from the activation or
  deactivation response. It keeps its resume cursor and mounted realtime
  projection. After resume, ConnectRPC resource reads update effective room
  permissions in place before catch-up completes.
- At the 15-minute deadline, the server closes each affected realtime
  connection with a reconnect instruction. The resumed subscription replaces
  effective permissions with privileged mode inactive.
- The event log records successful activation and explicit deactivation
  transitions. The activation entry includes the fixed deadline. Automatic
  expiry does not add a second event because the deadline is already durable.
- Each connected server has independent state.
- Bot API keys keep their current direct permission behavior. Bots do not use
  privileged mode.
- The admin entry point remains visible to entitled users while the mode is
  inactive. Protected capabilities and actions remain unavailable.
- The Moderation link and page require effective server-wide
  `room.ban-member`. Deactivation or permission loss hides the link and removes
  an open Moderation page. Direct access shows an access-denied screen.
- A user can edit or delete their own message without privileged mode. Editing
  or deleting another user's message requires active `message.manage`.

## Elevation-Required Permissions

The initial catalog requires privileged mode for these permissions:

- `server.manage` and `server.manage-neighbors`
- `room.create`, `room.manage`, and `room.ban-member`
- `message.manage`
- `role.manage` and `role.assign`
- `admin.view-users` and `admin.view-audit`
- `user.invite`, `user.delete-any`, `user.manage-accounts`, and
  `user.manage-permissions`
- `bot.manage`
- owner-only system diagnostics

Ordinary room listing, joining, reading, posting, thread posting, attachments,
reactions, message echo, `user.delete-self`, and `bot.create` remain active.

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

### 4. Keep runtime authority out of EVT

**Decision:** Activation is mutable runtime credential state. Minimal
activation and explicit deactivation facts are also written to EVT for audit.
These facts do not restore or change authority during replay.

**Why:** The state has the same lifecycle as the session and must disappear
with session revocation. Privileged domain actions already create their normal
audit facts.

**Tradeoff:** An EVT outage cannot prevent deactivation. If the runtime-state
change succeeds and the audit append fails, Chatto keeps the safer runtime
result and logs the audit failure.

## Compatibility

- A new client with an old server does not show the control.
- An old client with a new server cannot activate elevated permissions.
- All replicas must run the new authorization code before an operator relies
  on the gate.

## Related

- **ADRs:** ADR-040, ADR-052, ADR-079, ADR-081, ADR-087, ADR-096
- **FDRs:** FDR-001 (Roles & Permissions), FDR-004 (Message Editing &
  Deletion), FDR-021 (Admin Dashboard), FDR-023 (Authentication & Sessions),
  FDR-031 (Client–Server Compatibility Discovery), FDR-038 (Bot Accounts)
