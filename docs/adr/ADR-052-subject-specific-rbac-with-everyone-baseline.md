# ADR-052: Subject-Specific RBAC with an Everyone Baseline

**Date:** 2026-07-19

## Context

ADR-040 combined every applicable decision with literal deny-wins semantics.
Because every authenticated user implicitly carries `everyone`, denying a
permission on `everyone` also denied it to admins and to custom roles that
explicitly granted it. A common policy such as "only the Engineering role can
see and join this room" was therefore impossible to express with a local
`everyone` deny and an Engineering allow.

The model also combined server, group, and room decisions for the same subject.
That contradicted the permission editor's inheritance model: a room allow did
not actually replace the same role's group or server deny. Operators had to
reason about every stored decision rather than the value visible at the nearest
scope.

We still need restriction roles such as `suspended` to be reliable, and role
position must remain display metadata rather than an authorization rank.

## Decision

Resolve each known permission in this order:

1. Effective owners are allowed.
2. For non-owners in DMs, fixed category and privacy restrictions are applied.
3. The direct user and each explicitly assigned named role contribute at most
   one decision: their nearest explicit value at room, group, or server scope.
4. Combine those direct-user and named-role decisions with deny-wins. Any deny
   blocks; otherwise any allow grants.
5. Only when none of those subjects decides, consult the implicit `everyone`
   role's nearest decision as the baseline.
6. If nothing decides, the API boundary denies the action. The small set of
   participant actions that DMs allow by default remains unchanged.

Scope specificity is evaluated independently per subject. A room decision for
Engineering replaces Engineering's group and server decisions for that room;
it does not erase a decision from a different named role. Direct-user decisions
participate alongside named roles rather than having a separate precedence
rank.

Examples:

- `admin: allow` plus `everyone: deny` resolves to allow.
- `admin: allow` plus `suspended: deny` resolves to deny.
- Engineering's room allow replaces Engineering's server deny in that room.
- With no named or direct-user decision, `everyone: deny` resolves to deny.

This supersedes ADR-040's literal all-subject, all-scope deny-wins combination
rule. ADR-040's owner override, permission-only authorization, and non-ranking
role positions remain active.

## Consequences

- Role allowlists are expressible: deny `room.list` and `room.join` for
  `everyone`, then allow them for the selected named role.
- A restriction role remains effective because its deny is combined with all
  other named roles and direct-user decisions.
- Room and group values genuinely override less-specific values for the same
  subject, matching the UI's inheritance model.
- The built-in announcements room's `everyone.message.post` deny blocks normal
  members, while named roles with their own posting grant can post. Owners
  remain allowed virtually.
- The permission explainer can show the ignored `everyone` baseline alongside
  the winning named decision. An ignored deny is therefore not necessarily the
  winning trace row.

## Compatibility and migration

This is a behavioral authorization change, not a protobuf or persisted-event
schema change. Existing EVT permission facts and projections replay unchanged;
no data migration or generated-client update is required.

Upgrading can widen access where stored state combines an `everyone` deny with
a direct-user or named-role allow, or where a nearer allow exists above a
less-specific deny for the same subject. Operators should inspect those
conflicts before upgrading if they previously relied on the broader deny. To
preserve a hard restriction, attach the deny directly to the user or to an
explicit restriction role such as `suspended`.

Mixed versions accept the same API messages but can return different effective
decisions. A new client talking to an old server cannot assume baseline
semantics; an old client talking to a new server receives the new server's
authorization result. No capability flag is added because clients do not
resolve permissions locally, but release notes must call out the behavior
change and the operator audit above.
