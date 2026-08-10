# ADR-068: Select Event Mutation Consistency Boundaries Explicitly

**Date:** 2026-08-10

**Status:** Accepted

## Context

An event-sourced mutation commonly derives a decision from one aggregate and
then appends to that aggregate with optimistic concurrency control (OCC). A
wildcard subject tail such as `evt.room.{roomId}.>` is an efficient consistency
boundary: another mutation of that room forces the decision to be recomputed,
while unrelated EVT traffic does not contend.

Some privileged mutations need a stronger guarantee. A reaction decision uses
room membership, room and group layout, RBAC, effective-owner state, message
identity, and current reaction state. Guarding only the room aggregate leaves a
race in which authority can be revoked after the decision but before the
reaction event commits.

Chatto already has an authorization-fence lane for message posting. Every
authorization-changing batch advances the lane, and message posts check its
tail without advancing it. Extending that design to every privileged mutation
would add a synthetic event to every relevant authorization change and require
each application to maintain an exhaustive classification of authority-changing
facts. Comparing the authorization tail only after a room OCC failure also
does not close the race: an authorization event can arrive while the room tail
remains unchanged, so the reaction append would not fail.

JetStream provides both expected-last-subject-sequence and
expected-last-sequence publication guards. The latter can make the complete EVT
stream the decision boundary without writing another domain event.

## Decision

The shared `pkg/events` framework exposes explicit mutation boundaries:

- `AtSubject(subjectOrFilter)` captures and checks the tail of one exact
  subject or wildcard subject filter; and
- `AtStreamTail()` captures and checks the last sequence of the complete bound
  stream.

`EncodedEventLog.ExecuteMutation` owns the reusable mutation loop. For every
attempt it captures the selected boundary, invokes an application callback,
and atomically publishes the callback's opaque records with that captured OCC
token. An OCC conflict captures a fresh boundary and reruns the complete
callback. Other errors return immediately, an empty decision is a successful
no-op, and retries are bounded. Results report commit sequences, attempts, and
conflicts.

The framework remains envelope-neutral. Applications own event IDs, codecs,
subjects, projection catch-up, authorization, and domain decisions. Logical
event IDs must remain stable across callback invocations. The callback must
wait until its application projections include every captured fact it uses
before returning a decision.

Single-record decisions use ordinary JetStream OCC publication. Multi-record
decisions require atomic publication and place whole-stream OCC on the first
record using JetStream's `Nats-Expected-Last-Sequence` header. Subject
boundaries use `Nats-Expected-Last-Subject-Sequence` and its optional filter
header. The low-level batch API continues to support application-selected OCC
guards for advanced multi-aggregate operations.

Chatto reaction add/remove is the first stream-boundary consumer. Each attempt
waits the room directory, room timeline, reaction, room-group, RBAC, and actor
projections through their relevant EVT tails, then rechecks membership,
`message.react`, room state, canonical message identity, duplicate state, and
the per-user reaction limit. Any EVT event committed before the reaction batch
forces the complete decision to run again. A concurrent revocation therefore
prevents the reaction from committing once that revocation is in EVT.

Authorized message edits are the second consumer and exercise the multi-record
shape. Each attempt catches the room directory, timeline, room-group, RBAC, and
actor projections up to their relevant tails, reruns membership, archive,
identity, authorship, edit-window, and permission checks, then rebuilds the
latest body and optional echo reconciliation. The body and semantic edit facts,
plus any echo change, commit atomically against the global EVT tail. Logical
event IDs are allocated once per operation and reused across retries. Internal
linked-message propagation remains aggregate-scoped with `AtSubject`; message
retractions are not migrated by this proof-of-concept slice.

Subject-boundary mutations remain the normal choice for invariants confined to
one aggregate. Stream-tail consistency is selected only when a strict
cross-aggregate commit boundary is worth contention with unrelated EVT
traffic. The existing authorization fence remains in place for message posts,
and room-scoped OCC remains in place for message retractions; this decision
does not migrate every privileged mutation at once.

## Consequences

Privileged reaction and authorized message-edit writes no longer require a new
fence event, and a missed classification of an authority-changing event cannot
bypass their commit-time check. The mechanism is reusable by Chatto, Authling,
and future applications without importing product policy into the framework.

Whole-stream OCC is deliberately coarse. An unrelated message, reaction,
account event, or background EVT write can reject an in-flight reaction or
authorized edit and make it re-read projections and re-authorize. Five
conflicts exhaust the operation with `ErrConflict`; JetStream provides
correctness, not starvation freedom. We accept that tradeoff for these proof
consumers and will measure conflict frequency and latency before applying
stream-tail consistency more broadly.

The API makes the cost visible at the call site instead of hiding it behind an
"important" flag. Code review can distinguish aggregate-local invariants from
strict cross-aggregate decisions, and mutation results provide the attempts and
conflicts needed for diagnostics.

The stream sequence is an internal storage coordinate. It remains inside the
server/framework boundary and is not exposed through public client APIs.

## Related

- [ADR-016](ADR-016-occ-for-message-publishing.md)
- [ADR-033](ADR-033-event-sourced-state-with-projections.md)
- [ADR-034](ADR-034-single-event-stream.md)
- [ADR-056](ADR-056-extractable-nats-event-sourcing-framework.md)
- [FDR-005](../fdr/FDR-005-reactions.md)
