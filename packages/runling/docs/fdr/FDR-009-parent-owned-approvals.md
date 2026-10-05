# FDR-009: Parent-owned approvals

**Status:** Experimental
**Last reviewed:** 2026-10-03

## Overview

An owning agent can decide whether a child may perform a proposed action. The
child waits while the owner continues to receive user input.

## Behavior

- Applications grant standing permission for routine operations and select
  actions that need approval.
- A pending action wakes the owner through a child notice. Only an explicit
  allow decision permits execution.
- If more human authority is necessary, the owner asks the user and leaves the
  action pending. The child cannot grant itself authority.
- Each decision applies once to one action in one owner's queue. Changed tool
  arguments need a new decision.
- Cancellation, restart, timeout, a full queue, and notification failure cannot
  grant permission. Requests expire after five minutes by default.
- Approval cannot add tools, remove host restrictions, or undo completed effects.

## Design Decisions

### 1. The owner decides

**Decision:** Requests go to the agent that owns the work.
**Why:** That agent knows the user's request and controls external communication.
**Tradeoff:** Each approval costs an owner turn. Routine work needs standing
permission to avoid repeated decisions.

### 2. Permissions belong to live actions

**Decision:** Decisions are single-use and do not survive cancellation or restart.
**Why:** Old approval must not authorize changed work.
**Tradeoff:** A resumed task must request approval again.

## Related

- [ADR-008: Parent-owned approvals](../adr/ADR-008-parent-owned-approvals.md)
- [ADR-006: Parent-child communication](../adr/ADR-006-parent-child-communication.md)
- [FDR-002: Background agent tasks](FDR-002-background-tasks.md)
- [FDR-007: Authorization classifier](FDR-007-authorization-classifier.md)
- [Approval API guide](../approvals.md)
