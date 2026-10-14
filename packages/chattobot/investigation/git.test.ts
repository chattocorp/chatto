import { execFileSync } from 'node:child_process';
import { mkdtemp, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from 'vitest';
import { gitPolicy, runGit } from './git.ts';

test('allows read commands and refuses writes and options that write or run programs', () => {
  expect(gitPolicy(['log', '-n', '5', '--', 'cli'])).toBeUndefined();
  expect(gitPolicy(['blame', '-L', '1,20', '-C', 'a.go'])).toBeUndefined();
  expect(gitPolicy(['show', 'HEAD:a.go'])).toBeUndefined();
  expect(gitPolicy(['commit', '-m', 'x'])).toMatch('Only these git commands');
  expect(gitPolicy(['-c', 'core.pager=sh', 'log'])).toMatch('Only these git commands');
  expect(gitPolicy(['log', '--output=/tmp/x'])).toBe('The option --output is not available.');
  expect(gitPolicy(['diff', '--no-index', '/etc/passwd', 'a'])).toMatch('--no-index');
  expect(gitPolicy(['grep', '-Ovim', 'x'])).toMatch('-Ovim');
  expect(gitPolicy(['show', '--ext-diff'])).toMatch('--ext-diff');
});

test('runs git in the directory without the host configuration', async () => {
  const root = await mkdtemp(join(tmpdir(), 'git-'));
  const git = (...args: string[]) =>
    execFileSync('git', ['-c', 'user.name=T', '-c', 'user.email=t@example.com', ...args], {
      cwd: root
    });
  git('init', '-q');
  await writeFile(join(root, 'a.txt'), 'hello\n');
  git('add', '.');
  git('commit', '-q', '-m', 'First commit');
  expect(await runGit(root, ['log', '--format=%s'])).toBe('First commit\n');
  await expect(runGit(root, ['show', 'missing'])).rejects.toThrow();
});
