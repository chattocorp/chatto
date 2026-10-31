import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import test from 'node:test';
import { createDesktopUpdates, createUpdateInstallRequest, updatePreferences } from './updates.mjs';

function fixture(overrides = {}) {
  const updater = new EventEmitter();
  const notifications = [];
  const saved = [];
  let installs = 0;
  let downloads = 0;
  let checks = 0;
  updater.result = { isUpdateAvailable: true, updateInfo: { version: '0.6.0' } };
  updater.checkForUpdates = async () => {
    checks++;
    return updater.result;
  };
  updater.downloadUpdate = async () => {
    downloads++;
    updater.emit('update-downloaded', updater.result.updateInfo);
  };
  updater.quitAndInstall = () => {
    installs++;
  };
  // Match electron-updater's surprising channel setter behavior.
  Object.defineProperty(updater, 'channel', {
    set(value) {
      this.selectedChannel = value;
      this.allowDowngrade = true;
    }
  });
  const preferences = updatePreferences(overrides, 0.5, '0.5.0');
  const controller = createDesktopUpdates({
    updater,
    preferences,
    save: async (value) => saved.push(value),
    notify: async (...args) => notifications.push(args),
    now: () => Date.parse('2026-10-29T18:00:00Z')
  });
  return {
    updater,
    preferences,
    controller,
    notifications,
    saved,
    counts: () => ({ installs, downloads, checks })
  };
}

test('background downloads never restart; explicit installation revalidates the release', async () => {
  const f = fixture();
  await f.controller.check();
  await f.controller.check();
  assert.deepEqual(f.counts(), { installs: 0, downloads: 1, checks: 2 });
  assert.equal(f.notifications.length, 1);
  assert.equal(f.updater.autoInstallOnAppQuit, false);
  assert.equal(f.updater.allowDowngrade, false);
  await f.controller.install();
  assert.deepEqual(f.counts(), { installs: 1, downloads: 1, checks: 3 });
});

test('snapshots report serialized progress, preferences and completion without private data', async () => {
  const f = fixture();
  const states = [];
  const stop = f.controller.subscribe((state) => states.push(state));
  let complete;
  f.updater.downloadUpdate = () =>
    new Promise((resolve) => {
      complete = resolve;
    });
  const checking = f.controller.check(true);
  await new Promise(setImmediate);
  assert.equal(f.controller.snapshot().status, 'downloading');
  assert.equal(f.controller.snapshot().busy, true);
  f.updater.emit('download-progress', { percent: 42.8, url: 'secret' });
  assert.equal(f.controller.snapshot().progress, 42);
  await f.controller.check(true);
  await f.controller.install();
  assert.equal(await f.controller.setPreference('automatic', false), false);
  f.updater.emit('update-downloaded', { version: '0.6.0' });
  complete();
  await checking;
  assert.equal(f.controller.snapshot().status, 'ready');
  assert.equal(f.controller.snapshot().busy, false);
  assert.equal('bucket' in f.controller.snapshot(), false);
  assert.equal('url' in f.controller.snapshot(), false);
  assert.ok(
    states.every((state, index) => index === 0 || state.revision > states[index - 1].revision)
  );
  await f.controller.setPreference('channel', 'beta');
  assert.equal(f.controller.snapshot().channel, 'beta');
  assert.equal(f.controller.snapshot().availableVersion, null);
  stop();
  const count = states.length;
  await f.controller.setPreference('automatic', false);
  assert.equal(states.length, count);
});

test('failed preference persistence keeps the committed preference and unlocks retry', async () => {
  const updater = new EventEmitter();
  let fail = true;
  const controller = createDesktopUpdates({
    updater,
    preferences: updatePreferences({}, 0.5, '0.5.0'),
    save: async () => {
      if (fail) throw new Error('private path');
    },
    notify: async () => {}
  });
  await assert.rejects(controller.setPreference('channel', 'beta'), /Could not save/);
  assert.equal(controller.snapshot().channel, 'stable');
  assert.equal(controller.snapshot().status, 'error');
  assert.equal(controller.snapshot().busy, false);
  fail = false;
  assert.equal(await controller.setPreference('channel', 'beta'), true);
  assert.equal(controller.snapshot().channel, 'beta');
});

for (const result of [
  { isUpdateAvailable: false, updateInfo: { version: '0.6.0' } },
  { isUpdateAvailable: true, updateInfo: { version: '0.6.1' } }
])
  test('withdrawn or superseded downloads are not installed', async () => {
    const f = fixture();
    await f.controller.check();
    f.updater.result = result;
    await f.controller.install();
    assert.equal(f.counts().installs, 0);
    assert.equal(f.notifications.at(-1)[0], 'changed');
  });

test('switching channels discards a downloaded update and never enables downgrade', async () => {
  const f = fixture({ channel: 'beta' });
  await f.controller.check(true);
  assert.equal(f.updater.selectedChannel, 'beta');
  assert.equal(f.updater.allowPrerelease, true);
  await f.controller.setPreference('channel', 'stable');
  await f.controller.install();
  assert.equal(f.counts().installs, 0);
  await f.controller.check(true);
  assert.equal(f.updater.selectedChannel, 'latest');
  assert.equal(f.updater.allowPrerelease, false);
  assert.equal(f.updater.allowDowngrade, false);
});

test('checks and channel changes cannot race an in-flight download', async () => {
  const f = fixture();
  let release;
  f.updater.downloadUpdate = () =>
    new Promise((resolve) => {
      release = resolve;
    });
  const first = f.controller.check();
  await new Promise(setImmediate);
  await f.controller.check(true);
  assert.equal(await f.controller.setPreference('channel', 'beta'), false);
  assert.equal(f.counts().checks, 1);
  f.updater.emit('update-downloaded', { version: '0.6.0' });
  release();
  await first;
});

test('automatic checks can be disabled while manual checks still work', async () => {
  const f = fixture({ automatic: false });
  await f.controller.check();
  assert.equal(f.counts().checks, 0);
  await f.controller.check(true);
  assert.equal(f.counts().checks, 1);
});

test('rollout uses time and a persistent bucket; manual and beta checks bypass it', async () => {
  const f = fixture();
  const info = { releaseDate: '2026-10-29T06:00:00Z', rolloutHours: 36 };
  assert.equal(await f.updater.isUserWithinRollout(info), false);
  assert.equal(
    await f.updater.isUserWithinRollout({ ...info, releaseDate: '2026-10-28T00:00:00Z' }),
    true
  );
  f.updater.checkForUpdates = async () => {
    assert.equal(await f.updater.isUserWithinRollout(info), true);
    return { isUpdateAvailable: false };
  };
  await f.controller.check(true);
  await f.controller.setPreference('channel', 'beta');
  await f.controller.check();
});

test('download failure cannot leave an installable update or expose request data', async () => {
  const f = fixture();
  f.updater.downloadUpdate = async () => {
    throw new Error('https://private.invalid/?token=secret');
  };
  await f.controller.check(true);
  await f.controller.install();
  assert.deepEqual(f.notifications, [['error']]);
  assert.equal(f.counts().installs, 0);
});

test('asynchronous native installation failure is reported and unlocks checks', async () => {
  const f = fixture();
  await f.controller.check(true);
  await f.controller.install();
  assert.equal(await f.controller.setPreference('channel', 'beta'), false);
  f.updater.emit('error', new Error('Native signature verification failed'));
  assert.equal(f.notifications.at(-1)[0], 'error');
  assert.equal(await f.controller.setPreference('channel', 'beta'), true);
});

test('settings validate channel and rollout identity; unsupported values never change the feed', () => {
  assert.deepEqual(
    updatePreferences(
      { channel: 'nightly', bucket: -1, url: 'https://evil.invalid' },
      0.25,
      '0.5.0'
    ),
    {
      channel: 'stable',
      automatic: true,
      bucket: 0.25
    }
  );
});

test('initial channel follows the release version, including prereleases with build metadata', () => {
  for (const [version, channel] of [
    ['0.5.0', 'stable'],
    ['0.5.0+build-123', 'stable'],
    ['0.5.0-beta.24', 'beta'],
    ['0.5.0-rc.1+build-123', 'beta']
  ]) {
    for (const stored of [undefined, {}, { channel: 'invalid' }]) {
      assert.equal(updatePreferences(stored, 0.5, version).channel, channel);
    }
    for (const savedChannel of ['stable', 'beta']) {
      assert.equal(
        updatePreferences({ channel: savedChannel }, 0.5, version).channel,
        savedChannel
      );
    }
  }
});

test('restart confirmation is serialized and cannot install a changed target', async () => {
  const f = fixture();
  await f.controller.check();
  let confirm;
  let prompts = 0;
  const request = createUpdateInstallRequest(f.controller, () => {
    prompts++;
    return new Promise((resolve) => {
      confirm = resolve;
    });
  });
  const first = request();
  await request();
  assert.equal(prompts, 1);
  await f.controller.setPreference('channel', 'beta');
  confirm(true);
  await first;
  assert.equal(f.counts().installs, 0);
  await f.controller.check();
  const cancelled = request();
  confirm(false);
  await cancelled;
  assert.equal(f.counts().installs, 0);
  const accepted = request();
  confirm(true);
  await accepted;
  assert.equal(f.counts().installs, 1);
});
