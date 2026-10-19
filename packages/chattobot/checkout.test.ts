import { execFileSync } from 'node:child_process';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { afterEach, expect, test } from 'vitest';
import { readCheckout, refreshProjectContext } from './checkout.ts';

const roots: string[] = [];
const signal = new AbortController().signal;
const git = (cwd: string, ...args: string[]) =>
  execFileSync('git', ['-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', ...args], {
    cwd,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe']
  }).trim();

afterEach(async () => {
  await Promise.all(roots.splice(0).map((root) => rm(root, { recursive: true, force: true })));
});

/** Real local remotes exercise fetch and fast-forward behavior without network access. */
async function fixture() {
  const scratch = resolve('../../.context/checkout-tests');
  await mkdir(scratch, { recursive: true });
  const root = await mkdtemp(join(scratch, 'repo-'));
  roots.push(root);
  const origin = join(root, 'origin');
  const copy = join(root, 'copy');
  await mkdir(origin);
  git(origin, 'init', '-b', 'main');
  async function commit(text: string) {
    await writeFile(join(origin, 'CHATTOBOT.md'), text);
    git(origin, 'add', '.');
    git(origin, 'commit', '-m', 'Update briefing');
    return git(origin, 'rev-parse', 'HEAD');
  }
  await commit('Old briefing');
  git(root, 'clone', origin, copy);
  return { origin, copy, commit };
}

test('concurrent starts fast-forward and read the committed briefing with its revision', async () => {
  const { copy, commit } = await fixture();
  const revision = await commit('Development notes: https://dev-docs.chatto.run/releases/0-5-0/');
  const contexts = await Promise.all([
    refreshProjectContext(copy, signal),
    refreshProjectContext(copy, signal)
  ]);
  expect(contexts[0]).toBe(contexts[1]);
  expect(contexts[0]).toContain(revision);
  expect(contexts[0]).toContain('Development notes:');
  expect(git(copy, 'rev-parse', 'HEAD')).toBe(revision);
  expect(await readFile(join(copy, 'CHATTOBOT.md'), 'utf8')).toContain('Development notes:');
});

test.each(['dirty', 'branch', 'ahead', 'diverged', 'offline'])(
  'preserves a %s checkout and uses its existing committed briefing',
  async (kind) => {
    const { copy, commit } = await fixture();
    if (kind !== 'ahead') await commit('New upstream briefing');
    if (kind === 'dirty') await writeFile(join(copy, 'CHATTOBOT.md'), 'Uncommitted instructions');
    if (kind === 'branch') git(copy, 'checkout', '-b', 'work');
    if (kind === 'ahead' || kind === 'diverged') {
      await writeFile(join(copy, 'local.txt'), 'Local work');
      git(copy, 'add', '.');
      git(copy, 'commit', '-m', 'Local commit');
    }
    if (kind === 'offline') git(copy, 'remote', 'set-url', 'origin', join(copy, 'missing'));
    const before = git(copy, 'rev-parse', 'HEAD');
    const context = await refreshProjectContext(copy, signal);
    expect(git(copy, 'rev-parse', 'HEAD')).toBe(before);
    expect(context).toContain('Old briefing');
    expect(context).toContain('Freshness is unconfirmed');
    expect(context).not.toContain('Uncommitted instructions');
    if (kind === 'dirty')
      expect(await readFile(join(copy, 'CHATTOBOT.md'), 'utf8')).toBe('Uncommitted instructions');
  }
);

test('defers updates while an investigation reads, then refreshes on the next start', async () => {
  const { copy, commit } = await fixture();
  const revision = await commit('New briefing');
  {
    using reader = await readCheckout(copy, signal);
    expect(await refreshProjectContext(copy, signal)).toContain('Old briefing');
    expect(git(copy, 'rev-parse', 'HEAD')).not.toBe(revision);
  }
  expect(await refreshProjectContext(copy, signal)).toContain('New briefing');
  expect(git(copy, 'rev-parse', 'HEAD')).toBe(revision);
});

test('a concurrent reader and refresh keep the investigated revision stable', async () => {
  const { copy, commit } = await fixture();
  await commit('New briefing');
  const refresh = refreshProjectContext(copy, signal);
  using reader = await readCheckout(copy, signal);
  const revision = git(copy, 'rev-parse', 'HEAD');
  await refresh;
  expect(git(copy, 'rev-parse', 'HEAD')).toBe(revision);
});

test('missing briefing does not block conversation startup', async () => {
  const { copy, origin } = await fixture();
  git(origin, 'rm', 'CHATTOBOT.md');
  git(origin, 'commit', '-m', 'Remove briefing');
  expect(await refreshProjectContext(copy, signal)).toBeUndefined();
});

test('cancellation does not start a refresh', async () => {
  const { copy, commit } = await fixture();
  const before = git(copy, 'rev-parse', 'HEAD');
  await commit('New briefing');
  const controller = new AbortController();
  controller.abort(new Error('Stopped'));
  await expect(refreshProjectContext(copy, controller.signal)).rejects.toThrow('Stopped');
  expect(git(copy, 'rev-parse', 'HEAD')).toBe(before);
});
