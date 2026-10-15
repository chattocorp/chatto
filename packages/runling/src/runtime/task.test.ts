import { createTaskContext, type TaskContext } from './context.ts';
import { describe, expect, expectTypeOf, test } from 'vitest';
import { Type } from 'typebox';
import { task, isTask } from './task.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';

describe('task', () => {
  test('a direct call validates input and output and returns a promise of the output', async () => {
    const echo = task(
      { name: 'Echo', input: Type.String(), output: Type.String() },
      (_ctx, input) => input
    );
    expectTypeOf<Parameters<typeof echo>>().toEqualTypeOf<[TaskContext<any, any>, string]>();
    expectTypeOf<ReturnType<typeof echo>>().toEqualTypeOf<Promise<string>>();
    expect(echo.name).toBe('Echo');
    expect(echo.input).toEqual(Type.String());
    expect(echo).not.toHaveProperty('receives');
    expect(isTask(echo)).toBe(true);
    expect(isTask(() => 'not a task')).toBe(false);
    await expect(echo(createTaskContext(), '')).resolves.toBe('');
  });

  test('invalid input rejects without calling the implementation', async () => {
    let called = false;
    const echo = task(
      { name: 'Echo', input: Type.String(), output: Type.String() },
      (_ctx, input) => {
        called = true;
        return input;
      }
    );
    if (false) {
      // @ts-expect-error Input is required.
      void echo(createTaskContext());
    }
    await expect(echo(createTaskContext(), 42 as never)).rejects.toThrow(
      new TypeError('Task "Echo" input is invalid: /: must be string')
    );
    expect(called).toBe(false);
  });

  test('nested tasks preserve inputs and activity parentage', async () => {
    const events: RunlingEvent[] = [];
    const child = task(
      { name: 'Child', input: Type.String(), output: Type.String() },
      (_ctx, value) => `${value} child`
    );
    const parent = task(
      { name: 'Parent', input: Type.String(), output: Type.String() },
      (ctx, value) => child(ctx, value)
    );
    await observeRunlingEvents(
      (e) => events.push(e),
      async () => {
        expect(await parent(createTaskContext(), 'hello')).toBe('hello child');
      }
    );
    const starts = events.filter((e) => e.type === 'step.started');
    expect(starts.map((e) => e.label)).toEqual(['Parent', 'Child']);
    expect(starts[1]?.activityId).toBe(starts[0]?.id);
  });

  test.each(['input', 'output', 'receives', 'emits'] as const)(
    'rejects an invalid %s schema',
    (boundary) => {
      expect(() =>
        task(
          {
            name: 'Invalid',
            input: Type.String(),
            output: Type.String(),
            [boundary]: { type: 'invalid' }
          } as never,
          () => 'unused'
        )
      ).toThrow(`Task "Invalid" ${boundary} schema is invalid`);
    }
  );

  test.each(['', '  ', undefined])('rejects the task name %j', (name) => {
    expect(() =>
      task({ name, input: Type.String(), output: Type.String() } as never, () => 'unused')
    ).toThrow('Task name must be a non-empty string');
  });

  test('validates synchronous and asynchronous outputs', async () => {
    const definition = { name: 'Broken', input: Type.String(), output: Type.String() };
    const sync = task(definition, () => 42 as never);
    const asynchronous = task(definition, async () => 42 as never);
    for (const broken of [sync, asynchronous]) {
      await expect(broken(createTaskContext(), 'input')).rejects.toThrow(
        'Task "Broken" output is invalid'
      );
    }
  });

  test('preserves thrown errors and records failure', async () => {
    const error = new Error('failed');
    const definition = { name: 'Fail', input: Type.Null(), output: Type.Null() };
    const events: RunlingEvent[] = [];
    await observeRunlingEvents(
      (e) => events.push(e),
      async () => {
        await expect(
          task(definition, () => {
            throw error;
          })(createTaskContext(), null)
        ).rejects.toBe(error);
        await expect(
          task(definition, async () => {
            throw error;
          })(createTaskContext(), null)
        ).rejects.toBe(error);
      }
    );
    expect(events.filter((e) => e.type === 'step.finished').map((e) => e.status)).toEqual([
      'failed',
      'failed'
    ]);
  });

  test('preserves null input', async () => {
    const run = task(
      { name: 'Null', input: Type.Null(), output: Type.Null() },
      (_ctx, value) => value
    );
    expect(await run(createTaskContext(), null)).toBeNull();
  });
});

test('child contexts share usage and run identity but not the inbox', async () => {
  const ctx = createTaskContext();
  const seen: TaskContext[] = [];
  const child = task(
    { name: 'Child', input: Type.Number(), output: Type.Number() },
    async (context, value) => {
      await Promise.resolve();
      seen.push(context);
      context.recordUsage({ input: value, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0 });
      return value;
    }
  );
  const parent = task(
    { name: 'Parent', input: Type.Null(), output: Type.Array(Type.Number()) },
    (context) => Promise.all([child(context, 1), child(context, 2)])
  );
  expect(await parent(ctx, null)).toEqual([1, 2]);
  expect(seen).toHaveLength(2);
  expect(seen[0]).not.toBe(ctx);
  expect(seen[0]?.usage).toBe(ctx.usage);
  expect(seen[0]?.inbox).not.toBe(seen[1]?.inbox);
  expect(ctx.usage.input).toBe(3);
});

test('a direct call closes the task input', async () => {
  const drain = task(
    { name: 'Drain', input: Type.Null(), output: Type.Array(Type.Unknown()) },
    async (ctx) => {
      const messages: unknown[] = [];
      for await (const message of ctx.inbox) messages.push(message);
      return messages;
    }
  );
  expect(await drain(createTaskContext(), null)).toEqual([]);
});

test('a direct call records updates in the journal and then drops them', async () => {
  const events: RunlingEvent[] = [];
  const chatty = task(
    { name: 'Chatty', input: Type.Null(), output: Type.Null(), emits: Type.Number() },
    async (ctx) => {
      // More updates than a channel buffers: nobody reads them, so they must not fill it.
      for (let i = 0; i < 100; i++) await ctx.emit(i);
      return null;
    }
  );
  await observeRunlingEvents(
    (e) => events.push(e),
    () => chatty(createTaskContext(), null)
  );
  const sent = events.filter((e) => e.type === 'message.sent');
  expect(sent).toHaveLength(100);
  expect(sent.every((e) => e.direction === 'update')).toBe(true);
  expect(sent[99]?.payload).toBe('99');
  expect(events.some((e) => e.type === 'message.read')).toBe(false);
});

test('a direct call still validates emitted updates', async () => {
  const invalid = task(
    { name: 'Invalid update', input: Type.Null(), output: Type.Null(), emits: Type.Number() },
    async (ctx) => {
      await ctx.emit('not a number' as never);
      return null;
    }
  );
  await expect(invalid(createTaskContext(), null)).rejects.toThrow(
    new TypeError('Task "Invalid update" update is invalid: /: must be number')
  );
});

test('rejects new tasks on an aborted context without starting an activity', async () => {
  const ctx = createTaskContext();
  const events: RunlingEvent[] = [];
  let called = false;
  const run = task({ name: 'Never', input: Type.Null(), output: Type.Null() }, () => {
    called = true;
    return null;
  });
  try {
    ctx.abort('Stop');
  } catch {}
  await observeRunlingEvents(
    (event) => events.push(event),
    () => expect(run(ctx, null)).rejects.toBe(ctx.signal.reason)
  );
  expect(called).toBe(false);
  expect(events).toEqual([]);
});
