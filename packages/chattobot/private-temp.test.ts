import { mkdir, mkdtemp, readdir, rm, stat, symlink, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';

let base: string;
beforeEach(async () => {
  base = await mkdtemp(join(tmpdir(), 'chattobot-temp-test-'));
  vi.resetModules();
  vi.stubEnv('TMPDIR', process.env.TMPDIR ?? '');
});
afterEach(async () => {
  vi.unstubAllEnvs();
  await rm(base, { recursive: true, force: true });
});

const load = async () => (await import('./private-temp.ts')).usePrivateTempDirectory;

test.skipIf(process.platform === 'win32')(
  'creates an empty directory that only the bot user can read and points TMPDIR at it',
  async () => {
    const directory = join(base, `chattobot-${process.getuid!()}`);
    await mkdir(directory, { mode: 0o755 });
    await writeFile(join(directory, 'pi-codemode-old.txt'), 'thread messages');
    const usePrivateTempDirectory = await load();
    expect(await usePrivateTempDirectory(base)).toBe(directory);
    expect((await stat(directory)).mode & 0o777).toBe(0o700);
    expect(await readdir(directory)).toEqual([]);
    expect(process.env.TMPDIR).toBe(directory);
    expect(tmpdir()).toBe(directory);
    // A second call in the same process keeps the files of this process.
    await writeFile(join(directory, 'pi-codemode-new.txt'), 'current output');
    expect(await usePrivateTempDirectory(join(base, 'other'))).toBe(directory);
    expect(await readdir(directory)).toEqual(['pi-codemode-new.txt']);
  }
);

test.skipIf(process.platform === 'win32')(
  'refuses a symbolic link in place of the directory',
  async () => {
    const target = join(base, 'elsewhere');
    await mkdir(target);
    await symlink(target, join(base, `chattobot-${process.getuid!()}`));
    const usePrivateTempDirectory = await load();
    await expect(usePrivateTempDirectory(base)).rejects.toThrow('not a private directory');
  }
);
