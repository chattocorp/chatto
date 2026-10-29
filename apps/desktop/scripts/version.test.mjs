import assert from 'node:assert/strict';
import test from 'node:test';
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { macOSVersions, releaseBuildVersion, restoreReleaseVersion } from './version.mjs';
import { updatePreferences } from '../updates.mjs';

test('restores release metadata before archiving a macOS beta', async (t) => {
  const scratch = new URL('../../../.context/', import.meta.url);
  await mkdir(scratch, { recursive: true });
  const buildPath = await mkdtemp(fileURLToPath(new URL('desktop-version-', scratch)));
  t.after(() => rm(buildPath, { recursive: true, force: true }));
  const filename = `${buildPath}/package.json`;
  // The packager writes appVersion after the build's afterCopy hook.
  await writeFile(filename, JSON.stringify({ version: '0.5.0', chattoAutoUpdate: true }));
  await restoreReleaseVersion(buildPath, '0.5.0-beta.24');
  const packaged = JSON.parse(await readFile(filename, 'utf8'));
  assert.equal(packaged.version, '0.5.0-beta.24');
  assert.equal(packaged.chattoAutoUpdate, true);
  assert.equal(updatePreferences(undefined, 0.5, packaged.version).channel, 'beta');
});

test('converts stable and prerelease SemVer to numeric build versions', () => {
  assert.equal(releaseBuildVersion('1.2.3'), '1.2.3');
  assert.equal(releaseBuildVersion('0.1.0-alpha.4'), '0.1.0.4');
});

test('rejects versions the packaging metadata cannot represent', () => {
  assert.throws(() => releaseBuildVersion('desktop-next'), TypeError);
});

test('maps desktop releases to macOS bundle versions', () => {
  assert.deepEqual(macOSVersions('0.1.0'), {
    shortVersion: '0.1.0',
    bundleVersion: '0.1.0'
  });
  assert.deepEqual(macOSVersions('0.1.0-alpha.4'), {
    shortVersion: '0.1.0',
    bundleVersion: '0.1.0a4'
  });
  assert.deepEqual(macOSVersions('0.1.0-beta.2'), {
    shortVersion: '0.1.0',
    bundleVersion: '0.1.0b2'
  });
  assert.deepEqual(macOSVersions('0.1.0-rc.3'), {
    shortVersion: '0.1.0',
    bundleVersion: '0.1.0fc3'
  });
  assert.deepEqual(macOSVersions('0.1.0-dev'), {
    shortVersion: '0.1.0',
    bundleVersion: '0.1.0d1'
  });
});

test('rejects invalid macOS desktop versions', () => {
  assert.throws(() => macOSVersions('desktop-dev'), TypeError);
});
