# ADR-035: Per-Aggregate Phased Migration to Event Sourcing

**Date:** 2026-05-24

## Context

The move to event sourcing ([ADR-033](ADR-033-event-sourced-state-with-projections.md)) cannot ship as a single big-bang change. Chatto has live deployments with real user data; migration must preserve every record. There are also too many aggregates (rooms, memberships, users, RBAC, threads, reactions, read state, messages, …) to migrate atomically, and each has its own quirks (encryption, bulk operations, cross-references).

A staged, per-aggregate approach is required. The questions this ADR settles:

- **How does pre-existing state get into the event stream?**
- **What does the dual-write transitional period look like?**
- **How is correctness validated at cutover?**

## Decision

Migrate one aggregate at a time. Each aggregate moves through the same seven-phase template.

### Phases (per aggregate)

1. **Define event types.** Add or reuse protobuf event types in `proto/`. Existing types (`UserJoinedRoom`, `UserLeftRoom`, etc. defined for the live-event system) are reused where they cover the aggregate's lifecycle; new types are added only where current types do not. This is a per-aggregate call; the introducing PR enumerates additions.
2. **Build the projection.** Implements the framework's `Projection` interface (`Apply`, `Snapshot`, `Restore`). Tested in isolation by feeding it events. Not yet wired to any read path.
3. **Register the migration to run at boot.** A new function (e.g. `MigrateRoomMembership`) is added that reads the aggregate's current KV state and emits real events into the `EVT` stream with original metadata preserved (timestamps, actor IDs where known). It's wired into `NewChattoCore` alongside the existing `migrations.RunAll` call, so it runs on every boot. Replayable (see below) — already-migrated subjects no-op via OCC, so the steady-state cost is just listing keys + one OCC check per subject.
4. **Enable dual-write.** Every mutation that touched the aggregate's KV bucket now publishes the event first, then writes KV. Publish-event-first ensures the worst-case partial failure (event without KV mirror) matches the post-migration steady state.
5. **Cut over reads.** Read paths (GraphQL resolvers, internal authz helpers, etc.) switch from KV to the projection. Writes still dual-write.
6. **Stop writing KV.** The mutation becomes event-only. KV is effectively dead but not yet removed.
7. **Decommission.** Delete the KV keys, the dual-write code, and the migration command for this aggregate. **DEFERRED — see "Phase 7 on hold" below.**

Each phase is one or a small number of PRs. Phases 1–3 can land independently of any user-visible change. Phases 4 and 6 are the two real gates (entering dual-write; entering event-only). Phase 5 is the cutover and is the place to revert from if anything's wrong.

### Phase 7 on hold (until full ES shape lands)

Phase 7 is **deferred for every aggregate** until the full set of aggregates has been migrated through phase 6 and the new ES system's shape has settled. Current end-state per aggregate is phase 6: event-only writes, KV bucket kept populated and quiescent.

Rationale:

- **Rollback safety.** Keeping the legacy KV bucket populated (but not written to) means a rollback to a pre-phase-6 binary boots cleanly against the existing data — no recovery dance.
- **Migrations stay live.** The boot-time migrators are what would let a fresh phase-5-or-earlier deployment ingest an ES-only state (or vice versa). Removing them in lockstep with KV deletion would block any future rollback.
- **Interface review window.** Holding off on irreversible deletion lets us shape the new event/projection/manager APIs across all aggregates before committing to them. Once we've seen every aggregate land, we can revisit phase 7 as a single coordinated sweep.

Phase 7 unblocks once: (a) every aggregate has reached phase 6, (b) the new system has burned in across at least one production cycle, and (c) we've agreed the projection and mutator APIs are stable.

### Why migrations run at boot, not as a CLI subcommand

An earlier draft of this ADR (and a now-deleted `chatto evt migrate` CLI) had each aggregate's migration as a one-shot operator command. That can't work in the typical embedded-NATS deployment: with no TCP listener on the embedded NATS server, a second process can only connect by taking a temporary file lock on the data directory — which requires stopping `chatto run` first. That isn't an acceptable footgun for an alpha product where operators run a single binary.

Running the migrations at boot inside `NewChattoCore` avoids the multi-process problem entirely. The cost is one extra step at startup; the steady-state cost (after first boot) is a KV key scan and per-subject OCC check, both O(aggregates).

Manual re-runs are not currently exposed. If we ever need them — e.g. for testing or rolling back a botched stream — we'd add the surface (likely a GraphQL admin mutation) at that point.

### First aggregate: room membership

The first aggregate migrated end-to-end is **room membership** (`SERVER_CONFIG` keys `room_membership.{kind}.{roomId}.{userId}`). It is small, well-scoped, has multiple writers and multiple readers, and exercises bulk-mutation paths (account deletion, room deletion) that we will later need for messages. Once it is done, the seven-phase template is concrete and subsequent aggregates follow it mechanically.

### Migration events look like real events

A migration event is indistinguishable from one written at the time of the original action:

- `created_at`: the original record's creation timestamp, preserved from KV.
- `actor_id`: the original actor where known; a synthetic `system:migration` actor otherwise.
- No "this was migrated" flag.

Once a migration completes, no code branches on event provenance. The audit log treats the migrated record as canonical.

### Migrations are safely replayable

The always-OCC invariant from ADR-033 makes migration replayability automatic:

- The migration command iterates KV in a deterministic order per subject.
- Each event is published with `Nats-Expected-Last-Subject-Sequence` matching its position (0 for the first event on a subject, 1 for the second, …).
- A second full run against an already-migrated subject fails the OCC check on the first event and aborts that subject's migration without writing duplicates.
- A migration that crashed midway can resume: re-running iterates the same KV order, the already-emitted prefix is no-op'd by OCC, and the remainder appends.

Determinism is the migration command's responsibility: events for a given subject must be emitted in the same order across runs given the same KV state. This is a property of the iteration code, not of the framework.

### Write order during dual-write

Phase 4 mutations publish-event-first, then write KV. Rationale:

- If publish succeeds and KV write fails (process crash, NATS-OK-but-process-died, etc.), the event is in the log without a KV mirror. This matches the eventual steady state where KV is gone, so no special recovery is needed — the projection is already correct and reads after cutover will see consistent state.
- If we ordered KV-first, a crash between writes would leave KV ahead of the event log, requiring a reconciliation pass.

The OCC check on publish also protects against double-publishing across concurrent writers.

### No shadow-read divergence metric

An earlier design proposed serving reads from both KV and projection in parallel during a burn-in period, with a divergence counter to validate correctness before cutover. We are not doing this. The rationale:

- Chatto is alpha. Test-based validation of projection correctness is consistent with the project posture.
- Each migration is small (one aggregate). The blast radius of a bad cutover is bounded.
- Building and operating the divergence path is non-trivial and would slow every migration.
- If a specific migration is later judged high-risk (most plausibly: messages), we add shadow reads for that one aggregate without committing to it as the default.

If we hit a migration where this turns out to be the wrong call, we add the shadow-read path then.

## Consequences

- **Per-aggregate cadence.** Each migration is roughly seven PRs. Many can land in parallel across aggregates once the framework stabilises.
- **Two systems coexist for the duration.** The old `SERVER_EVENTS` stream, KV-as-source-of-truth code, and the new `EVT` stream all run side by side until the last aggregate is migrated. Test coverage spans both.
- **No big-bang failure mode.** Each aggregate's cutover (phase 5) is independently revertable while the migration is in flight. After phase 6, revert requires a recovery path — but by that point the aggregate has burned in.
- **Migration functions accumulate temporary surface.** Each aggregate's boot-migration call lives in `NewChattoCore` until phase 7. With phase 7 deferred indefinitely (see "Phase 7 on hold"), expect to carry the migration surface for the foreseeable future as the rollback-compatibility insurance policy.
- **No divergence safety net at cutover.** Cutover relies on test coverage and (for high-risk aggregates) opt-in shadow reads. A latent projection bug could cause user-visible incorrectness. We accept this for migration velocity in alpha and revisit if any migration burns us.
- **The framework matures through use.** Room membership shakes out the first version of the internal events package. Aggregates two through five will refine it; the remainder should be mechanical.
- **Messages migrate last.** Highest volume, largest blast radius, and the aggregate that ADR-033's RAM win actually unlocks. Migrating it after the framework has been validated against five smaller aggregates is the intended discipline.

## Out of scope for this ADR

- The specific protobuf event additions for each aggregate — decided per-aggregate at migration time.
- Snapshot strategy and the projection-restore-from-snapshot path — deferred.
- Long-term retention of the legacy `SERVER_EVENTS` stream after the last aggregate migrates — handled as a one-off cleanup later.
- A general "framework readiness" gate before starting aggregate two. We start aggregate two when room membership is fully done (phase 7); no separate framework-quality checkpoint.

## Related

- [ADR-033](ADR-033-event-sourced-state-with-projections.md) — the umbrella decision this ADR operationalises.
- [ADR-034](ADR-034-single-event-stream.md) — the shape of the new `EVT` stream.
- [ADR-006](ADR-006-kv-source-of-truth-streams-audit-log.md) — superseded by ADR-033. Each phase-7 decommission is a step toward fully retiring ADR-006's pattern.
