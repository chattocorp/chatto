# Agent connections

Import agent APIs from `runling/agents`. This entrypoint exports `agent`,
`runAgent`, `defineAgentExtension`, their types, `connectAgent`, and `taskTool`. Existing
agent exports from `runling` remain available.

Choose `output: "text"` for conversational agents:

```ts
const bot = await agent({ cwd: directory, model, output: "text" });
const result = await bot.run(ctx, "Hello", { onText: text => console.log(text) });
```

Set `systemPrompt` to replace Pi's default coding-agent prompt for a different
role, such as a chat assistant. `instructions` remain additional instructions;
Runling still adds its outcome rules in report mode.

Text mode delivers assistant messages through `onText` and finishes naturally.
It does not register `report_outcome`, add Runling's report instructions, or retry
for a missing report. The result keeps the usual usage and outcome fields, with
the final response in `summary`. An empty response or provider error produces a
failed outcome; `run()` throws for that outcome. If `onText` already sends replies
to the user, do not send the returned summary again.

The default, `output: "report"`, retains structured outcome reporting for coding
and specialist tasks. Both modes support steering, cancellation, and connections.
`runOutcome()` results can include a host-owned `failureReason`: `provider_error`
for provider failure, or `missing_outcome` when report mode exhausts its format
retry without a valid report. These differ from a model's own blocked assessment.
Use the reason instead of parsing the summary or assuming that research failed.
Recognized expired-login errors return a safe summary that asks the operator to
sign in again on the agent host. Raw provider diagnostics are not in this summary.

Use a connection when a task needs live input and asynchronous output:

```ts
import { agent, connectAgent } from "runling/agents";
import { task, type WorkflowContext } from "runling";

type Update = { type: "text"; text: string }
  | { type: "delivery"; text: string; consumed: boolean };

export const investigate = task(async (
  ctx: WorkflowContext<string, Update>,
  directory: string,
  question: string,
) => {
  await using worker = await agent({
    cwd: directory,
    model: "openai-codex/gpt-5.6-sol",
    tools: ["read", "grep", "find", "ls"],
  });
  await using connection = connectAgent(ctx, worker, {
    inbox: ctx.inbox,
    onText: text => ctx.emit({ type: "text", text }),
    onDelivery: (text, consumed) => ctx.emit({ type: "delivery", text, consumed }),
  });

  return await connection.runOutcome(question);
});
```

The parent can [spawn this task](task-channels.md), send messages, and consume
its updates. Input and output are explicit options; neither is connected
automatically. The callbacks can instead post to a host or collect test results.

A connection owns one inbox iterator across sequential turns. Call
`connection.runOutcome(prompt)` again for a repair or follow-up. Overlapping
turns are rejected. Input starts draining after the first interaction starts.
Closing input does not end the interaction. Pending consumption acknowledgements
do not prevent later messages from being forwarded. Delivery callbacks retain
the original message order.

`onDelivery` reports whether the agent consumed a message. Idle, rejected, or
unsupported steering reports `false`. The sender must retain or reroute missed
messages; the connection does not retry them. The Chatto coordinator retains a
copy of every user message.

Text callbacks run in order. A turn waits for queued text before returning,
including when the agent fails. Callback or input failure cancels the connection
and fails active work. Agent errors alone permit another turn.

The context signal, an optional per-turn `signal`, and disposal cancel pending
work. Cancellation releases the connection even if an agent or callback does
not cooperate, but it cannot stop that underlying code or undo side effects.
Dispose the connection before disposing its agent.

Use `await using`, or call `await connection.dispose()` in `finally`. Disposal
is idempotent, closes the inbox iterator, and prevents reuse. It does not dispose
the supplied agent. Within an `await using` scope, use `return await` so the
connection stays alive until the interaction finishes.

## Tasks as agent tools

`taskTool(ctx, definition, run)` adapts an explicit task call to a text-result
agent tool. Register it inside the run so it uses that run's context:

```ts
import { agent, defineAgentExtension, taskTool } from "runling/agents";
import { task, Type } from "runling";

const greet = task({
  name: "Greet",
  input: Type.Object({ name: Type.String() }),
  output: Type.String(),
}, (_ctx, { name }) => `Hello, ${name}`);

export const workflow = task(async (ctx) => {
  const tools = defineAgentExtension(pi => {
    pi.registerTool(taskTool(ctx, {
      name: "greet",
      label: "Greet",
      description: "Return a greeting for a name.",
      parameters: greet.input,
    }, greet));
  });

  await using worker = await agent({
    cwd: ".",
    tools: ["greet"],
    extensions: [tools],
  });
  return await worker.runOutcome(ctx, "Greet Ada.");
});
```

The callback receives the context and inferred arguments. It returns a string
or a promise of a string. Use a callback when you need to spawn a task, consume
its updates, or format its result. Those decisions remain explicit.

The adapter combines tool cancellation with the workflow signal, preserves
context fields and shared usage, and rejects an already-cancelled call before
invoking the callback. Running work must cooperate with the signal. Errors
propagate unchanged. The task still validates its input and output; the adapter
does not add validation or convert schemas. For a Standard Schema task, supply
a matching JSON schema in `parameters` and call the task in the callback.

## Conversations

### Background workflows owned by an agent

Start background work with `ctx.spawn(ctx => work(ctx, input))`, as for any other task.
Use `observeAgentTasks(ctx)` from `runling/agents` when an owning agent needs
snapshots and selected notifications from those runs. This adapter consumes run
output and owns cleanup after adoption. It does not create another execution or
identity, and does not depend on a chat service or model provider.

Each task has a local context buffer. `get(id)` and `list()` return detached
snapshots with the current lifecycle status, final result, workflow state, and
recent published output. Include these fresh snapshots in supervisor prompts,
including when a user asks for progress. Do not infer current state from an old
notification or a worker's earlier intention.

```ts
await ctx.emit({ type: "state", value: {
  phase: "validating", completedChecks: ["types"], pendingChecks: ["tests"],
} });
const connection = connectAgent(ctx, worker, {
  onText: text => ctx.emit({ type: "output", text }),
});
```

`state` replaces the complete workflow state and must be a JSON object of at
most 16,000 serialized characters. Invalid state causes observation to fail and
cancels unfinished work. Treat published objects as immutable: ordinary channels
pass values by reference, and the observer copies state when it receives it.
It is application-owned data, not a schema that Runling interprets. `stateAt`
and `stateAgeMs` identify its age. Terminal task status takes precedence over
the last phase; a completed task can also return a blocked result.

`output` silently buffers published agent text. It does not collect reasoning
or raw tool output. The buffer retains the last 16 messages, with at most 4,000
characters each, a sequence number, receipt time, and `output` or `finding` kind.
Entries set `truncated` when text was shortened; `droppedOutput` counts evicted
messages. Findings and legacy text updates also
enter the buffer. Output is historical, untrusted reference data, not proof of
progress. State and output updates alone do not wake the supervisor. Notifications
omit the buffer; read fresh snapshots to get it. Buffers remain available after
task completion and are lost when the manager is discarded or the process stops.

- `observe(name, run)` adopts an ordinary run with string input messages and
  `AgentTaskUpdate` output messages. It returns a snapshot with the run's ID.
  The task can return any result type. The run retains that value; the agent
  snapshot formats it as text, with JSON for non-string values. Do not separately
  iterate the adopted run's output. Observing the same run again is idempotent.
- Failed adoption leaves ownership with the caller. Dispose the run on rejection.
  Start work from the owning conversation's context so it can outlive a tool call.
- The child sends progress with `ctx.emit(text)` and reads clarifications from
  `ctx.inbox`. A coding agent can use `connectAgent` for steering and text output.
- Use `ctx.emit({ type: "finding", text })` for substantive findings that must
  remain in snapshots across later tool activity. Runling retains the receipt
  time and sends each update once through the coalesced progress channel.
  Applications must check the evidence; Runling does not validate these claims.
  The legacy `progress` field is removed when tool activity supersedes plain
  text; its historical buffer entry remains.
- Forward the agent's `onActivity` callback with `ctx.emit(activity)` to report
  tool attempts, successes, and failures. These records contain operation categories
  and failure counts, without arguments, paths, commands, or output. Failed records
  include a safe `error` category: `not_found`, `permission_denied`,
  `invalid_arguments`, `timeout`, or `unknown`. These categories describe a tool
  failure, not its root cause. Tool activity
  supersedes older prose in task snapshots. `lastToolFailure` retains an unresolved
  failure until that operation succeeds. Routine activity updates are coalesced;
  failures notify the owner after two failures of the same operation without a
  success. Set `toolFailureNoticeThreshold` to change this default. Permission
  failures notify immediately. Isolated failures remain visible in snapshots.
- Set `maxToolFailures` on the manager to stop children after repeated failures
  of an operation without success. The default has no limit. The final notification
  has `failureReason: "tool_failure_limit"` and follows cooperative child cleanup.
- `get(id)`, `list()`, `send(id, message)`, and `cancel(id)` operate only on this
  manager's handles. Include `list()` snapshots in the owner's incoming prompts
  for current status, latest progress, and `progressAgeMs`.
  `agentTasksExtension(manager)` exposes `task_send` and `task_cancel`; include
  those names in the agent's `tools`. It does not expose a status polling tool.
  Remove `task_status` from existing tool lists and use `list()` in host code.
- An agent's `onStatus` callback reports safe provider states: `retrying`
  (attempt, limit, and delay), `blocked` (`provider_error`), and `working`
  (recovered). Forward them with `ctx.emit(status)` and handle rejected sends
  after cancellation. These updates bypass the progress interval and contain
  no provider error text. Repeated retries do not send duplicate notifications.
  Recovery updates the snapshot without waking the owner. Provider failure does
  not trigger a report-format retry.
- Pass `manager.notifications` to `runAgentConversation` to wake an idle owner
  or steer its active turn. Pass `keepAlive: () => manager.active` to omit the
  idle deadline while delegated work is active. Completion must reach the owner
  through the notification stream.
- Use `prepareMessage(text, origin)` on `runAgentConversation` to add context
  once per input, including the initial prompt. `origin` is `user` for inbox
  messages and `notification` for background messages. Runling supplies this
  value from the input stream; message text cannot change it. The prepared
  message is reused if steering is rejected and it starts a later turn.
  `connectAgent` supports the same callback for its inbox and notification
  streams; its caller prepares the initial `runOutcome` prompt separately.
- Set `notifyActivity: false` on the task manager to keep routine tool activity
  in snapshots without waking the owner. Findings, failures, provider trouble,
  and completion still produce notifications.
- Always `await manager.dispose()` when the owning conversation ends. Parent
  cancellation also cancels children. Disposal waits for cooperative child
  cleanup; workflows must respect their abort signal.

For example, inside a tool callback:

```ts
const run = ctx.spawn((ctx: WorkflowContext<string, AgentTaskUpdate>) =>
  investigate(ctx, { question: input.question }));
try {
  return JSON.stringify(tasks.observe("Investigate", run));
} catch (error) {
  await run[Symbol.asyncDispose]();
  throw error;
}
```

`createAgentTasks` remains an alias for `observeAgentTasks`. Its legacy
`start(name, workflow, input)` method creates and observes an ordinary run,
returns its snapshot, and retains its string-result and state-validation behavior.
Existing callers do not need to migrate immediately.

The owning conversation connects the manager as follows:

```ts
try {
  return await runAgentConversation(ctx, owner, initialMessage, {
    notifications: tasks.notifications,
    keepAlive: () => tasks.active,
  });
} finally {
  await tasks.dispose();
  owner.dispose();
}
```

Notifications are JSON strings with `type`, `task`, and, for progress, `progress`.
Types are `task.progress`, `task.activity`, `task.tool_failed`, `task.retrying`, `task.blocked`,
`task.completed`, `task.failed`, and `task.cancelled`. Provider state is separate
from task completion. Latest progress and its age remain available during retries.
Tell the owner to treat these as reference data, summarize useful developments,
forward relevant user clarifications, and avoid repeated status polling. Child
updates are not sent directly to the user.

Progress is coalesced per task over 30-second intervals by default. Completion
replaces pending progress and is delivered immediately. At most one notification
per task is buffered. The default lifetime limit is 32 handles per manager;
completed handles remain queryable. Progress is limited to 4,000 characters and
results to 64,000 characters with a truncation marker. Raw failure messages are
not included in notifications. Handles and results are process-local.

### Conversation lifecycle

`runAgentConversation(ctx, agent, initialMessage, { timeout: 900 })` keeps one
connection open inside a task with `WorkflowContext<string, string>`. Incoming
`ctx.inbox` messages steer an active interaction or start another turn when idle.
Assistant text leaves through `ctx.emit`. Use a text-mode agent for chat replies.
For an owner that can finish a turn without a user-facing reply, configure its
agent with `output: "text", allowEmptyResponse: true`. A normal empty response
then completes successfully without emitting text. This does not accept provider
errors, aborted responses, or missing responses. Empty responses fail by default.

Spawn the task and send messages through its handle while consuming `updates`.
The helper returns the last summary after the idle timeout (seconds), which
restarts after each interaction. It throws on agent failure or cancellation.
The caller owns and disposes the agent. Optional `onBusy(boolean)` reports work
versus idle state, for example to control a typing indicator. Closing the input
stream stops delivery; timeout or cancellation still controls conversation exit.

The helper emits a conversation marker so its model turns and input waits share
one timeline lane. Working and waiting intervals retain their events and logs;
other task types keep their existing layout. Channels provide queued delivery,
not acknowledgement that the external chat service has posted an update.
