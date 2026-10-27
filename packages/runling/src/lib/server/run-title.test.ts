/** Verify titles through the runtime, live reducers, journal replay, and reactivation. */
import { mkdir, mkdtemp, rm } from 'node:fs/promises';
import { resolve } from 'node:path';
import { expect, test } from 'vitest';
import { task, Type, runWorkflow, type RunContext } from 'runling';
import { RunStore } from './run-store.ts';
import { applyRecord, type RunDetail } from '../runs.ts';

test('titles update live and survive restart and reactivation without renaming the workflow', async () => {
  await mkdir(resolve('../../.context'), { recursive: true });
  const directory = await mkdtemp(resolve('../../.context/run-title-'));
  let store = new RunStore(directory);
  let browser: RunDetail | undefined;
  let updates = 0;
  const root = task({ name: 'Report', input: Type.String(), result: Type.Null() }, (ctx, title) => {
    if (title) {
      ctx.run!.setTitle(title);
      ctx.run!.setTitle(title);
    } else expect(ctx.run?.title).toBe('Compare JSON and SQLite');
    return null;
  });
  try {
    await store.init();
    await store.restore({ report: root });
    store.subscribe((_id, record) => {
      if (record.type === 'started') browser = structuredClone(record.run);
      else browser = applyRecord(browser!, record);
      if (record.type === 'event' && record.event.type === 'run.title') updates++;
    });
    const run = await store.start('report', root, '  Compare JSON\n and SQLite  ', 'source');
    await run.completion;
    expect(updates).toBe(1);
    expect(browser).toMatchObject({ title: 'Compare JSON and SQLite', workflow: 'Report' });
    expect(store.list()[0]?.title).toBe('Compare JSON and SQLite');
    const reference = browser!.reference;
    await store.close();
    store = new RunStore(directory);
    await store.init();
    await store.restore({ report: root });
    expect((await store.get(run.id))?.title).toBe('Compare JSON and SQLite');
    const next = await store.start('report', root, '', 'source', run.id);
    await next.completion;
    expect(await store.get(run.id)).toMatchObject({ title: 'Compare JSON and SQLite', reference });
  } finally {
    await store.close();
    await rm(directory, { recursive: true, force: true });
  }
});

test('rejects invalid titles and setters retained after execution; preserves plain text', async () => {
  let retained: RunContext | undefined;
  const titles: string[] = [];
  const root = task({ name: 'Title', input: Type.Null(), result: Type.Null() }, (ctx) => {
    retained = ctx.run;
    for (const value of ['', ' ', 'x'.repeat(121), 'bad\u0000title'])
      expect(() => ctx.run!.setTitle(value)).toThrow('plain-text');
    ctx.run!.setTitle('<b>Plain text</b>');
    expect(ctx.run!.title).toBe('<b>Plain text</b>');
    return null;
  });
  const result = await runWorkflow(root, {
    input: null,
    run: { id: 'test' },
    onEvent: (event) => {
      if (event.type === 'run.title') titles.push(event.title);
    }
  });
  expect(result.ok).toBe(true);
  expect(titles).toEqual(['<b>Plain text</b>']);
  expect(() => retained!.setTitle('Too late')).toThrow('finished');
  expect(titles).toHaveLength(1);
});
