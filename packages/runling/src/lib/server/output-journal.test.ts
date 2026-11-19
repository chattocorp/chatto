/** Exercise cursor replay and subtree selection against real, unchanged JSONL journals. */
import { mkdtemp, writeFile, appendFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test } from 'vitest';
import { newRun, type RunRecord } from '../../runtime/run-journal.ts';
import type { RunlingEvent } from '../../runtime/events.ts';
import { journalOutput, readOutputPage } from './output-journal.ts';
import { OUTPUT_PAGE_BYTES } from '../log-page.ts';
import { RunStore } from './run-store.ts';

const directories: string[] = [];
afterEach(async () => {
  for (const directory of directories.splice(0))
    await rm(directory, { recursive: true, force: true });
});

async function fixture(events: RunlingEvent[]) {
  const directory = await mkdtemp(join(tmpdir(), 'runling-output-'));
  directories.push(directory);
  const run = newRun({ webhook: 'test', workflow: 'Tests', source: 'cli', input: {} });
  const records: RunRecord[] = [
    { type: 'started', run },
    ...events.map((event) => ({ type: 'event' as const, event })),
    {
      type: 'finished',
      status: 'completed',
      finishedAt: Date.now(),
      durationMs: 100,
      usage: run.usage,
      output: null,
      error: null
    }
  ];
  const path = join(directory, `${run.id}.jsonl`);
  await writeFile(path, records.map((record) => JSON.stringify(record)).join('\n') + '\n');
  return { directory, path, run };
}
const line = (
  message: string,
  activityId = 'go',
  sourceId = 'docker-go'
): Extract<RunlingEvent, { type: 'log' }> => ({
  type: 'log',
  level: 'info',
  depth: 2,
  color: 'blue',
  source: 'command',
  sourceId,
  activityId,
  message,
  timestamp: 10
});
const starts: RunlingEvent[] = [
  { type: 'step.started', id: 'root', label: 'Tests', timestamp: 0 },
  { type: 'step.started', id: 'go', label: 'Go', activityId: 'root', timestamp: 1 },
  { type: 'step.started', id: 'web', label: 'Web', activityId: 'root', timestamp: 2 },
  { type: 'command.started', id: 'docker-go', command: 'go test', activityId: 'go', timestamp: 3 },
  { type: 'command.started', id: 'docker-web', command: 'vitest', activityId: 'web', timestamp: 4 }
];

test('pages interleaved task output in journal order without sibling copies or deduplication', async () => {
  const { path } = await fixture([
    ...starts,
    line('go-1'),
    line('web-1', 'web', 'docker-web'),
    line('repeat'),
    line('repeat'),
    line('web-2', 'web', 'docker-web')
  ]);
  const all = await readOutputPage(path, { activity: 'docker-go' });
  expect(all.entries.map((entry) => entry.text)).toEqual(['go-1', 'repeat', 'repeat']);
  const tail = await readOutputPage(path, { activity: 'go', limit: 2 });
  expect(tail.entries.map((entry) => entry.text)).toEqual(['repeat', 'repeat']);
  expect(tail.hasEarlier).toBe(true);
  const earlier = await readOutputPage(path, {
    activity: 'go',
    before: tail.entries[0]!.cursor,
    limit: 2
  });
  expect(earlier.entries.map((entry) => entry.text)).toEqual(['Started task-2', 'go-1']);
  const later = await readOutputPage(path, {
    activity: 'go',
    after: earlier.entries.at(-1)!.cursor,
    limit: 2
  });
  expect(later.entries).toEqual(tail.entries);
  const root = await readOutputPage(path, { activity: 'root' });
  expect(root.entries.map((entry) => entry.text)).toContain('web-2');
  expect(all.entries[0]!.cursor).toBe(root.entries.find((entry) => entry.text === 'go-1')!.cursor);
  const quiet = await readOutputPage(path, { activity: 'docker-go', after: all.next });
  expect(quiet.entries).toEqual([]);
  expect(quiet.next).toBe(all.next);
});

test('bounds large tails by count and bytes while retaining complete downloadable output', async () => {
  const { path } = await fixture([
    ...starts,
    ...Array.from({ length: 3000 }, (_, index) => line(`${index} ${'x'.repeat(4000)}`))
  ]);
  const tail = await readOutputPage(path, { activity: 'go' });
  expect(tail.entries.length).toBeLessThan(200);
  expect(tail.hasEarlier).toBe(true);
  expect(tail.entries.at(-1)!.text).toMatch(/^2999 /);
  expect(
    tail.entries.reduce((size, entry) => size + Buffer.byteLength(entry.text), 0)
  ).toBeLessThanOrEqual(OUTPUT_PAGE_BYTES);
  let count = 0;
  for await (const entry of journalOutput(path, { activity: 'docker-go' }))
    if (entry.matches) count++;
  expect(count).toBe(3000);
});

test('resumes after a partial tail, preserves blank lines and splits old Unicode buffers without loss', async () => {
  const text = 'a'.repeat(4095) + '🐈'.repeat(3000);
  const { path } = await fixture([
    ...starts,
    line(''),
    {
      type: 'command.finished',
      id: 'docker-go',
      status: 'completed',
      durationMs: 10,
      output: { stdout: text, stderr: 'warning' },
      timestamp: 20
    }
  ]);
  const page = await readOutputPage(path, { activity: 'docker-go' });
  expect(page.entries[0]!.text).toBe('');
  expect(
    page.entries
      .slice(1)
      .map((entry) => entry.text + (entry.more ? '' : '\n'))
      .join('')
  ).toBe(text + '\nwarning\n');
  const pending = JSON.stringify({ type: 'event', event: line('after reconnect') });
  await appendFile(path, pending.slice(0, 20));
  expect((await readOutputPage(path, { after: page.next })).entries).toEqual([]);
  await appendFile(path, pending.slice(20) + '\n');
  const resumed = await readOutputPage(path, { activity: 'docker-go', after: page.next });
  expect(resumed.entries.map((entry) => entry.text)).toEqual(['after reconnect']);
  expect(
    (await readOutputPage(path, { activity: 'docker-go', after: resumed.next })).entries
  ).toEqual([]);
});

test('replayed console metadata stays small and log cursors survive a new store', async () => {
  const { path, directory, run } = await fixture([
    ...starts,
    ...Array.from({ length: 1000 }, (_, index) => line(`line-${index}`)),
    {
      type: 'command.finished',
      id: 'docker-go',
      status: 'completed',
      durationMs: 20,
      output: { stdout: 'old buffered output', stderr: '' },
      timestamp: 30
    }
  ]);
  const store = new RunStore(directory);
  await store.init();
  try {
    const consoleRun = (await store.console(run.id))!;
    expect(consoleRun.events.some((event) => event.type === 'log')).toBe(false);
    expect(consoleRun.events.find((event) => event.type === 'command.finished')).toMatchObject({
      output: { stdout: '', stderr: '' }
    });
    expect(JSON.stringify(consoleRun).length).toBeLessThan(3000);
    expect((await store.get(run.id))!.events.filter((event) => event.type === 'log')).toHaveLength(
      1000
    );
    const page = await store.output(run.id, { limit: 2 });
    expect(page).toEqual(await readOutputPage(path, { limit: 2 }));
  } finally {
    await store.close();
  }
});

test('keeps repeated agent invocations and execution-relative times distinct in old journals', async () => {
  const { path, run } = await fixture([
    { type: 'agent.started', agentId: 'a', model: 'test', color: 'blue', timestamp: 0 },
    { type: 'agent.action', agentId: 'a', action: 'Legacy output', timestamp: 1 },
    { type: 'agent.started', agentId: 'a', model: 'test', color: 'blue', timestamp: 2 },
    { type: 'agent.action', agentId: 'a', action: 'Already formatted', timestamp: 3 },
    { ...line('Formatted once'), source: 'agent', sourceId: 'a', timestamp: 4 }
  ]);
  const first = await readOutputPage(path, { activity: 'a:0' });
  expect(first.entries.map((entry) => entry.text)).toEqual(['Legacy output']);
  const modern = await readOutputPage(path, { activity: 'a:2' });
  expect(modern.entries.map((entry) => entry.text)).toEqual(['Formatted once']);
  await appendFile(
    path,
    JSON.stringify({ type: 'started', run: { ...run, startedAt: run.startedAt + 1000 } }) +
      '\n' +
      JSON.stringify({
        type: 'event',
        event: { type: 'agent.started', agentId: 'a', model: 'test', color: 'blue', timestamp: 0 }
      }) +
      '\n' +
      JSON.stringify({
        type: 'event',
        event: { ...line('Later attempt'), source: 'agent', sourceId: 'a', timestamp: 5 }
      }) +
      '\n'
  );
  const all = await readOutputPage(path);
  expect(all.entries.map((entry) => entry.text)).toEqual([
    'Legacy output',
    'Formatted once',
    'Later attempt'
  ]);
  expect(all.entries.at(-1)!.timestamp).toBe(1005);
  expect((await readOutputPage(path, { activity: 'a:2' })).entries).toEqual(modern.entries);
});
