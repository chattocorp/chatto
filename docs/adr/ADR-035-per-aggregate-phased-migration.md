# ADR-035: Per-Aggregate Phased Migration to Event Sourcing

**Date:** 2026-05-24

**Status update:** The 0.1.x event-sourcing rollout has completed across the
existing Chatto servers, and the pre-0.1 boot importers plus ES boot verifier
were removed on 2026-06-12.

## Context

The move to event sourcing ([ADR-033](ADR-033-event-sourced-state-with-projections.md)) cannot ship as a single big-bang change. Chatto has live deployments with real user data; migration must preserve every record. There are also too many aggregates (rooms, memberships, users, RBAC, threads, reactions, read state, messages, …) to migrate atomically, and each has its own quirks (encryption, bulk operations, cross-references).

A staged, per-aggregate approach is required. The questions this ADR settles:

- **How does pre-existing state get into the event stream?**
- **Is there a dual-write transitional period?**
- **How is correctness validated at cutover?**

## Decision

Migrate one aggregate at a time, but make the switch directly: import
pre-ES data into `EVT`, cut reads to projections, and make writes
event-only. There is **no dual-write phase**.

Dual-write was part of the first draft of this ADR. We are deliberately
not doing it. It would keep two write paths alive for every aggregate,
double the failure modes, and force us to reconcile KV and `EVT` during
the riskiest part of the migration. The actual rollout model is:

1. Preserve every pre-ES record by importing it at boot.
2. Validate the import and projections against a copy of real community
   server data before touching production.
3. Deploy the ES build as the new source of truth.
4. Keep legacy KV/stream data around as import source and inspection
   evidence, not as an actively-maintained mirror.

### Phases (per aggregate)

1. **Define event types.** Add or reuse protobuf event types in `proto/`. Existing types (`UserJoinedRoom`, `UserLeftRoom`, etc. defined for the live-event system) are reused where they cover the aggregate's lifecycle; new types are added only where current types do not. This is a per-aggregate call; the introducing PR enumerates additions.
2. **Build the projection.** Implements the framework's `Projection` interface (`Apply`, `Snapshot`, `Restore`). Tested in isolation by feeding it events. Not yet wired to any read path.
3. **Register the boot import.** During the rollout window, each aggregate had an idempotent boot importer that read pre-ES KV/stream state and emitted real events into `EVT` with original metadata preserved where available (timestamps, actor IDs, message IDs, encrypted bodies, etc.). These importers have now been removed.
4. **Cut over reads and writes.** Public API handlers, internal authz helpers, and other read paths switch from KV/`SERVER_EVENTS` to projections. Mutations publish only to `EVT` plus any required non-durable live mirror; they do not write legacy KV or `SERVER_EVENTS`.
5. **Validate on production-like data.** Before production rollout, restore a backup/copy of the community server into a scratch instance running the ES build. Boot imports must complete, projections must populate, and smoke tests must cover room lists, memberships, server config, sidebar layout, messages, threads, reactions, edits/deletes, and live delivery.
6. **Decommission.** Delete legacy KV/stream usage and boot importers after every aggregate has moved to event-only writes and the ES build has burned in. **Completed 2026-06-12.**

Each phase is one or a small number of PRs. Phases 1-3 can land independently of user-visible behavior. Phase 4 is the real gate: once merged and deployed, the aggregate's source of truth is `EVT`. A rollback after new writes requires restoring a pre-cutover backup or building an explicit ES-to-legacy recovery tool; there is no live KV mirror to fall back to.

### Cleanup

Cleanup was deferred for every aggregate until the full set of aggregates had been migrated and the new ES system's shape had settled. That condition has now been met for the 0.1.x rollout, and the accumulated boot importers and verifier have been removed.

Original rationale for deferring cleanup:

- **Import evidence.** Keeping legacy data available made it possible to inspect what was imported if a projection or event-shape bug appeared during rollout.
- **Importer safety.** Boot importers were idempotent and cheap after first run. Keeping them around let scratch restores, partial boots, and re-runs use the same production code path during the rollout window.
- **Interface review window.** Holding off on irreversible deletion lets us shape the new event/projection/manager APIs across all aggregates before committing to the cleanup.

Cleanup unblocked once: (a) every aggregate had reached event-only writes, (b) the new system had burned in across the existing servers, and (c) the projection and mutator APIs were stable enough for the 0.1.x lane.

### Why migrations run at boot, not as a CLI subcommand

An earlier draft of this ADR (and a now-deleted `chatto evt migrate` CLI) had each aggregate's migration as a one-shot operator command. That can't work in the typical embedded-NATS deployment: with no TCP listener on the embedded NATS server, a second process can only connect by taking a temporary file lock on the data directory — which requires stopping `chatto run` first. That isn't an acceptable footgun for an alpha product where operators run a single binary.

Running the migrations at boot inside `NewChattoCore` avoids the multi-process problem entirely. The cost is one extra step at startup; the steady-state cost (after first boot) is a KV key scan and per-subject OCC check, both O(aggregates).

Manual re-runs were never exposed, and the boot importers have since been removed. Any future repair or backfill would need a new explicit tool rather than relying on this rollout path.

### First aggregate: room membership

The first aggregate migrated end-to-end is **room membership** (`SERVER_CONFIG` keys `room_membership.{kind}.{roomId}.{userId}`). It is small, well-scoped, has multiple writers and multiple readers, and exercises bulk-mutation paths (account deletion, room deletion) that we will later need for messages. Once it is done, the direct-import/direct-cutover template is concrete and subsequent aggregates follow it mechanically.

### Migration events look like real events

A migration event is indistinguishable from one written at the time of the original action:

- `created_at`: the original record's creation timestamp, preserved from KV.
- `actor_id`: the original actor where known; a synthetic `system:migration` actor otherwise.
- No "this was migrated" flag.

Once a migration completes, no code branches on event provenance. The audit log treats the migrated record as canonical.

### Migrations are safely replayable

The always-OCC invariant from ADR-033 makes migration replayability automatic:

- The migration command iterates KV in a deterministic order per subject.
- Each event is published with `Nats-Expected-Last-Subject-Sequence` matching the stream sequence of the previous event on that subject (0 for the first event on a fresh subject; then the sequence returned by the prior append).
- A second full run against an already-migrated subject fails the OCC check on the first event and aborts that subject's migration without writing duplicates.
- A migration that crashed midway can resume: re-running iterates the same KV order, the already-emitted prefix is no-op'd by OCC, and the remainder appends.

### Rollout observability

Boot migrations emitted structured log markers during the rollout. The
`core.es_boot_verify` and `core.es_boot_verify_strict` settings were temporary
rollout diagnostics and no longer exist.

Determinism is the migration command's responsibility: events for a given subject must be emitted in the same order across runs given the same KV state. This is a property of the iteration code, not of the framework.

### Why no dual-write

Dual-write would preserve a downgrade path only while both stores remain
perfectly in sync. In practice it would introduce a second mutation path
for every aggregate, require ordering rules for every partial failure,
and make projection bugs harder to diagnose because KV could disagree
with `EVT` for reasons unrelated to the importer.

Instead, the migration is a big switch per aggregate:

- Pre-ES state is imported once into `EVT`.
- New writes append to `EVT` only.
- Legacy stores are retained for audit/import comparison, not updated.
- Production safety comes from backup, scratch restore against a copy of
  real community-server data, and post-cutover smoke tests.

This makes the failure mode explicit. If the ES build is wrong after
users have produced new writes, downgrading to the old binary would lose
those writes because the old stores are no longer maintained. Recovery is
restore-and-fix-forward, not automatic rollback.

### No shadow-read divergence metric

An earlier design proposed serving reads from both KV and projection in parallel during a burn-in period, with a divergence counter to validate correctness before cutover. We are not doing this. The rationale:

- Chatto is alpha. Test-based validation of projection correctness is consistent with the project posture.
- Each migration is small (one aggregate). The blast radius of a bad cutover is bounded.
- Building and operating the divergence path is non-trivial and would slow every migration.
- If a specific migration is later judged high-risk (most plausibly: messages), we add shadow reads for that one aggregate without committing to it as the default.

If we hit a migration where this turns out to be the wrong call, we add the shadow-read path then.

## Consequences

- **Per-aggregate cadence.** Each migration landed as one or more PRs: event/projection/importer first, then direct read/write cutover.
- **Two systems coexisted, but not as mirrors.** The old `SERVER_EVENTS` stream, legacy KV buckets, and the new `EVT` stream existed together during the migration window. For migrated aggregates, only `EVT` was written; legacy storage was pre-cutover data and import evidence.
- **Cutover is not independently revertable after new writes.** This is the cost of avoiding dual-write. Rollout discipline moves to backup, scratch restore, real-data validation, and fix-forward readiness.
- **Migration functions accumulated temporary surface.** The boot-import surface was intentionally carried through rollout and removed once cleanup unblocked.
- **No divergence safety net at cutover.** Cutover relies on tests plus validation against a copy of production/community data. A latent projection bug could cause user-visible incorrectness. We accept this for migration velocity in alpha and revisit if any migration burns us.
- **The framework matured through use.** Room membership shook out the first version of the internal events package, and later aggregates refined it.
- **Messages were pulled forward once the framework existed.** Messages were the highest-volume migration and the aggregate that ADR-033's RAM win actually unlocked.

## Out of scope for this ADR

- The specific protobuf event additions for each aggregate — decided per-aggregate at migration time.
- Snapshot strategy and the projection-restore-from-snapshot path — deferred.
- Long-term retention of the legacy `SERVER_EVENTS` stream after the last aggregate migrates — now handled by the 2026-06-12 cleanup.
- A general "framework readiness" gate before starting aggregate two. We start the next aggregate when the previous direct-cutover path has enough test and rollout confidence; no separate framework-quality checkpoint.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md) — the umbrella decision this ADR operationalises.
- [ADR-034](ADR-034-single-event-stream.md) — the shape of the new `EVT` stream.
- [ADR-006](ADR-006-kv-source-of-truth-streams-audit-log.md) — superseded by ADR-033. Each phase-7 decommission is a step toward fully retiring ADR-006's pattern.
