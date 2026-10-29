/** Exercise Botto's root task against the persistent host, including source replacement and restart. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { RunStore } from '../../../packages/runling/src/lib/server/run-store.ts';
import { closeConversations, conversation, runTurn } from './conversation.ts';
import { BatchQueue } from 'runling';
import type { EventSourceContext } from 'runling/web';
import { parseSettings } from './settings.ts';

const sessions = vi.hoisted(() => vi.fn());
vi.mock('runling', async (original) => ({
  ...(await original<typeof import('runling')>()),
  createAgentSession: sessions
}));
const settings = parseSettings(
  '[chatto]\nurl="https://example.com"\napi_key="test"\n[agent]\nmodel="test/model"',
  'botto.toml'
);

const stores: RunStore[] = [];
const directories: string[] = [];
afterEach(async () => {
  await Promise.all(stores.splice(0).map((store) => store.close()));
  await Promise.all(
    directories.splice(0).map((path) => rm(path, { recursive: true, force: true }))
  );
});

async function setup(directory?: string) {
  if (!directory) {
    await mkdir(resolve('../../.context'), { recursive: true });
    directory = await mkdtemp(resolve('../../.context/conversation-runs-'));
    directories.push(directory);
  }
  const store = new RunStore(directory);
  stores.push(store);
  await store.init();
  await store.restore({ bottoConversation: conversation });
  const controller = new AbortController();
  const source: EventSourceContext = {
    signal: controller.signal,
    state: new Map(),
    runs: (options) => store.sourceRuns('source', options),
    dispatch: async (router, input) => {
      const started: { id: string }[] = [];
      await router(
        {
          start: async (task, options) => {
            const run = await store.start('source', task, options.input, 'source', options.runId);
            started.push({ id: run.id });
            return run;
          }
        },
        input
      );
      return started;
    }
  };
  return { store, source, controller, directory };
}

test('reactivates retained keys after success and failure, and leaves no idle executions', async () => {
  const { store, source } = await setup();
  const ids: string[] = [];
  const work = vi.fn(async (ctx) => {
    ids.push(ctx.run.id);
  });
  await runTurn(source, 'one', work);
  await runTurn(source, 'one', work);
  await expect(
    runTurn(source, 'one', async () => {
      throw new Error('private failure');
    })
  ).rejects.toThrow('private failure');
  await runTurn(source, 'one', work);
  await runTurn(source, 'other', work);
  expect(ids.slice(0, 3)).toEqual([ids[0], ids[0], ids[0]]);
  expect(ids[3]).not.toBe(ids[0]);
  expect(store.sourceRuns('source')).toHaveLength(0);
  expect(store.sourceRuns('source', { includeFinished: true })).toHaveLength(2);
  await expect(
    store.sourceRuns('source', { includeFinished: true })[0]!.steer('Execute this text as a turn')
  ).rejects.toThrow('not running');
  expect(work).toHaveBeenCalledTimes(4);
});

test('closing steering waits for active work; a later batch reactivates the root', async () => {
  const { store, source } = await setup();
  const release = Promise.withResolvers<void>();
  const started = Promise.withResolvers<void>();
  const turn = runTurn(source, 'one', async () => {
    started.resolve();
    await release.promise;
  });
  await started.promise;
  const run = store.sourceRuns('source')[0]!;
  expect(store.sourceRuns('source')).toHaveLength(1);
  run.closeSteering();
  let finished = false;
  void run.finished.then(() => {
    finished = true;
  });
  await Promise.resolve();
  expect(finished).toBe(false);
  release.resolve();
  await turn;
  await run.finished;
  await runTurn(source, 'one', async () => {});
  expect(store.sourceRuns('source', { includeFinished: true })[0]!.id).toBe(run.id);
});

test('a replacement source can deliver to the existing thread root', async () => {
  const { store, source } = await setup();
  await runTurn(source, 'one', async () => {});
  const id = store.sourceRuns('source', { includeFinished: true })[0]!.id;
  await store.restore({ bottoConversation: conversation });
  await runTurn({ ...source, state: new Map() }, 'one', async (ctx) => {
    expect(ctx.run?.id).toBe(id);
  });
  expect(store.sourceRuns('source')).toHaveLength(0);
});

test('restart retains the ID without replaying completed work', async () => {
  const original = await setup();
  const work = vi.fn(async () => {});
  await runTurn(original.source, 'one', work);
  const id = original.store.sourceRuns('source', { includeFinished: true })[0]!.id;
  await original.store.close();
  // The replacement owns this directory now. Do not close the old store again
  // alongside it in afterEach: that would start two snapshot writers.
  stores.splice(stores.indexOf(original.store), 1);
  const resumed = await setup(original.directory);
  expect(resumed.store.sourceRuns('source')).toEqual([]);
  expect(
    resumed.store.sourceRuns('source', { includeFinished: true }).map((run) => run.id)
  ).toEqual([id]);
  expect(work).toHaveBeenCalledOnce();
  await runTurn(resumed.source, 'one', async (ctx) => {
    expect(ctx.run?.id).toBe(id);
  });
});

test('an old waiting root closes on recovery and is reused without replaying its work', async () => {
  const original = await setup();
  // Simulate a prior version's still-open root, before a private ticket arrives.
  const legacy = await original.store.start('source', conversation, 'legacy', 'source');
  await original.store.close();
  stores.splice(stores.indexOf(original.store), 1);
  const restarted = await setup(original.directory);
  const callback = vi.fn(async (ctx) => expect(ctx.run?.id).toBe(legacy.id));
  // Deliver immediately: recovery has launched the old root but need not have
  // finished its journal and snapshot writes yet.
  await runTurn(restarted.source, 'legacy', callback);
  expect(callback).toHaveBeenCalledOnce();
  expect(restarted.store.sourceRuns('source')).toEqual([]);
  expect((await restarted.store.get(legacy.id))?.status).toBe('completed');
});

test('source cancellation reaches the task and waits for its cleanup', async () => {
  const { source, controller } = await setup();
  const started = Promise.withResolvers<void>();
  const cleanup = Promise.withResolvers<void>();
  let finished = false;
  const delivery = runTurn(source, 'one', async (ctx) => {
    started.resolve();
    await new Promise<void>((resolve) =>
      ctx.signal.addEventListener('abort', () => resolve(), { once: true })
    );
    await cleanup.promise;
    ctx.signal.throwIfAborted();
  });
  const rejected = expect(delivery).rejects.toThrow();
  void delivery
    .finally(() => {
      finished = true;
    })
    .catch(() => {});
  await started.promise;
  controller.abort();
  await Promise.resolve();
  expect(finished).toBe(false);
  cleanup.resolve();
  await rejected;
});

test('the batch scheduler serializes one conversation and bounds work across keys', async () => {
  const { source } = await setup();
  const release = Promise.withResolvers<void>();
  const batches: string[][] = [];
  const queue = new BatchQueue<{ id: string; key: string }>({
    key: (item) => item.key,
    id: (item) => item.id,
    delay: () => 0,
    maxKeys: 2,
    maxItems: 4,
    failed: (error) => {
      throw error;
    },
    run: (items) =>
      runTurn(source, items[0]!.key, async () => {
        batches.push(items.map((item) => item.id));
        if (items[0]!.id === 'first') await release.promise;
      })
  });
  queue.accept({ id: 'first', key: 'one' });
  await vi.waitFor(() => expect(batches).toEqual([['first']]));
  queue.accept({ id: 'second', key: 'one' });
  queue.accept({ id: 'third', key: 'one' });
  queue.accept({ id: 'other', key: 'two' });
  expect(queue.accept({ id: 'overflow', key: 'three' })).toBe('full');
  await vi.waitFor(() => expect(batches).toEqual([['first'], ['other']]));
  release.resolve();
  await vi.waitFor(() => expect(batches).toEqual([['first'], ['other'], ['second', 'third']]));
  await queue.close();
});

test('accepts follow-ups during a child and serializes automatic completion behind the parent turn', async () => {
  const { source, store } = await setup();
  const childResult = Promise.withResolvers<{ outcome: string; summary: string }>();
  const session = {
    runOutcome: vi.fn(() => childResult.promise),
    steer: vi.fn(async () => true),
    [Symbol.asyncDispose]: vi.fn(async () => {})
  };
  sessions.mockResolvedValue(session);
  const events: string[] = [];
  let childId = '';
  await runTurn(source, 'one', async (_ctx, children) => {
    childId = children.start(settings, { task: 'Research' }, async (result) => {
      expect(result.result).toBe('Findings');
      events.push('completion');
    });
    events.push('initial reply');
  });
  const root = store.sourceRuns('source')[0]!;
  expect(events).toEqual(['initial reply']);
  const entered = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const followup = runTurn(source, 'one', async (ctx, children) => {
    expect(ctx.run?.id).toBe(root.id);
    expect(children.inspect(childId)[0]?.status).toBe('running');
    await children.steer(childId, 'Focus on costs');
    events.push('followup started');
    entered.resolve();
    await release.promise;
    events.push('followup done');
  });
  await entered.promise;
  await vi.waitFor(() => expect(session.steer).toHaveBeenCalledWith('Focus on costs'));
  childResult.resolve({ outcome: 'completed', summary: 'Findings' });
  await vi.waitFor(() => expect(session[Symbol.asyncDispose]).toHaveBeenCalledOnce());
  expect(events).toEqual(['initial reply', 'followup started']);
  release.resolve();
  await followup;
  await root.finished;
  expect(events).toEqual(['initial reply', 'followup started', 'followup done', 'completion']);
  expect(store.sourceRuns('source')).toEqual([]);
});

test('idle parent wakes for child completion without another user message', async () => {
  const { source, store } = await setup();
  const result = Promise.withResolvers<{ outcome: string; summary: string }>();
  sessions.mockResolvedValue({
    runOutcome: () => result.promise,
    [Symbol.asyncDispose]: async () => {}
  });
  const delivered = vi.fn(async () => {});
  await runTurn(source, 'one', async (_ctx, children) => {
    children.start(settings, { task: 'Work' }, delivered);
  });
  const root = store.sourceRuns('source')[0]!;
  result.resolve({ outcome: 'completed', summary: 'Done' });
  await root.finished;
  expect(delivered).toHaveBeenCalledOnce();
});

test('source shutdown cancels children between batches and suppresses completion callbacks', async () => {
  const { source, store, controller } = await setup();
  const entered = Promise.withResolvers<void>();
  const disposed = vi.fn(async () => {});
  sessions.mockResolvedValue({
    runOutcome: async (_ctx: unknown, _prompt: unknown, { signal }: { signal: AbortSignal }) => {
      entered.resolve();
      return new Promise((_resolve, reject) =>
        signal.addEventListener('abort', () => reject(signal.reason), { once: true })
      );
    },
    [Symbol.asyncDispose]: disposed
  });
  const delivered = vi.fn(async () => {});
  await runTurn(source, 'one', async (_ctx, children) => {
    children.start(settings, { task: 'Work' }, delivered);
  });
  const root = store.sourceRuns('source')[0]!;
  await entered.promise;
  await expect(runTurn(source, 'other', async () => {}, 1)).rejects.toThrow('capacity');
  controller.abort();
  await root.finished;
  expect(disposed).toHaveBeenCalledOnce();
  expect(delivered).not.toHaveBeenCalled();
});

test('source failure closes background roots even without source-signal cancellation', async () => {
  const { source, store } = await setup();
  const entered = Promise.withResolvers<void>();
  const disposed = vi.fn(async () => {});
  sessions.mockResolvedValue({
    runOutcome: async (_ctx: unknown, _prompt: unknown, { signal }: { signal: AbortSignal }) => {
      entered.resolve();
      return new Promise((_resolve, reject) =>
        signal.addEventListener('abort', () => reject(signal.reason), { once: true })
      );
    },
    [Symbol.asyncDispose]: disposed
  });
  const delivered = vi.fn(async () => {});
  await runTurn(source, 'one', async (_ctx, children) => {
    children.start(settings, { task: 'Work' }, delivered);
  });
  await entered.promise;
  await closeConversations(source);
  expect(source.signal.aborted).toBe(false);
  expect(store.sourceRuns('source')).toEqual([]);
  expect(disposed).toHaveBeenCalledOnce();
  expect(delivered).not.toHaveBeenCalled();
});

test('reserves capacity before asynchronous root admission', async () => {
  const { source } = await setup();
  const entered = Promise.withResolvers<void>();
  const release = Promise.withResolvers<void>();
  const original = source.dispatch;
  source.dispatch = async (...args) => {
    entered.resolve();
    await release.promise;
    return original(...args);
  };
  const first = runTurn(source, 'one', async () => {}, 1);
  await entered.promise;
  await expect(runTurn(source, 'two', async () => {}, 1)).rejects.toThrow('capacity');
  release.resolve();
  await first;
});
