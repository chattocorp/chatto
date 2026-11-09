import assert from 'node:assert/strict';
import test from 'node:test';
import { configureScreenSharing } from './screen_sharing.mjs';

const request = { securityOrigin: 'chatto://desktop', userGesture: true };
const portalSource = { id: 'window:123:0', name: '' };
const screen = { id: 'screen:0:0', name: 'Entire screen' };
const windowSource = { id: 'window:456:0', name: 'Example window' };

/** Exercise the installed Electron handler with controlled capture and dialog results. */
function setup({ sources = [portalSource], platform = 'linux', response = 0 } = {}) {
  let handler;
  const captures = [];
  const dialogs = [];
  const replies = [];
  const mainWindow = {};
  configureScreenSharing(
    { setDisplayMediaRequestHandler: (value) => (handler = value) },
    {
      platform,
      desktopCapturer: {
        async getSources(options) {
          captures.push(options);
          return typeof sources === 'function' ? sources() : sources;
        }
      },
      dialog: {
        async showMessageBox(parent, options) {
          assert.equal(parent, mainWindow);
          dialogs.push(options);
          return { response };
        }
      },
      getMainWindow: () => mainWindow
    }
  );
  return {
    captures,
    dialogs,
    replies,
    run: (value = request) =>
      handler(value, (streams) => {
        // Match Electron's callback contract: {} does not deny video capture.
        if (streams !== null && !streams.video) {
          throw new TypeError('Video was requested, but no video stream was provided');
        }
        replies.push(streams);
      })
  };
}

test('uses the Linux portal source without an empty host picker or system audio', async () => {
  const capture = setup();
  await capture.run();
  assert.deepEqual(capture.captures, [{ types: ['screen', 'window'] }]);
  assert.deepEqual(capture.dialogs, []);
  assert.deepEqual(capture.replies, [{ video: portalSource }]);
});

test('waits for portal selection before granting capture', async () => {
  let select;
  const selected = new Promise((resolve) => (select = resolve));
  const capture = setup({ sources: () => selected });
  const pending = capture.run();
  assert.deepEqual(capture.replies, []);
  select([portalSource]);
  await pending;
  assert.deepEqual(capture.replies, [{ video: portalSource }]);
});

test('rejects portal cancellation, unavailable capture, and empty source lists', async () => {
  for (const sources of [
    [],
    () => {
      throw new Error('Portal cancelled or unavailable');
    }
  ]) {
    const capture = setup({ sources });
    await capture.run();
    assert.deepEqual(capture.replies, [null]);
    assert.deepEqual(capture.dialogs, []);
  }
});

test('rejects untrusted origins and missing gestures before querying sources', async () => {
  for (const value of [
    { ...request, securityOrigin: 'https://example.com' },
    { ...request, securityOrigin: 'chatto://desktop.evil' },
    { ...request, userGesture: false }
  ]) {
    const capture = setup();
    await capture.run(value);
    assert.deepEqual(capture.replies, [null]);
    assert.deepEqual(capture.captures, []);
    assert.deepEqual(capture.dialogs, []);
  }
});

test('requires an explicit source choice for named Linux sources and other platforms', async () => {
  for (const platform of ['linux', 'win32', 'darwin']) {
    const capture = setup({ platform, sources: [screen, windowSource], response: 1 });
    await capture.run();
    assert.deepEqual(capture.dialogs[0].buttons, ['Entire screen', 'Example window', 'Cancel']);
    assert.equal(capture.dialogs[0].cancelId, 2);
    assert.equal(capture.dialogs[0].defaultId, 2);
    assert.deepEqual(capture.replies, [{ video: windowSource }]);
  }
});

test('does not bypass the chooser for a single named X11 source', async () => {
  const capture = setup({ sources: [screen], response: 1 });
  await capture.run();
  assert.equal(capture.dialogs.length, 1);
  assert.deepEqual(capture.replies, [null]);
});

test('labels unnamed sources outside the single Linux portal case', async () => {
  for (const platform of ['linux', 'win32', 'darwin']) {
    const capture = setup({
      platform,
      sources: [{ ...screen, name: ' ' }, portalSource],
      response: 2
    });
    await capture.run();
    assert.deepEqual(capture.dialogs[0].buttons, ['Screen 1', 'Window 2', 'Cancel']);
    assert.deepEqual(capture.replies, [null]);
  }
  const capture = setup({ platform: 'win32' });
  await capture.run();
  assert.deepEqual(capture.dialogs[0].buttons, ['Window 1', 'Cancel']);
});
