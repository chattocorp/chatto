# ADR-010: Periodic workflow state snapshots

**Status:** Amended by ADR-012
**Date:** 2026-10-18

Amends [ADR-001](ADR-001-process-local-workflows.md) and
[ADR-009](ADR-009-task-invocations.md).

## Context

Long-lived workflows need application state after a server restart. Restoring
JavaScript execution or a complete task hierarchy would make Runling more complex.
The application can decide how to recover if it receives its previous state.

## Decision

[ADR-012](ADR-012-router-controlled-reactivation.md) adds explicit reactivation
of finished workflows and a final state save when each execution ends.

The server owns one plain object with the original input, a recovery key, an open
flag, and application state for each workflow. It writes the complete object to
`state.json` in the run journal directory every 60 seconds and after cooperative
task cleanup at shutdown. Writes use a temporary file and rename. Only one write
runs at a time. There are no mutation observers, explicit task save calls, or new
storage dependencies.

The root receives its mutable state as `ctx.state`. Child invocations do not
inherit that object. Parents can pass selected sub-objects through ordinary task
input. The parent owns the shape and lifetime of those sub-objects.

The `workflows` configuration map gives recoverable root definitions stable keys.
Routers must start the same task objects that this map contains. State is saved
for other server roots too, but Runling cannot recover their definitions. A
missing key leaves the workflow interrupted and preserves its state.

After restart, the server calls open, registered roots again with their original
input and saved state. It keeps the run ID and reference, and records a new
execution attempt in the existing journal. The console retains activity and logs
from all attempts, with total token usage and cost across all attempts.
A newer terminal journal record prevents recovery from a stale open snapshot.
Runling does not rebuild child tasks or restore message queues, local variables,
connections, or agent sessions. The root owns those decisions.

The server purges terminal runs 30 days after their recorded closure. Cleanup runs
at startup and daily through the persistence timer.
It preserves open and interrupted runs. Recorded runs have a Runling-owned file
directory, exposed as `ctx.run.filesDirectory`. `agentSessionFile(ctx, key)` creates
stable transcript paths there. Cleanup saves state removal before deleting run
files and then journals, so partial cleanup remains retryable. Retention behavior is
specified in [FDR-012](../fdr/FDR-012-workflow-state.md).

## Consequences

- State updates are ordinary JavaScript mutations. State must be JSON data.
- A crash can lose changes since the last snapshot, including newly started
  workflows. Failed saves can make the recovery point older than 60 seconds.
- Atomic replacement prevents partial JSON files. The snapshot is not a
  transaction with external systems and has no power-loss durability guarantee.
- A root must reconcile uncertain external outcomes before repeating actions.
- Shutdown interrupts execution but keeps the workflow open for recovery. Normal
  completion, task failure, and explicit cancellation close it.
- Source adapters can use `ctx.runs()` to rebuild routing to recovered roots.
  Source state maps remain process-local.
- One server owns a snapshot file. Concurrent server writers are unsupported.
- The library and one-shot CLI do not own this server snapshot. Library callers
  can supply `runWorkflow(root, { input, state })`; their host owns persistence.
- Registering a root opts it into re-entry. Existing applications must first make
  their root recovery-aware; Runling does not assume old tasks are safe to repeat.
