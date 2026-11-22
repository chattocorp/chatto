/** Release guards must fail before publication when identity or registry checks fail. */
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { checkRelease } from './check-release.mjs';

const manifest = { name: '@chatto/client', version: '0.1.0' };

test('rejects a mismatched package identity before contacting npm', async () => {
  const request = async () => assert.fail('Must not contact npm');
  for (const invalid of [
    { ...manifest, name: '@chatto/other' },
    { ...manifest, version: '0.1.0-beta.01' }
  ]) {
    await assert.rejects(checkRelease(invalid, {}, request), /Expected a valid/);
  }
  for (const version of ['0.5.0', '0.5.0-beta.22', '0.6.0-alpha.1']) {
    await checkRelease({ ...manifest, version }, { verifyOnly: true }, request);
  }
});

test('accepts an unused version or a first publication', async () => {
  for (const response of [
    new Response(null, { status: 404 }),
    Response.json({ versions: {}, time: {} })
  ]) {
    await checkRelease(manifest, {}, async (url) => {
      assert.equal(url, 'https://registry.npmjs.org/@chatto%2Fclient');
      return response;
    });
  }
});

test('rejects versions in either current or deleted package history', async () => {
  for (const metadata of [
    { versions: { '0.1.0': {} }, time: {} },
    { versions: {}, time: { '0.1.0': '2026-01-01' } }
  ]) {
    await assert.rejects(
      checkRelease(manifest, {}, async () => Response.json(metadata)),
      /already been published/
    );
  }
});

test('rejects registry errors and malformed responses', async () => {
  for (const status of [401, 403, 429, 500]) {
    await assert.rejects(
      checkRelease(manifest, {}, async () => new Response(null, { status })),
      new RegExp(`HTTP ${status}`)
    );
  }
  await assert.rejects(
    checkRelease(manifest, {}, async () => Response.json({})),
    /Invalid npm/
  );
  await assert.rejects(
    checkRelease(manifest, {}, async () => {
      throw new Error('network failure');
    }),
    /network failure/
  );
});
