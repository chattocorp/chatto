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
3. **A setting on the user decides.** The user's nearest setting (room, then
   room group, then server, or Direct messages, then server) is final, allow
   or deny.
4. **Otherwise, roles and `everyone` decide.** A role allow wins when it is at
   the same scope as the nearest `everyone` setting, or at a more specific one.
   Otherwise, the nearest `everyone` setting decides.
5. No setting means no access.

An allow of an including permission still allows the included permission, and
elevation-required permissions still need privileged mode (ADR-105).

Only `everyone` and single users can deny. The API rejects a deny for a named
role with `INVALID_ARGUMENT`. Stored denies of named roles have no effect.
Role reads, such as `AdminRole.permission_denials`, leave them out. At startup,
the server logs how many such denies it ignores. It does not change or delete
them.

Bots are unchanged: they hold no roles and do not inherit `everyone`. Their
allowlist is capped by their owner (FDR-038).

## Consequences

- Giving a person a role never removes access.
- A setting on the user is the exception tool: it can deny one person a
  permission that their roles allow, or allow what `everyone` denies. A
  server-level user allow also applies in rooms that deny `everyone`. Use a
  room-level user setting for an exception in one room.
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
