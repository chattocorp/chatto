# ADR-116: Roles Only Grant Permissions

**Date:** 2026-10-08

## Status

Accepted. Supersedes the subject rules and the `everyone` baseline of
[ADR-052](ADR-052-subject-specific-rbac-with-everyone-baseline.md). The scopes
and the owner override of ADR-052 and
[ADR-040](ADR-040-permission-only-rbac-with-owner-override.md) stay.

## Context

ADR-052 let every subject allow or deny. The user and each named role gave
their nearest decision, and a deny from any of them won. An `everyone` deny
lost only to a named allow at the same or a nearer scope. Operators had to
apply four rules to explain one result:

- the nearest scope wins for one subject;
- a deny wins across subjects;
- `everyone` is a baseline with its own scope rule;
- a role can add or remove access.

The consequences were hard to predict. Giving a person a role could remove
access, and a server-level deny on a restriction role beat a room-level allow
on a different role. Restriction roles, such as `suspended`, also had to rank
below their managers (ADR-115) to work at all. The planned suspension feature
will restrict accounts directly, so restriction roles are no longer needed.

## Decision

All roles, `everyone` included, only allow permissions. Only single users can
deny, at any scope. Resolve a known permission for a human in this order:

1. Permissions that do not apply in direct messages are denied there.
2. Effective owners in privileged mode are allowed (ADR-105).
3. An allow of an including permission allows the included permission, also
   when the user denies the included permission. To stop the included
   permission, deny the including permission too.
4. **A deny on the user decides.** When the user's nearest setting (room, then
   room group, then server, or Direct messages, then server) is a deny, the
   result is deny.
5. **Otherwise, any allow decides.** An allow of the user, of a role, or of
   `everyone` at any applicable scope gives access.
6. No allow means no access.

Elevation-required permissions still need privileged mode (ADR-105). Scope
order matters only for the user's own settings. Between subjects, it does not
matter. Rank (ADR-115) does not change resolution.

The API rejects a deny for any role, `everyone` included, with
`INVALID_ARGUMENT`. Stored role denies have no effect, and role reads leave
them out. `AdminRole.permission_denials`, `TierPermissions.permission_denials`,
`TierRole.inherited_denials`, and the core helper `DenyServerPermission` are removed. At
startup, the server logs a warning with the number of ignored role denies and
the IDs of the affected rooms and room groups. It does not change or delete
the denies.

A Server-scope allow reaches every room, so new servers start closed:

- `everyone` gets only `user.delete-self` at Server scope.
- `everyone` gets `message.read`, `message.post`, `message.attach`,
  `message.react`, `message.echo`, and the `call.*` permissions at Direct
  messages scope, so direct messages work.
- `admin` gets no Server-scope `room.list` or `room.join`. Admins reach rooms
  like other members, or with `room.manage` in privileged mode.
- The seeded `#general` room is open to `everyone` at Room scope: `room.list`,
  `room.join`, `message.read`, `message.post`, `message.attach`,
  `message.react`, `message.echo`, and the `call.*` permissions.
- The seeded universal `#announcements` room is open to `everyone` at Room
  scope for `room.list`, `room.join`, `message.read`, `message.react`, and
  `message.post-in-thread`. `admin` gets `message.post` there.
- The seeded `Lobby` room group has no `everyone` allows.

Every other new room and room group starts without `everyone` allows. The
operator opens it. To show the result, the room and room-group settings pages
show an access summary from `AdminPermissionService.GetAccessSummary`. The
server permissions page warns that its settings reach every room.

Upgraded servers keep their stored settings. There is no migration.

Bots are unchanged: they hold no roles and do not inherit `everyone`. Their
allowlist is capped by their owner (FDR-038).

## Consequences

- Giving a person a role never removes access. One rule explains every
  result: a user deny, else any allow.
- A setting on the user is the only exception tool. A deny on the user wins
  over every role allow, except an allow of an including permission.
- Private rooms work by omission: do not allow room access for `everyone` at
  a scope that reaches the room, and allow it for a role at the room or its
  room group. A Server-scope allow, of `everyone` or of a role, opens every
  room for its holders.
- The announcements room has no deny: `everyone` is not allowed
  `message.post` there, and `admin` is.
- Restriction roles no longer work. Until account suspension exists, restrict
  an account with denies on the user.
- On servers upgraded from 0.4 or from an earlier 0.5 beta, every role deny
  stops having an effect at once. A room that was private only through an
  `everyone` deny becomes open to everyone who has the Server-scope allows.
  We accept this exposure. The upgrade notes tell operators to make such rooms
  private the new way before they upgrade, and the startup log lists the
  affected rooms and room groups.
- Operators must open each new room and room group. The access summary warns
  when only owners and explicitly added members can open a room.
- Delegated role assignment, revocation, and deletion need only the
  permissions that the role allows (ADR-115). Role denies no longer count.
- The permission explainer, the permission matrices, and the access summary
  use the same resolver as authorization, so they show the same result.
