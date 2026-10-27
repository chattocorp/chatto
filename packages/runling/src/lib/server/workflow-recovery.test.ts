/** Recovery tests use real snapshots and journals, including an abruptly killed process. */
import { afterEach, expect, test, vi } from 'vitest';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { task, Type } from 'runling';
import { RunStore } from './run-store.ts';
import { applyRecord, type RunDetail } from '../runs.ts';
import { createRunJournal, newRun } from '../../runtime/run-journal.ts';
import { appendFile } from 'node:fs/promises';

test('older attempts and an incomplete tail retain usage when interruption is repaired', async () => {
  const history = await create();
  const run = newRun({ webhook: 'legacy', workflow: 'Meter', source: 'cli', input: null });
  const journal = await createRunJournal(history.directory, run);
  const firstUsage = { input: 10, output: 1, cacheRead: 0, cacheWrite: 0, cost: 0.5 };
  await journal.append({
    type: 'event',
    event: { type: 'usage.updated', timestamp: 1, usage: firstUsage }
  });
  await journal.append({
    type: 'finished',
    status: 'interrupted',
    finishedAt: Date.now(),
    durationMs: 1,
    usage: firstUsage,
    output: null,
    error: null
  });
  // Legacy starts reset usage and have no previousUsage field.
  await journal.append({ type: 'started', run });
  const secondUsage = { input: 5, output: 2, cacheRead: 1, cacheWrite: 0 };
  await journal.append({
    type: 'event',
    event: { type: 'usage.updated', timestamp: 1, usage: secondUsage }
  });
  await appendFile(journal.path, '{"type":"event"');
  const repaired = await create(history.directory);
  const expected = {
    input: 15,
    output: 3,
    cacheRead: 1,
    cacheWrite: 0,
    cost: 0.5,
    costIncomplete: true
  };
  expect((await repaired.get(run.id))?.usage).toEqual(expected);
  const again = await create(history.directory);
  expect((await again.get(run.id))?.usage).toEqual(expected);
  expect(again.list()[0]?.usage).toEqual(expected);
});

test('run usage includes prior attempts in live records and repeated journal reads', async () => {
  let history = await create();
  let ready = Promise.withResolvers<void>();
  let priced = true;
  const root = task({ name: 'Meter', input: Type.Null(), result: Type.Null() }, async (ctx) => {
    ctx.recordUsage({
      input: 10,
      output: 2,
      cacheRead: 3,
      cacheWrite: 4,
      ...(priced ? { cost: 0.5 } : {})
    });
    ctx.recordUsage({ input: 1, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0 });
    ready.resolve();
    for await (const _ of ctx.steering);
    return null;
  });
  await history.restore({ meter: root });
  const first = await history.start('meter', root, null, 'source');
  await ready.promise;
  await history.close();
  stores.splice(stores.indexOf(history), 1);
  for (let attempt = 2; attempt <= 3; attempt++) {
    history = await create(history.directory);
    ready = Promise.withResolvers<void>();
    priced = false;
    let browser: RunDetail | undefined;
    const unsubscribe = history.subscribe((_id, record) => {
      if (record.type === 'started') browser = structuredClone(record.run);
      else browser = applyRecord(browser!, record);
    });
    await history.restore({ meter: root });
    await ready.promise;
    const expected = {
      input: 11 * attempt,
      output: 2 * attempt,
      cacheRead: 3 * attempt,
      cacheWrite: 4 * attempt,
      cost: 0.5,
      costIncomplete: true
    };
    await vi.waitFor(() => expect(browser?.usage).toEqual(expected));
    expect(history.list()[0]?.usage).toEqual(expected);
    if (attempt === 3) history.closeSteering(first.id);
    await history.close();
    unsubscribe();
    stores.splice(stores.indexOf(history), 1);
    expect((await history.get(first.id))?.usage).toEqual(expected);
    expect((await history.get(first.id))?.usage).toEqual(expected);
  }
});

const directories: string[] = [];
const stores: RunStore[] = [];
afterEach(async () => {
  for (const store of stores.splice(0)) await store.close();
  for (const directory of directories.splice(0))
    await rm(directory, { recursive: true, force: true });
});
async function create(directory?: string) {
  directory ??= await mkdtemp(resolve(tmpdir(), 'runling-recovery-'));
  if (!directories.includes(directory)) directories.push(directory);
  const store = new RunStore(directory);
  stores.push(store);
  await store.init();
  return store;
}

test('shutdown saves cleanup mutations; restart re-enters the same root and accepts messages', async () => {
  const first = await create();
  const ready = Promise.withResolvers<void>();
  const root = task(
    { name: 'Conversation', input: Type.String(), result: Type.Unknown() },
    async (ctx, input) => {
      ctx.state.input = input;
      try {
        ready.resolve();
        for await (const _ of ctx.steering);
      } finally {
        ctx.state.cleanup = true;
      }
    }
  );
  await first.restore({ conversation: root });
  const handle = await first.start('chat', root, 'original input', 'source');
  await ready.promise;
  const reference = (await first.get(handle.id))!.reference;
  await first.close();
  stores.splice(stores.indexOf(first), 1);
  const second = await create(first.directory);
  const resumed = task(
    { name: 'Conversation', input: Type.String(), result: Type.Unknown() },
    async (ctx, input) => {
      expect(input).toBe('original input');
      expect(ctx.run?.resumed).toBe(true);
      expect(ctx.state).toEqual({ input, cleanup: true });
      for await (const message of ctx.steering) return message;
    }
  );
  await second.restore({ conversation: resumed });
  const [recovered] = second.sourceRuns('chat');
  expect(recovered).toMatchObject({
    id: handle.id,
    workflow: 'conversation',
    input: 'original input'
  });
  expect(second.sourceRuns('another-source')).toEqual([]);
  await recovered!.steer('after restart');
  await recovered!.finished;
  expect(await second.get(handle.id)).toMatchObject({
    reference,
    status: 'completed',
    output: 'after restart'
  });
});

test('recovery and source routing preserve an undefined original input', async () => {
  const first = await create();
  let entered = Promise.withResolvers<void>();
  const root = task(
    { name: 'No input', input: Type.Unknown(), result: Type.Unknown() },
    async (ctx, input) => {
      expect(input).toBeUndefined();
      entered.resolve();
      for await (const _ of ctx.steering);
    }
  );
  await first.restore({ noInput: root });
  await first.start('events', root, undefined, 'source');
  await entered.promise;
  await first.close();
  stores.splice(stores.indexOf(first), 1);
  entered = Promise.withResolvers<void>();
  const second = await create(first.directory);
  await second.restore({ noInput: root });
  await entered.promise;
  expect(second.sourceRuns('events')[0]?.input).toBeUndefined();
});

test.each(['completed', 'cancelled', 'failed'] as const)(
  'a newer %s journal wins over a stale open snapshot',
  async (status) => {
    const first = await create();
    const ready = Promise.withResolvers<void>();
    const root = task({ name: 'Work', input: Type.Null(), result: Type.Unknown() }, async (ctx) => {
      ready.resolve();
      for await (const _ of ctx.steering) {
        if (status === 'failed') throw new Error('expected');
        return 'done';
      }
    });
    await first.restore({ work: root });
    const run = await first.start('work', root, null, 'web');
    await ready.promise;
    await first.state.save();
    const stale = await readFile(first.state.path, 'utf8');
    if (status === 'cancelled') run.cancel();
    else await run.steer('finish');
    await run.finished;
    await first.close();
    stores.splice(stores.indexOf(first), 1);
    await writeFile(first.state.path, stale);
    const second = await create(first.directory);
    const forbidden = task({ name: 'Work', input: Type.Null(), result: Type.Null() }, () => {
      throw new Error('must not execute');
    });
    await second.restore({ work: forbidden });
    expect((await second.get(run.id))?.status).toBe(status);
    expect(second.state.data.workflows[run.id]?.open).toBe(false);
  }
);

test('a missing registry key preserves state without running a different task', async () => {
  const first = await create();
  const root = task({ name: 'Wait', input: Type.Null(), result: Type.Unknown() }, async (ctx) => {
    ctx.state.marker = 'saved';
    for await (const _ of ctx.steering);
  });
  await first.restore({ wait: root });
  const run = await first.start('wait', root, null, 'web');
  await first.close();
  stores.splice(stores.indexOf(first), 1);
  const second = await create(first.directory);
  await second.restore({});
  expect((await second.get(run.id))?.status).toBe('interrupted');
  expect(second.state.data.workflows[run.id]?.open).toBe(true);
});

test('a killed process recovers the last snapshot, not later unsaved mutations', async () => {
  const original = await create();
  original.state.stop();
  const program = `
    import { RunStore } from ${JSON.stringify(new URL('./run-store.ts', import.meta.url).href)};
    import { task, Type } from 'runling';
    const store = new RunStore(process.argv[1]);
    await store.init();
    let state;
    const ready = Promise.withResolvers();
    const root = task({ name: 'Counter', input: Type.Null(), result: Type.Unknown() }, async ctx => {
      state = ctx.state;
      state.count = 1;
      ready.resolve();
      for await (const _ of ctx.steering);
    });
    await store.restore({ counter: root });
    await store.start('counter', root, null, 'source');
    await ready.promise;
    await store.state.save();
    state.count = 2;
    process.kill(process.pid, 'SIGKILL');
  `;
  await expect(
    promisify(execFile)(
      process.execPath,
      ['--import', 'tsx', '--input-type=module', '--eval', program, original.directory],
      { timeout: 15_000 }
    )
  ).rejects.toMatchObject({ signal: 'SIGKILL' });
  // The reader instance never owned or wrote the child process's snapshot.
  stores.splice(stores.indexOf(original), 1);
  const restarted = await create(original.directory);
  const seen = Promise.withResolvers<unknown>();
  const root = task({ name: 'Counter', input: Type.Null(), result: Type.Unknown() }, (ctx) => {
    seen.resolve(ctx.state.count);
    return ctx.state.count;
  });
  await restarted.restore({ counter: root });
  const [recovered] = restarted.sourceRuns('counter');
  expect(await seen.promise).toBe(1);
  await recovered!.finished;
  await restarted.close();
  expect(restarted.list()[0]?.status).toBe('completed');
}, 20_000);
