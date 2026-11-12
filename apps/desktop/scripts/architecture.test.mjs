import assert from 'node:assert/strict';
import test from 'node:test';
import { desktopArchitecture } from './architecture.mjs';

test('default builds retain the host architecture', () => {
  for (const platform of ['darwin', 'linux', 'win32']) {
    for (const arch of ['arm64', 'x64']) {
      assert.equal(desktopArchitecture(platform, arch, {}), arch);
      assert.equal(desktopArchitecture(platform, arch, { CHATTO_DESKTOP_ARCH: '' }), arch);
    }
  }
});

test('macOS builds accept Intel, Apple Silicon, and universal targets', () => {
  for (const arch of ['x64', 'arm64', 'universal']) {
    assert.equal(desktopArchitecture('darwin', 'arm64', { CHATTO_DESKTOP_ARCH: arch }), arch);
  }
  assert.throws(
    () => desktopArchitecture('darwin', 'arm64', { CHATTO_DESKTOP_ARCH: 'ia32' }),
    /Unsupported/
  );
  assert.throws(
    () => desktopArchitecture('linux', 'x64', { CHATTO_DESKTOP_ARCH: 'universal' }),
    /macOS/
  );
});
