import { execFileSync } from 'node:child_process';
import { mkdtemp, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from 'vitest';
import type { AgentExtensionAPI } from 'runling';
import { checkPath, checkRevision, gitExtension } from './git.ts';

type Tool = { execute(id: string, input: unknown): Promise<{ content: { text: string }[] }> };

test('accepts revisions and relative paths, never options or outside paths', () => {
  for (const revision of ['HEAD', 'HEAD~3', 'abc123', 'main', 'v1.0^{commit}', 'main~5..main'])
    expect(checkRevision(revision)).toBe(revision);
  for (const revision of ['--output=x', '-p', 'HEAD:secret', '', 'a b'])
    expect(() => checkRevision(revision)).toThrow('is not a valid revision');
  expect(checkPath('cli/core/a.go')).toBe('cli/core/a.go');
  for (const path of ['--contents=/etc/passwd', '/etc/passwd', '../x', 'a/../../b', 'a:b', ''])
    expect(() => checkPath(path)).toThrow('is not a relative path inside the repository');
});

test('the git tools read history in the repository', async () => {
  const root = await mkdtemp(join(tmpdir(), 'git-'));
  const git = (...args: string[]) =>
    execFileSync('git', ['-c', 'user.name=Tester', '-c', 'user.email=t@example.com', ...args], {
      cwd: root
    });
  git('init', '-q');
  await writeFile(join(root, 'a.txt'), 'hello\nworld\n');
  git('add', '.');
  git('commit', '-q', '-m', 'Add greeting');
  const tools = new Map<string, Tool>();
  const extension = gitExtension(root);
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    registerTool: (tool: { name: string }) => void tools.set(tool.name, tool as never)
  } as unknown as AgentExtensionAPI);

  const log = await tools.get('gitLog')!.execute('1', { path: 'a.txt', added: 'world' });
  expect(log.content[0]!.text).toMatch(/Tester\n {4}Add greeting/);
  const blame = await tools.get('gitBlame')!.execute('1', { path: 'a.txt', start: 2, end: 2 });
  expect(blame.content[0]!.text).toContain('world');
  const show = await tools.get('gitShow')!.execute('1', { revision: 'HEAD', stat: true });
  expect(show.content[0]!.text).toContain('a.txt');
  await expect(
    tools.get('gitBlame')!.execute('1', { path: '--contents=/etc/passwd', start: 1, end: 1 })
  ).rejects.toThrow('is not a relative path');
});
