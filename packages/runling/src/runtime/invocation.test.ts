import { z } from 'zod';
import { expect, expectTypeOf, test, vi } from 'vitest';
import { Type, type Static } from 'typebox';
import { createTaskContext, type TaskContext } from './context.ts';
import type { Invocation } from './invocation.ts';
import { ChannelClosedError, ChannelFullError } from './channel.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';
import { task } from './task.ts';
import { runWorkflow } from './runner.ts';

const Command = Type.Object({ add: Type.Number() });
const Update = Type.Object({ total: Type.Number() });
type Command = Static<typeof Command>;
type Update = Static<typeof Update>;

/** Adds each command to its total, reports the new total, and returns it when the input closes. */
const accumulator = task(
  {
    name: 'Accumulator',
    input: Type.Number(),
    result: Type.Number(),
    steering: Command,
    output: Update
  },
  async (ctx, initial) => {
    let total = initial;
    for await (const message of ctx.steering) {
      total += message.add;
      await ctx.emit({ total });
    }
    return total;
  }
);

/** Waits until its context is cancelled. */
const waitForCancel = task(
  { name: 'Wait for cancel', input: Type.Null(), result: Type.Null() },
  async (ctx) => {
    await new Promise<void>((resolve) =>
      ctx.signal.addEventListener('abort', () => resolve(), { once: true })
    );
    return null;
  }
);

/** A task without schemas for its messages, which accepts any message. */
const untyped = <Output>(
  name: string,
  run: (ctx: TaskContext<unknown, unknown>) => Output | Promise<Output>
) => task({ name, input: Type.Null(), result: Type.Unknown() }, (ctx) => run(ctx));

test('sends messages while the task runs, streams updates, then returns its typed result', async () => {
  const handle = createTaskContext().start(accumulator, 10);

  expectTypeOf(handle).toEqualTypeOf<Invocation<Command, Update, number>>();
  expectTypeOf(handle.steer).parameter(0).toEqualTypeOf<Command>();
  expectTypeOf(handle.output).toEqualTypeOf<AsyncIterable<Update>>();
  expectTypeOf(handle.result).toEqualTypeOf<Promise<number>>();
  expect(handle.id).toEqual(expect.any(String));
  expect(handle.status).toBe('running');

  await handle.steer({ add: 2 });
  const updates = handle.output[Symbol.asyncIterator]();
  expect(await updates.next()).toEqual({ done: false, value: { total: 12 } });

  await handle.steer({ add: 3 });
  handle.closeSteering();

  expect(await updates.next()).toEqual({ done: false, value: { total: 15 } });
  expect((await updates.next()).done).toBe(true);
  expect(await handle.result).toBe(15);
  expect(handle.status).toBe('completed');

  await expect(handle.steer({ add: 1 })).rejects.toBeInstanceOf(ChannelClosedError);
  await handle[Symbol.asyncDispose]();
  expect(handle.status).toBe('completed');
});

test('validates messages against the message schemas with a TypeError', async () => {
  const events: RunlingEvent[] = [];
  const invalidUpdate = task(
    { name: 'Invalid update', input: Type.Null(), result: Type.Null(), output: Update },
    async (ctx) => {
      await ctx.emit({ total: 'many' } as never);
      return null;
    }
  );
  await observeRunlingEvents(
    (event) => events.push(event),
    async () => {
      const handle = createTaskContext().start(accumulator, 0);
      await expect(handle.steer({ add: 'one' } as never)).rejects.toThrow(
        new TypeError('Task "Accumulator" steering is invalid: /add: must be number')
      );
      handle.closeSteering();
      expect(await handle.result).toBe(0);

      const failing = createTaskContext().start(invalidUpdate, null);
      await expect(failing.result).rejects.toThrow(
        new TypeError('Task "Invalid update" output is invalid: /total: must be number')
      );
    }
  );
  // Rejected messages are not recorded.
  expect(events.some((event) => event.type === 'message.sent')).toBe(false);
});

test('delivers the parsed values of Standard Schema message schemas', async () => {
  const child = task(
    {
      name: 'Parsed messages',
      input: z.string().transform(Number),
      result: z.number().transform(String),
      steering: z.string().transform(Number),
      output: z.number().transform((total) => ({ total }))
    },
    async (ctx, initial) => {
      expectTypeOf(initial).toEqualTypeOf<number>();
      expectTypeOf(ctx.steering).toEqualTypeOf<AsyncIterable<number>>();
      expectTypeOf(ctx.emit).parameter(0).toEqualTypeOf<number>();
      let total = initial;
      for await (const message of ctx.steering) {
        expectTypeOf(message).toEqualTypeOf<number>();
        total += message;
        await ctx.emit(total);
      }
      return total;
    }
  );
  const handle = createTaskContext().start(child, '10');

  expectTypeOf(handle.steer).parameter(0).toEqualTypeOf<string>();
  expectTypeOf(handle.output).toEqualTypeOf<AsyncIterable<{ total: number }>>();
  expectTypeOf(handle.result).toEqualTypeOf<Promise<string>>();

  await handle.steer('2');
  handle.closeSteering();
  const updates: unknown[] = [];
  for await (const update of handle.output) updates.push(update);
  expect(updates).toEqual([{ total: 12 }]);
  expect(await handle.result).toBe('12');
});

test('nested invocations inherit the child signal, including context overrides', async () => {
  const controller = new AbortController();
  const root = { ...createTaskContext(), signal: controller.signal };
  const grandchild = Promise.withResolvers<Invocation<unknown, unknown, null>>();
  const child = root.start(
    task({ name: 'Child', input: Type.Null(), result: Type.Null() }, async (ctx) => {
      const nested = ctx.start(waitForCancel, null);
      grandchild.resolve(nested);
      return nested.result;
    }),
    null
  );
  const sibling = root.start(waitForCancel, null);
  const nested = await grandchild.promise;
  child.cancel(new Error('child stopped'));
  await expect(nested.result).rejects.toThrow('child stopped');
  await expect(child.result).rejects.toThrow('child stopped');
  expect(sibling.status).toBe('running');
  controller.abort(new Error('parent stopped'));
  await expect(sibling.result).rejects.toThrow('parent stopped');
  await Promise.all([child, nested, sibling].map((handle) => handle[Symbol.asyncDispose]()));
});

test('disposal waits for cleanup while cancelled results settle immediately', async () => {
  const cleanup = Promise.withResolvers<void>();
  const started = Promise.withResolvers<void>();
  const aborted = Promise.withResolvers<void>();
  const child = createTaskContext().start(
    untyped('Slow cleanup', async (ctx) => {
      started.resolve();
      await new Promise<void>((resolve) =>
        ctx.signal.addEventListener(
          'abort',
          () => {
            aborted.resolve();
            resolve();
          },
          { once: true }
        )
      );
      await cleanup.promise;
      return 'late';
    }),
    null
  );
  // Input validation is asynchronous: wait until the task listens for cancellation.
  await started.promise;
  const disposed = vi.fn();
  const closing = child[Symbol.asyncDispose]().then(disposed);
  await aborted.promise;
  await expect(child.result).rejects.toThrow('Task cancelled');
  expect(child.status).toBe('cancelled');
  expect(disposed).not.toHaveBeenCalled();
  cleanup.resolve();
  await closing;
  expect(disposed).toHaveBeenCalledOnce();
});

test('failure keeps the original error and ignores later cancellation', async () => {
  const error = new Error('failed');
  const child = createTaskContext().start(
    untyped('Fail', () => {
      throw error;
    }),
    null
  );
  await expect(child.result).rejects.toBe(error);
  expect(child.status).toBe('failed');
  child.cancel();
  expect(child.status).toBe('failed');
  await child[Symbol.asyncDispose]();
});

test('sibling invocations share usage but not cancellation', async () => {
  const parent = createTaskContext();
  const child = task(
    {
      name: 'Counted',
      input: Type.Null(),
      result: Type.Number(),
      steering: Command,
      output: Update
    },
    async (ctx) => {
      ctx.recordUsage({ input: 1, output: 0, cacheRead: 0, cacheWrite: 0 });
      let total = 0;
      for await (const message of ctx.steering) {
        total += message.add;
        await ctx.emit({ total });
      }
      return total;
    }
  );

  const a = parent.start(child, null);
  const b = parent.start(child, null);
  await vi.waitFor(() => expect(parent.usage.input).toBe(2));

  a.cancel(new Error('stop a'));
  await expect(a.result).rejects.toThrow('stop a');
  expect(parent.signal.aborted).toBe(false);

  // Cancelling one child must leave its sibling able to communicate.
  await b.steer({ add: 5 });
  b.closeSteering();
  const updates: Update[] = [];
  for await (const update of b.output) updates.push(update);
  expect(updates).toEqual([{ total: 5 }]);
  expect(await b.result).toBe(5);
});

test('parent cancellation releases readers and rejects the result', async () => {
  const ctx = createTaskContext();
  const handle = ctx.start(accumulator, 0);
  const next = handle.output[Symbol.asyncIterator]().next();

  try {
    ctx.abort('parent stopped');
  } catch {
    // abort() both signals cancellation and throws to stop the caller.
  }

  await expect(handle.result).rejects.toThrow('parent stopped');
  await expect(next).rejects.toThrow('parent stopped');
  expect(handle.status).toBe('cancelled');
});

test('an already cancelled parent does not start work', async () => {
  const ctx = createTaskContext();
  try {
    ctx.abort('stop');
  } catch {
    // abort() both signals cancellation and throws to stop the caller.
  }

  let called = false;
  const handle = ctx.start(
    untyped('Never', () => {
      called = true;
    }),
    null
  );
  await expect(handle.result).rejects.toThrow('stop');
  expect(called).toBe(false);
});

test.each([false, true])(
  'synchronous and asynchronous failures reject both result and updates (async: %s)',
  async (asynchronous) => {
    const failure = new Error('work failed');
    const handle = createTaskContext().start(
      untyped('Fail', () => {
        if (asynchronous) return Promise.reject(failure);
        throw failure;
      }),
      null
    );
    await expect(handle.result).rejects.toBe(failure);
    await expect(handle.output[Symbol.asyncIterator]().next()).rejects.toBe(failure);
  }
);

test('cancellation settles the handle even if work ignores the signal', async () => {
  const finish = Promise.withResolvers<null>();
  const handle = createTaskContext().start(
    untyped('Ignore cancel', () => finish.promise),
    null
  );
  handle.cancel();
  await expect(handle.result).rejects.toThrow('Task cancelled');
  expect(handle.status).toBe('cancelled');
  finish.resolve(null);
});

test('unread updates fail at the bound instead of deadlocking or growing forever', async () => {
  const handle = createTaskContext().start(
    untyped('Flood', async (ctx) => {
      for (let i = 0; i <= 64; i++) await ctx.emit(i);
    }),
    null
  );
  await expect(handle.result).rejects.toBeInstanceOf(ChannelFullError);
});

test('a reader that leaves the updates closes them and a later emit fails', async () => {
  const proceed = Promise.withResolvers<void>();
  const handle = createTaskContext().start(
    untyped('Two updates', async (ctx) => {
      await ctx.emit('first');
      await proceed.promise;
      await ctx.emit('second');
    }),
    null
  );
  for await (const _ of handle.output) break;
  proceed.resolve();
  await expect(handle.result).rejects.toBeInstanceOf(ChannelClosedError);
});

test('started tasks validate input and output and nest in the timeline', async () => {
  const work = task(
    { name: 'Child', input: Type.Number(), result: Type.Number() },
    (_ctx, value) => value + 1
  );
  const events: RunlingEvent[] = [];
  const root = task(
    { name: 'Root', input: Type.Null(), result: Type.Number() },
    (ctx) => ctx.start(work, 1).result
  );
  const execution = await runWorkflow(root, {
    input: null,
    onEvent: (event) => events.push(event)
  });

  expect(execution.output).toBe(2);
  const starts = events.filter((e) => e.type === 'step.started');
  expect(starts.map((e) => e.label)).toEqual(['Root', 'Child']);
  expect(starts[1]?.activityId).toBe(starts[0]?.id);
  const bad = createTaskContext().start(work, 'bad' as unknown as number);
  await expect(bad.result).rejects.toThrow('input is invalid');
});

// Compile-only checks: channel types and task inputs must not widen to unknown or any.
function typeChecks() {
  const handle = createTaskContext().start(accumulator, 0);
  // @ts-expect-error wrong message type
  void handle.steer({ add: 'one' });
  // @ts-expect-error missing required task input
  createTaskContext().start(accumulator);
  // @ts-expect-error wrong task input
  createTaskContext().start(accumulator, 'zero');
  const ctx = {} as TaskContext<Command, Update>;
  // @ts-expect-error wrong update type
  void ctx.emit('bad');
  // @ts-expect-error a direct call needs the task input
  void accumulator(ctx);
}
void typeChecks;

test('cancellation before result settlement cannot report success with failed channels', async () => {
  const handle = createTaskContext().start(
    untyped('Fast', () => 1),
    null
  );
  handle.cancel(new Error('cancelled before settlement'));
  await expect(handle.result).rejects.toThrow('cancelled before settlement');
  await expect(handle.output[Symbol.asyncIterator]().next()).rejects.toThrow(
    'cancelled before settlement'
  );
});

test('a completed result is unchanged by late cancellation', async () => {
  const handle = createTaskContext().start(
    untyped('Fast', () => 1),
    null
  );

  expect(await handle.result).toBe(1);
  handle.cancel();

  expect(await handle.result).toBe(1);
  expect(handle.status).toBe('completed');
  expect((await handle.output[Symbol.asyncIterator]().next()).done).toBe(true);
});

test('journals message identities, reads, and task links without changing values', async () => {
  const events: RunlingEvent[] = [];
  const child = untyped('Read two', async (ctx) => {
    const received: unknown[] = [];
    for await (const message of ctx.steering) received.push(message);
    await ctx.emit('result');
    return received;
  });

  await observeRunlingEvents(
    (event) => events.push(event),
    async () => {
      const handle = createTaskContext().start(child, null);
      await handle.steer('same');
      await handle.steer('same');
      handle.closeSteering();
      const updates: unknown[] = [];
      for await (const update of handle.output) updates.push(update);
      expect(updates).toEqual(['result']);
      expect(await handle.result).toEqual(['same', 'same']);
    }
  );

  const sent = events.filter((e) => e.type === 'message.sent');
  expect(sent.map((e) => [e.direction, e.payload])).toEqual([
    ['input', 'same'],
    ['input', 'same'],
    ['update', 'result']
  ]);
  expect(new Set(sent.map((e) => e.id)).size).toBe(3);
  expect(new Set(sent.map((e) => e.channelId)).size).toBe(1);
  expect(events.filter((e) => e.type === 'message.read')).toHaveLength(3);
  const linked = events.filter((e) => e.type === 'task.linked');
  expect(linked).toEqual([
    expect.objectContaining({
      channelId: sent[0]?.channelId,
      taskId: events.find((e) => e.type === 'step.started')?.id
    })
  ]);
  for (const message of sent) {
    expect(events.indexOf(message)).toBeLessThan(
      events.findIndex((e) => e.type === 'message.read' && e.id === message.id)
    );
  }
});

test('a task without message schemas accepts and emits any message', async () => {
  const echo = untyped('Echo messages', async (ctx) => {
    for await (const message of ctx.steering) await ctx.emit(message);
  });
  const handle = createTaskContext().start(echo, null);
  expectTypeOf(handle.steer).parameter(0).toEqualTypeOf<unknown>();
  expectTypeOf(handle.output).toEqualTypeOf<AsyncIterable<unknown>>();

  await handle.steer({ any: 'value' });
  await handle.steer(42);
  handle.closeSteering();
  const updates: unknown[] = [];
  for await (const update of handle.output) updates.push(update);
  expect(updates).toEqual([{ any: 'value' }, 42]);
});

test('message previews tolerate circular values and do not record rejected sends', async () => {
  const events: RunlingEvent[] = [];
  const circular: { self?: unknown } = {};
  circular.self = circular;

  await observeRunlingEvents(
    (event) => events.push(event),
    async () => {
      const handle = createTaskContext().start(
        untyped('Never reads', () => new Promise(() => {})),
        null
      );
      await handle.steer(circular);
      for (let i = 1; i < 64; i++) await handle.steer({ i });
      await expect(handle.steer({ overflow: true })).rejects.toBeInstanceOf(ChannelFullError);
      handle.cancel();
      await expect(handle.result).rejects.toThrow('cancelled');
    }
  );

  const sent = events.filter((event) => event.type === 'message.sent');
  expect(sent).toHaveLength(64);
  expect(sent[0]!.payload).toBe('[Value cannot be represented as JSON]');
  expect(events.some((event) => event.type === 'message.read')).toBe(false);
});

test('records a wait when the task reads an empty inbox', async () => {
  const events: RunlingEvent[] = [];
  const reading = Promise.withResolvers<void>();
  const child = untyped('Wait for messages', async (ctx) => {
    const received: unknown[] = [];
    const inbox = ctx.steering[Symbol.asyncIterator]();
    reading.resolve();
    for (let next = await inbox.next(); !next.done; next = await inbox.next()) {
      received.push(next.value);
    }
    return received;
  });

  await observeRunlingEvents(
    (event) => events.push(event),
    async () => {
      const handle = createTaskContext().start(child, null);
      await reading.promise;
      await vi.waitFor(() => expect(events.some((e) => e.type === 'wait.started')).toBe(true));
      await handle.steer('first');
      await vi.waitFor(() =>
        expect(events.filter((e) => e.type === 'wait.started')).toHaveLength(2)
      );
      handle.closeSteering();
      expect(await handle.result).toEqual(['first']);
    }
  );

  const step = events.find((e) => e.type === 'step.started');
  const sent = events.find((e) => e.type === 'message.sent');
  const started = events.filter((e) => e.type === 'wait.started');
  const finished = events.filter((e) => e.type === 'wait.finished');
  // The wait belongs to the reading task and to its input channel.
  expect(started).toEqual([
    expect.objectContaining({ channelId: sent?.channelId, activityId: step?.id }),
    expect.objectContaining({ channelId: sent?.channelId, activityId: step?.id })
  ]);
  expect(finished).toEqual([
    expect.objectContaining({ id: started[0]?.id, status: 'received' }),
    expect.objectContaining({ id: started[1]?.id, status: 'closed' })
  ]);
  expect(finished.every((e) => e.durationMs >= 0)).toBe(true);
});

test('does not record a wait when a message is already queued', async () => {
  const events: RunlingEvent[] = [];
  const child = untyped('Read queued', async (ctx) => {
    const received: unknown[] = [];
    for await (const message of ctx.steering) received.push(message);
    return received;
  });

  await observeRunlingEvents(
    (event) => events.push(event),
    async () => {
      const handle = createTaskContext().start(child, null);
      await handle.steer('queued');
      handle.closeSteering();
      expect(await handle.result).toEqual(['queued']);
    }
  );

  // Only the read after the input closed waits.
  expect(events.filter((e) => e.type === 'wait.finished')).toEqual([
    expect.objectContaining({ status: 'closed' })
  ]);
});
