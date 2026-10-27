/** Exercise Botto's root task against the persistent host, including source replacement and restart. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { RunStore } from '../../../packages/runling/src/lib/server/run-store.ts';
import { conversation, runTurn } from './conversation.ts';
import { BatchQueue } from 'runling';
import type { EventSourceContext } from 'runling/web';

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
