import { createChannel } from './channel.ts';
import { createTaskContext, type TaskContext } from './context.ts';
import { stripVTControlCharacters } from 'node:util';
import { afterEach, describe, expect, expectTypeOf, test, vi } from 'vitest';
import { formatDuration, formatWorkflowOutput, runRunling, runWorkflow } from './runner.ts';
import type { RunOptions } from './cli.ts';
import { observeRunlingEvents, type RunlingEvent } from './events.ts';
import { Type } from 'typebox';
import { task } from './task.ts';
import { log } from './log.ts';
import type { RunRecord } from './run-journal.ts';
import { mkdtemp, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const initialExitCode = process.exitCode;

afterEach(() => {
  process.exitCode = initialExitCode ?? 0;
});

const runlingModule = fileURLToPath(new URL('./index.ts', import.meta.url));

/**
 * Run a workflow file with `runling run` in a temporary working directory. The file imports
 * `task`, `Type`, and `log` from this runtime, so it shares the test's event runtime.
 */
async function runCli(
  source: string,
  { prompt = '', ...options }: Partial<RunOptions> & { prompt?: string; path?: string } = {}
) {
  const directory = await mkdtemp(join(tmpdir(), 'runling-run-'));
  const path = options.path ?? join(directory, 'workflow.ts');
  await writeFile(
    join(directory, 'workflow.ts'),
    `import { task, Type, log, agent } from ${JSON.stringify(runlingModule)};\n${source}\n`
  );
  const stdout: string[] = [];
  const stderr: string[] = [];
  const spies = [
    vi.spyOn(process, 'cwd').mockReturnValue(directory),
    vi
      .spyOn(console, 'log')
      .mockImplementation((message: string) => stdout.push(stripVTControlCharacters(message))),
    vi
      .spyOn(console, 'error')
      .mockImplementation((message: string) => stderr.push(stripVTControlCharacters(message)))
  ];
  try {
    await runRunling(path, prompt, { json: false, verbose: false, ...options });
    const runs = join(directory, '.runling/runs');
    const [journal] = await readdir(runs).catch(() => []);
    const records: RunRecord[] = journal
      ? (await readFile(join(runs, journal), 'utf8'))
          .trim()
          .split('\n')
          .map((line) => JSON.parse(line))
      : [];
    return { stdout, stderr, records };
  } finally {
    for (const spy of spies) spy.mockRestore();
    await rm(directory, { recursive: true, force: true });
  }
}

const done = (body: string) =>
  `export default task({ name: 'Demo', input: Type.String(), result: Type.Unknown() }, async (ctx, input) => {\n${body}\n});`;

describe('runRunling', () => {
  test('loads an agent task directly as the default export', async () => {
    const { stdout, records } = await runCli(
      `
      export default agent({ name: 'Direct agent', model: 'test/model', cwd: '.' }, async () => ({
        id: 'fake',
        async runOutcome(ctx, prompt, options) {
          options.onText('Working');
          return { outcome: 'completed', summary: 'Answer: ' + prompt, usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } };
        },
        async steer() { return false; },
        async [Symbol.asyncDispose]() {}
      }));
    `,
      { prompt: 'Hello', json: true }
    );
    expect(JSON.parse(stdout.join(''))).toMatchObject({ ok: true, output: 'Answer: Hello' });
    expect(
      records.filter((record) => record.type === 'event' && record.event.type === 'step.started')
    ).toHaveLength(1);
  });

  test('shows run details and prints the root output last without indentation', async () => {
    const { stdout, stderr, records } = await runCli(
      done(`log.info('inside the workflow'); return 'Made the change';`)
    );

    expect(stdout.at(-1)).toBe('Made the change');
    expect(stdout.at(-2)).toBe('');
    expect(stdout.join('\n')).toContain('Runling starting run');
    expect(stdout.join('\n')).toContain('inside the workflow');
    expect(stdout.join('\n')).toContain('Demo');
    expect(stdout.join('\n')).toContain('Finished in');
    expect(stdout.join('\n')).toContain('journal: .runling/runs/');
    expect(stderr).toEqual([]);
    expect(records).toContainEqual(
      expect.objectContaining({
        type: 'event',
        event: expect.objectContaining({ type: 'log', message: 'inside the workflow' })
      })
    );
    expect(process.exitCode ?? 0).toBe(0);
  });

  test('prints structured output as JSON', async () => {
    const { stdout } = await runCli(done(`return { branchName: 'fix', count: 2 };`));
    expect(stdout.at(-1)).toBe(JSON.stringify({ branchName: 'fix', count: 2 }, null, 2));
  });

  test.each([false, true])(
    'only prints the root result with verbose=%s, including nested tasks',
    async (verbose) => {
      const { stdout, stderr, records } = await runCli(
        `
      const child = task({ name: 'Child', input: Type.String(), result: Type.String() }, async (ctx) => {
        log.info('Child progress');
        log.debug('Child debug');
        await ctx.emit('Child update');
        return 'Child answer';
      });
      export default task({ name: 'Root', input: Type.String(), result: Type.String() }, async (ctx) => {
        const answer = await child(ctx, 'Start');
        await ctx.emit(answer);
        return 'Root answer';
      });
    `,
        { verbose }
      );
      expect(stdout.at(-1)).toBe('Root answer');
      expect(stdout.join('\n')).toContain('Child progress');
      expect(stdout.join('\n').includes('Child debug')).toBe(verbose);
      expect(stdout.join('\n')).not.toContain('Child answer');
      expect(stdout.join('\n')).not.toContain('Child update');
      expect(stderr).toEqual([]);
      expect(records).toContainEqual(
        expect.objectContaining({
          type: 'event',
          event: expect.objectContaining({ type: 'log', message: 'Child progress' })
        })
      );
      const debug = records.some(
        (record) =>
          record.type === 'event' &&
          record.event.type === 'log' &&
          record.event.message === 'Child debug'
      );
      expect(debug).toBe(verbose);
    }
  );

  test('prints an emitted answer once while retaining the final journal output', async () => {
    const { stdout, records } = await runCli(
      done(`await ctx.emit('Answer once'); return 'Answer once';`)
    );
    expect(stdout.filter((line) => line.includes('Answer once'))).toEqual(['Answer once']);
    expect(records.at(-1)).toMatchObject({ type: 'finished', output: 'Answer once' });
  });

  test('prints a final answer when it differs from the last update', async () => {
    const { stdout } = await runCli(done(`await ctx.emit('Progress'); return 'Final answer';`));
    expect(stdout.at(-1)).toBe('Final answer');
    expect(stdout.join('\n')).not.toContain('Progress');
  });

  test('ignored empty updates do not make an answer print twice', async () => {
    const { stdout } = await runCli(
      done(
        `await ctx.emit('Answer once'); await ctx.emit('  '); await ctx.emit(undefined); return 'Answer once';`
      )
    );
    expect(stdout.filter((line) => line.includes('Answer once'))).toHaveLength(1);
  });

  test('JSON output retains the answer even when it was emitted as an update', async () => {
    const { stdout, stderr } = await runCli(
      done(`await ctx.emit('Answer once'); return 'Answer once';`),
      { json: true }
    );
    expect(stdout).toHaveLength(1);
    expect(JSON.parse(stdout[0]!)).toMatchObject({ ok: true, output: 'Answer once' });
    expect(stderr.join('\n')).toContain('Finished in');
    expect(stderr.join('\n')).not.toContain('Answer once');
  });

  test('passes the prompt or JSON input to the task', async () => {
    const workflow = `export default task({ name: 'Echo', input: Type.Unknown(), result: Type.Unknown() }, (ctx, input) => ({ input }));`;
    const prompt = await runCli(workflow, { prompt: 'hello', json: true });
    expect(JSON.parse(prompt.stdout[0]!).output).toEqual({ input: 'hello' });
    const structured = await runCli(workflow, { input: '{"a":1}', json: true });
    expect(JSON.parse(structured.stdout[0]!).output).toEqual({ input: { a: 1 } });
  });

  test('records root updates without printing them', async () => {
    const { stdout, stderr, records } = await runCli(
      done(`
        await ctx.emit('Plain progress');
        await ctx.emit('  ');
        await ctx.emit({ type: 'finding', text: 'Validation passed' });
        await ctx.emit('First line\\nSecond line');
        return 'done';`)
    );
    expect(stdout.at(-1)).toBe('done');
    expect(stdout.join('\n')).not.toMatch(
      /Plain progress|Validation passed|First line|Second line/
    );
    expect(stderr).toEqual([]);
    const updates = records.flatMap((record) =>
      record.type === 'event' && record.event.type === 'message.sent' ? [record.event.payload] : []
    );
    expect(updates).toEqual([
      'Plain progress',
      '  ',
      '{"type":"finding","text":"Validation passed"}',
      'First line\nSecond line'
    ]);
  });

  test('writes only the execution document to stdout in JSON mode', async () => {
    const markdown = '## Details\n\n**Still raw.**';
    const { stdout, stderr } = await runCli(done(`return ${JSON.stringify(markdown)};`), {
      json: true
    });
    expect(stdout).toHaveLength(1);
    expect(JSON.parse(stdout[0]!)).toEqual({
      ok: true,
      error: null,
      output: markdown,
      durationMs: expect.any(Number),
      usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }
    });
    expect(stderr.join('\n')).toContain('Runling starting run');
    expect(stderr.join('\n')).toContain('Finished in');
    expect(stderr.join('\n')).not.toContain('Still raw');
  });

  test('logs failures and applies a nonzero exit code', async () => {
    const { stdout, stderr } = await runCli(done(`throw 'Tests failed';`));
    expect(stderr.some((line) => line.includes('Tests failed'))).toBe(true);
    expect(stdout.join('\n')).toContain('Finished in');
    expect(stdout.at(-1)).toContain('journal:');
    expect(process.exitCode).toBe(1);
  });

  test.each([
    {
      name: 'a missing workflow file',
      options: { path: 'missing-workflow.ts' },
      error: 'missing-workflow.ts'
    },
    { name: 'malformed JSON input', options: { input: '{' }, error: 'JSON' }
  ])('reports $name as a failed execution document in JSON mode', async ({ options, error }) => {
    const { stdout, stderr } = await runCli(done(`return 'unused';`), { json: true, ...options });
    expect(stdout).toHaveLength(1);
    expect(JSON.parse(stdout[0]!)).toMatchObject({
      ok: false,
      output: null,
      error: expect.stringContaining(error)
    });
    expect(stderr.some((line) => line.includes(error))).toBe(true);
    expect(process.exitCode).toBe(1);
  });

  test('prints and records accumulated token usage', async () => {
    const used = await runCli(
      done(`
        ctx.recordUsage({ input: 100, output: 20, cacheRead: 500, cacheWrite: 10 });
        ctx.recordUsage({ input: 50, output: 25, cacheRead: 550, cacheWrite: 15 });
        return 'done';`)
    );
    expect(used.stdout.at(-1)).toBe('done');
    expect(used.stdout.join('\n')).toContain('Total token usage:');
    expect(used.records.at(-1)).toMatchObject({
      type: 'finished',
      usage: { input: 150, output: 45, cacheRead: 1050, cacheWrite: 25 }
    });
  });

  test('records the start, every event, and the output in the run journal', async () => {
    const { stdout, records } = await runCli(
      done(`await ctx.emit('Opened the pull request'); return 'done';`),
      { input: JSON.stringify('Fix') }
    );
    const started = records[0];
    expect(started).toMatchObject({
      type: 'started',
      run: { source: 'cli', pid: process.pid, input: 'Fix', status: 'running' }
    });
    expect(records).toContainEqual(
      expect.objectContaining({
        type: 'event',
        event: expect.objectContaining({ type: 'message.sent', payload: 'Opened the pull request' })
      })
    );
    expect(records.at(-1)).toMatchObject({ type: 'finished', status: 'completed', output: 'done' });
    expect(stdout.at(-1)).toBe('done');
    expect(stdout.join('\n')).toContain('journal: .runling/runs/');
  });
});

describe('runWorkflow', () => {
  test("preserves the task's output type", async () => {
    const done = task(
      { name: 'Done', input: Type.String(), result: Type.Literal('done') },
      () => 'done' as const
    );
    const execution = await runWorkflow(done, { input: 'input' });

    expectTypeOf(execution.output).toEqualTypeOf<'done' | null>();
    expect(execution).toMatchObject({ ok: true, error: null, output: 'done' });
  });

  test('does not use the removed prompt option as workflow input', async () => {
    let started = false;
    const echo = task(
      { name: 'Echo', input: Type.String(), result: Type.String() },
      (ctx, input) => {
        started = true;
        return input;
      }
    );
    // @ts-expect-error Callers must provide input, not a legacy prompt option.
    const execution = await runWorkflow(echo, { prompt: 'Legacy fallback' });
    expect(execution.ok).toBe(false);
    expect(execution.error).toContain('Task "Echo" input is invalid');
    expect(started).toBe(false);
    expect(await runWorkflow(echo, { input: '' })).toMatchObject({ ok: true, output: '' });
  });

  test('captures failures without changing the process exit code', async () => {
    const exitCode = process.exitCode;
    const failing = task(
      { name: 'Fail', input: Type.String(), result: Type.String() },
      async () => {
        throw new Error('Nope');
      }
    );
    const execution = await runWorkflow(failing, { input: '' });

    expect(execution).toMatchObject({ ok: false, error: 'Nope', output: null });
    expect(process.exitCode).toBe(exitCode);
  });

  test('exchanges messages between the host and the top-level task', async () => {
    const chat = task(
      {
        name: 'Chat',
        input: Type.Null(),
        result: Type.Number(),
        steering: Type.String(),
        output: Type.String()
      },
      async (ctx) => {
        let count = 0;
        for await (const message of ctx.steering) {
          count++;
          await ctx.emit(`Reply to ${message}`);
        }
        return count;
      }
    );
    const messages = createChannel<unknown>();
    const updates: unknown[] = [];
    const events: RunlingEvent[] = [];
    const running = runWorkflow(chat, {
      input: null,
      messages,
      onUpdate: async (update) => {
        updates.push(update);
        if (updates.length === 1) {
          // The host reacts to an update while the task runs. An invalid message is dropped.
          await messages.send(42);
          await messages.send('second');
          messages.close();
        }
      },
      onEvent: (event) => events.push(event)
    });
    await messages.send('first');
    const execution = await running;

    expect(execution).toMatchObject({ ok: true, output: 2 });
    expect(updates).toEqual(['Reply to first', 'Reply to second']);
    expect(events).toContainEqual(
      expect.objectContaining({
        type: 'log',
        level: 'error',
        message: expect.stringContaining('Message not delivered: Task "Chat" steering is invalid')
      })
    );
  });

  test('closes the input of the top-level task without host messages', async () => {
    const drain = task(
      { name: 'Drain', input: Type.Null(), result: Type.Number() },
      async (ctx) => {
        let count = 0;
        for await (const _ of ctx.steering) count++;
        // Nobody reads these updates, so they must not fill a buffer.
        for (let i = 0; i < 100; i++) await ctx.emit(i);
        return count;
      }
    );
    expect(await runWorkflow(drain, { input: null })).toMatchObject({ ok: true, output: 0 });
  });

  test('fails the run when the update handler fails', async () => {
    const emit = task({ name: 'Emit', input: Type.Null(), result: Type.Null() }, async (ctx) => {
      await ctx.emit('update');
      return null;
    });
    const execution = await runWorkflow(emit, {
      input: null,
      onUpdate: () => {
        throw new Error('delivery failed');
      }
    });
    expect(execution).toMatchObject({ ok: false, error: 'delivery failed' });
  });
});

describe('formatWorkflowOutput', () => {
  test('renders text as Markdown for an interactive terminal', () => {
    vi.stubEnv('NO_COLOR', undefined);
    vi.stubEnv('FORCE_COLOR', undefined);
    const formatted = formatWorkflowOutput('## Findings\n\n**Important**', {
      isTTY: true,
      columns: 80
    });

    expect(formatted).toContain('\x1b[');
    expect(stripVTControlCharacters(formatted)).not.toContain('##');
    expect(stripVTControlCharacters(formatted)).not.toContain('**');
    vi.unstubAllEnvs();
  });

  test.each(['NO_COLOR', 'FORCE_COLOR'])('respects %s in the final result', (flag) => {
    vi.stubEnv(flag, flag === 'NO_COLOR' ? '1' : '0');
    try {
      expect(formatWorkflowOutput('**Answer**', { isTTY: true })).toBe('Answer');
    } finally {
      vi.unstubAllEnvs();
    }
  });

  test('preserves Markdown for redirected output', () => {
    expect(formatWorkflowOutput('## Findings\n\n**Important**', { isTTY: false })).toBe(
      '## Findings\n\n**Important**'
    );
  });

  test('formats other values as JSON', () => {
    expect(formatWorkflowOutput({ count: 2 }, { isTTY: true })).toBe('{\n  "count": 2\n}');
    expect(formatWorkflowOutput(null, { isTTY: false })).toBe('null');
  });
});

describe('formatDuration', () => {
  test('formats sub-second durations in milliseconds', () => {
    expect(formatDuration(0)).toBe('0ms');
    expect(formatDuration(42.4)).toBe('42ms');
    expect(formatDuration(999)).toBe('999ms');
  });

  test('formats sub-minute durations in seconds', () => {
    expect(formatDuration(1000)).toBe('1.0s');
    expect(formatDuration(12_345)).toBe('12.3s');
  });

  test('formats durations in minutes and seconds', () => {
    expect(formatDuration(59_949)).toBe('59.9s');
    expect(formatDuration(59_950)).toBe('1m');
    expect(formatDuration(60_000)).toBe('1m');
    expect(formatDuration(61_000)).toBe('1m1s');
    expect(formatDuration(125_000)).toBe('2m5s');
  });

  test('formats durations in hours, minutes, and seconds', () => {
    expect(formatDuration(3_599_600)).toBe('1h');
    expect(formatDuration(3_600_000)).toBe('1h');
    expect(formatDuration(3_660_000)).toBe('1h1m');
    expect(formatDuration(3_661_000)).toBe('1h1m1s');
    expect(formatDuration(7_385_000)).toBe('2h3m5s');
  });
});

test('isolates workflow contexts and usage events across concurrent executions', async () => {
  const seen: unknown[] = [];
  const events: RunlingEvent[][] = [[], []];
  const run = task(
    { name: 'Count', input: Type.Number(), result: Type.Number() },
    async (ctx, count) => {
      seen.push(ctx);
      await Promise.all(
        [1, 2].map(async () => {
          await Promise.resolve();
          ctx.recordUsage({ input: count, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.5 });
        })
      );
      return count;
    }
  );
  const [first, second] = await Promise.all([
    runWorkflow(run, { input: 10, onEvent: (event) => events[0]!.push(event) }),
    runWorkflow(run, { input: 100, onEvent: (event) => events[1]!.push(event) })
  ]);
  expect(seen[0]).not.toBe(seen[1]);
  expect(first.usage.input).toBe(20);
  expect(second.usage.input).toBe(200);
  expect(first.usage.cost).toBe(1);
  expect(
    events.map((list) => list.filter((e) => e.type === 'usage.updated').map((e) => e.usage.input))
  ).toEqual([
    [10, 20],
    [100, 200]
  ]);
});

const Nothing = { input: Type.Null(), result: Type.Unknown() };

test('a new host run does not inherit the dispatching run event listeners or activity', async () => {
  const outer: RunlingEvent[] = [];
  const inner: RunlingEvent[] = [];
  const release = Promise.withResolvers<void>();
  let continuation: ReturnType<typeof runWorkflow> | undefined;
  const first = task({ name: 'Intake', ...Nothing }, async () => {
    continuation = runWorkflow(
      task({ name: 'Conversation', ...Nothing }, async (ctx) => {
        await release.promise;
        log.info('Conversation continues');
        ctx.recordUsage({ input: 7, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0 });
        return 'reply';
      }),
      { input: null, onEvent: (event) => inner.push(event) }
    );
    return 'accepted';
  });
  await runWorkflow(first, { input: null, onEvent: (event) => outer.push(event) });
  const count = outer.length;
  release.resolve();
  expect(await continuation).toMatchObject({ ok: true, output: 'reply' });
  expect(outer).toHaveLength(count);
  expect(
    outer.some((event) => event.type === 'step.started' && event.label === 'Conversation')
  ).toBe(false);
  expect(inner.find((event) => event.type === 'step.started')).toMatchObject({
    activityId: undefined
  });
  expect(
    inner.find((event) => event.type === 'log' && event.message === 'Conversation')
  ).toMatchObject({ depth: 0 });
  expect(inner.find((event) => event.type === 'usage.updated')).toMatchObject({
    activityId: undefined,
    usage: { input: 7 }
  });
});

test('keeps parent accounting independent of a failed nested workflow execution', async () => {
  const parent = task({ name: 'Parent', ...Nothing }, async (ctx) => {
    ctx.recordUsage({ input: 10, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.5 });
    const child = await runWorkflow(
      task({ name: 'Child', ...Nothing }, (inner) => {
        expect(inner.usage).not.toBe(ctx.usage);
        inner.recordUsage({ input: 100, output: 0, cacheRead: 0, cacheWrite: 0, cost: 5 });
        throw new Error('Child failed');
      }),
      { input: null }
    );
    expect(child.ok).toBe(false);
    expect(child.usage.input).toBe(100);
    expect(ctx.usage.input).toBe(10);
    throw new Error('Parent failed');
  });
  const result = await runWorkflow(parent, { input: null });
  expect(result.error).toBe('Parent failed');
  expect(result.usage.input).toBe(10);
});

test('emits context usage updates from callbacks created outside the run', async () => {
  const { AsyncResource } = await import('node:async_hooks');
  const resource = new AsyncResource('external-usage');
  const events: RunlingEvent[] = [];
  try {
    const execution = await runWorkflow(
      task({ name: 'Usage', ...Nothing }, (ctx) => {
        resource.runInAsyncScope(() => {
          ctx.recordUsage({ input: 10, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
        });
        return 'done';
      }),
      { input: null, onEvent: (event) => events.push(event) }
    );
    expect(execution.usage.input).toBe(10);
    expect(
      events.filter((event) => event.type === 'usage.updated').map((event) => event.usage)
    ).toEqual([execution.usage]);
  } finally {
    resource.emitDestroy();
  }
});

test('aborts nested tasks, runs cleanup, and preserves recorded usage', async () => {
  const calls: string[] = [];
  const child = task({ name: 'Child', ...Nothing }, (ctx) => {
    ctx.recordUsage({ input: 10, output: 2, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
    ctx.abort('Budget exceeded');
  });
  const parent = task({ name: 'Parent', ...Nothing }, async (ctx) => {
    try {
      await child(ctx, null);
      calls.push('continued');
    } finally {
      calls.push('cleanup');
    }
  });
  const execution = await runWorkflow(parent, { input: null });
  expect(calls).toEqual(['cleanup']);
  expect(execution).toMatchObject({
    ok: false,
    error: 'Budget exceeded',
    output: null,
    usage: { input: 10, output: 2, cost: 0.25 }
  });
});

test('cannot turn a caught abort into a successful run', async () => {
  const run = task({ name: 'Catch', ...Nothing }, (ctx) => {
    try {
      ctx.abort('Stop');
    } catch {}
    return 'success';
  });
  const execution = await runWorkflow(run, { input: null });
  expect(execution).toMatchObject({ ok: false, error: 'Stop', output: null });
});

test('cancels cooperative parallel work without cancelling another run', async () => {
  const started = Promise.withResolvers<void>();
  let cleanedUp = false;
  const waiting = task({ name: 'Waiting', ...Nothing }, async (ctx) => {
    try {
      await new Promise<void>((resolve) => {
        ctx.signal.addEventListener('abort', () => resolve(), { once: true });
        started.resolve();
      });
    } finally {
      cleanedUp = true;
    }
  });
  const parent = task({ name: 'Parent', ...Nothing }, async (ctx) => {
    const pending = waiting(ctx, null).catch(() => {});
    await started.promise;
    try {
      ctx.abort('Stop siblings');
    } finally {
      await pending;
    }
  });
  const [aborted, successful] = await Promise.all([
    runWorkflow(parent, { input: null }),
    runWorkflow(
      task({ name: 'Other', ...Nothing }, (ctx) => {
        expect(ctx.signal.aborted).toBe(false);
        return 'done';
      }),
      { input: null }
    )
  ]);
  expect(cleanedUp).toBe(true);
  expect(aborted).toMatchObject({ ok: false, error: 'Stop siblings' });
  expect(successful).toMatchObject({ ok: true, output: 'done' });
});

test('completed executions retain usage snapshots after the context changes', async () => {
  const contexts: TaskContext[] = [];
  const result = await runWorkflow(
    task({ name: 'Usage', ...Nothing }, (ctx) => {
      contexts.push(ctx);
      ctx.recordUsage({ input: 1, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.25 });
      return 'done';
    }),
    { input: null }
  );
  contexts[0]!.recordUsage({ input: 2, output: 0, cacheRead: 0, cacheWrite: 0, cost: 0.5 });
  expect(result.usage.input).toBe(1);
  expect(result.usage.cost).toBe(0.25);
  expect(contexts[0]!.usage.input).toBe(3);
});

test('does not start a workflow when the host signal is already aborted', async () => {
  let called = false;
  const execution = await runWorkflow(
    task({ name: 'Never', ...Nothing }, () => {
      called = true;
    }),
    { input: null, signal: AbortSignal.abort(new Error('Cancelled by host')) }
  );
  expect(called).toBe(false);
  expect(execution).toMatchObject({ ok: false, error: 'Cancelled by host' });
});

test('ctx.run identifies the run in the workflow and its started tasks', async () => {
  const run = { id: 'run-id', reference: 'funky-comics-8426' };
  const child = task({ name: 'Child', ...Nothing }, (ctx) => ctx.run);
  const execution = await runWorkflow(
    task({ name: 'Root', ...Nothing }, async (ctx) => [
      ctx.run,
      await ctx.start(child, null).result,
      await child(ctx, null)
    ]),
    { input: null, run }
  );
  expect(execution.output).toEqual([
    expect.objectContaining(run),
    expect.objectContaining(run),
    expect.objectContaining(run)
  ]);
  expect(createTaskContext().run).toBeUndefined();
});

test('waits for the cooperative cleanup of a cancelled top-level task', async () => {
  const host = new AbortController();
  const started = Promise.withResolvers<void>();
  const calls: string[] = [];
  const running = runWorkflow(
    task({ name: 'Cleanup', ...Nothing }, async (ctx) => {
      try {
        started.resolve();
        await new Promise((resolve) =>
          ctx.signal.addEventListener('abort', resolve, { once: true })
        );
      } finally {
        await new Promise((resolve) => setTimeout(resolve, 20));
        calls.push('cleaned up');
      }
    }),
    { input: null, signal: host.signal }
  );
  await started.promise;
  host.abort(new Error('Stopped by host'));
  expect(await running).toMatchObject({ ok: false, error: 'Stopped by host' });
  expect(calls).toEqual(['cleaned up']);
});
