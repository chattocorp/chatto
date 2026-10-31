import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdtemp, readFile, readdir, rm, writeFile, mkdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import { EventEmitter } from 'node:events';
import { writeUpdateManifests } from './update-manifests.mjs';
import { bundleUpdater } from './bundle-updater.mjs';

const scratch = fileURLToPath(new URL('../../../.context/', import.meta.url));
async function temporary(t) {
  await mkdir(scratch, { recursive: true });
  const directory = await mkdtemp(path.join(scratch, 'desktop-update-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  return directory;
}

for (const version of ['0.5.0', '0.5.0-beta.24']) {
  test(`manifests describe the exact ${version} artifacts and parse with the real updater`, async (t) => {
    const directory = await temporary(t);
    for (const suffix of [
      'macOS_ARM64.zip',
      'Windows_X64_Setup.exe',
      'Linux_X64.AppImage',
      'Linux_ARM64.AppImage'
    ]) {
      await writeFile(path.join(directory, `chatto-desktop_${version}_${suffix}`), suffix);
    }
    await writeUpdateManifests(directory, version, '2026-10-29T00:00:00.000Z');
    const require = createRequire(import.meta.url);
    const { Provider, parseUpdateInfo } = require('electron-updater/out/providers/Provider.js');
    const channel = version.includes('-') ? 'beta' : 'latest';
    for (const suffix of ['-mac', '', '-linux', '-linux-arm64']) {
      const name = `${channel}${suffix}.yml`;
      const info = parseUpdateInfo(
        await readFile(path.join(directory, name), 'utf8'),
        name,
        new URL('https://example.com')
      );
      assert.equal(info.version, version);
      const file = info.files[0];
      if (suffix.startsWith('-linux')) {
        const arch = suffix === '-linux' ? 'X64' : 'ARM64';
        assert.equal(file.url, `chatto-desktop_${version}_Linux_${arch}.AppImage`);
      }
      const bytes = await readFile(path.join(directory, file.url));
      assert.equal(file.sha512, createHash('sha512').update(bytes).digest('base64'));
      assert.equal(file.size, bytes.length);
      assert.equal(info.rolloutHours, 36);
    }
    // Use the installed updater's channel rules, so an upstream naming change
    // cannot silently disconnect Linux installations from published manifests.
    const previousArch = process.env.TEST_UPDATER_ARCH;
    try {
      for (const arch of ['x64', 'arm64']) {
        process.env.TEST_UPDATER_ARCH = arch;
        const provider = new Provider({ platform: 'linux' });
        const name = `${provider.getCustomChannelName(channel)}.yml`;
        const info = parseUpdateInfo(
          await readFile(path.join(directory, name), 'utf8'),
          name,
          new URL('https://example.com')
        );
        assert.equal(
          info.files[0].url,
          `chatto-desktop_${version}_Linux_${arch.toUpperCase()}.AppImage`
        );
      }
    } finally {
      if (previousArch === undefined) delete process.env.TEST_UPDATER_ARCH;
      else process.env.TEST_UPDATER_ARCH = previousArch;
    }
  });
}

test('incomplete platform uploads produce no manifests', async (t) => {
  const directory = await temporary(t);
  await writeFile(path.join(directory, 'chatto-desktop_0.5.0_macOS_ARM64.zip'), 'mac');
  await assert.rejects(writeUpdateManifests(directory, '0.5.0'));
  assert.equal(
    (await readdir(directory)).some((file) => file.endsWith('.yml')),
    false
  );
});

test('missing Linux ARM64 upload prevents manifest publication', async (t) => {
  const directory = await temporary(t);
  for (const suffix of ['macOS_ARM64.zip', 'Windows_X64_Setup.exe', 'Linux_X64.AppImage']) {
    await writeFile(path.join(directory, `chatto-desktop_0.5.0_${suffix}`), suffix);
  }
  await assert.rejects(writeUpdateManifests(directory, '0.5.0'), /Linux_ARM64/);
  assert.equal(
    (await readdir(directory)).some((file) => file.endsWith('.yml')),
    false
  );
});

test('bundled updater loads without node_modules and retains beta version and HTTP transport', async (t) => {
  const directory = await temporary(t);
  await bundleUpdater(directory);
  const require = createRequire(import.meta.url);
  const module = { exports: {} };
  const nativeUpdater = new EventEmitter();
  vm.runInNewContext(await readFile(path.join(directory, 'updater-vendor.cjs'), 'utf8'), {
    module,
    exports: module.exports,
    process,
    Buffer,
    URL,
    console,
    setTimeout,
    clearTimeout,
    global: {},
    __dirname: directory,
    require: (name) => {
      if (name === 'electron')
        return {
          app: { getVersion: () => '0.5.0', getName: () => 'Chatto Desktop', isPackaged: true },
          autoUpdater: nativeUpdater
        };
      assert.ok(require('node:module').isBuiltin(name), `Unexpected unbundled dependency: ${name}`);
      return require(name);
    }
  });
  const updater = module.exports.createUpdater('0.5.0-beta.24');
  assert.equal(updater.currentVersion.version, '0.5.0-beta.24');
  assert.ok(updater.httpExecutor);
  assert.match(
    await readFile(path.join(directory, 'updater-licenses.txt'), 'utf8'),
    /electron-updater/
  );
});
