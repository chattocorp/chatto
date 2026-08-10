# ADR-068: Select Event Mutation Consistency Boundaries Explicitly

**Date:** 2026-08-10

**Status:** Accepted

## Context

An event-sourced command commonly derives a decision from one aggregate and
then appends to that aggregate with optimistic concurrency control (OCC). A
wildcard subject tail such as `evt.room.{roomId}.>` is an efficient consistency
boundary: another mutation of that room forces the decision to be recomputed,
while unrelated EVT traffic does not contend.

Authorization introduces a separate question: when does an authorization
decision take effect? The usual request-time model accepts a command that was
authorized when evaluated, provided its domain aggregate has not changed. A
stricter command can require relevant authorization state to remain unchanged
until commit. Neither rule should arise accidentally from the storage API.

JetStream provides expected-last-subject-sequence and expected-last-sequence
publication guards. The latter can make the complete EVT stream a decision
boundary, but it also serializes a command with every unrelated event. Chatto
already has a narrower authorization-fence lane: every classified
authorization-changing batch advances the lane, while privileged commands can
check its tail without advancing it.

Comparing an authorization tail only after a room OCC failure does not close a
race. An authorization event can arrive while the room tail remains unchanged,
so the room append would succeed without triggering that comparison. Strict
commit-time authorization therefore requires an authorization guard in the
same atomic publish as the domain guard.

## Decision

The shared `pkg/events` framework exposes explicit mutation boundaries:

- `AtSubject(subjectOrFilter)` captures and checks the tail of one exact
  subject or wildcard subject filter; and
- `AtStreamTail()` captures and checks the last sequence of the complete bound
  stream.

`EncodedEventLog.ExecuteMutation` owns the reusable single-boundary mutation
loop. For every attempt it captures the selected boundary, invokes an
application callback, and atomically publishes the callback's opaque records
with that captured OCC token. An OCC conflict captures a fresh boundary and
reruns the complete callback. Other errors return immediately, an empty
decision is a successful no-op, and retries are bounded. Results report commit
sequences, attempts, and conflicts.

The framework remains envelope-neutral. Applications own event IDs, codecs,
subjects, projection catch-up, authorization, and domain decisions. Logical
event IDs must remain stable across callback invocations. The callback must
wait until its application projections include every captured fact it uses
before returning a decision.

Single-record decisions use ordinary JetStream OCC publication. Multi-record
decisions require atomic publication. Subject boundaries use
`Nats-Expected-Last-Subject-Sequence` and its optional filter header;
stream-tail boundaries use `Nats-Expected-Last-Sequence`. The low-level batch
API continues to support multiple application-selected OCC guards on different
entries for advanced multi-aggregate operations.

Aggregate or subject-filter OCC is the default. `AtStreamTail` is reserved for
commands whose invariant genuinely spans the complete stream and is worth
contention with unrelated EVT traffic. It remains a reusable framework
capability, but applications must not use it merely as a shortcut for choosing
an authorization policy.

Chatto reaction add/remove uses request-time authorization and room-aggregate
OCC. Each attempt waits the room directory, timeline, reaction, room-group,
RBAC, and actor projections through their relevant EVT tails, then checks
membership, `message.react`, room state, canonical message identity, duplicate
state, and the per-user reaction limit. A concurrent room mutation conflicts
and reruns the complete decision. A cross-aggregate authorization change does
not retroactively cancel an already-authorized, otherwise conflict-free
reaction attempt; subsequent requests observe the new authorization state.

Authorized message edits deliberately use stricter commit-time authorization.
Each attempt captures the narrow authorization-fence tail before the room tail,
waits the room directory, timeline, room-group, RBAC, and actor projections,
and reruns the complete operation-level decision. The atomic batch guards its
replacement body against the room aggregate and its semantic edit event against
the authorization fence. A change to either boundary rejects the whole batch
and reruns authorization and domain validation. Unrelated EVT traffic does not
contend. Logical event IDs remain stable across retries, and edit-driven echo
changes share the same batch. Internal linked-message propagation and message
retractions remain room-scoped.

## Consequences

The public framework supports both efficient aggregate-local mutation loops and
explicit whole-stream invariants without importing Chatto or Authling policy.
Its low-level atomic batch API also supports narrow multi-boundary commands such
as Chatto message edits.

Reaction mutations avoid global contention and use ordinary request-time
authorization semantics. This leaves a documented in-flight window in which a
revocation can commit immediately before an already-authorized reaction. That
tradeoff is accepted for reactions; stricter operations must opt into a narrow
commit-time fence.

Authorized message edits reject concurrent classified authorization changes
without contending with messages, reactions, account events, or background EVT
writes. Their correctness depends on every authorization-changing writer
advancing the authorization fence. Tests and reviews must preserve that writer
classification.

Whole-stream OCC remains deliberately coarse. If selected, any unrelated event
can reject an attempt, and repeated conflicts can exhaust the bounded retry
budget. The explicit API makes that cost visible at the call site.

JetStream stream and subject sequences remain internal storage coordinates and
are not exposed through public client APIs.

## Related

- [ADR-016](ADR-016-occ-for-message-publishing.md)
- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-040](ADR-040-permission-only-rbac-with-owner-override.md)
- [ADR-056](ADR-056-extractable-nats-event-sourcing-framework.md)
- [FDR-004](../fdr/FDR-004-message-editing-and-deletion.md)
- [FDR-005](../fdr/FDR-005-reactions.md)
