import { afterEach, expect, test, vi } from 'vitest';
import { Type } from 'typebox';
import { createChannel } from './channel.ts';
import type { TaskContext } from './context.ts';
import type { RunlingEvent } from './events.ts';
import { runWorkflow } from './runner.ts';
import { task } from './task.ts';
import { TimeoutError } from './timeout.ts';

// Fake clocks make message/deadline ordering explicit without real sleeps.
afterEach(() => vi.useRealTimers());

/** Return the first message from the host. */
const firstMessage = task(
  { name: 'First message', input: Type.Null(), output: Type.Unknown() },
  async (ctx) => {
    for await (const message of ctx.inbox) return message;
    throw new Error('No message');
  }
);

test('workflow timeout includes message waits and preserves usage and its first abort reason', async () => {
  vi.useFakeTimers();
  let ctx!: TaskContext;
  const events: RunlingEvent[] = [];
  const run = runWorkflow(
    task({ name: 'Wait', input: Type.Null(), output: Type.Null() }, async (context) => {
      ctx = context;
      ctx.recordUsage({ input: 5, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.1 });
      try {
        for await (const _ of ctx.inbox) break;
      } finally {
        expect(() => ctx.abort('later reason')).toThrow(TimeoutError);
      }
      return null;
    }),
    {
      input: null,
      timeout: 2,
      messages: createChannel(),
      onEvent: (e) => events.push(e)
    }
  );
  await vi.advanceTimersByTimeAsync(2000);
  expect(await run).toMatchObject({
    ok: false,
    error: 'Workflow timed out after 2 seconds',
    usage: { input: 5, cost: 0.1 }
  });
  expect(ctx.signal.reason).toBeInstanceOf(TimeoutError);
  expect(events.filter((e) => e.type === 'wait.finished')).toMatchObject([{ status: 'closed' }]);
  expect(vi.getTimerCount()).toBe(0);
});

test.each(['success', 'failure'])('workflow %s disposes its deadline', async (kind) => {
  vi.useFakeTimers();
  let ctx!: TaskContext;
  await runWorkflow(
    task({ name: 'Finish', input: Type.Null(), output: Type.Number() }, (context) => {
      ctx = context;
      if (kind === 'failure') throw new Error('failed');
      return 1;
    }),
    { input: null, timeout: 1 }
  );
  expect(vi.getTimerCount()).toBe(0);
  await vi.advanceTimersByTimeAsync(2000);
  expect(ctx.signal.aborted).toBe(false);
});

test('concurrent workflow deadlines are isolated', async () => {
  vi.useFakeTimers();
  const slow = runWorkflow(firstMessage, {
    input: null,
    timeout: 1,
    messages: createChannel()
  });
  const answers = createChannel<string>();
  const other = runWorkflow(firstMessage, { input: null, timeout: 10, messages: answers });
  await vi.advanceTimersByTimeAsync(1000);
  expect((await slow).ok).toBe(false);
  await answers.send('finished');
  expect(await other).toMatchObject({ ok: true, output: 'finished' });
});

test('zero timeout skips user work', async () => {
  const work = vi.fn(() => null);
  const run = task({ name: 'Never', input: Type.Null(), output: Type.Null() }, work);
  expect(await runWorkflow(run, { input: null, timeout: 0 })).toMatchObject({
    ok: false,
    error: 'Workflow timed out after 0 seconds'
  });
  expect(work).not.toHaveBeenCalled();
});

test.each([-1, NaN, Infinity, 2_147_483.648])(
  'invalid timeout %s rejects before user work',
  async (timeout) => {
    const work = vi.fn(() => null);
    const run = task({ name: 'Never', input: Type.Null(), output: Type.Null() }, work);
    await expect(runWorkflow(run, { input: null, timeout })).rejects.toThrow(RangeError);
    expect(work).not.toHaveBeenCalled();
  }
);
