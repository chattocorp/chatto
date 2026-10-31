import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';
import { configureUpdateBridge } from './update_bridge.mjs';

test('update IPC rejects untrusted callers before and after async setup', async () => {
  const handlers = new Map();
  let resolve;
  let trusted = true;
  let calls = 0;
  configureUpdateBridge({
    ipcMain: { handle: (name, fn) => handlers.set(name, fn) },
    controller: new Promise((done) => {
      resolve = done;
    }),
    isTrusted: () => trusted,
    send: () => {}
  });
  const pending = handlers.get('chatto:updates:check')({});
  trusted = false;
  for (const handler of handlers.values()) await assert.rejects(handler({}), /only to/);
  resolve({ check: () => calls++, subscribe: () => {} });
  await assert.rejects(pending, /expired/);
  assert.equal(calls, 0);
});

test('unsupported installations return null and operation errors are sanitized', async () => {
  for (const controller of [
    null,
    {
      subscribe: () => {},
      snapshot: () => {
        throw new Error('private');
      }
    }
  ]) {
    const handlers = new Map();
    configureUpdateBridge({
      ipcMain: { handle: (name, fn) => handlers.set(name, fn) },
      controller: Promise.resolve(controller),
      isTrusted: () => true,
      send: () => {}
    });
    const status = handlers.get('chatto:updates:status');
    if (controller) await assert.rejects(status({}), /^Error: The update operation failed\.$/);
    else assert.equal(await status({}), null);
  }
});

test('preload strips IPC events, sends only narrow commands, and removes subscriptions', async () => {
  let bridge;
  const listeners = new Map();
  const calls = [];
  const ipcRenderer = {
    on: (name, fn) => listeners.set(name, fn),
    removeListener: (name, fn) => {
      assert.equal(listeners.get(name), fn);
      listeners.delete(name);
    },
    invoke: async (...args) => calls.push(args)
  };
  vm.runInNewContext(readFileSync(new URL('./preload.cjs', import.meta.url), 'utf8'), {
    require: () => ({
      contextBridge: {
        exposeInMainWorld: (_name, value) => {
          bridge = value;
        }
      },
      ipcRenderer
    }),
    window: { addEventListener: () => {} },
    process: { argv: [], platform: 'linux' }
  });
  let received;
  const stop = bridge.updates.onStatus((value) => {
    received = value;
  });
  const state = { status: 'ready' };
  listeners.get('chatto:updates:changed')({ secret: true }, state);
  assert.equal(received, state);
  await bridge.updates.check();
  await bridge.updates.install();
  await bridge.updates.getStatus();
  assert.deepEqual(
    calls.map((call) => call[0]),
    ['chatto:updates:check', 'chatto:updates:install', 'chatto:updates:status']
  );
  stop();
  assert.equal(listeners.has('chatto:updates:changed'), false);
});
