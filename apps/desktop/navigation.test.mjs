// SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
// SPDX-License-Identifier: Apache-2.0

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';
import { runInNewContext } from 'node:vm';
import { EventEmitter } from 'node:events';
import { configureNativeNavigation } from './native_navigation.mjs';

const preload = readFileSync(new URL('./preload.cjs', import.meta.url), 'utf8');

test('macOS native mouse swipes navigate history and respect its boundaries', () => {
  const window = new EventEmitter();
  let position = 1;
  window.webContents = {
    navigationHistory: {
      canGoBack: () => position > 0,
      canGoForward: () => position < 2,
      goBack: () => position--,
      goForward: () => position++
    }
  };
  configureNativeNavigation(window, 'darwin');
  window.emit('swipe', {}, 'left');
  assert.equal(position, 0);
  window.emit('swipe', {}, 'left');
  assert.equal(position, 0);
  window.emit('swipe', {}, 'right');
  assert.equal(position, 1);
  window.emit('swipe', {}, 'right');
  window.emit('swipe', {}, 'right');
  assert.equal(position, 2);
  for (const direction of ['up', 'down', 'unknown']) window.emit('swipe', {}, direction);
  assert.equal(position, 2);
});

test('native swipe support is limited to macOS', () => {
  for (const platform of ['win32', 'linux']) {
    const window = new EventEmitter();
    configureNativeNavigation(window, platform);
    assert.equal(window.listenerCount('swipe'), 0);
  }
});

/** Run the actual sandboxed preload with a minimal Electron and DOM boundary. */
function loadPreload(platform = 'darwin') {
  const listeners = new Map();
  const navigations = [];
  runInNewContext(preload, {
    require: () => ({ contextBridge: { exposeInMainWorld() {} }, ipcRenderer: { on() {} } }),
    process: { platform, argv: [] },
    window: {
      history: {
        back: () => navigations.push('back'),
        forward: () => navigations.push('forward')
      },
      addEventListener: (type, listener, capture) => {
        assert.equal(capture, true);
        listeners.set(type, listener);
      }
    }
  });
  return {
    navigations,
    dispatch(type, values) {
      const result = { prevented: false, stopped: false };
      listeners.get(type)({
        type,
        ...values,
        preventDefault: () => (result.prevented = true),
        stopImmediatePropagation: () => (result.stopped = true)
      });
      return result;
    }
  };
}

test('side buttons navigate once per click and consume both press and release', () => {
  const app = loadPreload();
  for (const button of [3, 4]) {
    for (const type of ['mousedown', 'mouseup']) {
      assert.deepEqual(app.dispatch(type, { button }), { prevented: true, stopped: true });
    }
  }
  assert.deepEqual(app.navigations, ['back', 'forward']);
});

test('ordinary mouse buttons remain available to the page', () => {
  const app = loadPreload();
  for (const button of [0, 1, 2]) {
    for (const type of ['mousedown', 'mouseup']) {
      assert.deepEqual(app.dispatch(type, { button }), { prevented: false, stopped: false });
    }
  }
  assert.deepEqual(app.navigations, []);
});

test('navigation shortcuts support mouse drivers on each platform', () => {
  for (const platform of ['darwin', 'win32', 'linux']) {
    const app = loadPreload(platform);
    for (const key of ['ArrowLeft', 'ArrowRight']) {
      assert.deepEqual(app.dispatch('keydown', { key, altKey: true }), {
        prevented: platform !== 'darwin',
        stopped: platform !== 'darwin'
      });
    }
    assert.deepEqual(app.navigations, platform === 'darwin' ? [] : ['back', 'forward']);
    for (const key of ['[', ']']) app.dispatch('keydown', { key, metaKey: true });
    assert.deepEqual(app.navigations, ['back', 'forward']);
  }
});

test('typing and modified shortcuts remain available to the page', () => {
  const app = loadPreload();
  for (const input of [
    { key: '[' },
    { key: 'ArrowLeft' },
    { key: '[', metaKey: true, shiftKey: true },
    { key: 'ArrowLeft', altKey: true, ctrlKey: true },
    { key: 'ArrowRight', altKey: true, metaKey: true }
  ]) {
    assert.deepEqual(app.dispatch('keydown', input), { prevented: false, stopped: false });
  }
  assert.deepEqual(app.navigations, []);
});
