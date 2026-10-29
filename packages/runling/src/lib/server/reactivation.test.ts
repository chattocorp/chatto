/** Exercise explicit reactivation across persistence, concurrent routing, and old handles. */
import { mkdir, mkdtemp, readFile, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { afterEach, expect, test, vi } from 'vitest';
import { log, task, Type } from 'runling';
import { RunStore } from './run-store.ts';
import { applyRecord, type RunDetail } from '../runs.ts';
import { buildTimeline } from '../timeline.ts';
import { runLogRows } from '../run-log.ts';

const stores = new Set<RunStore>();
const directories: string[] = [];
afterEach(async () => {
  for (const store of stores) await store.close();
  stores.clear();
  for (const directory of directories.splice(0))
    await rm(directory, { recursive: true, force: true });
});

async function create(directory?: string) {
  if (!directory) {
    await mkdir(resolve('../../.context'), { recursive: true });
    directory = await mkdtemp(resolve('../../.context/reactivation-'));
    directories.push(directory);
  }
  const store = new RunStore(directory);
  stores.add(store);
  await store.init();
  return store;
}

test('live, reconnected and restarted consoles retain every turn without duplicating journal history', async () => {
  const release = Promise.withResolvers<void>();
  const root = task(
    { name: 'Turn', input: Type.Number(), result: Type.Number() },
    async (ctx, input) => {
      log.info(`Turn ${input}`);
      ctx.recordUsage({ input: 2, output: 1, cacheRead: 0, cacheWrite: 0, cost: 0.1 });
      if (input === 2) await release.promise;
      return input;
    }
  );
  let store = await create();
  await store.restore({ root });
  let browser: RunDetail | undefined;
  const unsubscribe = store.subscribe((_id, record) => {
    const copy = structuredClone(record);
    browser = copy.type === 'started' ? copy.run : applyRecord(browser!, copy);
  });
  const first = await store.start('chat', root, 1, 'source');
  await first.completion;
  const initial = structuredClone(browser!);
  const second = await store.start('chat', root, 2, 'source', first.id);
  try {
    await vi.waitFor(() =>
      expect(browser?.events.some((e) => e.type === 'log' && e.message === 'Turn 2')).toBe(true)
    );
    expect(browser!.events.slice(0, initial.events.length)).toEqual(initial.events);
    expect(browser!.startedAt).toBe(initial.startedAt);
    expect(buildTimeline(browser!.events, 'running').map((node) => node.status)).toEqual([
      'completed',
      'running'
    ]);
    expect((await store.get(first.id))!.events).toEqual(browser!.events);
  } finally {
    release.resolve();
  }
  await second.completion;
  const final = structuredClone(browser!);
  expect(
    runLogRows(final.events)
      .filter((row) => /^Turn [12]$/.test(row.event.message))
      .map((row) => row.event.message)
  ).toEqual(['Turn 1', 'Turn 2']);
  expect(final.usage).toMatchObject({ input: 4, output: 2, cost: 0.2 });
  expect((await store.get(first.id))!.events).toEqual(final.events);
  const records = (await readFile(resolve(store.directory, `${first.id}.jsonl`), 'utf8'))
    .trim()
    .split('\n')
    .map((line) => JSON.parse(line));
  expect(
    records.filter((record) => record.type === 'started').map((record) => record.run.events)
  ).toEqual([[], []]);
  unsubscribe();
  await store.close();
  stores.delete(store);
  store = await create(store.directory);
  expect((await store.get(first.id))!.events).toEqual(final.events);
  expect((await store.get(first.id))!.durationMs).toBe(final.durationMs);
});

test('reactivates with fresh input and saved state, preserves history and usage, and stays closed on restart', async () => {
  const calls: number[] = [];
  const root = task(
    { name: 'Counter', input: Type.Number(), result: Type.Number() },
    (ctx, input) => {
      calls.push(input);
      expect(ctx.run?.resumed).toBe(false);
      ctx.recordUsage({ input: 2, output: 1, cacheRead: 0, cacheWrite: 0, cost: 0.1 });
      ctx.state.total = Number(ctx.state.total ?? 0) + input;
      return Number(ctx.state.total);
    }
  );
  let store = await create();
  await store.restore({ counter: root });
  const first = await store.start('counter', root, 3, 'source');
  await first.completion;
  const reference = (await store.get(first.id))!.reference;
  const second = await store.start('counter', root, 4, 'source', first.id);
  expect((await second.completion).output).toBe(7);
  expect(second.id).toBe(first.id);
  expect((await store.get(first.id))?.usage).toMatchObject({ input: 4, output: 2, cost: 0.2 });
  expect(store.sourceRuns('counter')).toEqual([]);
  expect(store.sourceRuns('counter', { includeFinished: true })[0]?.input).toBe(4);
  const path = resolve(store.directory, `${first.id}.jsonl`);
  const before = await readFile(path, 'utf8');
  expect(before.split('\n').filter((line) => line.includes('"type":"started"'))).toHaveLength(2);
  await store.close();
  stores.delete(store);
  expect(await readFile(path, 'utf8')).toBe(before);
  store = await create(store.directory);
  await store.restore({ counter: root });
  expect(calls).toEqual([3, 4]);
  expect(store.sourceRuns('counter', { includeFinished: true })[0]?.id).toBe(first.id);
  const third = await store.start('counter', root, 5, 'source', first.id);
  expect((await third.completion).output).toBe(12);
  expect((await store.get(first.id))?.reference).toBe(reference);
});

test('rejects invalid ownership and input without changing history; reserves concurrent reactivation', async () => {
  const root = task(
    { name: 'Wait', input: Type.Boolean(), result: Type.Null(), steering: Type.String() },
    async (ctx, wait) => {
      if (wait) for await (const _ of ctx.steering) break;
      return null;
    }
  );
  const other = task({ name: 'Other', input: Type.Boolean(), result: Type.Null() }, () => null);
  const store = await create();
  await store.restore({ root, other });
  const first = await store.start('owner', root, false, 'source');
  await first.completion;
  const prior = await store.get(first.id);
  await expect(store.start('another', root, true, 'source', first.id)).rejects.toThrow(
    'not available'
  );
  await expect(store.start('owner', root, true, 'webhook', first.id)).rejects.toThrow(
    'not available'
  );
  await expect(store.start('owner', other, true, 'source', first.id)).rejects.toThrow(
    'not available'
  );
  await expect(store.start('owner', root, 'invalid', 'source', first.id)).rejects.toThrow(
    'input is invalid'
  );
  expect(await store.get(first.id)).toEqual(prior);
  const next = store.start('owner', root, true, 'source', first.id);
  await expect(store.start('owner', root, 'invalid', 'source', first.id)).rejects.toThrow(
    'still active'
  );
  const second = await next;
  first.cancel();
  first.closeSteering();
  await expect(first.steer('stale')).rejects.toThrow('not running');
  expect((await store.get(first.id))?.status).toBe('running');
  await second.steer('finish');
  expect((await second.completion).ok).toBe(true);
});

test('shutdown waits for a pending reactivation and interrupts only its new execution', async () => {
  const root = task(
    { name: 'Wait', input: Type.Boolean(), result: Type.Null() },
    async (ctx, wait) => {
      if (wait) for await (const _ of ctx.steering);
      return null;
    }
  );
  const store = await create();
  await store.restore({ root });
  const first = await store.start('owner', root, false, 'source');
  await first.completion;
  const registration = store.start('owner', root, true, 'source', first.id);
  await store.close();
  stores.delete(store);
  await (
    await registration
  ).finished;
  expect((await store.get(first.id))?.status).toBe('interrupted');
});
