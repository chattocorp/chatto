# Task channels

Use `spawn` when a parent must exchange data with a running child. Tasks remain
ordinary functions. The context type declares incoming messages and outgoing
updates:

```ts
import { spawn, task, type WorkflowContext } from "runling";

const sum = task(async (ctx: WorkflowContext<number, { total: number }>) => {
  let total = 0;
  for await (const amount of ctx.inbox) {
    total += amount;
    await ctx.emit({ total });
  }
  return total;
});

const child = spawn(ctx, sum);
await child.send(2);
await child.send(3);
child.closeInput();

for await (const update of child.updates) {
  console.log(update.total);
}
const total = await child.result; // 5
```

`spawn(ctx, task, ...args)` starts the task immediately and returns a handle:

- `send(value)` queues input. It does not acknowledge processing.
- `updates` is a single-consumer `AsyncIterable` of emitted values.
- `result` resolves with the task's return value, or rejects with its error.
- `closeInput()` stops new input and lets the child drain pending messages.
- `cancel(reason?)` cancels the child without cancelling its parent or siblings.

Each channel holds up to 64 queued values. Sending to a full channel rejects
with `ChannelFullError`; sending to a closed channel rejects with
`ChannelClosedError`. No send waits for free space or silently drops a value.
Consume updates while work runs if the child can emit more than the buffer holds.

Successful completion closes both channels. Buffered updates remain readable.
Failure or cancellation discards values in open channels and rejects pending
reads and future sends with the original reason. The first terminal transition
of each channel wins. Breaking out of the updates loop closes that channel and
discards its buffer; subsequent child emissions fail. Iterators permit only one
pending `next()` call.

Parent cancellation reaches the child through `ctx.signal`. Cancellation settles
the handle even if the task ignores that signal, but JavaScript cannot stop that
task's code or undo its side effects. Task code must cooperate. The parent must
await its child handles and cancel unfinished children in `finally` as needed;
parent return does not automatically join spawned work. `ctx.abort()` retains
its existing whole-workflow meaning.

A normal context from `createWorkflowContext` or `runWorkflow` has an empty inbox
and a no-op `emit`. Direct calls retain their synchronous or asynchronous return
behavior. Passing a spawned context directly to another task shares its inbox and
emitter; use another `spawn` to give that child separate channels. Usage accounting
and existing context callbacks are shared with the parent.

For standalone use, `createChannel<T>({ capacity, signal })` exposes `send`,
`close`, `fail`, and async iteration. Channels use no Node-specific imports and
hold data only in memory.

See the [task channel example](../examples/channel-demo.md) and
[Chatto coordinator example](../examples/chatto-coordinator-demo.md).

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
