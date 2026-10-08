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
startup, the server logs a warning with the number of ignored role denies, the
IDs of the affected rooms and room groups, and each deny as
`role:permission@scope` or `role:permission@scope:id`. Each list shows at most
50 entries. The server does not change or delete the denies.

A Server-scope allow reaches every room, so new servers start closed:

- `everyone` gets only `user.delete-self` at Server scope.
- `everyone` gets `message.read`, `message.post`, `message.attach`,
  `message.react`, `message.echo`, and the `call.*` permissions at Direct
  messages scope, so direct messages work.
- `admin` gets no Server-scope `room.list` or `room.join`. Admins do not see
  or join a closed room. In privileged mode, their Server-scope `room.manage`
  lets them open the room. Adding members, also themselves, does not open it:
  members also need `message.read`.
- The seeded `#general` room is open to `everyone` at Room scope: `room.list`,
  `room.join`, `message.read`, `message.post`, `message.attach`,
  `message.react`, `message.echo`, and the `call.*` permissions.
- The seeded universal `#announcements` room is open to `everyone` at Room
  scope for `room.list`, `room.join`, `message.read`, `message.react`, and
  `message.post-in-thread`. `admin` gets `message.post` and `message.attach`
  there.
- The seeded `Lobby` room group has no `everyone` allows.

In builds with the `bootstrap` tag, rooms from `[bootstrap.server] rooms` in
`chatto.toml` get the `#general` allows. Every other new room and room group
starts without `everyone` allows, also rooms from `chatto operator room
create`. The operator opens it. `room.create` alone does not let a user open
the rooms that they create, and the creator gets no automatic allow.

To change a role or user decision, a non-owner must hold the permission at
that scope (ADR-115). One exception lets room managers open rooms: for a role
decision at Room or Room group scope, `room.manage` there is enough for each
room permission that does not need privileged mode, such as `room.list`,
`room.join`, `message.read`, `message.post`, and the `call.*` permissions. The
exception gives access only to rooms that the actor already manages, and a
role allow never overrides a deny on a user.
Admins can thus open every new room and room group, and a room-group manager
can open the rooms of the group. Permissions that need privileged mode, such
as `room.manage`, `room.remove-member`, `message.manage`, and `room.create`,
still need the actor to hold them. The exception also applies to the bounds
of role assignment, revocation, and deletion, and to the cells that the role
permission matrices let the viewer change.

The exception does not apply to decisions on a single user or to bot grants.
An allow on a user can lift a deny on that user. Without this limit, an admin
with a deny on `message.post` could allow `message.post` for themselves in a
room that they manage. To change a user decision or a bot grant, a non-owner
must hold the permission. The owner ceiling of bots does not change.

To show the result, the room and room-group settings pages show an access
summary from `AdminPermissionService.GetAccessSummary`. It resolves
`room.list`, `room.join`, and `message.read`. A role counts as able to join
only when its holders can join and read. The summary warns when everyone can
join but not read. The server permissions page warns that its settings reach
every room.

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
  when nobody can find, join, or read a room, except owners in privileged
  mode. Room membership alone does not give access: a member without a
  `message.read` allow sees the room but cannot read it, and posting also
  needs `message.post`. After a user creates a room in the sidebar, the app
  opens the room's settings, where the summary shows. The creator joins the
  room only when it lets them.
- A user who must create usable rooms needs `room.manage` in addition to
  `room.create` in the room group. With `room.create` alone, their rooms stay
  closed.
- Delegated role assignment, revocation, and deletion need only the
  permissions that the role allows, or `room.manage` for the room permissions
  of the exception above (ADR-115). Role denies no longer count.
- A room manager can open a room for roles and `everyone`, but cannot give a
  single user or a bot a permission that the manager does not hold.
- The permission explainer, the permission matrices, and the access summary
  use the same resolver as authorization, so they show the same result.
