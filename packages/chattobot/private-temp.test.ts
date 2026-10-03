import { chmod, mkdtemp, rm, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

const STATE = Symbol.for('chattobot.privateTemp');
const BASE = Symbol.for('chattobot.privateTempBase');
type State = { [STATE]?: unknown; [BASE]?: unknown };
let base: string;
beforeEach(async () => {
  base = await mkdtemp(join(tmpdir(), 'chattobot-temp-test-'));
  vi.stubEnv('TMPDIR', base);
  vi.resetModules();
  delete (globalThis as State)[STATE];
  delete (globalThis as State)[BASE];
});
afterEach(async () => {
  vi.unstubAllEnvs();
  delete (globalThis as State)[STATE];
  delete (globalThis as State)[BASE];
  await rm(base, { recursive: true, force: true });
});

// The test setup mocks this module for all other tests.
const load = async () =>
  (await vi.importActual<typeof import('./private-temp.ts')>('./private-temp.ts'))
    .usePrivateTempDirectory;

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

test.skipIf(process.platform === 'win32')(
  'replaces the directory when a system cleaner deleted it',
  async () => {
    const usePrivateTempDirectory = await load();
    const first = await usePrivateTempDirectory();
    await rm(first, { recursive: true, force: true });
    const second = await usePrivateTempDirectory();
    expect(second).not.toBe(first);
    expect(dirname(second)).toBe(base);
    expect((await stat(second)).mode & 0o777).toBe(0o700);
    expect(process.env.TMPDIR).toBe(second);
  }
);

test.skipIf(process.platform === 'win32' || process.getuid?.() === 0)(
  'a failed creation is tried again',
  async () => {
    const usePrivateTempDirectory = await load();
    await chmod(base, 0o500);
    try {
      await expect(usePrivateTempDirectory()).rejects.toThrow();
    } finally {
      await chmod(base, 0o700);
    }
    expect(dirname(await usePrivateTempDirectory())).toBe(base);
  }
);

test.skipIf(process.platform === 'win32' || process.getuid?.() === 0)(
  'a failed replacement keeps the system temporary directory as base',
  async () => {
    const usePrivateTempDirectory = await load();
    const first = await usePrivateTempDirectory();
    await rm(first, { recursive: true, force: true });
    await chmod(base, 0o500);
    try {
      await expect(usePrivateTempDirectory()).rejects.toThrow();
    } finally {
      await chmod(base, 0o700);
    }
    expect(dirname(await usePrivateTempDirectory())).toBe(base);
  }
);
