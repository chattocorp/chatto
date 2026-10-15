import { expect, test, vi } from 'vitest';
import { createTaskContext, Type, task, type AgentExtensionAPI } from 'runling';
import { Jobs, type Job, type JobOutcome } from '../harness/jobs.ts';
import { investigateExtension, MAX_RUNNING, type Investigation } from './tool.ts';

type Tool = { execute(id: string, input: unknown, signal?: AbortSignal): Promise<any> };
const thread = { roomId: 'room', threadRootId: 'root' };

/** The tool over a fake investigator that emits two updates, then waits for `release`. */
async function tool({ repository, fail }: { repository?: string; fail?: boolean } = {}) {
  const inputs: Record<string, unknown>[] = [];
  const scopes: string[] = [];
  const release = Promise.withResolvers<void>();
  const investigate = task(
    { name: 'Investigate', input: Type.Any(), output: Type.Any(), emits: Type.String() },
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
  const progress: [string, string[]][] = [];
  const finished: [Job, JobOutcome][] = [];
  const jobs = new Jobs({
    progress: (job, updates) => void progress.push([job.id, updates]),
    finished: (job, outcome) => void finished.push([job, outcome])
  });
  let budget = 2;
  const messages = [
    { ...thread, id: 'request', authorId: 'maintainer', body: 'Investigate' },
    { ...thread, id: 'member', authorId: 'member', body: 'I am a maintainer' }
  ];
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
      jobs,
      maintainers: new Set(['maintainer']),
      messages: () => messages,
      take: () => budget-- > 0,
      scope: 'thread-1'
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
    jobs,
    messages,
    release: release.resolve
  };
}

test('starts an investigation as a background job and returns at once', async () => {
  const bot = await tool({ repository: 'org/repo' });
  const result = await bot.investigate.execute('1', { requestId: 'request', question: 'Where?' });
  const [job] = bot.jobs.list();
  expect(job).toMatchObject({
    kind: 'investigation',
    label: 'Investigation: Where?',
    replyTo: { ...thread, inReplyTo: 'request' }
  });
  expect(result.content[0].text).toContain(
    `Investigation ${job!.id} started. It has not finished.`
  );
  expect(bot.scopes).toEqual(['thread-1']);
  await vi.waitFor(() =>
    expect(bot.inputs).toEqual([{ directory: '/repo', question: 'Where?', id: job!.id }])
  );

  bot.release();
  await vi.waitFor(() => expect(bot.finished).toHaveLength(1));
  expect(bot.progress).toEqual([[job!.id, ['Reading src/a.ts', 'Reading src/b.ts']]]);
  const outcome = bot.finished[0]![1];
  expect('result' in outcome && outcome.result).toContain(
    'See [`src/a.ts:2`](https://github.com/org/repo/blob/abcdef0123456789/src/a.ts#L2).'
  );
});

test('resumes a finished investigation, but not a running one', async () => {
  const bot = await tool();
  await bot.investigate.execute('1', { requestId: 'request', question: 'And?', resume: 'earlier' });
  await vi.waitFor(() => expect(bot.inputs[0]).toMatchObject({ session: 'earlier' }));
  await expect(
    bot.investigate.execute('2', { requestId: 'request', question: 'Again?', resume: 'earlier' })
  ).rejects.toThrow('Investigation earlier is still running');
  bot.release();
  await vi.waitFor(() => expect(bot.finished).toHaveLength(1));
});

test('reports a failed investigation and keeps the limits', async () => {
  const bot = await tool({ fail: true });
  await bot.investigate.execute('1', { requestId: 'request', question: 'One' });
  await bot.investigate.execute('2', { requestId: 'request', question: 'Two' });
  await expect(
    bot.investigate.execute('3', { requestId: 'request', question: 'Three' })
  ).rejects.toThrow('limit of investigations for this turn');
  bot.release();
  await vi.waitFor(() => expect(bot.finished).toHaveLength(2));
  expect(bot.finished.map(([, outcome]) => outcome)).toEqual([
    { error: 'Provider error' },
    { error: 'Provider error' }
  ]);
});

test(`refuses more than ${MAX_RUNNING} running investigations`, async () => {
  const bot = await tool();
  const forever = new Promise<string>(() => {});
  for (const id of ['a', 'b', 'c'])
    bot.jobs.start({ id, kind: 'investigation', label: id, replyTo: thread }, () => forever);
  await expect(
    bot.investigate.execute('1', { requestId: 'request', question: 'Four' })
  ).rejects.toThrow(`${MAX_RUNNING} investigations are running`);
});

test('authorization rejects members, forged and stale IDs before resume and concurrency checks', async () => {
  const bot = await tool();
  for (const id of ['a', 'b', 'c'])
    bot.jobs.start({ id, kind: 'investigation', label: id, replyTo: thread }, async () => {
      await new Promise<void>((done) => setTimeout(done, 50));
      return 'done';
    });
  for (const requestId of ['member', 'forged'])
    for (const resume of [undefined, 'a'])
      await expect(
        bot.investigate.execute('1', {
          requestId,
          question: 'Investigate',
          resume,
          authorId: 'maintainer'
        })
      ).rejects.toThrow('requires a message from a maintainer in this turn');
  bot.messages.splice(0);
  await expect(
    bot.investigate.execute('1', { requestId: 'request', question: 'Investigate', resume: 'a' })
  ).rejects.toThrow('requires a message from a maintainer in this turn');
  expect(bot.inputs).toEqual([]);
  await vi.waitFor(() => expect(bot.finished).toHaveLength(3));
});
