import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { test } from 'node:test';
import { configureMuteShortcut, protectReloadShortcuts } from './keyboard_shortcuts.mjs';

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

test('mute binding follows its trusted document and ignores late releases', () => {
  const contents = new EventEmitter();
  const sent = [];
  contents.isDestroyed = () => false;
  contents.send = (...message) => sent.push(message);
  const callbacks = new Map();
  let set;
  configureMuteShortcut({
    contents,
    isTrusted: (sender) => sender === 'app',
    ipcMain: {
      handle: (_channel, handler) => {
        set = handler;
      }
    },
    globalShortcut: {
      register: (chord, cb) => {
        if (chord === 'Alt+X') return false;
        callbacks.set(chord, cb);
        return true;
      },
      unregister: (chord) => callbacks.delete(chord)
    }
  });
  assert.equal(set('app', 'M', '1'), false);
  assert.equal(set('app', 'Alt+X', '1'), false);
  assert.equal(set('app', 'Alt+M', '1'), true);
  const previous = callbacks.get('Alt+M');
  assert.equal(set('popup', '', '1'), false);
  previous();
  assert.equal(set('app', 'Alt+M', '2'), true);
  set('app', '', '1');
  previous();
  callbacks.get('Alt+M')();
  assert.deepEqual(
    sent.map(([, token]) => token),
    ['1', '2']
  );
  contents.emit('did-start-navigation', { isSameDocument: false, isMainFrame: true });
  assert.equal(callbacks.size, 0);
});

test('checking a Wayland chord leaves portal registration to the call', () => {
  const handlers = new Map();
  const registrations = [];
  configureMuteShortcut({
    contents: new EventEmitter(),
    isTrusted: () => true,
    wayland: true,
    ipcMain: { handle: (channel, handler) => handlers.set(channel, handler) },
    globalShortcut: {
      register: (chord) => {
        registrations.push(chord);
        return true;
      },
      unregister: () => {}
    }
  });
  const available = handlers.get('chatto:mute-shortcut:available');
  assert.equal(available({}, 'M'), false);
  assert.equal(available({}, 'Control+Alt+M'), true);
  assert.deepEqual(registrations, []);
  assert.equal(handlers.get('chatto:mute-shortcut:set')({}, 'Control+Alt+M', 'call'), true);
  assert.deepEqual(registrations, ['Control+Alt+M']);
});
