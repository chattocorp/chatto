import { expect, test, vi } from 'vitest';
import { readFile } from 'node:fs/promises';
import { assertRelease, assertUnusedVersion, checkRelease } from '../scripts/check-release.mjs';

const manifest = { name: '@chatto/translatto', version: '0.1.0' };

test('rejects mismatched tags and invalid package identities', () => {
  expect(() => assertRelease(manifest, 'translatto/v0.1.0')).not.toThrow();
  expect(() => assertRelease(manifest)).not.toThrow();
  expect(() => assertRelease(manifest, 'translatto/v0.2.0')).toThrow('Tag must match');
  expect(() => assertRelease({ ...manifest, name: 'translatto' })).toThrow('Expected a stable');
  expect(() => assertRelease({ ...manifest, version: '0.1.0-alpha.1' })).toThrow(
    'Expected a stable'
  );
});

test('rejects both published and historical versions', async () => {
  const request: typeof fetch = async () =>
    Response.json({
      versions: { '0.2.0': {} },
      time: { '0.1.0': '2026-01-01' }
    });
  await expect(assertUnusedVersion('0.1.0', request)).rejects.toThrow('already been published');
  await expect(assertUnusedVersion('0.2.0', request)).rejects.toThrow('already been published');
  await expect(assertUnusedVersion('0.3.0', request)).resolves.toBeUndefined();
});

test('allows an initial package but rejects registry failures', async () => {
  await expect(
    assertUnusedVersion('0.1.0', async () => new Response(null, { status: 404 }))
  ).resolves.toBeUndefined();
  for (const status of [401, 403, 429, 500]) {
    await expect(
      assertUnusedVersion('0.1.0', async () => new Response(null, { status }))
    ).rejects.toThrow(`HTTP ${status}`);
  }
  await expect(
    assertUnusedVersion('0.1.0', async () => {
      throw new Error('network failure');
    })
  ).rejects.toThrow('network failure');
  await expect(assertUnusedVersion('0.1.0', async () => Response.json({}))).rejects.toThrow(
    'Invalid npm'
  );
});

test('keeps the release independent and the workspace exports on source', async () => {
  const readJson = async (path: string) =>
    JSON.parse(await readFile(new URL(path, import.meta.url), 'utf8'));
  const config = await readJson('../../../.release-please-config.json');
  const pkg = await readJson('../package.json');
  expect(config.packages['packages/translatto']).toMatchObject({
    component: 'translatto',
    'initial-version': '0.1.1',
    versioning: 'default',
    prerelease: false,
    // The workflow publishes this draft with Latest explicitly disabled.
    draft: true,
    'include-component-in-tag': true,
    'tag-separator': '/'
  });
  expect(config.packages['.']['exclude-paths']).toContain('packages/translatto');
  expect(pkg.exports['.']).toBe('./src/index.ts');
  expect((await readJson('../../../.release-please-manifest.json'))['packages/translatto']).toBe(
    pkg.version
  );
  expect((await readFile(new URL('../version.txt', import.meta.url), 'utf8')).trim()).toBe(
    pkg.version
  );
});

test('verification does not query the registry for an already published package', async () => {
  const request = vi.fn<typeof fetch>(async () => {
    throw new Error('Registry must not be needed');
  });
  await expect(checkRelease(manifest, { verifyOnly: true }, request)).resolves.toBeUndefined();
  expect(request).not.toHaveBeenCalled();
  await expect(
    checkRelease(manifest, { verifyOnly: true, tag: 'translatto/v0.2.0' }, request)
  ).rejects.toThrow('Tag must match');
});

test('publication still rejects an already published version', async () => {
  const request: typeof fetch = async () => Response.json({ versions: { '0.1.0': {} }, time: {} });
  await expect(checkRelease(manifest, { tag: 'translatto/v0.1.0' }, request)).rejects.toThrow(
    'already been published'
  );
});
