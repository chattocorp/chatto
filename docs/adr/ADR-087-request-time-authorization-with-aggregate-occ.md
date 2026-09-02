# ADR-087: Use Request-Time Authorization with Aggregate OCC

**Date:** 2026-09-01

**Status:** Accepted

## Context

Chatto used an empty `AuthorizationFenceAdvancedEvent` as a shared optimistic
concurrency control (OCC) lane. Authority-changing commands appended this event
with their domain facts. Selected commands guarded the lane so that a concurrent
authority change rejected the complete command batch.

The fence made authorization depend on a manually maintained writer list. It
also stored records that did not represent domain state. Replacing the fence
with an authorization marker in every applicable subject would keep the same
classification problem in the persisted subject contract.

JetStream can guard a subject or wildcard filter on each batch record. However,
one record can carry only one subject guard. A message command can depend on its
room, RBAC, room-group placement, actor state, and bot-owner state. The current
event model cannot guard every input without extra records, a shared lane, or a
whole-stream guard.

Normal command authorization does not need this stronger commit-time rule. A
revocation does not have to cancel a command that is already in progress. The
domain aggregate still needs OCC because concurrent domain changes can alter the
meaning of the command.

## Decision

Use stable request-time authorization as the default rule. Use aggregate OCC to
protect domain invariants.

For a stable authorization read, the command:

1. Captures the current tails of each cross-aggregate authorization input.
2. Waits until the related projections apply those positions.
3. Evaluates the complete authorization decision.
4. Reads the same tails again.
5. Repeats the read when an input changed during the decision.

The final validation is the authorization decision point. A change after that
point is concurrent with the command and does not cancel it. If target aggregate
OCC rejects the append, the command repeats its domain checks and authorization
from the original command intent.

Chatto uses the existing `evt.rbac.>`, `evt.group.>`, and `evt.user.>` filters
for common cross-aggregate authorization inputs. A low-frequency decision that
can inspect permissions in many rooms also uses `evt.room.>`. Ordinary room
commands use the exact `evt.room.{roomId}.>` aggregate instead, so unrelated
room traffic does not affect their authorization read.

Stop all current authorization-fence reads and writes. Do not add a replacement
event, subject facet, KV value, durable mirror, process lock, or coordinator.
Keep `AuthorizationFenceAdvancedEvent`, protobuf field 830, its event token, and
`evt.authorization.server.fence_advanced` decoding support for historical EVT
records. Current writers must not emit this event.

Use whole-stream OCC only when a documented domain invariant spans the complete
EVT stream and its contention cost is acceptable. Do not use whole-stream OCC
only to strengthen ordinary authorization.

Deploy this change with an exclusive-version cutover. Stop all old Chatto
processes and background workers before the new version starts command or
background writes. Chatto does not support old and new request-serving replicas
during this cutover.

This decision supersedes the authorization-fence parts of ADR-016, ADR-040, and
ADR-068. It does not change their aggregate OCC, RBAC, or mutation-loop rules.

## Consequences

- Durable events again record domain facts instead of coordination records.
- Existing EVT payloads and subjects do not change. Current code can replay all
  historical fence events.
- Authorization input lists are runtime read dependencies. They are not part of
  the persisted subject grammar.
- A revocation that completes before the final authorization validation is
  observed. A later concurrent revocation can overlap a successful command.
- Room, RBAC, user, group, and other domain invariants keep their existing OCC
  filters and atomic batches.
- Broad authorization input filters can cause a short read-phase retry. They do
  not cause a commit conflict.
- Rollback is safe after a full stop because current writers keep the existing
  domain payloads and subjects. The old version rebuilds its projections before
  it accepts traffic and resumes its historical fence writes.
- A rolling deployment with old and new writers is not supported for this
  change.
