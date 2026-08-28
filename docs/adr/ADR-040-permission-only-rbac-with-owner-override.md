# ADR-040: Permission-Only RBAC with Owner Override

**Date:** 2026-06-15

**Status:** Partially superseded

> **Amended 2026-08-11:** Configured owner emails now converge on the durable
> `owner` role instead of acting as a separate permission-time fallback. This
> keeps live authorization and event-time visibility on one representation.
>
> **Amended 2026-08-26:** Permission identifiers can contain more than two
> dot-separated components. A dotted prefix has no automatic meaning.
> Permission inclusion must be explicit.
>
> **Amended 2026-08-28:** This amendment supersedes the permission-name rule
> from 2026-08-26. Registered dotted prefixes now define transitive permission
> inclusion.
>
> **Amended 2026-08-28:** The 0.5 permission catalog now uses the inclusion
> rule consistently. This is a breaking replacement without aliases or event
> migration. Old permission decisions remain in event history but have no
> effect. Existing grants for a retained parent include its new descendants.
>
> **Partially superseded by [ADR-052](ADR-052-subject-specific-rbac-with-everyone-baseline.md).**
> The effective-owner override, permission-only gates, and non-ranking role
> positions remain active. ADR-052 replaces the literal all-subject,
> all-scope deny-wins combination rule.

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

- Effective owners are users with the durable `owner` role. A verified email
  matching `owners.emails` is materialized into that role at boot and through a
  retryable durable worker after verification; verification waits for the
  materialization before reporting success. Owners are always granted all
  permissions regardless of stored allow/deny state.
- Every other role, including `admin`, confers only its explicit permission
  decisions. Runtime code does not attach additional authority to role names.
- For non-owners, permission resolution is deny-wins: any applicable user or
  role deny blocks the permission; otherwise any applicable allow grants it;
  otherwise the result is no decision and the API treats it as denied.
- Role position remains as ordering/display metadata and for compatibility with
  existing role events. It is not an authorization rank.
- Targeted operations are gated by concrete permissions only: for example
  `role.manage.assignments` gates role assignment, `user.manage` gates account
  lifecycle and recovery actions, `room.manage.bans` gates room bans, and
  `user.manage.permissions` gates direct per-user permission overrides.
- Authorization-sensitive writes normally evaluate permission checks inside
  their target aggregate's OCC retry. RBAC, relevant user lifecycle, and
  room-group/layout changes advance a narrow durable authorization fence
  atomically with their domain facts. Message posts and authorized edits check
  that fence without advancing it, so a concurrent classified authority change
  reruns their complete decision. Reactions and message retractions use room
  OCC with request-time authorization and accept eventual consistency for a
  cross-aggregate revocation already in flight.
- Default channel-room member permissions are granted at server scope on
  `everyone`, so normal rooms work immediately. Room and group decisions are
  local exceptions; the built-in announcements room adds a room-level
  `everyone` deny for `message.post`.
- Permission identifiers contain two or more non-empty dot-separated
  components. A component can contain hyphens. Each registered dotted prefix
  is a broader permission. For example, `message.read` includes
  `message.read.interactions`. Inclusion is transitive.
- An allow for a broader permission satisfies its narrower descendants. An
  allow for a descendant does not include an ancestor. A descendant deny
  cannot restrict an effective ancestor allow. An ancestor deny does not
  restrict a separate descendant allow.
- The normalized permission trees are `message.read.interactions`,
  `message.post.replies`, `room.manage.bans`, `role.manage.assignments`,
  `user.manage.permissions`, and `user.delete.self`. The catalog also uses
  `user.read` and `audit.read` for the independent admin views.
- The 0.5 replacement does not translate old permission identifiers during
  replay or writes. Operators must replace all server replicas and clients,
  then review stored RBAC decisions. Old decisions remain as inert history.

This supersedes ADR-005.

## Consequences

- The authorization model is easier to reason about: owners are special and
  everyone else is permission-based.
- Custom roles and per-user overrides cover ordinary moderation and
  administration cases without role-rank comparisons.
- `#announcements` uses a local room-level `everyone` deny for `message.post`.
  This blocks root posts from normal members. A separate
  `message.post.replies` grant lets them reply in threads unless the room also
  denies that permission.
- Deny-wins enables future broad restriction roles such as a suspended role.
- Operators cannot revoke the durable owner role while its verified email
  remains in `owners.emails`. Existing matching users are repaired at boot and
  new verifications are repaired by durable redelivery, so protecting the
  configuration and verified-email ownership remains security-critical.
- Existing role position fields and protobuf event fields remain for
  compatibility. Removing or reserving them can be considered separately if the
  persisted event contract is migrated.
- Permission inclusion changes effective authorization only. It does not write
  or synthesize an additional RBAC grant.
- A new permission name can change authority if it is below an existing
  permission. Each new nested permission must therefore have a registered
  immediate parent with the same category and scopes.
- A retained parent grant can gain authority when a capability moves below
  it. A retained child grant stays narrow. Removed identifiers have no effect,
  but remain available as historical event data.
- The authorization fence adds an empty operational fact to protected batches.
  During a mixed-version rollout, its full concurrency guarantee starts only
  after all writing replicas understand and advance the fence.
- An authorized message edit cannot commit across a classified role or
  permission change that advanced the authorization fence after its decision.
  Unrelated EVT traffic does not contend. An in-flight reaction or message
  retraction can still commit before the serving replica projects a
  cross-aggregate revocation; room membership and lifecycle changes remain
  room-OCC guarded.
