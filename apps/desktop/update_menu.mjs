/** Native update controls; no renderer gets access to the installer or feed configuration. */
import { randomInt } from 'node:crypto';
import { readFile, writeFile, rename } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';
import { app, dialog, Menu, MenuItem, Notification } from 'electron';
import { createDesktopUpdates, createUpdateInstallRequest, updatePreferences } from './updates.mjs';
import { hasWindowsUninstaller } from './update_installation.mjs';

/** Enable only release builds and supported installations. Startup never waits for the feed. */
export async function configureUpdateMenu(packageJson) {
  if (!app.isPackaged || !packageJson.chattoAutoUpdate) return;
  if (process.platform === 'linux' && !process.env.APPIMAGE) return;
  if (process.platform === 'win32' && !hasWindowsUninstaller(process.execPath)) return;

  const settingsPath = path.join(app.getPath('userData'), 'updates.json');
  let stored;
  try {
    stored = JSON.parse(await readFile(settingsPath, 'utf8'));
  } catch {
    // Missing or invalid preferences use the build's channel.
  }
  const preferences = updatePreferences(
    stored,
    randomInt(1_000_000) / 1_000_000,
    packageJson.version
  );
  const save = async (value) => {
    await writeFile(`${settingsPath}.tmp`, JSON.stringify(value), { mode: 0o600 });
    await rename(`${settingsPath}.tmp`, settingsPath);
  };
  await save(preferences);
  const { createUpdater } = createRequire(import.meta.url)('./updater-vendor.cjs');
  const readyItem = new MenuItem({
    label: 'Restart to Update…',
    enabled: false,
    visible: false,
    click: async () => {
      await requestInstall();
      if (['changed', 'error'].includes(updates.snapshot().status))
        await dialog.showMessageBox({
          message: 'Chatto could not install this update. Check for updates again.'
        });
    }
  });
  const updates = createDesktopUpdates({
    updater: createUpdater(packageJson.version),
    preferences,
    save,
    version: packageJson.version,
    notify: async (state, version, isManual) => {
      if (state === 'ready' && !isManual) {
        if (Notification.isSupported())
          new Notification({
            title: 'Chatto update ready',
            body: `Chatto ${version} is ready. Open Updates preferences to restart and install it.`
          }).show();
      }
    }
  });
  const requestInstall = createUpdateInstallRequest(updates, async (version) => {
    const { response } = await dialog.showMessageBox({
      type: 'question',
      message: `Restart Chatto to install ${version}?`,
      detail: 'This ends active calls and screen sharing.',
      buttons: ['Restart', 'Later'],
      defaultId: 1,
      cancelId: 1
    });
    return response === 0;
  });
  const setPreference = async (key, value, item) => {
    try {
      if (!(await updates.setPreference(key, value))) {
        await dialog.showMessageBox({ message: 'Wait for the current update check to finish.' });
      } else {
        readyItem.enabled = false;
        readyItem.visible = false;
      }
    } catch {
      await dialog.showMessageBox({ message: 'Chatto could not save the update settings.' });
    }
    if (key === 'automatic') item.checked = preferences.automatic;
    else
      for (const entry of channelMenu.items)
        entry.checked = entry.label.toLowerCase() === preferences.channel;
  };
  const channelMenu = Menu.buildFromTemplate(
    ['stable', 'beta'].map((channel) => ({
      label: channel === 'stable' ? 'Stable' : 'Beta',
      type: 'radio',
      checked: preferences.channel === channel,
      click: (item) => void setPreference('channel', channel, item)
    }))
  );
  const menu = Menu.getApplicationMenu() ?? new Menu();
  const updatesMenu = new MenuItem({
    label: 'Updates',
    submenu: [
      {
        id: 'check',
        label: 'Check for Updates…',
        click: async () => {
          await updates.check(true);
          const state = updates.snapshot();
          const messages = {
            current: 'No newer update is available on this channel.',
            ready: `Chatto ${state.availableVersion} is ready. Open Updates preferences to restart and install it.`,
            error: 'Chatto could not complete the update. Try again later.'
          };
          if (messages[state.status])
            await dialog.showMessageBox({ message: messages[state.status] });
        }
      },
      readyItem,
      { type: 'separator' },
      {
        id: 'automatic',
        label: 'Automatically Check and Download',
        type: 'checkbox',
        checked: preferences.automatic,
        click: (item) => void setPreference('automatic', item.checked, item)
      },
      {
        label: 'Release Channel',
        submenu: channelMenu
      },
      {
        label: 'About Updates…',
        click: () =>
          void dialog.showMessageBox({
            message: 'Updates are downloaded from GitHub.',
            detail:
              'GitHub receives your IP address and update requests. Chatto sends no account or connected-server information. Stable updates roll out over 36 hours. Manual checks and the Beta channel receive updates immediately. Switching to Stable never installs an older version.'
          })
      }
    ]
  });
  menu.append(updatesMenu);
  updates.subscribe((state) => {
    readyItem.enabled = state.status === 'ready' && !state.busy;
    readyItem.visible = state.status === 'ready';
    if (state.status === 'ready') readyItem.label = `Restart to Install ${state.availableVersion}…`;
    updatesMenu.submenu.getMenuItemById('check').enabled = !state.busy;
    const automatic = updatesMenu.submenu.getMenuItemById('automatic');
    automatic.checked = state.automatic;
    automatic.enabled = !state.busy;
    for (const item of channelMenu.items) {
      item.checked = item.label.toLowerCase() === state.channel;
      item.enabled = !state.busy;
    }
  });
  Menu.setApplicationMenu(menu);
  const initial = setTimeout(() => void updates.check(), 30_000);
  const interval = setInterval(() => void updates.check(), 30 * 60_000);
  initial.unref();
  interval.unref();
  app.once('will-quit', () => {
    clearTimeout(initial);
    clearInterval(interval);
  });
  return { ...updates, install: requestInstall };
}
