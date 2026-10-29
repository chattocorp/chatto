import { expect, test, vi } from 'vitest';
import { Type } from 'typebox';
import { SourceManager } from './source-manager.ts';
import { task } from './task.ts';
import {
  defineWebConfig,
  startWorkflow,
  type EventSourceContext,
  type RunHandle
} from './web-config.ts';

/** A registered run that accepts and records messages. */
const runHandle = (id: string, sent: unknown[] = []): RunHandle => ({
  id,
  steer: async (message) => {
    sent.push(message);
  },
  closeSteering: () => {},
  cancel: () => {},
  finished: Promise.resolve()
});

const noop = task({ name: 'Noop', input: Type.Null(), result: Type.Null() }, () => null);

const untilAbort = (signal: AbortSignal) =>
  new Promise<void>((resolve) => {
    if (signal.aborted) resolve();
    else signal.addEventListener('abort', () => resolve(), { once: true });
  });

test('passes retained-run discovery and explicit reactivation through source routing', async () => {
  const started = vi.fn(async () => runHandle('retained'));
  const listed = vi.fn(() => []);
  const ready = Promise.withResolvers<void>();
  const manager = new SourceManager(() => started, listed);
  try {
    await manager.replace(
      defineWebConfig({
        sources: {
          source: async (ctx) => {
            ctx.runs({ includeFinished: true });
            await ctx.dispatch(async (route) => {
              await route.start(noop, { input: null, runId: 'retained' });
            }, null);
            ready.resolve();
            await untilAbort(ctx.signal);
          }
        }
      })
    );
    await ready.promise;
    expect(listed).toHaveBeenCalledWith('source', { includeFinished: true });
    expect(started).toHaveBeenCalledWith(noop, { input: null, runId: 'retained' });
  } finally {
    await manager.close();
  }
});

test('starts eagerly, waits for cleanup, and retains source state across generations', async () => {
  const calls: string[] = [];
  const manager = new SourceManager(() => async () => runHandle('run'));
  let state: Map<string, unknown>;
  let release!: () => void;
  const cleanup = new Promise<void>((resolve) => {
    release = resolve;
  });
  await manager.replace(
    defineWebConfig({
      sources: {
        bot: async (ctx) => {
          state = ctx.state;
          ctx.state.set('conversation', 'active');
          calls.push('first');
          await untilAbort(ctx.signal);
          await cleanup;
          calls.push('closed');
        }
      }
    })
  );
  const replacement = manager.replace(
    defineWebConfig({
      sources: {
        bot: async (ctx) => {
          expect(ctx.state).toBe(state);
          expect(ctx.state.get('conversation')).toBe('active');
          calls.push('second');
          await untilAbort(ctx.signal);
        }
      }
    })
  );
  await Promise.resolve();
  expect(calls).toEqual(['first']);
  release();
  await replacement;
  expect(calls).toEqual(['first', 'closed', 'second']);
  await manager.close();
});

test('dispatch waits for registration, expires the router context, and rejects after shutdown', async () => {
  const register = vi.fn(async () => runHandle('registered'));
  const manager = new SourceManager(() => register);
  let context!: EventSourceContext;
  let lateStart!: () => Promise<unknown>;
  await manager.replace(
    defineWebConfig({
      sources: {
        bot: async (ctx) => {
          context = ctx;
          await untilAbort(ctx.signal);
        }
      }
    })
  );
  const runs = await context.dispatch((ctx) => {
    lateStart = () => ctx.start(noop, { input: null });
    void ctx.start(noop, { input: null });
  }, undefined);
  expect(runs).toEqual([{ id: 'registered' }]);
  await expect(lateStart()).rejects.toThrow('routing has finished');
  await manager.close();
  await expect(context.dispatch(startWorkflow(noop), null)).rejects.toThrow('source has stopped');
  expect(register).toHaveBeenCalledOnce();
});

test('source failure is isolated and does not restart until replacement', async () => {
  const log = vi.spyOn(console, 'error').mockImplementation(() => {});
  const source = vi.fn(async () => {
    throw new Error('private content');
  });
  const manager = new SourceManager(() => async () => runHandle('run'));
  try {
    await manager.replace(defineWebConfig({ sources: { broken: source } }));
    await vi.waitFor(() => expect(log).toHaveBeenCalled());
    expect(JSON.stringify(log.mock.calls)).not.toContain('private content');
    expect(source).toHaveBeenCalledOnce();
    await manager.replace(defineWebConfig({ sources: { broken: source } }));
    expect(source).toHaveBeenCalledTimes(2);
  } finally {
    await manager.close();
    log.mockRestore();
  }
});

test('removing a source discards its state before the name is reused', async () => {
  const states: Map<string, unknown>[] = [];
  const source = async (ctx: EventSourceContext) => {
    states.push(ctx.state);
    await untilAbort(ctx.signal);
  };
  const manager = new SourceManager(() => async () => runHandle('run'));
  await manager.replace(defineWebConfig({ sources: { bot: source } }));
  await manager.replace(defineWebConfig({}));
  await manager.replace(defineWebConfig({ sources: { bot: source } }));
  expect(states).toHaveLength(2);
  expect(states[1]).not.toBe(states[0]);
  await manager.close();
});

test('a router can keep a run handle and send the run later messages', async () => {
  const sent: unknown[] = [];
  const conversation = task(
    { name: 'Conversation', input: Type.String(), result: Type.Null(), steering: Type.String() },
    () => null
  );
  const manager = new SourceManager(() => async () => runHandle('chat', sent));
  let context!: EventSourceContext;
  await manager.replace(
    defineWebConfig({
      sources: {
        chat: async (ctx) => {
          context = ctx;
          await untilAbort(ctx.signal);
        }
      }
    })
  );
  let handle: RunHandle<string> | undefined;
  const runs = await context.dispatch(async (ctx) => {
    handle = await ctx.start(conversation, { input: 'Hello' });
  }, undefined);
  expect(runs).toEqual([{ id: 'chat' }]);
  // The handle outlives the delivery that started the run.
  await handle?.steer('Next message');
  expect(sent).toEqual(['Next message']);
  await manager.close();
});
