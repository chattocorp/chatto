# FDR-015: Managed Docker resources

**Status:** Experimental
**Last reviewed:** 2026-11-09

## Overview

Workflow authors can run commands and services in Docker containers. They can
share a private network between tasks and copy inputs and results. Explicit
ownership lets the workflow release resources when work finishes or fails.

## Behavior

- Image builds stream output, observe task cancellation and deadlines, and
  return an immutable image ID. Runling owns temporary build metadata. Images,
  tags, and daemon caches remain available for caller-controlled reuse.
- A command container streams its output while it runs. A nonzero exit fails
  the operation and preserves its exit code in the error.
- Server-loaded workflows share the host's Docker module so command events and
  output reach the same run journal, including after configuration reload.
- A service can wait for a command-based readiness probe before dependent work
  starts. Probes have a deadline that also limits a slow probe command.
- Cancellation and execution deadlines stop and remove containers. Cleanup can
  continue beyond the execution deadline.
- Operations inherit task cancellation and run identity from the required task
  context. Authors do not configure a separate client or copy these fields.
- Normal disposal stops a service and records successful resource completion.
  Cancellation, nonzero exit, and output failures record failed activity.
- A successfully exited container retains its files until disposal. Authors can
  copy artifacts before releasing it. A convenience operation removes a command
  container before returning its successful result.
- Networks belong to an explicit scope and must be released after their
  containers. Runling does not remove unrelated resources.
- All environment values are masked in streamed output, including values split
  across output chunks. Masking does not cover encoded or transformed values.
  Environment values cannot contain newlines.
- Container and network labels identify the owner and an optional run. Crashes and daemon
  failures can leave resources for application or operator cleanup.

## Design Decisions

### 1. Make ownership explicit

**Decision:** Use asynchronous disposal and require the task context for each
operation, so task cancellation and run identity are automatic.
**Why:** A service must live as long as the tasks that use it. This follows
[ADR-009](../adr/ADR-009-task-invocations.md) and
[ADR-014](../adr/ADR-014-managed-docker-resources.md).
**Tradeoff:** Authors must release resources in the correct order. Networks are
released by their owner, not automatically when a cancellation signal fires.

### 2. Stream output without retaining a complete copy

**Decision:** Send bounded log entries to Runling and optional output consumers.
**Why:** Long builds must show progress without an unbounded in-memory buffer.
**Tradeoff:** Callers who need captured output must collect it with their own
size limit. Probe diagnostics and raw Docker CLI errors are not recorded because
they can include credentials.

## Related

- **ADRs:** [ADR-009](../adr/ADR-009-task-invocations.md),
  [ADR-010](../adr/ADR-010-workflow-state-snapshots.md),
  [ADR-014](../adr/ADR-014-managed-docker-resources.md)
- **FDRs:** [FDR-012](FDR-012-workflow-state.md),
  [FDR-014](FDR-014-activity-log.md)

## Open Questions

- Whether workflows need managed named volumes and image pushes.
- Whether a separate application should reconcile labelled resources after a
  crash. Snapshot recovery does not currently perform this work.
