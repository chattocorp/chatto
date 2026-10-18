import { mkdir, mkdtemp, rm, stat, utimes, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { afterEach, expect, test } from 'vitest';
import { AgentSessionStore } from './agent-session-store.ts';

const directories: string[] = [];
afterEach(async () => {
  await Promise.all(
    directories.splice(0).map((path) => rm(path, { recursive: true, force: true }))
  );
});
async function store() {
  await mkdir(resolve('../../.context'), { recursive: true });
  const path = await mkdtemp(resolve('../../.context/shared-sessions-'));
  directories.push(path);
  return new AgentSessionStore(path);
}

test('new sessions can be resumed by a new store, with exclusive leases and isolated scopes', async () => {
  const sessions = await store();
  const first = await sessions.scope('thread-a').open();
  await writeFile(first.file, 'saved session', { mode: 0o600 });
  const reloaded = new AgentSessionStore(sessions.directory).scope('thread-a');
  await expect(reloaded.open({ id: first.id, resume: true })).rejects.toThrow('already in use');
  await first[Symbol.asyncDispose]();
  await expect(reloaded.open({ id: first.id })).rejects.toThrow('exists already');
  await using resumed = await reloaded.open({ id: first.id, resume: true });
  expect(resumed.file).toBe(first.file);
  await expect(sessions.scope('thread-b').open({ id: first.id, resume: true })).rejects.toThrow(
    'There is no agent session'
  );
  expect(() => sessions.scope('../escape')).toThrow('Invalid agent session scope');
  await expect(sessions.open({ id: '../escape' })).rejects.toThrow('Invalid agent session ID');
  if (process.platform !== 'win32')
    expect((await stat(reloaded.directory)).mode & 0o777).toBe(0o700);
});

test('cleanup removes only expired inactive sessions and preserves leased files', async () => {
  const sessions = await store();
  const old = new Date(Date.now() - 31 * 24 * 60 * 60 * 1000);
  const expired = await sessions.open();
  await writeFile(expired.file, 'old');
  await expired[Symbol.asyncDispose]();
  await using active = await sessions.open();
  await writeFile(active.file, 'active');
  await utimes(expired.file, old, old);
  await utimes(active.file, old, old);
  const unrelated = resolve(sessions.directory, 'unrelated.jsonl');
  await writeFile(unrelated, 'keep');
  await utimes(unrelated, old, old);
  await using recent = await sessions.open();
  await writeFile(recent.file, 'recent');
  await expect(stat(expired.file)).rejects.toMatchObject({ code: 'ENOENT' });
  await expect(stat(active.file)).resolves.toBeDefined();
  await expect(stat(unrelated)).resolves.toBeDefined();
  await sessions.prune();
  await expect(stat(recent.file)).resolves.toBeDefined();
});

test('explicit resume renews old history and a failed open releases its lease', async () => {
  const sessions = await store();
  const saved = await sessions.open();
  await saved[Symbol.asyncDispose]();
  await expect(sessions.open({ id: saved.id, resume: true })).rejects.toThrow(
    'There is no agent session'
  );
  await writeFile(saved.file, 'history');
  const old = new Date(0);
  await utimes(saved.file, old, old);
  await using resumed = await sessions.open({ id: saved.id, resume: true });
  expect((await stat(resumed.file)).mtimeMs).toBeGreaterThan(Date.now() - 10_000);
});
