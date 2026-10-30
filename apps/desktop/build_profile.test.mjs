import assert from 'node:assert/strict';
import test from 'node:test';
import { desktopBuildProfile } from './build_profile.mjs';

test('development and release installs have separate names, storage names, and identities', () => {
  const release = desktopBuildProfile();
  const dev = desktopBuildProfile('development');
  assert.equal(release.bundleIdentifier, 'run.chatto.desktop');
  assert.equal(release.productName, 'Chatto');
  assert.equal(dev.productName, 'Chatto Dev');
  assert.equal(release.userDataName, 'Chatto Desktop');
  assert.equal(dev.userDataName, 'Chatto Desktop Dev');
  for (const key of ['productName', 'packageName', 'bundleIdentifier']) {
    assert.notEqual(dev[key], release[key]);
  }
  assert.throws(() => desktopBuildProfile('typo'));
});
