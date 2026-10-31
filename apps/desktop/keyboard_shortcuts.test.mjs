import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { test } from 'node:test';
import { protectReloadShortcuts } from './keyboard_shortcuts.mjs';

test('production windows block reloads and hard reloads before menu dispatch', () => {
  const contents = new EventEmitter();
  protectReloadShortcuts(contents, true);
  for (const input of [
    { key: 'r', meta: true },
    { key: 'r', control: true },
    { key: 'R', meta: true, shift: true },
    { key: 'R', control: true, shift: true },
    { key: 'F5' },
    { key: 'F5', control: true },
    { key: 'F5', shift: true }
  ]) {
    let blocked = false;
    contents.emit(
      'before-input-event',
      {
        preventDefault: () => {
          blocked = true;
        }
      },
      { type: 'keyDown', ...input }
    );
    assert.equal(blocked, true);
  }
  for (const input of [
    { key: 'r' },
    { key: 'k', meta: true },
    { key: 'r', alt: true, control: true },
    { key: 'F5', type: 'keyUp' }
  ]) {
    contents.emit(
      'before-input-event',
      { preventDefault: () => assert.fail('Unrelated shortcut blocked') },
      { type: 'keyDown', ...input }
    );
  }
});

test('development windows retain reload shortcuts', () => {
  const contents = new EventEmitter();
  protectReloadShortcuts(contents, false);
  assert.equal(contents.listenerCount('before-input-event'), 0);
});
