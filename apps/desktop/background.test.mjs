import { EventEmitter } from 'node:events';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { configureBackgroundWindow } from './background.mjs';

function fixture(platform) {
  const app = new EventEmitter();
  const window = new EventEmitter();
  const calls = [];
  app.show = () => calls.push('show-app');
  Object.assign(window, {
    hide: () => calls.push('hide'),
    show: () => calls.push('show'),
    focus: () => calls.push('focus'),
    restore: () => calls.push('restore'),
    isMinimized: () => true
  });
  configureBackgroundWindow({ app, window, platform });
  return { app, window, calls };
}

test('macOS window close hides, Dock activation restores, and normal quit permits closing', () => {
  const { app, window, calls } = fixture('darwin');
  window.emit('close', { preventDefault: () => calls.push('prevent') });
  assert.deepEqual(calls, ['prevent', 'hide']);
  app.emit('activate');
  assert.deepEqual(calls.slice(2), ['show-app', 'restore', 'show', 'focus']);
  app.emit('before-quit', { preventDefault: () => assert.fail('Normal quit must not be blocked') });
  window.emit('close', { preventDefault: () => assert.fail('Quit must close the window') });
});

test('Windows and Linux window close is not intercepted without a tray', () => {
  for (const platform of ['win32', 'linux']) {
    const { window, calls } = fixture(platform);
    window.emit('close', { preventDefault: () => assert.fail('Window must close') });
    assert.deepEqual(calls, []);
  }
});
