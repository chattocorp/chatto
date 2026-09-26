# Task channels

Use `ctx.spawn()` when a parent must exchange data with a running child. Tasks remain
ordinary functions. The context type declares incoming messages and outgoing
updates:

```ts
import { task, type WorkflowContext } from 'runling';

const sum = task(async (ctx: WorkflowContext<number, { total: number }>) => {
  let total = 0;
  for await (const amount of ctx.inbox) {
    total += amount;
    await ctx.emit({ total });
  }
  return total;
});

await using run = ctx.spawn((ctx: WorkflowContext<number, { total: number }>) => sum(ctx));
await run.send(2);
await run.send(3);
run.closeInput();

for await (const update of run.output) {
  console.log(update.total);
}
const total = await run.result; // 5
```

`ctx.spawn(ctx => work(ctx, input))` calls the function immediately and returns a
`Run<Incoming, Update, Result>`:

- `id` identifies this child run and its recorded message channel. It is separate
  from the top-level server run's readable reference and journal ID.
- `status` is `running`, `completed`, `failed`, or `cancelled`.
- `send(value)` queues input. It does not acknowledge processing.
- `output` is a single-consumer `AsyncIterable` of emitted values.
- `result` resolves with the task's return value, or rejects with its error.
- `settled` resolves after the underlying work and cooperative cleanup finish,
  even when `result` rejected earlier on cancellation. It never rejects.
- `closeInput()` stops new input and lets the child drain pending messages.
- `cancel(reason?)` cancels the child without cancelling its parent or siblings.
- Async disposal cancels unfinished work and awaits `settled`. Use `await using`,
  or `await run[Symbol.asyncDispose]()` in `finally`.

Capture task arguments in the callback. Its `ctx` belongs to the new run and
shadows the outer context. Pass that `ctx` into the task so it uses the new run's
channels and cancellation.
Annotate the callback context when typed messages are needed; TypeScript cannot
infer its message types from calls inside the callback body.

Argument forwarding with `ctx.spawn(task, ...args)`, the standalone
`spawn(ctx, task, ...args)`, `TaskHandle` type, and `updates`
property remain compatible aliases. `output` and `updates` share one consumer;
do not iterate both. Custom context implementations must provide `spawn`; prefer
creating a context with `createWorkflowContext()` and overriding host callbacks.

Each channel holds up to 64 queued values. Sending to a full channel rejects
with `ChannelFullError`; sending to a closed channel rejects with
`ChannelClosedError`. No send waits for free space or silently drops a value.
Consume output while work runs if the child can emit more than the buffer holds.

Successful completion closes both channels. Buffered updates remain readable.
Failure or cancellation discards values in open channels and rejects pending
reads and future sends with the original reason. The first terminal transition
of each channel wins. Breaking out of the updates loop closes that channel and
discards its buffer; subsequent child emissions fail. Iterators permit only one
pending `next()` call.

Parent cancellation reaches the child through `ctx.signal`. Cancellation settles
the handle even if the task ignores that signal, but JavaScript cannot stop that
task's code or undo its side effects. Task code must cooperate. The parent must
await its child runs and dispose unfinished children in `finally` as needed;
parent return does not automatically join spawned work. `ctx.abort()` retains
its existing whole-workflow meaning.

A normal context from `createWorkflowContext` or `runWorkflow` has an empty inbox
and a no-op `emit`. Direct calls retain their synchronous or asynchronous return
behavior. Passing a spawned context directly to another task shares its inbox and
emitter; use `ctx.spawn()` to give that child separate channels. Usage accounting
and existing context callbacks are shared with the parent.

For standalone use, `createChannel<T>({ capacity, signal })` exposes `send`,
`close`, `fail`, and async iteration. Channels use no Node-specific imports and
hold data only in memory.

See the [task channel example](../examples/channel-demo.md) and
[Chatto coordinator example](../examples/chatto-coordinator-demo.md).

## State in the run inspector

A task can publish a JSON object for people who inspect the run in the web app:

```ts
const scan = task(async (ctx: WorkflowContext, files: string[]) => {
  ctx.publishState({ phase: 'scanning', checked: 0 });
  // Scan files and publish a new snapshot when the visible state changes.
  ctx.publishState({ phase: 'done', checked: files.length });
});
```

Select the task in the timeline to see its latest snapshot. Each call replaces
that task's visible state. Runling copies the object at the time of the call;
later changes to the original object need another call. The snapshot must be a
JSON object of at most 16,000 serialized characters. A call outside a task, or
one with an invalid snapshot, throws an error.

Published state appears in the run journal and web app. Do not publish secrets
or private working data. It does not send a message to the parent or change the
task result. Use task channels and results to communicate between tasks.

If you implement `WorkflowContext` yourself, add `publishState`. If you use an
exhaustive switch over `RunlingEvent`, handle the new `task.state` event.

## Timeline messages

Spawned tasks record queued messages, task reads, and agent consumption receipts.
The timeline places input (↓) and update (↑) markers on the child task's row.
Nearby messages share a numbered marker. Click a marker and select a message
to inspect its direction, payload, and delivery state.

Payload previews are captured when sent and limited to 16,000 characters.
Values remain unchanged in the channel. Values that cannot be represented as
JSON show a placeholder. A task read does not imply agent consumption.
Agent connections record consumption separately when reading a spawned inbox.

These events are stored in the run journal. Existing runs without message
events have no markers; they cannot be reconstructed from agent text.
