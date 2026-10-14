# ADR-009: Typed task invocations are the execution model

**Status:** Accepted
**Date:** 2026-10-14

Supersedes [ADR-003](ADR-003-task-context.md). Amends
[ADR-002](ADR-002-agent-ownership.md).

## Context

Runling has too many concepts for one execution model. A user must learn these
concepts:

- Five meanings of "task": the `task()` wrapper, the supervised agent task,
  the task tools, `taskTool`, and the task events.
- Two meanings of "run": the recorded top-level run and the handle that
  `ctx.spawn` returns.
- Two ways to start a task: a direct call without message channels, and
  `ctx.spawn` with message channels.
- Four ways to operate an agent, two ways to publish task state, and three
  result shapes.
- Two schema modes: TypeBox tasks return synchronously and Standard Schema tasks
  return a promise.

Most of the agent supervision layer was made for one application. It is
difficult to explain these concepts and difficult to change them.

## Decision

Runling has five primitives.

1. **A task is a function with schemas.** A task has a name, an input schema,
   and an output schema. A task can also have a `receives` schema for messages
   from its parent and an `emits` schema for messages to its parent. If a
   message schema is not given, the task accepts any message. A task always
   returns a promise.
2. **A workflow run is one top-level task invocation.** A run has one identity
   and one journal. The result of a run is the output of its task. Runling does
   not change the shape of the output.
3. **A task can invoke other tasks.** `ctx.start(task, input)` returns an
   invocation handle. The handle has `send`, `updates`, `result`, and `cancel`.
   A direct call, `await task(ctx, input)`, starts the task and waits for its
   result. Runling records the updates of a direct call in the journal.
4. **A parent and a child exchange messages while the child runs.** The parent
   sends messages to the child with `send`. The child reads them from
   `ctx.inbox`. The child sends messages to the parent with `ctx.emit`. The
   parent reads them from `updates`. Use this for agent steering, for agent
   turns and messages, and for progress of tasks that are not agents.
5. **Routers start workflow runs.** Event sources and webhook routes are
   routers. A task cannot start a workflow run. If a task must cause a different
   run, it uses an external mechanism, for example an HTTP request to a webhook
   route.

The host is the parent of the top-level task. The CLI, the web console, and a
router use the same channels as a parent task. The host shows the updates of
the top-level task. The host can send messages to a running workflow. This
replaces `onText`, `onInput`, and `input()`.

An agent is a task. It receives steering messages and emits its turns and
messages. `taskTool` makes a task available as a tool of an agent.

A run can be active for a long time, as an agent harness session can. A parent
can wait for a child for hours. Runs and invocations have no default timeout. A
caller can set a timeout. The web console must show a long wait clearly.

These items are not primitives:

- `step()` and usage accounting continue as internal infrastructure.
- `ctx.publishState`, the normalization of `WorkflowResult`, and the
  synchronous schema mode are removed.
- The agent supervision layer is removed: `observeAgentTasks`,
  `runAgentConversation`, task notifications, the approval queue, and the
  authorization classifier. Applications can implement this policy with the
  primitives.

## Consequences

Users learn one invocation model. The same task can run as a workflow, as a
child of another task, or as an agent tool. Message schemas make the channel
contract of a task visible in its definition.

This is a breaking API change. Runling is at version 0.x and ChattoBot is its
only known user. ChattoBot must change to the new API before Runling removes the
old API.

[ADR-001](ADR-001-process-local-workflows.md) continues to apply. A long run
stops when its process stops, and Runling does not resume it. Long runs make
this limit more visible.

A parent that waits for a long child does not process its own inbox during the
wait, unless its code does so. For example, an agent tool that waits for an
implementation task must decide what to do with new user messages. It can send
them to the child as steering messages, or it can reply that the work continues.

[ADR-006](ADR-006-parent-child-communication.md) continues to apply. The
`notice` update and `observe` in ADR-006 are application conventions, not
Runling primitives. [ADR-005](ADR-005-untrusted-context.md),
[ADR-007](ADR-007-authorization-classifier.md), and
[ADR-008](ADR-008-parent-owned-approvals.md) describe policy of one
application. Withdraw them when Runling removes the related code.
