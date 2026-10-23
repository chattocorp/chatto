import { afterEach, expect, test, vi } from 'vitest';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { WorkflowStateStore } from './workflow-state.ts';
import { runWorkflow } from './runner.ts';
import { task } from './task.ts';
import { Type } from 'typebox';

const directories: string[] = [];
const stores: WorkflowStateStore[] = [];
afterEach(async () => {
  for (const store of stores.splice(0)) store.stop();
  vi.useRealTimers();
  for (const path of directories.splice(0)) await rm(path, { recursive: true, force: true });
});
async function fixture() {
  const directory = await mkdtemp(join(tmpdir(), 'runling-state-'));
  directories.push(directory);
  const store = new WorkflowStateStore(join(directory, 'state.json'));
  stores.push(store);
  await store.load();
  const id = crypto.randomUUID();
  const state = { child: { count: 0 } };
  store.data.workflows[id] = { task: 'counter', input: null, state, open: true };
  return { store, id, state };
}

test('ordinary nested mutations reach periodic snapshots and the final shutdown snapshot', async () => {
  const { store, state, id } = await fixture();
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  const errors = vi.fn();
  store.start(errors);
  state.child.count = 1;
  await vi.advanceTimersByTimeAsync(60_000);
  // Wait for real filesystem I/O while the interval clock remains controlled.
  await vi.waitFor(async () => {
    const saved = JSON.parse(await readFile(store.path, 'utf8'));
    expect(saved.workflows[id].state.child.count).toBe(1);
  });
  state.child.count = 2;
  await store.close();
  const restored = new WorkflowStateStore(store.path);
  await restored.load();
  expect(restored.data.workflows[id]?.state).toEqual({ child: { count: 2 } });
  expect(errors).not.toHaveBeenCalled();
});

test('queued snapshots keep call order and serialization failure preserves the last good file', async () => {
  const { store, state, id } = await fixture();
  state.child.count = 1;
  const first = store.save();
  state.child.count = 2;
  const second = store.save();
  await Promise.all([first, second]);
  const good = await readFile(store.path, 'utf8');
  expect(JSON.parse(good).workflows[id].state.child.count).toBe(2);
  store.data.workflows[id]!.state.bad = new Map();
  await expect(store.save()).rejects.toThrow('JSON');
  expect(await readFile(store.path, 'utf8')).toBe(good);
  delete store.data.workflows[id]!.state.bad;
  await store.save();
});

test('shutdown waits for in-flight maintenance before writing the final snapshot', async () => {
  const { store, state, id } = await fixture();
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  const release = Promise.withResolvers<void>();
  const maintenance = vi.fn(async () => {
    await release.promise;
    state.child.count = 7;
  });
  store.start(vi.fn(), 60_000, maintenance);
  await vi.advanceTimersByTimeAsync(60_000);
  expect(maintenance).toHaveBeenCalledOnce();
  let closed = false;
  const closing = store.close().then(() => {
    closed = true;
  });
  await Promise.resolve();
  expect(closed).toBe(false);
  release.resolve();
  await closing;
  expect(JSON.parse(await readFile(store.path, 'utf8')).workflows[id].state.child.count).toBe(7);
  expect(vi.getTimerCount()).toBe(0);
});

test('a failed disk write is reported and a later save can succeed', async () => {
  const { store } = await fixture();
  await writeFile(`${store.path}.${process.pid}.tmp`, 'occupied');
  // A destination directory cannot be replaced by the snapshot file.
  await mkdir(store.path);
  await expect(store.save()).rejects.toThrow();
  await rm(store.path, { recursive: true });
  await store.save();
  expect(JSON.parse(await readFile(store.path, 'utf8')).version).toBe(1);
});

test('malformed or unsupported snapshots fail startup without overwriting the file', async () => {
  const { store } = await fixture();
  for (const content of [
    '{',
    '{"version":2,"workflows":{}}',
    '{"version":1,"workflows":{"bad":{}}}'
  ]) {
    await writeFile(store.path, content);
    await expect(store.load()).rejects.toThrow();
    expect(await readFile(store.path, 'utf8')).toBe(content);
  }
});

test('only the root receives workflow state; a parent can pass an ordinary sub-object', async () => {
  const state = { private: 'root only', child: { count: 0 } };
  const child = task(
    { name: 'Child', input: Type.Object({ count: Type.Number() }), result: Type.Null() },
    (ctx, input) => {
      expect(ctx.state).toEqual({});
      input.count++;
      return null;
    }
  );
  const root = task({ name: 'Root', input: Type.Null(), result: Type.Null() }, async (ctx) => {
    expect(ctx.state).toBe(state);
    await child(ctx, state.child);
    return null;
  });
  expect(await runWorkflow(root, { input: null, state })).toMatchObject({ ok: true });
  expect(state.child.count).toBe(1);
});
