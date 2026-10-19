# Task steering, output, and results

A task can start other tasks as children. The parent steers the child, and the
child streams output to the parent. The child returns one final result. Each
direction has a distinct purpose; the parent decides how to use the output.

The task's `input` schema describes its initial argument, and `result` describes
its final return value. Neither is a message stream.

## Message schemas

A task definition can have two optional message schemas:

- `steering` validates the messages from the parent. The task reads them from
  `ctx.steering`.
- `output` validates the messages to the parent. The task sends them with
  `ctx.emit()`.

If a task has no message schema, it accepts and emits any message. The schemas
also give the types of `ctx.steering`, `ctx.emit()`, `steer()`, and `output`.

```ts
import { log, task, Type } from 'runling';

const sum = task(
  {
    name: 'Sum',
    input: Type.Number(),
    result: Type.Number(),
    steering: Type.Number(),
    output: Type.Object({ total: Type.Number() })
  },
  async (ctx, total) => {
    for await (const amount of ctx.steering) {
      total += amount;
      await ctx.emit({ total });
    }
    return total;
  }
);

const parent = task({ name: 'Parent', input: Type.Null(), result: Type.Number() }, async (ctx) => {
  await using child = ctx.start(sum, 0);
  await child.steer(2);
  await child.steer(3);
  child.closeSteering();

  for await (const update of child.output) log.info(`Total: ${update.total}`);
  return await child.result; // 5
});
```

An invalid message rejects `steer()` or `ctx.emit()` with a `TypeError`. The
receiver gets the parsed value of the schema.

## Start a task

`ctx.start(task, input)` starts the task at once and returns an invocation
handle:

- `id` identifies the invocation and its recorded messages. It is not the ID of
  the workflow run.
- `status` is `running`, `completed`, `failed`, or `cancelled`.
- `steer(message)` validates a message and puts it in the child's queue. It does
  not wait until the child reads the message.
- `closeSteering()` tells the child that no more messages come. The child can
  still read the messages in the queue. Then its `ctx.steering` loop ends.
- `output` is an async iterable of the messages that the child emits. Only one
  reader can use it.
- `result` resolves with the validated final result of the child, or rejects with its
  error.
- `cancel(reason?)` cancels the child and its children. It does not cancel the
  parent or the siblings of the child.
- Async disposal cancels unfinished work and waits for cleanup. Use
  `await using`, or call `await handle[Symbol.asyncDispose]()` in `finally`.

A direct call, `await task(ctx, input)`, starts the task and waits for its
result. Steering for a direct call is closed, so `ctx.steering` in the child ends
at once. Runling records the updates of a direct call in the journal and then
discards them.

The parent must wait for its children or dispose of them. When a parent
returns, Runling does not wait for its children.

## Background children

Background work uses ordinary tasks. A call starts work without requiring the
parent to await the promise immediately:

```ts
const pending = researchTask(ctx, question);
// Handle rejection immediately, even while the parent does other work.
const delivery = pending.then(
  (result) => enqueueForNextTurn({ result }),
  (error) => enqueueForNextTurn({ error })
);

await processOtherMessages();
await delivery;
```

A direct call records updates but does not deliver them to the parent. Use
`ctx.start` when the parent needs progress, steering, or explicit cancellation.
Retain that handle, consume `output`, and observe `result`. Dispose the handle
in the owning parent's `finally` block or with `await using`; its lifetime must
not be limited to the short tool callback that started the work.

The parent implementation owns pending-work state, reply destinations, status
information, and result queues. It must keep itself alive while it wants results,
and wait for or dispose its children before it exits. Handle failures on every
pending promise. For several children, cancel all first, then await their disposal
so one child's cleanup does not delay cancellation of the others.

Runling has no separate background-task type or public child registry. It does not
automatically join children when a parent returns, retry them, or reconstruct them
after a restart. Cancellation and disposal cannot undo external effects.

## Inside a task

The task context gives these members:

- `ctx.steering`: messages from the parent. Only one reader can use it. It ends
  when the parent closes steering.
- `ctx.emit(message)`: send a message to the parent.
- `ctx.start(task, input)`: start a child task.
- `ctx.signal`: an abort signal for cancellation.
- `ctx.state`: the mutable root state saved by the server. Children get independent,
  unsaved objects; pass selected sub-objects as input to share state. See
  [workflow state](workflow-state.md).
- `ctx.abort(reason)`: stop the complete workflow run and report it as failed.
- `ctx.run`: the ID and reference of the run, when the host records runs.
- `ctx.usage` and `ctx.recordUsage()`: the token usage of the run.

The host is the parent of the top-level task. It uses the same channels. See
[workflow messages](workflow-messages.md).

Use `createTaskContext()` to call a task in a test or a script without a host.
That context has empty steering, and it discards the messages that it receives
through `emit`.

## Limits and errors

Each direction holds up to 64 messages in its queue:

- A send to a full queue rejects with `ChannelFullError`. Read `output` while
  the child runs if the child can emit more than 64 messages.
- A send to a closed queue rejects with `ChannelClosedError`.
- A send never waits for free space, and Runling never drops a message without
  an error.

When the child completes, Runling closes both directions. The parent can still
read the updates in the queue. When the child fails or is cancelled, Runling
discards the messages in the queue. Pending reads and later sends reject with
the original error. If the parent stops reading `output`, for example with
`break`, the updates close and later `ctx.emit()` calls fail.

Cancellation reaches the child through `ctx.signal`. The handle settles also if
the child ignores the signal. JavaScript cannot stop the code of the child or
undo its side effects, so task code must use the signal.

## Channels without tasks

`createChannel<T>({ capacity, signal })` gives `send`, `close`, `fail`, and
async iteration. Channels keep data only in memory and use no Node-specific
imports.

### Wake a processing loop

Use `createNotifier()` when a loop owns its queue or state and only needs a wake-up
signal. `notify()` wakes a pending `wait()` or retains one notification. Repeated
notifications coalesce. Only one wait can be pending at a time.

```ts
import { createNotifier } from 'runling';

const changed = createNotifier();
changed.notify(); // For example, after adding an item to an application queue.
const notified = await changed.wait({ signal: ctx.signal, timeoutMs: 60_000 });
// true: a notification arrived; false: the timeout expired.
// Inspect the application queue or state, then wait again.
```

The timeout is optional, in milliseconds; zero checks for a retained notification
without waiting. Cancellation rejects with the signal's reason. Each wait removes
its timer and abort listener when it ends. The helper holds no payload, persists
nothing, and does not start or track tasks. The caller owns batching and recovery.

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

## Migration from the message API

Update task definitions and callers together. These are TypeScript API changes;
existing journal records, HTTP payloads, and stored workflow state do not change.

| Previous API | Current API |
| --- | --- |
| Task `output` (final return schema) | `result` |
| Task `receives` | `steering` |
| Task `emits` | `output` (stream schema) |
| `ctx.inbox` | `ctx.steering` |
| `child.send(message)` | `child.steer(message)` |
| `child.updates` | `child.output` |
| `child.closeInput()` | `child.closeSteering()` |
| `TaskOutput<T>` (final return type) | `TaskResult<T>` |
| `TaskReceives<T>` | `TaskSteering<T>` |
| `TaskEmits<T>` | `TaskOutput<T>` (stream item type) |
| Classifier option `output` | `result` |

Run handles use `steer` and `closeSteering` too. The raw `Channel.send` API and
the session API's `output: 'text'` or `output: 'report'` option do not change.
The host's `onUpdate` callback still receives streamed root output. The journal
retains its `input` and `update` direction tags.
