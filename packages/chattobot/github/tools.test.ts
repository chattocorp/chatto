import { generateKeyPairSync } from 'node:crypto';
import { expect, test, vi } from 'vitest';
import type { AgentExtensionAPI } from 'runling';
import type { GitHubPermissions } from './app.ts';
import type { GhRunner } from './gh.ts';
import { githubExtension } from './tools.ts';

type Tool = { execute(id: string, input: unknown, signal?: AbortSignal): Promise<any> };

const settings = {
  clientId: 'Iv23liExample1',
  privateKey: generateKeyPairSync('rsa', { modulusLength: 2048 }).privateKey,
  repository: 'chattocorp/chatto'
};

async function tools({ maintainer = true, output = '' } = {}) {
  const commands: { args: readonly string[]; token: string; repository: string }[] = [];
  const permissions: (GitHubPermissions | undefined)[] = [];
  const run = vi.fn<GhRunner>(async (args, { token, repository }) => {
    commands.push({ args, token, repository });
    return { ok: true, output };
  });
  const registered = new Map<string, Tool>();
  const extension = githubExtension({
    settings,
    tokens: async (requested) => {
      permissions.push(requested);
      return 'token';
    },
    maintainerTurn: () => maintainer,
    run
  });
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool: (tool: { name: string }) => void registered.set(tool.name, tool as never)
  } as unknown as AgentExtensionAPI);
  return { tools: registered, commands, permissions };
}

test('gh runs read commands with a read-only token and refuses changes', async () => {
  const { tools: registered, commands, permissions } = await tools({ output: '#12 Bug' });
  const result = await registered.get('gh')!.execute('1', { args: ['issue', 'view', '12'] });
  expect(result.content[0].text).toBe('#12 Bug');
  expect(commands[0]).toMatchObject({
    args: ['issue', 'view', '12'],
    repository: 'chattocorp/chatto'
  });
  expect(permissions[0]).toMatchObject({ issues: 'read', metadata: 'read' });
  await expect(
    registered.get('gh')!.execute('2', { args: ['issue', 'close', '12'] })
  ).rejects.toThrow('Only read commands are available');
  expect(commands).toHaveLength(1);
});

test('createIssue files an issue with an issue-write token and returns its URL', async () => {
  const {
    tools: registered,
    commands,
    permissions
  } = await tools({
    output: 'https://github.com/chattocorp/chatto/issues/99\n'
  });
  const result = await registered.get('createIssue')!.execute('1', {
    title: '-Crash on start',
    body: '--details',
    labels: ['bug']
  });
  expect(result.content[0].text).toBe(
    'Issue filed: https://github.com/chattocorp/chatto/issues/99'
  );
  expect(commands[0]!.args).toEqual([
    'issue',
    'create',
    '--title=-Crash on start',
    '--body=--details',
    '--label=bug'
  ]);
  expect(permissions[0]).toEqual({ issues: 'write', metadata: 'read' });
});

test('commentOnIssue comments with an issue-write token', async () => {
  const { tools: registered, commands } = await tools({
    output: 'https://github.com/chattocorp/chatto/issues/7#issuecomment-1'
  });
  const result = await registered.get('commentOnIssue')!.execute('1', { number: 7, body: 'Fixed' });
  expect(result.content[0].text).toContain('issuecomment-1');
  expect(commands[0]!.args).toEqual(['issue', 'comment', '7', '--body=Fixed']);
});

test('changes run only in a turn from maintainers', async () => {
  const { tools: registered, commands } = await tools({ maintainer: false });
  await expect(
    registered.get('createIssue')!.execute('1', { title: 'T', body: 'B' })
  ).rejects.toThrow('Only maintainers can ask you to change GitHub');
  await expect(
    registered.get('commentOnIssue')!.execute('1', { number: 1, body: 'B' })
  ).rejects.toThrow('Only maintainers');
  expect(commands).toEqual([]);
});
