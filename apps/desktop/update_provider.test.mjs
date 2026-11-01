import assert from 'node:assert/strict';
import test from 'node:test';
import { GitHubProvider } from 'electron-updater/out/providers/GitHubProvider.js';
import { DesktopGitHubProvider } from './update_provider.mjs';

/** Exercise the real provider with GitHub responses and no network connection. */
function fixture(platform, tags, channel = 'beta', Provider = DesktopGitHubProvider) {
  const requests = [];
  const updater = {
    channel,
    allowPrerelease: channel === 'beta',
    currentVersion: { raw: '0.5.0-beta.24' },
    fullChangelog: false
  };
  const executor = {
    async request(options) {
      requests.push(options.path);
      if (options.path.endsWith('.atom')) {
        return `<feed>${tags
          .map(
            (tag) =>
              `<entry><title>${tag}</title><content>Release notes</content><link href="https://github.com/chattocorp/chatto/releases/tag/${tag}"/></entry>`
          )
          .join('')}</feed>`;
      }
      if (options.path.endsWith('/latest')) return JSON.stringify({ tag_name: 'v0.4.2' });
      const tag = options.path.split('/').at(-2);
      return JSON.stringify({
        version: tag.slice(1),
        files: [{ url: 'app.zip', sha512: 'hash', size: 1 }]
      });
    }
  };
  const provider = new Provider({ owner: 'chattocorp', repo: 'chatto' }, updater, {
    platform,
    executor
  });
  return { provider, updater, requests };
}

for (const [platform, suffix] of [
  ['darwin', '-mac'],
  ['win32', ''],
  ['linux', `-linux${process.arch === 'x64' ? '' : `-${process.arch}`}`]
]) {
  test(`Beta discovers RC tags and reads the Beta manifest on ${platform}`, async () => {
    const f = fixture(platform, ['runling-v0.9.0', 'v0.5.0-rc.1', 'v0.5.0-beta.24']);
    const info = await f.provider.getLatestVersion();
    assert.equal(info.version, '0.5.0-rc.1');
    assert.equal(info.tag, 'v0.5.0-rc.1');
    assert.equal(info.releaseNotes, 'Release notes');
    assert.equal(
      f.requests.at(-1),
      `/chattocorp/chatto/releases/download/v0.5.0-rc.1/beta${suffix}.yml`
    );
    assert.equal(
      f.provider.resolveFiles(info)[0].url.pathname,
      '/chattocorp/chatto/releases/download/v0.5.0-rc.1/app.zip'
    );
  });
}

test('Beta can advance through beta, RC, and stable releases', async () => {
  for (const [tag, manifest] of [
    ['v0.5.0-beta.25', 'beta.yml'],
    ['v0.5.0-rc.1', 'beta.yml'],
    ['v0.5.0-rc.2', 'beta.yml'],
    ['v0.5.0', 'latest.yml']
  ]) {
    const f = fixture('win32', [tag, 'v0.5.0-beta.24']);
    assert.equal((await f.provider.getLatestVersion()).tag, tag);
    assert.ok(f.requests.at(-1).endsWith(`/${tag}/${manifest}`));
  }
});

test('switching to Stable excludes RC releases with the same provider', async () => {
  const f = fixture('win32', ['v0.5.0-rc.1', 'v0.4.2']);
  assert.equal((await f.provider.getLatestVersion()).tag, 'v0.5.0-rc.1');
  f.updater.allowPrerelease = false;
  f.updater.channel = 'latest';
  assert.equal((await f.provider.getLatestVersion()).tag, 'v0.4.2');
  assert.equal(f.requests.at(-1), '/chattocorp/chatto/releases/download/v0.4.2/latest.yml');
});

test('a missing RC manifest fails without selecting an older release', async () => {
  const f = fixture('win32', ['v0.5.0-rc.1', 'v0.5.0-beta.24']);
  const request = f.provider.executor.request;
  f.provider.executor.request = async (options) => {
    if (options.path.endsWith('.yml')) throw new Error('Missing release manifest');
    return request(options);
  };
  await assert.rejects(f.provider.getLatestVersion(), /Missing release manifest/);
});

test('the old Beta provider can reach beta.29 before the fixed provider advances to RC', async () => {
  const tags = ['v0.5.0-rc.2', 'v0.5.0-beta.29', 'v0.5.0-beta.28'];
  const previous = fixture('win32', tags, 'beta', GitHubProvider);
  assert.equal((await previous.provider.getLatestVersion()).tag, 'v0.5.0-beta.29');
  const updated = fixture('win32', tags);
  assert.equal((await updated.provider.getLatestVersion()).tag, 'v0.5.0-rc.2');
});
