# FDR-003: Task context snapshots

**Status:** Experimental
**Last reviewed:** 2026-09-22

## Overview

Each managed background task can retain structured state and recent published
output. A supervisor can use this context to answer progress questions without
interrupting the child.

## Behavior

- A state update replaces the task's JSON object. Snapshots include its update
  time and age. The workflow defines the fields; Runling does not define phases
  such as investigation, validation, or publication.
- Buffer-only output retains commentary without replacing progress or notifying
  the owner. Findings enter the buffer and can notify the owner.
- History entries have a sequence, timestamp, kind, and text. The snapshot
  reports evicted entries and marks clipped text.
- Reads return detached copies. Mutation of a snapshot does not change retained
  state. Lifecycle status remains authoritative over an old workflow phase.
- The buffer contains explicitly published output, not an automatic transcript
  of tools, private reasoning, or every agent message.
- State and history remain after the task finishes, for the manager's lifetime.
  They do not survive process restart.

## Design Decisions

### 1. Keep a bounded recent history

**Decision:** Currently retain the latest 16 entries, with at most 4,000 characters
per entry. State is limited to 16,000 serialized JSON characters; oversized state
is rejected. The final result is separate and clipped at 64,000 characters, with
a truncation marker.
**Why:** Bound retained context and avoid unbounded prompt growth when hosts use it.
**Tradeoff:** These are provisional caps, not measured optimal defaults. A message
count does not represent token cost or importance. Earlier findings can be evicted.

### 2. Separate storage from attention

**Decision:** State and buffer-only updates do not create notifications.
**Why:** The owner needs context on demand without replying to every child event.
**Tradeoff:** Producers must explicitly publish findings when the owner needs to act.

## Related

- [ADR-003: Task context](../adr/ADR-003-task-context.md)
- [FDR-002: Background tasks](FDR-002-background-tasks.md)
- [Agent API guide](../agents.md)

## Open Questions

- Should a configurable character or token budget replace the 16-entry cap?
- Should accepted findings have a separate retention policy from commentary?
- How should applications select context for a prompt when many tasks exist?
