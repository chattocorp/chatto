# FDR-005: Task state in the run inspector

**Status:** Experimental
**Last reviewed:** 2026-09-24

## Overview

A task can show its current state to a person who inspects a run in the web app.
The task chooses when to publish a snapshot. This helps the person understand
work that has no useful log message at every step.

## Behavior

- The task publishes a JSON object. The latest object appears in that task's
  details while the run is active and after it ends. On wide screens, a task
  reference in the log opens these details beside the log.
- A new snapshot replaces the task's previous visible state. Other task
  invocations keep their own state.
- The console keeps details for recently viewed runs during the browser session.
  If a new run does not load, it shows a retry action after ten seconds.
- Published state remains in the run history. A process restart does not resume
  the task from that state.
- Publishing state does not send a message to another task or wake an agent.
  Tasks use input, update messages, and results to communicate.

## Design Decisions

### 1. Publish snapshots at chosen points

**Decision:** A task publishes a detached snapshot when its visible state changes.
Runling does not watch local objects for mutations.

**Why:** The task controls which data appears in the web app and when it changes.

**Tradeoff:** A later change to a local object needs another publish call.

### 2. Keep state with each task invocation

**Decision:** Each invocation has its own latest snapshot, identified by its
timeline task ID.

**Why:** The viewer can see which task published the data. A parent and child can
publish state without sharing a mutable object.

**Tradeoff:** The parent cannot read a child's state as a communication channel.

## Related

- [ADR-001: Process-local TypeScript workflows](../adr/ADR-001-process-local-workflows.md)
- [FDR-003: Task context snapshots](FDR-003-task-context.md)
- [Task channels](../task-channels.md)
