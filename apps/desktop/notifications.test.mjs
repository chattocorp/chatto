import { EventEmitter } from 'node:events';
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { configureDesktopNotifications } from './notifications.mjs';

function fixture({ fail = false } = {}) {
  const handlers = new Map();
  const ipcMain = new EventEmitter();
  ipcMain.handle = (channel, handler) => handlers.set(channel, handler);
  const alerts = [];
  const clicks = [];
  class Notification extends EventEmitter {
    static isSupported() {
      return true;
    }
    constructor(options) {
      super();
      this.options = options;
      alerts.push(this);
    }
    show() {
      this.emit(fail ? 'failed' : 'show');
    }
    close() {
      this.closed = true;
      this.emit('close');
    }
  }
  const controller = configureDesktopNotifications({
    ipcMain,
    Notification,
    getWindow: () => ({ webContents: { send: (...args) => clicks.push(args) } }),
    isTrusted: (event) => event.trusted === true,
    open: () => clicks.push('open')
  });
  return {
    alerts,
    clicks,
    controller,
    ipcMain,
    show: (request, event = { trusted: true }) =>
      handlers.get('chatto:notifications:show')(event, request)
  };
}
const request = { id: 'one', title: 'Chatto', body: 'Notifications' };

test('rejects untrusted senders and malformed payloads before showing an alert', () => {
  const f = fixture();
  assert.throws(() => f.show(null, {}), /Untrusted/);
  assert.throws(() => f.show({ ...request, body: 'x'.repeat(1025) }), /Invalid/);
  assert.equal(f.alerts.length, 0);
});

test('retains alerts, deduplicates IDs and routes a click only to the current renderer', async () => {
  const f = fixture();
  await f.show(request);
  await f.show(request);
  assert.equal(f.alerts.length, 1);
  assert.equal(f.alerts[0].options.silent, true);
  f.alerts[0].emit('click');
  assert.deepEqual(f.clicks, [['chatto:notifications:click', 'one'], 'open']);
  assert.equal(f.alerts[0].closed, true);
});

test('privacy cleanup closes an alert and invalidates its click callback', async () => {
  const f = fixture();
  await f.show(request);
  f.ipcMain.emit('chatto:notifications:clear', {}, ['one']);
  assert.equal(f.alerts[0].closed, undefined);
  f.controller.clear();
  f.alerts[0].emit('click');
  assert.equal(f.alerts[0].closed, true);
  assert.deepEqual(f.clicks, []);
});

test('reports native failure without exposing platform error details', async () => {
  const f = fixture({ fail: true });
  await assert.rejects(f.show(request), /operating system/);
  assert.equal(f.alerts[0].closed, true);
});
