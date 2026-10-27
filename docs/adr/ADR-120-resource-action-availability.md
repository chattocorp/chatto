# ADR-120: Resource Action Availability

**Status:** Accepted
**Date:** 2026-10-27

## Context

Account controls use permission booleans, provider-policy flags, client-side
cooldown calculations, and a disconnect eligibility enum. These representations
make each client combine different rules and invent different fallback behavior.
Permission grants alone cannot explain an operator restriction or a cooldown.

## Decision

Use the shared public `ActionAvailability` message for action snapshots. It has
`allowed`, one primary typed blocking `reason`, and an optional `available_at`
timestamp for a cooldown. An allowed action has no blocking metadata. A missing
message means that the server did not report a decision. Unknown blocking reasons
remain blocked and use a generic notice.

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
new decision. New clients prefer the shared message and use existing older-server
behavior only when it is absent. Remove alternatives introduced within the same
unmerged branch. Do not change persisted events or the connection floor for this
presentation contract.

## Consequences

Clients share one decision shape and can show a specific disabled-state notice
without copying server policy. New action types reuse it. Resource-specific
permissions, inputs, and mutation errors stay explicit.

Deprecated output remains during the compatibility period. This is migration
support, not permission to add more parallel policy representations. New reasons
must describe a reusable cause and have documented precedence and tests.
