# ADR-067: Event-Sourced Hierarchical Runtime Policies

**Date:** 2026-08-08

## Context

Chatto needs runtime-editable behavioral settings whose effective value can
vary by server, room group, and room. The first policy is the message author
edit window; Slow Mode and thread-first room behavior are expected to follow.
Implementing each setting as an independent field, store, resolver, API, and UI
would duplicate concurrency, authorization, inheritance, audit, replay, and
mixed-version behavior.

These settings are neither permissions nor process configuration. Permissions
answer whether a viewer may perform an action. Process configuration in
`chatto.toml` controls deployment and runtime wiring. A runtime policy instead
selects typed product behavior while the server is running.

The system also needs a path to role- and user-specific policy values. Those
values would use the same scope tiers and inheritance rules, but resolving
several applicable subjects requires a future, policy-specific combination
rule. The initial feature must not silently invent that rule.

## Decision

Chatto stores runtime-policy overrides as typed, durable facts in `EVT` on the
existing `evt.config.*` namespace. The configuration projection retains sparse
overrides; it does not materialize inherited values.

Each policy has a compile-time product default and a typed value. Baseline
values resolve independently for each policy in this order:

1. room override;
2. the room's current room-group override;
3. server override; and
4. product default.

The nearest stored override supplies the value. Parent values are defaults,
not constraints: a child may select any value accepted by that policy's global
validation rules. Moving a room between groups immediately changes inheritance
because resolution follows the current room-group relationship rather than a
copied value.

Policy targets carry both a scope dimension and a subject dimension. The
persisted shape reserves baseline, role, and user subject kinds from the start,
but the first API accepts baseline targets only. Role/user resolution is
deferred until its precedence and combination semantics are designed. This
keeps the storage model extensible without presenting behavior that is not yet
well defined.

Set and clear are distinct typed events. Clearing removes the sparse override
and reveals the next parent or product default. Writers use optimistic
concurrency on the target's configuration aggregate and retry from projected
state. Policy changes that can affect authorization-adjacent command checks
advance the existing authorization fence atomically, allowing a command to
prove it used policy and authorization state no older than its commit.

Administrative APIs expose both the stored override and the resolved effective
value with its source scope. Public room resources expose effective values
needed for client behavior, but not administrative event-log coordinates. The
realtime projection replaces affected room viewer state after a policy change.
Older clients keep their existing defaults and affordances; server enforcement
remains authoritative.

Administrative writes use field masks. A selected field with no value clears
that override; unselected fields remain unchanged. This lets older clients
update policies without erasing fields introduced by newer servers.

Room overrides apply only to channel rooms. Direct messages inherit the server
baseline directly because they have no room-group administration surface.
Deleting a room or room group removes its projected override state while its
durable history remains in `EVT`.

## Consequences

- Policy changes are auditable, replayable, included in normal EVT backup, and
  safe across multiple replicas without a new KV bucket.
- Each policy still requires explicit protobuf fields, validation, product
  behavior, documentation, and tests. There is deliberately no arbitrary-key
  or `Any`-valued escape hatch.
- Effective-value reads depend on current room-group topology as well as the
  configuration projection. Commands whose correctness depends on a policy
  must include both state boundaries in their OCC retry.
- A parent cannot cap or forbid child values. A future need for organisational
  constraints would be a separate feature with explicit min/max semantics.
- Adding role/user subjects later remains additive at the storage boundary,
  but it requires a new ADR or an amendment defining subject combination.
- A downgraded server safely ignores unknown additive policy events. It applies
  its compiled behavior until upgraded again; no destructive migration is
  required. Operators should avoid administering new policy values during a
  downgrade because the older process cannot enforce or display them.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-045](ADR-045-public-api-stability-tiers.md)
- [ADR-051](ADR-051-server-scoped-resumable-client-projection.md)
- [FDR-035](../fdr/FDR-035-runtime-policies.md)
