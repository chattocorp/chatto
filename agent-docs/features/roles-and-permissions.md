# Roles & Permissions (RBAC)

## Overview

Chatto uses role-based access control with a **single flat tier of
server roles** stored in `SERVER_RBAC`. The earlier instance-vs-space
two-tier split is gone after Phase 5 of #330. Per-room overrides
provide additional granularity on top of the flat role tier.

## Server Roles

The system roles are (highest rank first):

- **Owner** — Full server control. Top of the hierarchy. Holders pass
  every permission check and can never be demoted by an admin.
- **Admin** — Full administrative access except managing owner-rank
  users.
- **Moderator** — Moderation permissions without administrative reach.
- **Everyone** — Virtual role assigned to every authenticated user.
  Default-permission grants attach here.

Server admins can create **custom roles** that sit between the system
roles in the hierarchy. Custom roles can be reordered via
drag-and-drop.

## Permission Resolution

Permissions follow a **hierarchy-wins** model:

1. The user's roles are checked in rank order (lowest position number
   = highest rank, checked first).
2. The first explicit grant or deny found wins.
3. Denying a permission on the `everyone` role does NOT block
   higher-ranked roles.

For example: if `everyone` is denied `message.post` but `admin` is
granted it, admins can still post. This enables patterns like
read-only announcement channels where only certain roles can post,
while everyone retains `message.post-in-thread` to discuss in threads.

## Room-Level Overrides

Server admins can override any permission for any role in a specific
room:

- **Grant**: Allow a permission that's denied at the role level
- **Deny**: Block a permission that's granted at the role level
- **Clear**: Remove the override, falling back to the role default

Scope cascade: room > role default (more specific scopes win).

## Config-Designated Owners

Operators can designate owners via `owners.emails` in `chatto.toml`.
On email verification (registration / OAuth / admin-direct add),
matching users are auto-assigned the `owner` role. Existing
deployments can run `chatto reset rbac` after upgrading to re-seed
system roles and re-assign owners.

## Role Management

- Creating, editing, and deleting roles requires the `roles.manage`
  permission.
- Assigning roles to users requires the `roles.assign` permission.
- Users cannot assign or revoke roles equal to or higher than their
  own rank.
- System roles cannot be deleted. Custom roles can be deleted, which
  cascades to remove all assignments and permission grants.
