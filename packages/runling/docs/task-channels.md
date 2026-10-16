# Task invocations and messages

A task can start other tasks as children. A parent and a child can exchange
messages while the child runs. This guide describes the contract.

## Message schemas

A task definition can have two optional message schemas:

- `receives` validates the messages from the parent. The task reads them from
  `ctx.inbox`.
- `emits` validates the messages to the parent. The task sends them with
  `ctx.emit()`.

If a task has no message schema, it accepts and emits any message. The schemas
also give the types of `ctx.inbox`, `ctx.emit()`, `send()`, and `updates`.

```ts
import { log, task, Type } from 'runling';

const sum = task(
  {
    name: 'Sum',
    input: Type.Number(),
    output: Type.Number(),
    receives: Type.Number(),
    emits: Type.Object({ total: Type.Number() })
  },
  async (ctx, total) => {
    for await (const amount of ctx.inbox) {
      total += amount;
      await ctx.emit({ total });
    }
    return total;
  }
);

const parent = task({ name: 'Parent', input: Type.Null(), output: Type.Number() }, async (ctx) => {
  await using child = ctx.start(sum, 0);
  await child.send(2);
  await child.send(3);
  child.closeInput();

  for await (const update of child.updates) log.info(`Total: ${update.total}`);
  return await child.result; // 5
});
```

An invalid message rejects `send()` or `ctx.emit()` with a `TypeError`. The
receiver gets the parsed value of the schema.

## Start a task

`ctx.start(task, input)` starts the task at once and returns an invocation
handle:

- `id` identifies the invocation and its recorded messages. It is not the ID of
  the workflow run.
- `status` is `running`, `completed`, `failed`, or `cancelled`.
- `send(message)` validates a message and puts it in the child's queue. It does
  not wait until the child reads the message.
- `closeInput()` tells the child that no more messages come. The child can
  still read the messages in the queue. Then its `ctx.inbox` loop ends.
- `updates` is an async iterable of the messages that the child emits. Only one
  reader can use it.
- `result` resolves with the validated output of the child, or rejects with its
  error.
- `cancel(reason?)` cancels the child and its children. It does not cancel the
  parent or the siblings of the child.
- Async disposal cancels unfinished work and waits for cleanup. Use
  `await using`, or call `await handle[Symbol.asyncDispose]()` in `finally`.

A direct call, `await task(ctx, input)`, starts the task and waits for its
result. The input of a direct call is closed, so `ctx.inbox` in the child ends
at once. Runling records the updates of a direct call in the journal and then
discards them.

The parent must wait for its children or dispose of them. When a parent
returns, Runling does not wait for its children.

## Inside a task

The task context gives these members:

- `ctx.inbox`: messages from the parent. Only one reader can use it. It ends
  when the parent closes the input.
- `ctx.emit(message)`: send a message to the parent.
- `ctx.start(task, input)`: start a child task.
- `ctx.signal`: an abort signal for cancellation.
- `ctx.abort(reason)`: stop the complete workflow run and report it as failed.
- `ctx.run`: the ID and reference of the run, when the host records runs.
- `ctx.usage` and `ctx.recordUsage()`: the token usage of the run.

The host is the parent of the top-level task. It uses the same channels. See
[workflow messages](workflow-messages.md).

Use `createTaskContext()` to call a task in a test or a script without a host.
That context has an empty inbox, and it discards the messages that it receives
through `emit`.

## Limits and errors

Each direction holds up to 64 messages in its queue:

- A send to a full queue rejects with `ChannelFullError`. Read `updates` while
  the child runs if the child can emit more than 64 messages.
- A send to a closed queue rejects with `ChannelClosedError`.
- A send never waits for free space, and Runling never drops a message without
  an error.

When the child completes, Runling closes both directions. The parent can still
read the updates in the queue. When the child fails or is cancelled, Runling
discards the messages in the queue. Pending reads and later sends reject with
the original error. If the parent stops reading `updates`, for example with
`break`, the updates close and later `ctx.emit()` calls fail.

Cancellation reaches the child through `ctx.signal`. The handle settles also if
the child ignores the signal. JavaScript cannot stop the code of the child or
undo its side effects, so task code must use the signal.

## Channels without tasks

`createChannel<T>({ capacity, signal })` gives `send`, `close`, `fail`, and
async iteration. Channels keep data only in memory and use no Node-specific
imports.

## Timeline and journal

Runling records each message, each read, and each wait for a message in the run
journal. The timeline shows input (↓) and update (↑) markers on the row of the
child task. Messages that are near each other share a numbered marker. Select a
marker and a message to see its direction, payload, and delivery state.

When a task reads an empty inbox, the timeline shows "Waiting for a message"
until a message arrives or the input closes.

The journal keeps a preview of each payload of at most 16,000 characters. The
value in the channel does not change. A value that cannot be represented as JSON
shows a placeholder. A read by a task does not mean that an agent used the
message.

See the [task channel example](../examples/channel-demo.md).
