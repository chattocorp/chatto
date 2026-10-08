# ADR-116: Roles Only Grant Permissions

**Date:** 2026-10-08

## Status

Accepted. Supersedes the subject rules of
[ADR-052](ADR-052-subject-specific-rbac-with-everyone-baseline.md). The scope
rules, the `everyone` baseline, and the owner override of ADR-052 and
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

Roles only grant permissions. Resolve a known permission for a human in this
order:

1. Effective owners in privileged mode are allowed (ADR-105).
2. Permissions that do not apply in direct messages are denied there.
3. **A deny on the user decides.** When the user's nearest setting (room, then
   room group, then server, or Direct messages, then server) is a deny, the
   result is deny.
4. **Otherwise, allows and `everyone` decide.** An allow of the user or of a
   role wins when it is at the same scope as the nearest `everyone` setting, or
   at a more specific one. Otherwise, the nearest `everyone` setting decides.
5. No setting means no access.

An allow of an including permission still allows the included permission,
also when the user denies the included permission. To stop the included
permission, deny the including permission too. Elevation-required permissions
still need privileged mode (ADR-105).

Only `everyone` and single users can deny. `everyone` can deny only at Room
group, Room, and Direct messages scope, where the deny takes away what
`everyone` gets at a broader scope. At Server scope, an `everyone` deny means
the same as no setting, because every allow is at the same or a more specific
scope. The API rejects a deny for a named role, and an `everyone` deny at
Server scope, with `INVALID_ARGUMENT`. Stored denies of these kinds have no
effect, and role reads leave them out. `AdminRole.permission_denials`, which
held only Server-scope denies, is removed. At startup, the server logs how
many named-role denies it ignores. It does not change or delete them.

Bots are unchanged: they hold no roles and do not inherit `everyone`. Their
allowlist is capped by their owner (FDR-038).

## Consequences

- Giving a person a role never removes access.
- A setting on the user is the exception tool. A deny on the user wins over
  every role. An allow on the user follows the same scope rule as a role
  allow, so a server-level user allow does not open a room that denies
  `everyone`; use a room-level user allow for that. This also keeps a
  delegated permission manager from granting access to rooms that they cannot
  enter: the grant limit (ADR-115) checks the manager's authority only at the
  scope of the setting.
- Room allowlists work as before: deny `everyone` in the room and allow the
  role there. A server-level role allow does not open the room.
- The announcements room keeps its behavior: `everyone` is denied
  `message.post` in the room, and `admin` is allowed it there.
- Restriction roles no longer work. Until account suspension exists, restrict
  an account with denies on the user.
- Servers upgraded from 0.4 or from an earlier 0.5 beta lose the effect of
  their role denies at once. The startup log reports how many there are.
- Delegated role assignment, revocation, and deletion need only the
  permissions that the role allows (ADR-115). Role denies no longer count.
- The permission explainer and the permission matrices use the same resolver
  as authorization, so they show the same result.
