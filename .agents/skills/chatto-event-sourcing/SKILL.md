---
name: "chatto-event-sourcing"
description: "Use when designing, implementing, reviewing, debugging, or documenting Chatto event-sourced domain behavior, including EVT subjects, aggregate boundaries, Services, projections, optimistic concurrency control, read-your-writes, live/reconnect delivery, replay compatibility, migration safety, and rollback/deployment implications."
---

# Chatto Event Sourcing

Use this skill whenever touching durable domain state in Chatto. It is a guardrail checklist for the event-sourced architecture, not a replacement for the architecture docs.

## Start Here

Read only what is relevant to the task:

- `docs/architecture/INDEX.md` and the relevant category files for the current runtime inventory, registered projections, EVT subject patterns, and live delivery shape.
- `docs/adr/INDEX.md` to find relevant architecture decisions; read only the ADRs that apply to the current event-sourcing, stream, projection, or runtime-state question.
- Relevant FDRs in `docs/fdr/` before changing user-visible behavior.
- `AGENTS.md` and `cli/AGENTS.md` when changing backend writes, auth, or room visibility.

Authoritative code anchors:

- `cli/internal/events/publisher.go` - OCC-only event publishing.
- `cli/internal/events/subjects.go` - aggregate types, event tokens, subject helpers, and wildcard filters.
- `proto/chatto/core/v1/event.proto` and sibling `*_events.proto` files - durable event payloads.
- `cli/internal/core/core.go` - service/projection wiring and live/reconnect delivery.
- `cli/internal/core/*_service.go` - domain services and write orchestration.
- `cli/internal/core/*_projection.go` - in-memory read models.
- `cli/internal/core/projection_subjects_test.go` - subject-policy regression tests.

## Core Rules

- Durable domain facts go into `EVT`. Do not add durable mirrors in KV or object metadata unless the architecture explicitly calls the state runtime, ephemeral, secret, binary, or cache data.
- Domain state interactions should go through a Service for that domain. Avoid direct JetStream, KV, object-store, or projection manipulation from unrelated code.
- `ChattoCore` is a facade and wiring point. Prefer moving domain-specific write/readiness logic into a focused Service.
- Projections are process-local read models rebuilt from `EVT`. They are not locks, coordination points, or sources of durable truth.
- Multi-replica safety comes from JetStream OCC plus projection catch-up, not from in-process mutexes or "only one server will do this" assumptions.
- Every successful write that needs read-your-writes must wait for the local projector(s) that serve the next read path.
- Event subjects are part of the persisted data model. Changing an aggregate lane is a compatibility decision, not a refactor.

## Before Adding Or Changing A Write

Answer these questions before editing:

1. Is this a durable domain fact, runtime state, transient live sync, binary/object data, a secret, or a cache?
2. Which Service owns the domain? If none exists, should this change introduce one?
3. Which aggregate owns the event subject?
4. What invariant does the OCC filter protect?
5. Which projections must consume the event?
6. Which projections must be current before the mutation returns?
7. Does the write need to publish transient `LiveEvent`s, or will `EVT` republish through `live.evt.>` be enough?
8. What happens with multiple replicas racing the same write?
9. What happens on forward deploy, mixed-version rolling deploy, and rollback?
10. Which focused tests lock down the subject, replay, OCC, projection, and delivery behavior?

## Choosing An Aggregate Subject

Use `events.{Domain}Aggregate(...).SubjectFor(event)` helpers instead of hand-built subjects.

Subject guidance:

- Put the aggregate ID in the subject when it is the concurrency and ordering boundary.
- Keep actor IDs, room visibility, ownership details, and extra context in the protobuf payload unless they are truly the aggregate ID.
- Use event-type wildcard filters such as `evt.room.*.reaction_added` only for projections that need a narrow cross-aggregate event family.
- Use aggregate-wide filters such as `evt.room.>` when a projection genuinely needs the full aggregate namespace.
- If an invariant spans multiple aggregate instances or event types, use `AppendAtFilter` with an OCC filter that covers the invariant, not just the target subject.

When moving an event family to a new aggregate lane, decide explicitly:

- current write subject,
- legacy replay subject(s),
- whether old events and new events can coexist for one logical entity,
- whether old code can survive new subjects during rollback,
- how live delivery and reconnect replay find the authorization context.

The asset migration is the current example: new writes use `evt.asset.{assetId}.*`, while `AssetProjection` still consumes beta-era `evt.room.*.asset_*` lanes for replay compatibility. This supports forward deploy, but rollback to code that only understands room-scoped asset facts is not symmetric.

## OCC Checklist

Use `events.Publisher`; do not add non-OCC publish paths.

Common patterns:

- Per-aggregate append: use the typed aggregate helper and the publisher's append path.
- Cross-event-type invariant on one aggregate family: read the wildcard tail with `LastSubjectPosition(filter)`, wait for any needed projections to catch up, re-check the projected invariant, then `AppendAtFilter`.
- Multi-event atomic write: use atomic publish entries with explicit OCC on every entry that needs protection. Do not publish a batch with no OCC guard.
- Retry only after re-reading the OCC tail and re-checking the invariant from projections.

Pitfalls:

- Checking only the target subject when the invariant is broader.
- Using projection state from before the OCC tail caught up.
- Adding a best-effort publish because "it is only metadata".
- Depending on a process-local mutex for correctness across replicas.
- Having old and new versions write equivalent facts under different OCC filters during a rolling deploy.

## Projection Rules

Projection implementations should be boring and replay-safe:

- Treat projection subject declarations as three separate concepts:
  - `Subjects()` is the logical consumption and readiness contract used by waits, live delivery, diagnostics, and tests.
  - `ReplaySubjects()` is an optional physical replay filter and may be broader for shared fanout or legacy compatibility.
  - OCC scope is a correctness boundary and may intentionally be broader than the event families a projection mutates.
- `Subjects()` must be intentional and tested in `projection_subjects_test.go`.
- Broad filters are appropriate for aggregate-owner projections and projections whose snapshots expose an OCC token for the full aggregate tail. Focused event-family filters are appropriate for stable derived indexes that do not make full-tail OCC decisions.
- `Apply` should tolerate duplicate delivery, replay order within the stream, nil/partial payloads where possible, and historical compatible shapes.
- Store cloned protobuf messages when retaining them in memory.
- Keep derived indexes consistent when events remove, replace, tombstone, or supersede prior state.
- Return cloned/protective values from projection read methods.
- Let the projector framework own JetStream message metadata. Projection code should use the stream sequence passed into `Apply`; do not parse `msg.Metadata()` or substitute consumer sequence numbers in projection logic.
- Use stable stream sequence numbers for reconnect replay and delivery cursors.
- Add admin projection estimates when adding meaningful in-memory indexes.

When a projection consumes legacy lanes, name them as legacy compatibility in comments/docs/tests. New writes should still have one canonical subject family.
When optimizing projection replay or retained memory, follow the projection
benchmark workflow in `cli/AGENTS.md`: capture repeated before/after results
with `mise bench-projections`, use `mise bench-projections-profile` for exact
heap attribution, and confirm meaningful wins against a restored real EVT
history. Treat synthetic retained-heap results as regression signals rather
than production RSS measurements. Preserve read-your-writes and OCC semantics
even if a cheaper consumer, filter, or in-memory shape looks attractive.

## Read-Your-Writes

After appending to `EVT`, wait for every local projector needed by the response or by immediately following reads.

Check:

- The returned stream position uses the same subject or filter the projector consumes.
- Composite write paths wait for all affected projections, not just the one that owns the Service.
- Live `live.evt.>` delivery waits for projection readiness before authorizing or shaping realtime events.
- Reconnect replay waits for projections through the global cutoff before planning replay.

## Post-Commit Side Effects

Treat the interval between an EVT commit and completion of any KMS, LiveKit,
object-store, webhook, or other external side effect as a crash boundary.

- External side effects required by a committed fact must be retryable from
  durable state after process restart and lease-holder turnover.
- A process-local retry queue may reduce recovery latency, but must not be the
  only way to discover unfinished work.
- An elected worker must discover facts committed later by non-holder replicas;
  a one-time startup scan is not sufficient on a stable multi-replica cluster.
- Use an incremental stream cursor or durable consumer for ongoing recovery.
  Do not repeatedly materialize unbounded event history.
- Before a destructive external action based on projected state, wait for the
  relevant aggregate tail and re-check the decision.
- Order recovery so the durable retry signal exists before an irreversible or
  externally visible cleanup step can make the work undiscoverable.
- Before copying recovery metadata into a new event field, check whether the
  event's aggregate ID can locate an existing immutable fact that already
  carries it. Prefer that durable lookup when its cost is bounded and the
  historical subject shape is compatible; document any legacy lanes that
  cannot be located from the current aggregate ID.
- Keep history-wide repair and reconciliation out of request paths. A request
  may attempt the side effect for the fact it just committed, while background
  recovery owns scanning, retries, and cross-replica discovery.

## Live Delivery And Reconnect Replay

Durable EVT facts are not delivered directly to clients. `live.evt.>` is an internal raw committed-event feed.

When adding or moving deliverable events:

- Update the deliverable event switch in core live filtering.
- Ensure authorization can be resolved from projections.
- If the subject is not room-scoped, add a path to resolve room/user visibility from payload/projections.
- Include the event family in reconnect replay if clients need to recover it after disconnect.
- Keep replay ordered by global stream sequence and deduplicate by event ID where room and non-room projections can both see legacy facts.
- Transient `LiveEvent`s on `live.sync.>` are not replayed and are not projection input.

## Compatibility And Deployment

For existing persisted `EVT` histories, forward replay compatibility matters more than ideal cleanliness.

Before merging a subject or payload compatibility change, document:

- Which historical event shapes still replay.
- Whether the current writer ever emits the historical shape.
- Whether mixed historical and current facts for one entity are valid.
- Whether rollback after new writes is safe.
- Whether a rolling deploy can cause duplicate or conflicting facts because old and new code use different OCC filters.
- Whether a one-time stream migration is required. Avoid this unless the compatibility reader is impossible or unsafe.

Prefer additive protobuf changes. Avoid breaking persisted event payload fields. If a breaking protobuf change is unavoidable, call it out clearly and design a migration/replay story first.

## Test Expectations

For event-sourced changes, look for focused tests in addition to end-to-end behavior:

- Subject helper tests in `cli/internal/events`.
- Projection `Subjects()` policy in `cli/internal/core/projection_subjects_test.go`.
- Projection replay tests for canonical and legacy event shapes.
- OCC conflict/race tests for the invariant being protected.
- Read-your-writes tests that assert projections are current after writes.
- Live delivery authorization tests, especially when the subject is not room-scoped.
- Reconnect replay tests for cursor ordering, deduplication, limits, and authorization.
- Deployment compatibility tests when old and new subject families can coexist.
- Failure-injection tests for errors immediately after EVT commit and before an
  external side effect completes.
- Restart tests proving unfinished external work is rediscovered from durable
  state rather than a process-local queue.
- Multi-replica tests where the lease holder starts first and another replica
  later commits work whose immediate side effect fails.
- Partial-success tests where external cleanup succeeds but local acknowledgement
  or a later cleanup step fails.
- Bounded-cost tests or measurements when recovery may traverse historical
  events, especially to ensure normal request latency does not grow with stream
  history.

## Documentation Touchpoints

Update docs in the same change when architecture behavior shifts:

- The relevant files in `docs/architecture/` for current components,
  projections, resources, subject patterns, event inventories, and durable
  effects.
- `docs/adr/` for new or changed cross-cutting architectural decisions.
- `docs/fdr/` for user-facing behavior and rationale.
- `docs/GLOSSARY.md` when canonical terms change.

Keep docs crisp: current facts in `docs/architecture/`, decisions in ADRs/FDRs,
and event-sourcing pitfalls/checklists here.
