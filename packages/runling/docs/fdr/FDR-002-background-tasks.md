# FDR-002: Background agent tasks

**Status:** Experimental
**Last reviewed:** 2026-09-22

## Overview

An owning agent can delegate a workflow, continue receiving user input, and
receive selected updates while that workflow runs.

## Behavior

- Background agent work uses ordinary child runs, with the same identity,
  messages, cancellation, and typed result. Supervision adopts the runs to retain
  snapshots and issue notifications. A tool can return before the run completes.
- The run keeps its original result type. Agent snapshots format the result as
  text. Callers do not need a string-returning wrapper around each task.
- The owner can read task snapshots, send clarifications, and cancel its tasks.
  Sending input acknowledges queue acceptance, not agent consumption.
- Notifications have one consumer and at most one pending entry per task.
  Completion replaces stale progress. Progress notices are coalesced; selected
  provider and tool failures can request attention sooner.
- The manager stays active while a task runs or an unread notification remains.
  The host must connect notifications to its conversation loop.
- Completed and cancelled task snapshots remain available for the manager's
  lifetime. Disposal aborts children and awaits their run cleanup promises.
  Direct cancellation through a run handle also reaches supervision.
- A completed workflow can return a blocked agent outcome. Completion alone
  does not establish that the requested work succeeded.
- The server terminal reports live task, agent, and tool activity with a run
  reference and a static agent label or task number. Successful tools are grouped
  into periodic counts; failures appear immediately. State updates can explicitly
  supply a public host-owned `activity` message without waking the supervisor.
  State values, message content, and tool arguments remain out of these logs.

## Design Decisions

### 1. Let the owner decide what the user sees

**Decision:** Deliver task notifications to the owner rather than automatically
posting each child update to the user.
**Why:** The owner can interpret progress in the conversation's context.
**Tradeoff:** The host and agent instructions must arrange useful announcements
and updates. Runling does not guarantee a user-facing message before delegation.

### 2. Bound notifications

**Decision:** Coalesce pending updates and keep task state separately.
**Why:** Repeated progress should not create an ever-growing notification queue.
**Tradeoff:** Notifications are not a complete event history. Read the latest
snapshot when preparing a response.

## Related

- [ADR-002: Agent ownership](../adr/ADR-002-agent-ownership.md)
- [ADR-003: Task context](../adr/ADR-003-task-context.md)
- [FDR-003: Task context snapshots](FDR-003-task-context.md)
- [Agent API guide](../agents.md) and [task channels](../task-channels.md)
- [Server logs](../server-logs.md)

## Open Questions

- How should a long conversation retire completed task records without losing
  context that the owner still needs?
