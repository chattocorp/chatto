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

Use action-specific decision messages that follow the same convention:
`allowed` and one primary typed blocking `reason`. Each message owns its nested
`Reason` enum. Add metadata only to the action that needs it. An allowed action
has no blocking metadata. A missing message means that the server did not report
a decision. Unknown blocking reasons
remain blocked and use a generic notice.

| Decision | Reasons | Extra metadata |
| --- | --- | --- |
| Username change | Provider-managed, cooldown | Cooldown refresh time |
| Password change | Direct login disabled, unsupported account | None |
| Account deletion | Permission denied, unsupported account | None |
| Identity disconnect | Operator restriction, last sign-in method | None |
| Signup username choice | Provider-managed | None |

Keep the decision near its resource schema. Do not build a global reason enum,
reason union, or generic protobuf envelope. Client adapters and notice helpers
keep these types distinct. A private structural mapping helper may share code
without combining the reason types or their policies.

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
new decision. New clients prefer the action-specific message and use existing
older-server behavior only when it is absent. Remove alternatives introduced within the same
unmerged branch. Do not change persisted events or the connection floor for this
presentation contract.

## Consequences

Clients follow one decision convention and can show a specific disabled-state
notice without copying server policy. New actions define their own reasons.
The repeated fields are a small cost for preventing unrelated policies from
sharing one type. Permissions, inputs, and mutation errors stay explicit.

Deprecated output remains during the compatibility period. This is migration
support, not permission to add parallel policy representations. New reasons must
belong to their action and have documented precedence and tests. A similarly named
reason in another action does not by itself justify sharing an enum.
