/** Chatto owns version selection; Docker process behavior is tested in Runling. */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { imageVersions } from './image.ts';

test('Chatto image versions follow mise, the package manager pin, and installed Playwright', async () => {
  const repository = await mkdtemp(join(tmpdir(), 'chatto-ci-versions-'));
  try {
    await mkdir(join(repository, 'apps/frontend/node_modules/playwright'), { recursive: true });
    await writeFile(join(repository, 'mise.toml'), '[tools]\nnode = "24.1.0"\ngo = "1.24.0"\n');
    await writeFile(
      join(repository, 'package.json'),
      JSON.stringify({ packageManager: 'pnpm@10.1.0' })
    );
    await writeFile(
      join(repository, 'apps/frontend/node_modules/playwright/package.json'),
      JSON.stringify({ version: '1.50.0' })
    );
    assert.deepEqual(await imageVersions(repository), {
      NODE_VERSION: '24.1.0',
      GO_VERSION: '1.24.0',
      PNPM_VERSION: '10.1.0',
      PLAYWRIGHT_VERSION: '1.50.0'
    });
    await writeFile(join(repository, 'mise.toml'), '[tools]\nnode = "latest"\ngo = "1.24.0"\n');
    await assert.rejects(imageVersions(repository), /Missing pinned node version/);
  } finally {
    await rm(repository, { recursive: true, force: true });
  }
});
