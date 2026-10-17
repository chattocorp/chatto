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

test.each([
  ['api', 'orgs/{owner}/issue-fields'],
  ['api', '/orgs/chattocorp/issue-fields?per_page=100', '-X', 'GET'],
  ['api', 'orgs/{owner}/issue-fields', '--method=GET', '-F', 'per_page=100']
])('field discovery requests the optional organization read permission: %j', async (...args) => {
  const gh = setup();
  await gh.call(args);
  expect(gh.tokens).toHaveBeenCalledWith(
    expect.objectContaining({ issues: 'read', issue_fields: 'read' }),
    expect.any(AbortSignal)
  );
});

test('ordinary reads do not require the optional field discovery permission', async () => {
  const gh = setup();
  await gh.call(['api', 'repos/{owner}/{repo}/issues/12/issue-field-values']);
  expect(gh.tokens).toHaveBeenCalledWith(
    expect.not.objectContaining({ issue_fields: expect.anything() }),
    expect.any(AbortSignal)
  );
});

test.each([
  ['api', 'orgs/{owner}/issue-fields', '-f', 'name=Priority'],
  [
    'api',
    'repos/{owner}/{repo}/issues/12/issue-field-values',
    '-X',
    'POST',
    '-F',
    'issue_field_values[][field_id]=7',
    '-f',
    'issue_field_values[][value]=High'
  ],
  ['api', 'repos/{owner}/{repo}/issues/12/issue-field-values/7', '-X', 'DELETE']
])('field writes still require worker credentials: %j', async (...args) => {
  const conversation = setup();
  await expect(conversation.call(args)).rejects.toThrow('Only read commands');
  expect(conversation.tokens).not.toHaveBeenCalled();
  const worker = setup(true);
  await worker.call(args);
  expect(worker.tokens).toHaveBeenCalledWith(
    { issues: 'write', metadata: 'read' },
    expect.any(AbortSignal)
  );
});
