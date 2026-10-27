# ADR-009: Typed task invocations are the execution model

**Status:** Amended by ADR-010 and ADR-012
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
   and a final `result` schema. A task can also have a `steering` schema for messages
   from its parent and an `output` schema for messages to its parent. If a
   message schema is not given, the task accepts any message. A task always
   returns a promise.
2. **A workflow execution is one top-level task invocation.** Under
   [ADR-012](ADR-012-router-controlled-reactivation.md), routers can reactivate
   retained runs for another execution. A run has one identity
   and one journal. The result of a run is the output of its task. Runling does
   not change the shape of the output.
3. **A task can invoke other tasks.** `ctx.start(task, input)` returns an
   invocation handle. The handle has `steer`, `output`, `result`, and `cancel`.
   A direct call, `await task(ctx, input)`, starts the task and waits for its
   result. Runling records the updates of a direct call in the journal.
4. **A parent steers a child; the child streams output.** The parent
   sends steering to the child with `steer`. The child reads it from
   `ctx.steering`. The child streams output to the parent with `ctx.emit`. The
   parent reads them from `output`. Use this for agent steering, for agent
   turns and messages, and for progress of tasks that are not agents.
5. **Routers start workflow runs.** Event sources and webhook routes are
   routers. A task cannot start a workflow run. If a task must cause a different
   run, it uses an external mechanism, for example an HTTP request to a webhook
   route. A router gets a handle for each run that it starts. It can
   keep the handle and use it later to send messages to the run, to close the
   run's input, or to cancel the run. For example, a chat router sends each new
   message in a thread to the run of that thread.

The host is the parent of the top-level task. The CLI, the web console, and a
router use the same channels as a parent task. The host chooses which updates
of the top-level task to display. The host can send messages to a running
workflow. This replaces `onText`, `onInput`, and `input()`. When a task waits for a message,
the journal records the wait, and the console shows it.

An agent is a task. It receives steering messages and emits its turns and
messages. `taskTool` makes a task available as a tool of an agent.

The directional names distinguish steering, streamed output, and the final
result. Schemas infer TypeScript types and validate values at both stream
boundaries. A parent can consume, buffer, forward, or ignore output. Runling
does not make output into a new model turn. Async iterables and `ctx.emit` use
the existing bounded channels; no generator or second execution model is needed.

`agent()` defines that task without starting a model session. Each invocation
creates one session and owns its cancellation and disposal. Agent tasks return
their final text. They accept string steering messages and emit completed
assistant replies. Messages read but not delivered in the active turn become
subsequent turns. The task finishes when its current work ends, even if its
input is still open. Async setup resolves resources per invocation after input
validation.

`createAgentSession()` exposes the lower-level session API for custom
conversation loops, report outcomes, and forks. This replaces the old
session-returning `agent()` API. A live session is stateful and cannot run two
turns at once; a task definition is reusable across concurrent invocations.

A run can be active for a long time, as an agent harness session can. A parent
can wait for a child for hours. Runs and invocations have no default timeout. A
caller can set a timeout. The web console must show a long wait clearly.

These items are not primitives:

- `step()` continues as a helper that shows a named part of a task in the log
  and on the timeline. It has no schemas and no messages.
- `ctx.publishState`, the normalization of `WorkflowResult`, and the
  synchronous schema mode are removed.
- The agent supervision layer is removed: `observeAgentTasks`,
  `runAgentConversation`, task notifications, the approval queue, and the
  authorization classifier. Applications can implement this policy with the
  primitives.

Background work is an ordinary task invocation that the parent does not await
immediately. The parent retains the promises or handles it needs, processes their
updates and results, and waits for or disposes its children before exiting.
Runling has no separate background-job execution API or public child registry.
`createClassifier` defines a tool-free task with a validated output; its policy
belongs to the caller. It does not restore the former approval subsystem.

Runling also owns agent runtime temporary files and session storage mechanics.
Run-owned sessions follow run retention. `AgentSessionStore` supports explicitly
shared sessions with exclusive process-local leases and independent inactivity
retention. Applications select scopes and decide whether a new turn is authorized.

## Consequences

Users learn one invocation model. The same task can run as a workflow, as a
child of another task, or as an agent tool. Message schemas make the channel
contract of a task visible in its definition.

This is a breaking API change. Runling is at version 0.x and ChattoBot is its
only known user. ChattoBot must change to the new API before Runling removes the
old API.

[ADR-010](ADR-010-workflow-state-snapshots.md) adds snapshots and root re-entry
for registered server workflows. Execution remains process-local. Recovery does
not reconstruct the task hierarchy or resume JavaScript execution.

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
