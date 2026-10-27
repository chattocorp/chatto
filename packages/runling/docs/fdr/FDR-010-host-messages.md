# FDR-010: Messages between hosts, tasks, and runs

**Status:** Experimental
**Last reviewed:** 2026-10-18

## Overview

A parent can steer a task while it runs, and the task can stream output back.
The final result is separate from streamed output. The host of a
workflow run is the parent of the top-level task. Thus a person at a terminal, a
person in the web console, an HTTP client, or a router can talk to a running
workflow in the same way as a parent task talks to a child.

## Behavior

- A task can declare a schema for the messages that it receives and a schema for
  the messages that it emits. Without a schema, the task accepts any message.
  Runling rejects an invalid message at the sender.
- A parent that starts a child gets a handle. It can steer the child, close
  steering, read output, wait for the result, and cancel the child. A direct
  call waits for the result. Its steering is closed, and Runling records its
  streamed output and then discards it.
- Both stream directions have inferred types and runtime validation when the
  task declares their schemas. Output is data for the parent. It does not cause
  a model turn or user notification unless the parent implements that behavior.
- A sent message is in the queue of the receiver. Acceptance does not mean that
  the receiver read or used the message.
- Queues are bounded. A sender to a full or closed queue gets an error at once.
  Runling does not wait for free space and does not drop messages without an
  error.
- `runling run` on an interactive terminal sends each typed line to the
  top-level task and records updates in its journal. Terminal display follows
  [FDR-011](FDR-011-cli-output.md).
- The web console shows a message composer for a running run. Text that is
  valid JSON is sent as JSON. Other text is sent as a string. The Log view shows
  the messages between the host and the top-level task.
- An HTTP client can send a message to a running run. The server reports
  whether the run accepted the message, the message is invalid, the run is not
  running, or the run has too many unread messages.
- A router that starts a run gets a run handle. It can keep the handle after
  it returns and later send messages, close the input, cancel the run, or wait
  until the run finishes. For example, a chat router keeps one run for each
  thread.
- Each host run has its own event and log context. If a callback dispatches a
  new run while another run is active, the new run does not inherit that run's
  journal listeners, active task, or log indentation. It can continue after
  the earlier run finishes. Child task invocations still share their run.
- When a task waits for a message, the journal records the wait. The timeline
  shows "Waiting for a message". When a run has no other active work, the run
  list shows it as waiting for a message.
- The journal records each message, each read, and each wait. The server log
  records waits without message content.
- Runs and invocations have no default timeout. A run can wait for a message for
  a long time.
- Run handles and queued messages are process-local. A process restart loses
  them. Registered roots can re-enter with snapshot state, and sources can obtain
  new handles through `ctx.runs()`. Queued messages remain lost.

## Design Decisions

### 1. The host is the parent of the top-level task

**Decision:** The CLI, the web console, and routers use the same channels as a
parent task.
**Why:** One message model replaces separate callbacks for text output and input
requests. A task works the same as a workflow and as a child
([ADR-009](../adr/ADR-009-task-invocations.md)).
**Tradeoff:** A task that needs a reply must emit a question and then read its
inbox. Runling does not pair questions with answers.

### 2. Routers start runs, tasks do not

**Decision:** Only routers start workflow runs. A task that must cause a
different run uses an external mechanism, such as a webhook request.
**Why:** A run has one identity and one journal. Routers already own delivery,
routing state, and deduplication.
**Tradeoff:** Routers keep their own map of active runs, for example one run for
each chat thread.

### 3. Make waits visible

**Decision:** Record each wait for a message and show it in the console.
**Why:** A run that waits for a person looks idle. Without the record, a reader
cannot tell a waiting run from a stuck run.
**Tradeoff:** A task that reads its inbox in the background shows a wait also
while it does other work.

### 4. No default timeout

**Decision:** Runs and invocations wait without a deadline unless the caller
sets one.
**Why:** Agent sessions and conversations can be active for hours. A fixed
default would stop valid work.
**Tradeoff:** A run that waits for a message that never comes stays active
until a person or a router cancels it, or the process stops.

## Related

- **ADRs:** [ADR-001](../adr/ADR-001-process-local-workflows.md),
  [ADR-004](../adr/ADR-004-event-sources.md),
  [ADR-006](../adr/ADR-006-parent-child-communication.md),
  [ADR-009](../adr/ADR-009-task-invocations.md),
  [ADR-010](../adr/ADR-010-workflow-state-snapshots.md),
  [ADR-012](../adr/ADR-012-router-controlled-reactivation.md)
- **FDRs:** [FDR-001](FDR-001-agent-interactions.md),
  [FDR-004](FDR-004-event-sources.md), [FDR-011](FDR-011-cli-output.md)
- Guides: [task invocations and messages](../task-channels.md),
  [workflow messages and output](../workflow-messages.md),
  [webhook routing](../webhook-routing.md)

## Open Questions

- Should the console show which schema a run accepts for its messages?
- Should a host be able to close the input of a run from the console?
