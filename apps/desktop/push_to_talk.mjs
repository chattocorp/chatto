/** A trusted call owns one native registration. EOF or helper failure ends a hold. */
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { validSystemChord } from './keyboard_shortcuts.mjs';

export function configurePushToTalk({
  ipcMain,
  contents,
  isTrusted,
  executable,
  appId,
  powerMonitor,
  launch = spawn
}) {
  let owner;
  let stopped = Promise.resolve();
  const blocked = new Set();

  function send(binding, edge) {
    if (owner === binding && !contents.isDestroyed())
      contents.send('chatto:ptt:edge', binding.token, edge);
  }

  function stopHelper() {
    if (!owner?.child) return;
    send(owner, 'up');
    const child = owner.child;
    owner.child = null;
    child.kill();
  }

  function clear() {
    stopHelper();
    owner = undefined;
  }

  async function start(binding) {
    await stopped;
    if (owner !== binding || binding.child || blocked.size) return;
    const parts = binding.chord.split('+');
    const key = parts.pop();
    const mask = ['Control', 'Super', 'Alt', 'Shift'].reduce(
      (bits, modifier, index) => (parts.includes(modifier) ? bits | (1 << index) : bits),
      0
    );
    const args = [String(mask), key];
    if (process.platform === 'linux') args.push(appId);
    const child = launch(executable, args, {
      stdio: ['pipe', 'pipe', 'ignore'],
      windowsHide: true
    });
    binding.child = child;
    let held = false;
    let ready = false;
    const lines = createInterface({ input: child.stdout });
    lines.on('line', (line) => {
      if (binding.child !== child || owner !== binding) return;
      if (line === 'ready') ready = true;
      if (ready && (line === 'down' || line === 'up') && held !== (line === 'down')) {
        held = line === 'down';
        send(binding, line);
      }
    });
    child.on('error', () => {}); // Node also emits close for a failed spawn.
    stopped = new Promise((resolve) =>
      child.once('close', () => {
        lines.close();
        if (binding.child === child) {
          binding.child = null;
          send(binding, 'failed');
        }
        resolve();
      })
    );
  }

  ipcMain.handle('chatto:ptt:set', (event, chord, token) => {
    if (!isTrusted(event) || typeof token !== 'string') return false;
    if (chord === '') {
      if (owner?.token === token) clear();
      return true;
    }
    if (typeof chord !== 'string') return false;
    const accelerator =
      process.platform === 'darwin' ? chord.replace(/\+@([A-Z0-9/,])$/, '+$1') : chord;
    if (!validSystemChord.test(accelerator)) return false;
    clear();
    owner = { chord, token, child: null };
    const binding = owner;
    void start(binding).catch(() => send(binding, 'failed'));
    return true;
  });
  for (const [off, on] of [
    ['suspend', 'resume'],
    ['lock-screen', 'unlock-screen']
  ]) {
    powerMonitor.on(off, () => {
      blocked.add(off);
      stopHelper();
    });
    powerMonitor.on(on, () => {
      if (!blocked.delete(off) || !owner || blocked.size) return;
      const binding = owner;
      void start(binding).catch(() => send(binding, 'failed'));
    });
  }
  contents.on('render-process-gone', clear);
  contents.on('destroyed', clear);
  contents.on('did-start-navigation', ({ isMainFrame, isSameDocument }) => {
    if (isMainFrame && !isSameDocument) clear();
  });
}
