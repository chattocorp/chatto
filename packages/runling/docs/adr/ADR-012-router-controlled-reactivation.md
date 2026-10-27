# ADR-012: Router-controlled workflow reactivation

**Status:** Accepted
**Date:** 2026-10-27

Amends [ADR-009](ADR-009-task-invocations.md) and
[ADR-010](ADR-010-workflow-state-snapshots.md).

## Context

A workflow can need another input after its root has returned. Keeping the root
waiting only to preserve identity creates idle executions. Server shutdown then
interrupts those executions even when no work remains.

## Decision

A root return ends its execution. A router can call `ctx.start(task, { input,
runId })` to start another execution of a retained workflow. The route owner and
registered task key must match. An active execution rejects reactivation; the
router owns queuing and retry policy.

Reactivation retains the ID, reference, state, files, and journal. It uses fresh
input and calls the root from the beginning. It does not restore a JavaScript
stack, replay messages, or repeat external effects automatically. Earlier
executions remain in the journal; the console retains their logs and timeline
on one time axis, with combined usage. Input, result, and status describe the
latest execution. Each execution handle controls only its own execution.

Sources can request retained finished runs from `ctx.runs({ includeFinished:
true })`. Completed, failed, and cancelled executions remain inactive until a
router explicitly reactivates them. Existing recovery of interrupted executions
is unchanged. No paused status or background workflow instance is added.

Final workflow state is saved before execution completion settles. Periodic
snapshots still cover active executions. Snapshot and journal writes are not one
transaction with each other or with external systems. A crash during execution or
final persistence can still leave an older state snapshot.

## Consequences

- Idle workflows consume no task execution and produce no shutdown interruption.
- Applications can replace waiting loops with finite roots and explicit routing.
- Retention uses the latest execution's terminal time. A deleted workflow cannot
  be reactivated; the router must start a new one.
- Reactivation is not an automatic retry. Applications remain responsible for
  authorization, duplicate delivery, and uncertain external outcomes.
- A terminal execution performs a state snapshot write. A save failure rejects
  execution completion and is logged; it does not erase the task's journal outcome.
