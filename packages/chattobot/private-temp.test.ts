import { mkdtemp, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

const STATE = Symbol.for('chattobot.privateTemp');
let base: string;
beforeEach(async () => {
  base = await mkdtemp(join(tmpdir(), 'chattobot-temp-test-'));
  vi.stubEnv('TMPDIR', base);
  vi.resetModules();
  delete (globalThis as { [STATE]?: unknown })[STATE];
});
afterEach(async () => {
  vi.unstubAllEnvs();
  delete (globalThis as { [STATE]?: unknown })[STATE];
  await rm(base, { recursive: true, force: true });
});

const load = async () => (await import('./private-temp.ts')).usePrivateTempDirectory;

test.skipIf(process.platform === 'win32')(
  'creates a directory that only the bot user can read and points TMPDIR at it',
  async () => {
    const directory = await (await load())();
    expect(dirname(directory)).toBe(base);
    expect((await stat(directory)).mode & 0o777).toBe(0o700);
    expect(process.env.TMPDIR).toBe(directory);
    expect(tmpdir()).toBe(directory);
  }
);

test('a reloaded module returns the same directory instead of nesting a new one', async () => {
  const first = await (await load())();
  vi.resetModules();
  expect(await (await load())()).toBe(first);
});
