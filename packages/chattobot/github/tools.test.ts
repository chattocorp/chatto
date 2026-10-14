import { generateKeyPairSync } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import type { GitHubPermissions } from './app.ts';
import type { GhRunner } from './gh.ts';
import { githubTools } from './tools.ts';

const settings = {
  clientId: 'Iv23liExample1',
  privateKey: generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey,
  repository: 'chattocorp/chatto'
};

function tools(output = '') {
  const commands: { args: readonly string[]; token: string; repository: string }[] = [];
  const permissions: (GitHubPermissions | undefined)[] = [];
  const run = vi.fn<GhRunner>(async (args, { token, repository }) => {
    commands.push({ args, token, repository });
    return { ok: true, output };
  });
  const { read, writes } = githubTools({
    settings,
    tokens: async (requested) => {
      permissions.push(requested);
      return 'token';
    },
    run
  });
  const [createIssue, commentOnIssue] = writes;
  const call = (tool: typeof read, input: object) =>
    tool.execute('1', input as never, undefined, undefined, {} as never) as Promise<{
      content: { text: string }[];
    }>;
  return {
    read,
    createIssue: createIssue!,
    commentOnIssue: commentOnIssue!,
    call,
    commands,
    permissions
  };
}

test('gh runs read commands with a read-only token and refuses changes', async () => {
  const github = tools('#12 Bug');
  const result = await github.call(github.read, { args: ['issue', 'view', '12'] });
  expect(result.content[0]!.text).toBe('#12 Bug');
  expect(github.commands[0]).toMatchObject({
    args: ['issue', 'view', '12'],
    repository: 'chattocorp/chatto'
  });
  expect(github.permissions[0]).toMatchObject({ issues: 'read', metadata: 'read' });
  await expect(github.call(github.read, { args: ['issue', 'close', '12'] })).rejects.toThrow(
    'Only read commands are available'
  );
  expect(github.commands).toHaveLength(1);
});

test('createIssue describes the exact issue and files it with an issue-write token', async () => {
  const github = tools('https://github.com/chattocorp/chatto/issues/99\n');
  const input = { title: '-Crash on start', body: 'Steps\n--details', labels: ['bug'] };
  expect(github.createIssue.describe(input)).toBe(
    '**Approval needed:** file this issue in chattocorp/chatto\n\n**-Crash on start** (labels: bug)\n\n> Steps\n> --details'
  );
  const result = await github.call(github.createIssue, input);
  expect(result.content[0]!.text).toBe(
    'Issue filed: https://github.com/chattocorp/chatto/issues/99'
  );
  expect(github.commands[0]!.args).toEqual([
    'issue',
    'create',
    '--title=-Crash on start',
    '--body=Steps\n--details',
    '--label=bug'
  ]);
  expect(github.permissions[0]).toEqual({ issues: 'write', metadata: 'read' });
});

test('commentOnIssue describes the exact comment and adds it', async () => {
  const github = tools('https://github.com/chattocorp/chatto/issues/7#issuecomment-1');
  expect(github.commentOnIssue.describe({ number: 7, body: 'Fixed' })).toBe(
    '**Approval needed:** comment on chattocorp/chatto#7\n\n> Fixed'
  );
  const result = await github.call(github.commentOnIssue, { number: 7, body: 'Fixed' });
  expect(result.content[0]!.text).toContain('issuecomment-1');
  expect(github.commands[0]!.args).toEqual(['issue', 'comment', '7', '--body=Fixed']);
});
