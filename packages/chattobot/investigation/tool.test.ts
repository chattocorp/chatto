import { expect, test, vi } from 'vitest';
import { createTaskContext, Type, task, type AgentExtensionAPI } from 'runling';
import {
  investigateExtension,
  MAX_RUNNING,
  type Investigation,
  type InvestigationOutcome
} from './tool.ts';

type Tool = { execute(id: string, input: unknown, signal?: AbortSignal): Promise<any> };

/** The tool over a fake investigator that emits two updates, then waits for `release`. */
async function tool({ repository, fail }: { repository?: string; fail?: boolean } = {}) {
  const inputs: Record<string, unknown>[] = [];
  const scopes: string[] = [];
  const release = Promise.withResolvers<void>();
  const investigate = task(
    {
      name: 'Investigate',
      input: Type.Any(),
      output: Type.Any(),
      emits: Type.String()
    },
    async (ctx, input) => {
      inputs.push(input);
      await ctx.emit('Reading src/a.ts');
      await ctx.emit('Reading src/b.ts');
      await release.promise;
      if (fail) throw new Error('Provider error');
      return {
        session: input.session ?? input.id,
        answer: 'See `src/a.ts:2`.',
        commit: 'abcdef0123456789',
        citations: [{ path: 'src/a.ts', start: 2, end: 2 }]
      };
    }
  ) as unknown as ReturnType<Investigation['investigator']>;
  const running = new Set<string>();
  const progress: [string, string[]][] = [];
  const finished: [string, InvestigationOutcome][] = [];
  let budget = 2;
  const extension = investigateExtension(
    createTaskContext(),
    {
      directory: '/repo',
      investigator: (scope) => {
        scopes.push(scope);
        return investigate;
      },
      ...(repository ? { repository } : {})
    },
    {
      take: () => budget-- > 0,
      scope: 'thread-1',
      running: () => running.size,
      isRunning: (id) => running.has(id),
      started: (id) => void running.add(id),
      progress: (id, updates) => void progress.push([id, updates]),
      finished: (id, outcome) => {
        running.delete(id);
        finished.push([id, outcome]);
      }
    }
  );
  const tools = new Map<string, Tool>();
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool: (registered: { name: string }) =>
      void tools.set(registered.name, registered as never)
  } as unknown as AgentExtensionAPI);
  return {
    investigate: tools.get('investigate')!,
    inputs,
    scopes,
    progress,
    finished,
    running,
    release: release.resolve
  };
}

test('starts an investigation in the background and returns at once', async () => {
  const bot = await tool({ repository: 'org/repo' });
  const result = await bot.investigate.execute('1', { question: 'Where?' });
  const [id] = bot.running;
  expect(result.content[0].text).toContain(`Investigation ${id} started in the background`);
  expect(bot.scopes).toEqual(['thread-1']);
  await vi.waitFor(() =>
    expect(bot.inputs).toEqual([{ directory: '/repo', question: 'Where?', id }])
  );
  expect(bot.finished).toEqual([]);

  bot.release();
  await vi.waitFor(() => expect(bot.finished).toHaveLength(1));
  expect(bot.progress).toEqual([[id, ['Reading src/a.ts', 'Reading src/b.ts']]]);
  expect(bot.finished[0]).toEqual([
    id,
    {
      answer:
        '(Code at commit abcdef012.)\n\nSee [`src/a.ts:2`](https://github.com/org/repo/blob/abcdef0123456789/src/a.ts#L2).'
    }
  ]);
});

test('resumes a finished investigation, but not a running one', async () => {
  const bot = await tool();
  await bot.investigate.execute('1', { question: 'And?', resume: 'earlier' });
  await vi.waitFor(() => expect(bot.inputs[0]).toMatchObject({ session: 'earlier' }));
  await expect(
    bot.investigate.execute('2', { question: 'Again?', resume: 'earlier' })
  ).rejects.toThrow('Investigation earlier is still running');
  bot.release();
  await vi.waitFor(() => expect(bot.finished).toHaveLength(1));
});

test('reports a failed investigation and keeps the limits', async () => {
  const bot = await tool({ fail: true });
  await bot.investigate.execute('1', { question: 'One' });
  await bot.investigate.execute('2', { question: 'Two' });
  await expect(bot.investigate.execute('3', { question: 'Three' })).rejects.toThrow(
    'limit of investigations for this turn'
  );
  bot.release();
  await vi.waitFor(() => expect(bot.finished).toHaveLength(2));
  expect(bot.finished.map(([, outcome]) => outcome)).toEqual([
    { error: 'Provider error' },
    { error: 'Provider error' }
  ]);
});

test(`refuses more than ${MAX_RUNNING} running investigations`, async () => {
  const bot = await tool();
  for (const id of ['a', 'b', 'c']) bot.running.add(id);
  await expect(bot.investigate.execute('1', { question: 'Four' })).rejects.toThrow(
    `${MAX_RUNNING} investigations are running`
  );
});
