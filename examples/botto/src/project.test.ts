/** Load the actual Runling project without starting its external Chatto connection. */
import { mkdir, mkdtemp, readdir, rm, stat, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { expect, test, vi } from 'vitest';
import { ConfigReloader } from 'runling/config-reloader';

/** Detect writes to the user's normal cache while loading the real project in a test. */
async function cacheSnapshot() {
  const directory = resolve('node_modules/.cache/jiti');
  const names = await readdir(directory).catch((error: NodeJS.ErrnoException) => {
    if (error.code === 'ENOENT') return [];
    throw error;
  });
  return Promise.all(
    names.sort().map(async (name) => {
      const entry = await stat(join(directory, name));
      return [name, entry.size, entry.mtimeMs, entry.uid, entry.gid];
    })
  );
}

// The real loader compiles the complete project graph on a cold CI worker.
test(
  'project configuration registers Botto and scopes history to its TOML data directory',
  { timeout: 30_000 },
  async () => {
    await mkdir(resolve('../../.context'), { recursive: true });
    const directory = await mkdtemp(resolve('../../.context/botto-project-'));
    const path = join(directory, 'instance.toml');
    const mask = process.umask();
    const loader = new ConfigReloader(resolve('runling.config.ts'));
    try {
      // Privileged test runs must never leave compiled files in the operator's cache.
      vi.stubEnv('JITI_FS_CACHE', 'false');
      const cacheBefore = await cacheSnapshot();
      await writeFile(
        path,
        '[chatto]\nurl="http://127.0.0.1:1"\napi_key="synthetic"\n[agent]\nmodel="test/model"\n[runtime]\ndata_directory="private"\n[announcements.github]\nenabled=true\nrepository="chattocorp/chatto"\nroom_id="room"\nwebhook_secret="synthetic-secret-with-at-least-32-chars"\n'
      );
      vi.stubEnv('BOTTO_CONFIG', path);
      vi.stubEnv('RUNLING_DATA_DIRECTORY', undefined);
      const config = await loader.load();
      expect(config.workflows?.bottoConversation?.name).toBe('Botto conversation');
      expect(config.sources?.chatto).toBeTypeOf('function');
      expect(config.sources?.githubAnnouncements).toBeTypeOf('function');
      expect(config.workflows?.githubAnnouncement?.name).toBe('GitHub announcement');
      expect(config.webhooks).toEqual({});
      expect(process.env.RUNLING_DATA_DIRECTORY).toBe(join(directory, 'private/runling'));
      expect(process.umask()).toBe(0o077);
      expect(await cacheSnapshot()).toEqual(cacheBefore);
    } finally {
      await loader.close();
      vi.unstubAllEnvs();
      process.umask(mask);
      await rm(directory, { recursive: true, force: true });
    }
  }
);
