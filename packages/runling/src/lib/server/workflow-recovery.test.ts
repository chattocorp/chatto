/** Recovery tests use real snapshots and journals, including an abruptly killed process. */
import { afterEach, expect, test } from 'vitest';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { task, Type } from 'runling';
import { RunStore } from './run-store.ts';

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
    { name: 'Conversation', input: Type.String(), output: Type.Unknown() },
    async (ctx, input) => {
      ctx.state.input = input;
      try {
        ready.resolve();
        for await (const _ of ctx.inbox);
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
    { name: 'Conversation', input: Type.String(), output: Type.Unknown() },
    async (ctx, input) => {
      expect(input).toBe('original input');
      expect(ctx.run?.resumed).toBe(true);
      expect(ctx.state).toEqual({ input, cleanup: true });
      for await (const message of ctx.inbox) return message;
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
  await recovered!.send('after restart');
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
    { name: 'No input', input: Type.Unknown(), output: Type.Unknown() },
    async (ctx, input) => {
      expect(input).toBeUndefined();
      entered.resolve();
      for await (const _ of ctx.inbox);
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
    const root = task({ name: 'Work', input: Type.Null(), output: Type.Unknown() }, async (ctx) => {
      ready.resolve();
      for await (const _ of ctx.inbox) {
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
    else await run.send('finish');
    await run.finished;
    await first.close();
    stores.splice(stores.indexOf(first), 1);
    await writeFile(first.state.path, stale);
    const second = await create(first.directory);
    const forbidden = task({ name: 'Work', input: Type.Null(), output: Type.Null() }, () => {
      throw new Error('must not execute');
    });
    await second.restore({ work: forbidden });
    expect((await second.get(run.id))?.status).toBe(status);
    expect(second.state.data.workflows[run.id]?.open).toBe(false);
  }
);

test('a missing registry key preserves state without running a different task', async () => {
  const first = await create();
  const root = task({ name: 'Wait', input: Type.Null(), output: Type.Unknown() }, async (ctx) => {
    ctx.state.marker = 'saved';
    for await (const _ of ctx.inbox);
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
    const root = task({ name: 'Counter', input: Type.Null(), output: Type.Unknown() }, async ctx => {
      state = ctx.state;
      state.count = 1;
      ready.resolve();
      for await (const _ of ctx.inbox);
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
      [
        '--conditions=runling-source',
        '--import',
        'tsx',
        '--input-type=module',
        '--eval',
        program,
        original.directory
      ],
      { timeout: 15_000 }
    )
  ).rejects.toMatchObject({ signal: 'SIGKILL' });
  // The reader instance never owned or wrote the child process's snapshot.
  stores.splice(stores.indexOf(original), 1);
  const restarted = await create(original.directory);
  const seen = Promise.withResolvers<unknown>();
  const root = task({ name: 'Counter', input: Type.Null(), output: Type.Unknown() }, (ctx) => {
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
