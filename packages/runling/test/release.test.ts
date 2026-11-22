import { expect, test } from 'vitest';
import { readFile } from 'node:fs/promises';
import { assertUnusedVersion } from '../scripts/check-release.mjs';

test('rejects both available and unpublished historical versions', () => {
  const metadata = {
    versions: { '0.3.0': {} },
    time: { '0.1.0': '2017-08-29' }
  };
  expect(() => assertUnusedVersion('0.3.0', metadata)).toThrow('already been published');
  expect(() => assertUnusedVersion('0.1.0', metadata)).toThrow('already been published');
  expect(() => assertUnusedVersion('0.3.1', metadata)).not.toThrow();
});

test('keeps Runling versioning independent of Chatto prereleases', async () => {
  const readJson = async (path: string) =>
    JSON.parse(await readFile(new URL(path, import.meta.url), 'utf8'));
  const release = await readFile(new URL('../.sweet.yml', import.meta.url), 'utf8');
  const pkg = await readJson('../package.json');
  expect(release).toMatch(/^tag-prefix: runling\/v\r?$/m);
  expect(release.match(/^version: (.+?)\r?$/m)?.[1]).toBe(pkg.version);
  expect(release).toMatch(/path: package\.json\r?\n\s+format: json\r?\n\s+key: version/);
  expect(release).toMatch(/path: version\.txt\r?\n\s+format: text/);
  expect((await readFile(new URL('../version.txt', import.meta.url), 'utf8')).trim()).toBe(
    pkg.version
  );
  expect(pkg.repository).toEqual({
    type: 'git',
    url: 'git+https://github.com/chattocorp/chatto.git',
    directory: 'packages/runling'
  });
});
