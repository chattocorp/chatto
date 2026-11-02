import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import test from 'node:test';
import vm from 'node:vm';
import { transformSync } from 'esbuild';
import { configureUpdateBridge } from './update_bridge.mjs';
import * as updates from './updates.mjs';
import * as notifications from './update_notifications.mjs';

test('preferences install without a second prompt while the native menu still confirms', async () => {
  const dialogs = [];
  let installs = 0;
  let checks = 0;
  let menu;
  const updater = new EventEmitter();
  updater.checkForUpdates = async () => {
    checks++;
    return { isUpdateAvailable: true, updateInfo: { version: '0.6.0' } };
  };
  updater.downloadUpdate = async () => updater.emit('update-downloaded', { version: '0.6.0' });
  updater.quitAndInstall = () => installs++;
  class Menu {
    items = [];
    static buildFromTemplate(items) {
      const result = new Menu();
      result.items = items;
      return result;
    }
    static getApplicationMenu() {
      return null;
    }
    static setApplicationMenu(value) {
      menu = value;
    }
    append(item) {
      this.items.push(item);
    }
    getMenuItemById(id) {
      return this.items.find((item) => item.id === id);
    }
  }
  const electron = {
    app: Object.assign(new EventEmitter(), { isPackaged: true, getPath: () => '/unused' }),
    dialog: {
      showMessageBox: async (options) => {
        dialogs.push(options);
        return { response: 1 };
      }
    },
    Menu,
    MenuItem: class {
      constructor(options) {
        Object.assign(this, options);
        if (Array.isArray(this.submenu)) this.submenu = Menu.buildFromTemplate(this.submenu);
      }
    },
    Notification: { isSupported: () => false }
  };
  // Run the host entry point with native dependencies replaced, without a feed or installer.
  const source = transformSync(
    readFileSync(new URL('./update_menu.mjs', import.meta.url), 'utf8'),
    {
      format: 'cjs',
      define: {
        'import.meta.url': JSON.stringify(new URL('./update_menu.mjs', import.meta.url).href)
      }
    }
  ).code;
  const require = createRequire(import.meta.url);
  const dependencies = {
    electron,
    'node:fs/promises': {
      readFile: async () => '{}',
      writeFile: async () => {},
      rename: async () => {}
    },
    'node:module': { createRequire: () => () => ({ createUpdater: () => updater }) },
    './updates.mjs': updates,
    './update_notifications.mjs': notifications
  };
  const module = { exports: {} };
  vm.runInNewContext(source, {
    module,
    require: (name) => dependencies[name] ?? require(name),
    process: { platform: 'darwin' },
    setTimeout: () => ({ unref() {} }),
    setInterval: () => ({ unref() {} })
  });
  const controller = await module.exports.configureUpdateMenu({
    version: '0.5.0',
    chattoAutoUpdate: true
  });
  await controller.check();
  assert.equal(installs, 0);
  const restart = menu.items[0].submenu.items[1];
  assert.equal(restart.enabled, true);
  await restart.click();
  assert.equal(dialogs.length, 1);
  assert.match(dialogs[0].detail, /active calls and screen sharing/);
  assert.equal(installs, 0);

  const handlers = new Map();
  configureUpdateBridge({
    ipcMain: { handle: (name, handler) => handlers.set(name, handler) },
    controller: Promise.resolve(controller),
    isTrusted: () => true,
    send: () => {}
  });
  await handlers.get('chatto:updates:install')({});
  assert.equal(installs, 1);
  assert.equal(checks, 2);
  assert.equal(dialogs.length, 1);
});
