import { generateKeyPairSync } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import { runWorkflow, task, Type, type RunlingEvent } from 'runling';
import { githubCommandLabel, githubTool } from './tools.ts';

const settings = {
  clientId: 'Iv23liExample1',
  privateKey: generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey,
  repository: 'chattocorp/chatto'
};

test('activity labels show commands and numeric targets without argument content', () => {
  expect(githubCommandLabel(['issue', 'edit', '123', '--body=private content'])).toBe(
    'gh issue edit #123'
  );
  expect(githubCommandLabel(['issue', 'create', '--title=private title', '--body=secret'])).toBe(
    'gh issue create'
  );
  expect(githubCommandLabel(['issue', 'list', '--search=private search'])).toBe('gh issue list');
  expect(githubCommandLabel(['api', 'repos/owner/repo/issues?secret=value'])).toBe('gh api');
  expect(githubCommandLabel(['private-program', 'secret'])).toBe('gh command');
});

test('a failed command has a failed timeline step with a safe label', async () => {
  const events: RunlingEvent[] = [];
  const tool = githubTool(
    {
      settings,
      tokens: async () => 'token',
      run: async () => ({ ok: false, output: 'Command failed' })
    },
    true
  );
  const workflow = task(
    { name: 'Check command activity', input: Type.Null(), output: Type.Null() },
    async () => {
      await tool.execute(
        '1',
        { args: ['issue', 'edit', '12', '--body=private content'] },
        undefined,
        undefined,
        {} as never
      );
      return null;
    }
  );
  expect(
    await runWorkflow(workflow, {
      input: null,
      onEvent: (event) => {
        events.push(event);
      }
    })
  ).toMatchObject({ ok: false });
  const started = events.find(
    (event) => event.type === 'step.started' && event.label === 'Executing: gh issue edit #12'
  );
  expect(started).toBeDefined();
  if (started?.type !== 'step.started') throw new Error('Missing command activity');
  expect(events).toContainEqual(
    expect.objectContaining({ type: 'step.finished', id: started.id, status: 'failed' })
  );
  expect(JSON.stringify(events)).not.toContain('private content');
});

function setup(issueWrites = false) {
  const tokens = vi.fn(async () => 'token');
  const run = vi.fn(async () => ({
    ok: true,
    output: 'https://github.com/chattocorp/chatto/issues/5'
  }));
  const tool = githubTool({ settings, tokens, run }, issueWrites);
  const call = (args: string[]) => tool.execute('1', { args }, undefined, undefined, {} as never);
  return { call, tokens, run };
}

test('the conversation can read but cannot obtain a write token', async () => {
  const gh = setup();
  await gh.call(['issue', 'view', '12']);
  expect(gh.tokens).toHaveBeenCalledWith(
    expect.objectContaining({ issues: 'read' }),
    expect.any(AbortSignal)
  );
  await expect(gh.call(['issue', 'edit', '12', '--title=Changed'])).rejects.toThrow(
    'Only read commands'
  );
  expect(gh.tokens).toHaveBeenCalledTimes(1);
  expect(gh.run).toHaveBeenCalledTimes(1);
});

test.each([
  ['issue', 'create', '--title=Crash', '--body=See https://example.com/repro'],
  ['issue', 'edit', '12', '--body=Updated reproduction'],
  ['issue', 'comment', '12', '--body=Fixed'],
  ['api', 'repos/{owner}/{repo}/issues/12', '-X', 'PATCH', '-f', 'title=Updated']
])('the worker executes %j with only issue-write permissions', async (...args) => {
  const gh = setup(true);
  const result = await gh.call(args);
  expect(result.content).toEqual([
    { type: 'text', text: 'https://github.com/chattocorp/chatto/issues/5' }
  ]);
  expect(gh.tokens).toHaveBeenCalledWith(
    { issues: 'write', metadata: 'read' },
    expect.any(AbortSignal)
  );
  expect(gh.run).toHaveBeenCalledWith(
    args,
    expect.objectContaining({ token: 'token', repository: 'chattocorp/chatto' })
  );
});

test('worker reads retain read-only credentials and forbidden commands never get credentials', async () => {
  const gh = setup(true);
  await gh.call(['issue', 'list']);
  expect(gh.tokens).toHaveBeenCalledWith(
    expect.objectContaining({ issues: 'read' }),
    expect.any(AbortSignal)
  );
  for (const args of [
    ['auth', 'token'],
    ['issue', 'create', '--body-file=/etc/passwd'],
    ['repo', 'view', 'https://evil.example/org/repo']
  ])
    await expect(gh.call(args)).rejects.toThrow();
  expect(gh.tokens).toHaveBeenCalledTimes(1);
  expect(gh.run).toHaveBeenCalledTimes(1);
});
