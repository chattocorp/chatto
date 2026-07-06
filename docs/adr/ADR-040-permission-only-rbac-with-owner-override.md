# ADR-040: Permission-Only RBAC with Owner Override

**Date:** 2026-06-15

## Context

Chatto's earlier RBAC resolver used role position as part of authorization:
higher-ranked role decisions could override lower-ranked decisions, and many
targeted operations required both a permission and a strict actor-vs-target
rank comparison. That model fit the older multi-space design, but it became
hard to explain and easy to misapply in the current single-server model.

The main pressure points were:

- operators had to understand when rank affected capability and when it only
  affected display order;
- direct per-user permission editing was coupled to role-management concepts;
- announcement rooms depended on hierarchy behavior instead of a simple local
  exception to broad member defaults;
- owners could be locked out by unusual RBAC configuration unless runtime code
  treated effective owners specially.

## Decision

Use a permission-only RBAC model for everyone except effective owners.

- Effective owners are users with the durable `owner` role or a verified email
  matching `owners.emails` in Chatto configuration. Owners are always granted
  all permissions regardless of stored allow/deny state.
- For non-owners, permission resolution is deny-wins: any applicable user or
  role deny blocks the permission; otherwise any applicable allow grants it;
  otherwise the result is no decision and the API treats it as denied.
- Role position remains as ordering/display metadata and for compatibility with
  existing role events. It is not an authorization rank.
- Targeted operations are gated by concrete permissions only: for example
  `role.assign` gates role assignment, `user.manage-accounts` gates account
  lifecycle and recovery actions, `room.ban-member` gates room bans, and
  `user.manage-permissions` gates direct per-user permission overrides.
- Default channel-room member permissions are granted at server scope on
  `everyone`, so normal rooms work immediately. Room and group decisions are
  local exceptions; the built-in announcements room adds a room-level
  `everyone` deny for `message.post`.

This supersedes ADR-005.

## Consequences

- The authorization model is easier to reason about: owners are special and
  everyone else is permission-based.
- Custom roles and per-user overrides cover ordinary moderation and
  administration cases without role-rank comparisons.
- `#announcements` works by a local room-level `everyone` deny for
  `message.post`. Because deny-wins is literal, that deny blocks every
  non-owner in the room.
- Deny-wins enables future broad restriction roles such as a suspended role.
- Operators cannot lock out effective owners through RBAC state, but owner
  access now depends on protecting `owners.emails` configuration and verified
  email ownership.
- Existing role position fields and protobuf event fields remain for
  compatibility. Removing or reserving them can be considered separately if the
  persisted event contract is migrated.
