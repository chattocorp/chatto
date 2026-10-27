# ADR-120: Resource Action Availability

**Status:** Accepted
**Date:** 2026-10-27

## Context

Account controls use permission booleans, provider-policy flags, client-side
cooldown calculations, and a disconnect eligibility enum. These representations
make each client combine different rules and invent different fallback behavior.
Permission grants alone cannot explain an operator restriction or a cooldown.

A universal reason enum would mix unrelated policies and let actions carry
reasons that cannot apply to them. Shared conventions do not require shared
domain types.

## Decision

Use one state enum per action: `UNSPECIFIED`, `ALLOWED`, and its blocking states.
Do not pair an `allowed` boolean with a `reason`; that permits contradictory
combinations. Only `ALLOWED` enables an action. Unknown future states remain
disabled and use a generic notice.

Use optional enum fields to distinguish absence from an explicit `UNSPECIFIED`.
Absence means not reported and can use the documented older-server fallback.
Explicit `UNSPECIFIED` stays disabled. The published `DisconnectEligibility`
contract is an exception: its `UNSPECIFIED` means not evaluated, so it retains
the count-based fallback. Explicit disconnect denials and unknown future values
are never overridden by a stored password or an identity count.

| Decision | Blocking states | Extra metadata |
| --- | --- | --- |
| Username change | Provider-managed, cooldown | Cooldown refresh time |
| Password change | Direct login disabled, unsupported account | None |
| Account deletion | Permission denied, unsupported account | None |
| Identity disconnect | Operator restriction, last sign-in method | None |
| Signup username choice | Provider-managed | None |

Keep each enum near its resource schema. Reuse `DisconnectEligibility` instead
of adding a second disconnect field. Username changes alone need a message:
it contains a state enum and `available_at` for the cooldown. Do not add wrappers
to actions without metadata, a global state enum, a state union, or a universal
envelope. Client adapters and notice helpers use the distinct enum values directly.

Resources expose explicit action fields. The viewer groups username changes,
password changes, and account deletion in `AccountActions`. Each linked identity
exposes its own disconnect action. Pending signup exposes username choice. Do not
use a string-keyed action registry or move all resource actions into the viewer.

The server derives availability from existing domain queries. Permissions remain
a separate authority contract. Availability describes an attempt, not a guarantee:
input validation, case-only username exceptions, and fresh-authentication proofs
belong to the command. Commands retain their concurrency and authorization checks.

Snapshots can become stale. Clients refresh them after relevant mutations or a
rejection based on changed state. `available_at` is the earliest time to refresh
a cooldown decision; it does not automatically enable the action. For username
changes, external management takes precedence over a cooldown. For disconnects,
an operator restriction takes precedence over the last-sign-in-method restriction.

Keep published legacy fields as deprecated compatibility output derived from the
new decision. New clients prefer the action-specific decision and use the
documented fallback only when it was not reported. Remove unmerged alternatives
without reservations. Do not change persisted events or the connection floor for this
presentation contract.

## Consequences

Clients follow one decision convention and can show a specific disabled-state
notice without copying server policy. New actions define their own states.
Enums cannot express contradictory allowed/blocked combinations. Permissions,
inputs, and mutation errors stay explicit.

Deprecated output remains during the compatibility period. This is migration
support, not permission to add parallel policy representations. New states must
belong to their action and have documented precedence and tests. A similarly named
state in another action does not by itself justify sharing an enum.
