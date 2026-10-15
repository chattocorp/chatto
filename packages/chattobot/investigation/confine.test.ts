import { execFileSync } from 'node:child_process';
import { mkdtemp, mkdir, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from 'vitest';
import type { AgentExtensionAPI } from 'runling';
import { confineExtension, confinementProblem } from './confine.ts';

async function repository() {
  const root = await mkdtemp(join(tmpdir(), 'confine-'));
  execFileSync('git', ['init', '-q'], { cwd: root });
  await mkdir(join(root, 'src'));
  await writeFile(join(root, 'src/a.ts'), 'code\n');
  await writeFile(join(root, '.gitignore'), '.env\n*.pem\n');
  await writeFile(join(root, '.env'), 'SECRET=1\n');
  await writeFile(join(root, 'key.pem'), 'key\n');
  const outside = await mkdtemp(join(tmpdir(), 'outside-'));
  await symlink(outside, join(root, 'escape'));
  return root;
}

test('allows repository files and refuses outside, .git, and ignored paths', async () => {
  const root = await repository();
  expect(await confinementProblem(root, 'src/a.ts')).toBeUndefined();
  expect(await confinementProblem(root, 'src')).toBeUndefined();
  expect(await confinementProblem(root, undefined)).toBeUndefined();
  expect(await confinementProblem(root, '/etc/passwd')).toMatch('outside the repository');
  expect(await confinementProblem(root, '../other')).toMatch('outside the repository');
  expect(await confinementProblem(root, 'escape/file')).toMatch('outside the repository');
  expect(await confinementProblem(root, '.git/config')).toMatch('inside .git');
  expect(await confinementProblem(root, '.env')).toMatch('ignored by Git');
  expect(await confinementProblem(root, 'key.pem')).toMatch('ignored by Git');
});

test('blocks file tool calls with refused paths or patterns that leave the repository', async () => {
  const root = await repository();
  let handler!: (event: unknown) => Promise<unknown>;
  const extension = confineExtension(root);
  const factory = typeof extension === 'function' ? extension : extension.factory;
  await factory({
    on: (_event: string, registered: typeof handler) => void (handler = registered)
  } as unknown as AgentExtensionAPI);
  const call = (toolName: string, input: unknown) => handler({ toolName, input });
  expect(await call('read', { path: 'src/a.ts' })).toBeUndefined();
  expect(await call('read', { path: '.env' })).toMatchObject({ block: true });
  expect(await call('grep', { pattern: 'SECRET', path: '/' })).toMatchObject({ block: true });
  expect(await call('grep', { pattern: 'x', glob: '../**' })).toMatchObject({ block: true });
  expect(await call('find', { pattern: '../../**/*.pem' })).toMatchObject({ block: true });
  expect(await call('ls', { path: '..' })).toMatchObject({ block: true });
  expect(await call('gitLog', { path: '/etc' })).toBeUndefined();
});
