/** Retention uses terminal journal timestamps and retries partial filesystem cleanup. */
import { afterEach, expect, test, vi } from 'vitest';
import { mkdir, mkdtemp, readFile, rm, access, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { RunStore } from './run-store.ts';
import { createRunJournal, newRun, type RunStatus } from '../../runtime/run-journal.ts';
import { agentSessionFile } from '../../runtime/agent-sessions.ts';

const failures = vi.hoisted(() => new Set<string>());
vi.mock('node:fs/promises', async (original) => {
  const fs = await original<typeof import('node:fs/promises')>();
  return {
    ...fs,
    rm: async (path: Parameters<typeof fs.rm>[0], options: Parameters<typeof fs.rm>[1]) => {
      if (failures.has(String(path))) throw new Error('filesystem unavailable');
      return fs.rm(path, options);
    }
  };
});

const DAY = 24 * 60 * 60_000;
const stores: RunStore[] = [];
afterEach(async () => {
  failures.clear();
  for (const store of stores.splice(0)) {
    await store.close();
    await rm(store.directory, { recursive: true, force: true });
  }
  vi.restoreAllMocks();
  vi.useRealTimers();
});

async function fixture() {
  await mkdir(resolve('../../.context'), { recursive: true });
  const directory = await mkdtemp(resolve('../../.context/runling-retention-'));
  const store = new RunStore(directory);
  stores.push(store);
  return store;
}

/** Seed an old run without running a model or a task. State can deliberately lag its journal. */
async function seed(store: RunStore, status: RunStatus, finishedAt = Date.now() - 31 * DAY) {
  const run = newRun({ webhook: 'fixture', workflow: 'Fixture', source: 'cli', input: null });
  if (status === 'running') run.pid = process.ppid;
  run.startedAt = Date.now() - 90 * DAY;
  const journal = await createRunJournal(store.directory, run);
  if (status !== 'running')
    await journal.append({
      type: 'finished',
      status,
      finishedAt,
      durationMs: 1,
      usage: run.usage,
      output: null,
      error: null
    });
  // Even a stale open flag must not override a terminal journal.
  store.state.data.workflows[run.id] = { task: null, input: null, state: { count: 1 }, open: true };
  await store.state.save();
  const filesDirectory = resolve(store.directory, run.id);
  const session = (await agentSessionFile(
    { run: { id: run.id, filesDirectory } },
    'conversation'
  ))!;
  await writeFile(session, 'saved transcript');
  return { id: run.id, path: journal.path, session, filesDirectory };
}

test('startup purges only runs closed over 30 days ago, using closure rather than creation time', async () => {
  vi.spyOn(Date, 'now').mockReturnValue(Date.now());
  const store = await fixture();
  const expired = await Promise.all(
    ['completed', 'failed', 'cancelled'].map((status) => seed(store, status as RunStatus))
  );
  const retained = [
    await seed(store, 'completed', Date.now() - DAY),
    await seed(store, 'completed', Date.now() - 30 * DAY),
    await seed(store, 'running'),
    await seed(store, 'interrupted')
  ];
  await store.init();
  for (const { id, path, session } of expired) {
    await expect(access(path)).rejects.toMatchObject({ code: 'ENOENT' });
    expect(await store.get(id)).toBeUndefined();
    expect(store.state.data.workflows[id]).toBeUndefined();
    await expect(access(session)).rejects.toMatchObject({ code: 'ENOENT' });
  }
  for (const { id, path, session } of retained) {
    await access(path);
    expect(store.state.data.workflows[id]).toBeDefined();
    expect(await readFile(session, 'utf8')).toBe('saved transcript');
  }
  expect((await store.get(retained[2]!.id))?.status).toBe('running');
  const saved = JSON.parse(await readFile(store.state.path, 'utf8'));
  expect(Object.keys(saved.workflows).sort()).toEqual(retained.map((run) => run.id).sort());
});

test('the existing persistence timer runs cleanup daily and discovers journals added after startup', async () => {
  const now = Date.now();
  const clock = vi.spyOn(Date, 'now').mockReturnValue(now);
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  const store = await fixture();
  await store.init();
  const old = await seed(store, 'completed');
  await vi.advanceTimersByTimeAsync(60_000);
  await access(old.path);
  clock.mockReturnValue(now + DAY);
  // Allow the prior asynchronous save to finish and schedule its next tick.
  await vi.waitFor(() => expect(vi.getTimerCount()).toBeGreaterThan(0));
  await vi.advanceTimersByTimeAsync(60_000);
  await vi.waitFor(async () =>
    expect(await readFile(store.state.path, 'utf8')).toContain('"workflows":{}')
  );
  await vi.waitFor(async () => {
    await expect(access(old.path)).rejects.toMatchObject({ code: 'ENOENT' });
  });
  await expect(access(old.session)).rejects.toMatchObject({ code: 'ENOENT' });
});

test('run-file cleanup failures leave the journal retryable while other runs and saves proceed', async () => {
  const now = Date.now();
  const clock = vi.spyOn(Date, 'now').mockReturnValue(now);
  const store = await fixture();
  const failed = await seed(store, 'completed');
  const good = await seed(store, 'failed');
  failures.add(failed.filesDirectory);
  await expect(store.init()).resolves.toBeUndefined();
  await access(failed.path);
  await access(failed.session);
  expect(store.state.data.workflows[failed.id]).toBeUndefined();
  await expect(access(good.path)).rejects.toMatchObject({ code: 'ENOENT' });
  await expect(store.state.save()).resolves.toBeUndefined();
  failures.clear();
  clock.mockReturnValue(now + DAY);
  await store.cleanup();
  await expect(access(failed.path)).rejects.toMatchObject({ code: 'ENOENT' });
  await expect(access(failed.session)).rejects.toMatchObject({ code: 'ENOENT' });
  expect(store.state.data.workflows[failed.id]).toBeUndefined();
});

test('a failed snapshot keeps journals and restores in-memory state for the next cleanup', async () => {
  const now = Date.now();
  const clock = vi.spyOn(Date, 'now').mockReturnValue(now);
  const store = await fixture();
  const old = await seed(store, 'completed');
  const save = vi.spyOn(store.state, 'save').mockRejectedValueOnce(new Error('disk unavailable'));
  await store.init();
  await access(old.path);
  await access(old.session);
  expect(store.state.data.workflows[old.id]).toBeDefined();
  save.mockRestore();
  clock.mockReturnValue(now + DAY);
  await store.cleanup();
  await expect(access(old.path)).rejects.toMatchObject({ code: 'ENOENT' });
  await expect(access(old.session)).rejects.toMatchObject({ code: 'ENOENT' });
});

test('a crash after snapshot cleanup but before journal deletion is safe to retry', async () => {
  const store = await fixture();
  const old = await seed(store, 'cancelled');
  delete store.state.data.workflows[old.id];
  await store.state.save();
  await store.init();
  await store.restore();
  await expect(access(old.path)).rejects.toMatchObject({ code: 'ENOENT' });
  expect(JSON.parse(await readFile(store.state.path, 'utf8')).workflows).toEqual({});
});
