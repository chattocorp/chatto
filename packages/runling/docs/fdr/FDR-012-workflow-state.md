# FDR-012: Workflow state and restart recovery

**Status:** Experimental
**Last reviewed:** 2026-10-18

## Overview

Server workflows keep application-owned state across restarts through periodic
snapshots. Recovery re-enters the root task; it does not resume JavaScript execution.

## Behavior

- `RUNLING_DATA_DIRECTORY` selects an instance's storage root. Run journals and
  snapshots use `runs/`; server diagnostics use `logs/`. This lets several
  application instances use one configuration module with separate storage.
  Without the setting, existing project storage paths remain in use.

- Each server root receives its own mutable `ctx.state` object, initially empty.
  State contains JSON data. Child contexts have independent, unsaved objects.
- The server snapshots all workflow state every 60 seconds, with the next interval
  starting after the previous save settles. Shutdown stops the timer, waits for
  cooperative cleanup, then saves once more.
- Invalid state or disk errors fail the save and retain the previous snapshot.
  Periodic failures are logged without state content; a final save failure rejects
  shutdown. Malformed snapshots fail startup rather than silently losing state.
- Open workflows whose roots have stable `workflows` configuration keys re-enter
  automatically before event sources start. They retain their IDs and references.
  Their `ctx.run.resumed` flag is true even when the saved state is empty.
- Missing definitions leave state available for a later configuration that restores
  the key. Completed, failed, or explicitly cancelled workflows do not restart.
- Roots decide which children and connections to create again. Child state can be
  passed as an ordinary sub-object in input. Input validators that transform data
  can replace that reference; task authors must account for this.
- The console shows the latest attempt's activity and total token usage and cost
  across all attempts. Usage updates and final records replace the current
  attempt's total; they do not add the same tokens again. The reader also totals
  older journals without a migration. Missing prices remain marked incomplete.
  Earlier attempts remain in the journal file. Logs and agent transcripts are
  not copied into the snapshot.
- A source can inspect `ctx.runs()` and use original inputs to rebuild routing.
  Inboxes, adapter state maps, and agent sessions are not automatically restored.
- `runling run` has no automatic recovery in this slice. `runWorkflow` accepts a
  state object for hosts that own their own storage.
- At startup, and at most once per day from the existing
  persistence timer, the server purges runs closed more than 30 days ago. The latest
  journal must record completion, failure, or explicit cancellation and a closure
  time. Creation time and file modification time do not determine retention.
- Running and interrupted workflows remain available regardless of age. Roots own
  their inactivity and completion rules. Missing or unreadable journals do not
  establish closure and therefore do not authorize state deletion.
- Cleanup removes state, run-owned files, journals, and cached run records. Recorded
  runs expose their file directory through `ctx.run.filesDirectory`.
  `agentSessionFile(ctx, key)` creates a stable private transcript path there;
  unrecorded hosts without a directory receive `undefined` for in-memory sessions.
  Filesystem failures are logged without application data and retried on the next
  cleanup; other runs and normal saves can proceed. No application callback is needed.
- Cleanup saves the reduced snapshot before deleting run files and then journals.
  If that save fails, in-memory state is restored and files remain. A remaining journal makes a
  partial cleanup safe to retry. Shutdown waits for in-flight cleanup.

## Design Decisions

### 1. Application-owned recovery

**Decision:** Re-enter registered roots with saved state instead of restoring their
execution hierarchy.
**Why:** Roots know how to reconnect resources and reconcile external outcomes.
**Tradeoff:** Applications must implement and test recovery before registering roots.

### 2. Periodic snapshots

**Decision:** Save ordinary state periodically and at graceful shutdown.
**Why:** This keeps task code free of observers and explicit persistence calls.
**Tradeoff:** Abrupt exit can lose changes since the last successful save.

### 3. Retention after closure

**Decision:** Keep terminal runs for 30 days, then purge them on startup or the daily check.
**Why:** Recent runs remain available for inspection without retaining closed workflow
state indefinitely. Active work has no generic age limit.
**Tradeoff:** Historical inspection ends when retention expires. Files stored outside
the run directory remain caller-owned.

## Related

- [ADR-010: Periodic workflow state snapshots](../adr/ADR-010-workflow-state-snapshots.md)
- [Workflow state guide](../workflow-state.md)
