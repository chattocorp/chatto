import { describe, expect, expectTypeOf, test } from 'vitest';
import { createRootContext, createTaskContext } from './context.ts';
import { emptyTokenUsage, type TokenUsage } from './usage.ts';

describe('task context', () => {
  test('exposes a stable read-only usage view with explicit snapshots', () => {
    const ctx = createTaskContext();
    expectTypeOf(ctx.usage).toEqualTypeOf<Readonly<TokenUsage>>();
    const before = { ...ctx.usage };
    ctx.recordUsage({ ...emptyTokenUsage(), input: 10, cost: 0.25 });
    expect(before).toEqual(emptyTokenUsage());
    expect(ctx.usage).toBe(ctx.usage);
    // A caller that bypasses readonly typing still cannot change the totals.
    expect(() => {
      (ctx.usage as TokenUsage).input = 999;
    }).toThrow(TypeError);
    expect(ctx.usage).toEqual({ ...emptyTokenUsage(), input: 10, cost: 0.25 });
    expect(ctx).not.toHaveProperty('cwd');
  });

  test('accumulates tokens and known costs while retaining missing-price information', () => {
    const ctx = createTaskContext();
    ctx.recordUsage({ input: 10, output: 2, cacheRead: 3, cacheWrite: 1, cost: { total: 0.25 } });
    ctx.recordUsage({ input: 4, output: 5, cacheRead: 6, cacheWrite: 2 });
    ctx.recordUsage({ ...emptyTokenUsage(), input: 1, cost: 0 });
    expect(ctx.usage).toEqual({
      input: 15,
      output: 7,
      cacheRead: 9,
      cacheWrite: 3,
      cost: 0.25,
      costIncomplete: true
    });
  });

  test('ignores invalid counts and preserves existing malformed-cost rules', () => {
    const snapshots: TokenUsage[] = [];
    const ctx = createRootContext((usage) => snapshots.push(usage));
    for (const input of [-1, NaN, Infinity, 1.5]) {
      ctx.recordUsage({ ...emptyTokenUsage(), input });
    }
    ctx.recordUsage(undefined as never);
    ctx.recordUsage({ input: 1 } as never);
    expect(ctx.usage).toEqual(emptyTokenUsage());
    expect(snapshots).toEqual([]);
    ctx.recordUsage({ ...emptyTokenUsage(), input: 1, cost: -1 });
    expect(ctx.usage).toEqual({ ...emptyTokenUsage(), input: 1, costIncomplete: true });
  });

  test('notifies observers with separate snapshots', () => {
    const snapshots: TokenUsage[] = [];
    const ctx = createRootContext((usage) => snapshots.push(usage));
    ctx.recordUsage({ ...emptyTokenUsage(), input: 1, cost: 0 });
    ctx.recordUsage({ ...emptyTokenUsage(), input: 2, cost: 0.5 });
    expect(snapshots.map((usage) => usage.input)).toEqual([1, 3]);
    snapshots[1]!.input = 999;
    expect(ctx.usage.input).toBe(3);
  });

  test('shares totals across parallel work only when the same context is passed', async () => {
    const run = async (input: number) => {
      const ctx = createTaskContext();
      await Promise.all(
        [1, 2].map(async () => {
          await Promise.resolve();
          ctx.recordUsage({ ...emptyTokenUsage(), input, cost: 0.25 });
        })
      );
      return ctx.usage;
    };
    const [first, second] = await Promise.all([run(10), run(100)]);
    expect(first).toEqual({ ...emptyTokenUsage(), input: 20, cost: 0.5 });
    expect(second).toEqual({ ...emptyTokenUsage(), input: 200, cost: 0.5 });
  });
});

test('aborts once with a stable error and signal', () => {
  const ctx = createTaskContext();
  expect(ctx.signal.aborted).toBe(false);
  expect(() => ctx.abort('Budget exceeded')).toThrow('Budget exceeded');
  const reason = ctx.signal.reason;
  expect(reason).toBeInstanceOf(Error);
  expect(reason.name).toBe('WorkflowAbortError');
  expect(ctx.signal.aborted).toBe(true);
  expect(() => ctx.abort('Another reason')).toThrow(reason);
  expect(ctx.signal.reason).toBe(reason);
  expect(createTaskContext().signal.aborted).toBe(false);
});

test('supplies a default abort reason and retains usage', () => {
  const ctx = createTaskContext();
  ctx.recordUsage({ ...emptyTokenUsage(), input: 10, cost: 0.25 });
  expect(() => ctx.abort()).toThrow('Workflow aborted');
  expect(ctx.usage).toEqual({ ...emptyTokenUsage(), input: 10, cost: 0.25 });
});

test('a context without a parent has an empty inbox, no run, and discards messages', async () => {
  const ctx = createTaskContext();
  const messages: unknown[] = [];
  for await (const message of ctx.steering) messages.push(message);
  expect(messages).toEqual([]);
  // There is no parent to read updates, so emitting never fills a buffer.
  for (let i = 0; i < 100; i++) await ctx.emit(i);
  expect(ctx.run).toBeUndefined();
  expect(createRootContext(undefined, undefined, { id: 'run' }).run).toMatchObject({
    id: 'run',
    setTitle: expect.any(Function)
  });
});

test('a host signal cancels the context', () => {
  const host = new AbortController();
  const ctx = createRootContext(undefined, host.signal);
  host.abort(new Error('host stopped'));
  expect(ctx.signal.aborted).toBe(true);
  expect(ctx.signal.reason).toEqual(new Error('host stopped'));
});

test('spread contexts share live usage and cancellation', () => {
  const parent = createTaskContext();
  const child = { ...parent };
  expect(child.usage).toBe(parent.usage);
  expect(child.signal).toBe(parent.signal);
  child.recordUsage({ ...emptyTokenUsage(), input: 10, cost: 0.25 });
  parent.recordUsage({ ...emptyTokenUsage(), output: 2, cost: 0.5 });
  expect(child.usage).toEqual({ ...emptyTokenUsage(), input: 10, output: 2, cost: 0.75 });
  expect(() => child.abort('Stop both')).toThrow('Stop both');
  expect(parent.signal.aborted).toBe(true);
});
