# Agent connections

Import agent APIs from `runling/agents`. This entrypoint exports `agent`,
`runAgent`, `defineAgentExtension`, their types, `connectAgent`, and `taskTool`. Existing
agent exports from `runling` remain available.

Choose `output: "text"` for conversational agents:

```ts
const bot = await agent({ cwd: directory, model, output: "text" });
const result = await bot.run(ctx, "Hello", { onText: text => console.log(text) });
```

Text mode delivers assistant messages through `onText` and finishes naturally.
It does not register `report_outcome`, add Runling's report instructions, or retry
for a missing report. The result keeps the usual usage and outcome fields, with
the final response in `summary`. An empty response or provider error produces a
failed outcome; `run()` throws for that outcome. If `onText` already sends replies
to the user, do not send the returned summary again.

The default, `output: "report"`, retains structured outcome reporting for coding
and specialist tasks. Both modes support steering, cancellation, and connections.

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

`runAgentConversation(ctx, agent, initialMessage, { timeout: 900 })` keeps one
connection open inside a task with `WorkflowContext<string, string>`. Incoming
`ctx.inbox` messages steer an active interaction or start another turn when idle.
Assistant text leaves through `ctx.emit`. Use a text-mode agent for chat replies.

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
