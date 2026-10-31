import { describe, expect, it, vi } from 'vitest';
import {
  DesktopUpdates,
  type DesktopUpdateStatus,
  type DesktopUpdatesBridge
} from './updates.svelte';

const initial: DesktopUpdateStatus = {
  revision: 1,
  version: '0.5.0',
  status: 'idle',
  busy: false,
  progress: null,
  availableVersion: null,
  automatic: true,
  channel: 'stable'
};

function fixture() {
  let emit: (value: unknown) => void = () => {};
  const stop = vi.fn();
  const bridge: DesktopUpdatesBridge = {
    getStatus: vi.fn(async () => initial),
    onStatus: (callback) => {
      emit = callback;
      return stop;
    },
    check: vi.fn(async () => {}),
    install: vi.fn(async () => {}),
    setPreference: vi.fn(async () => true)
  };
  return { bridge, stop, emit: (value: unknown) => emit(value) };
}

describe('desktop update state', () => {
  it('keeps the page available for a disabled updater without sending commands', async () => {
    const f = fixture();
    f.bridge.getStatus = async () => ({ ...initial, status: 'unavailable', automatic: false });
    const updates = new DesktopUpdates();
    const dispose = updates.start(f.bridge);
    await expect.poll(() => updates.loading).toBe(false);
    expect(updates.available).toBe(true);
    expect(updates.status?.version).toBe('0.5.0');
    await updates.check();
    await updates.setAutomatic(true);
    await updates.install();
    expect(f.bridge.check).not.toHaveBeenCalled();
    expect(f.bridge.setPreference).not.toHaveBeenCalled();
    expect(f.bridge.install).not.toHaveBeenCalled();
    dispose();
  });
  it('keeps newer events when the initial snapshot arrives late and cleans up on disposal', async () => {
    const f = fixture();
    let read: (value: unknown) => void = () => {};
    f.bridge.getStatus = () =>
      new Promise((resolve) => {
        read = resolve;
      });
    const updates = new DesktopUpdates();
    const dispose = updates.start(f.bridge);
    f.emit({ ...initial, revision: 2, status: 'ready', availableVersion: '0.6.0' });
    read(initial);
    await expect.poll(() => updates.loading).toBe(false);
    expect(updates.status?.status).toBe('ready');
    dispose();
    f.emit({ ...initial, revision: 3 });
    expect(updates.status).toBeNull();
    expect(f.stop).toHaveBeenCalledOnce();
  });

  it('rejects malformed snapshots and supports absent hosts', async () => {
    const updates = new DesktopUpdates();
    updates.start();
    expect(updates.available).toBe(false);
    const f = fixture();
    const dispose = updates.start(f.bridge);
    await expect.poll(() => updates.available).toBe(true);
    f.emit({ ...initial, revision: 2, channel: 'https://evil.test' });
    expect(updates.status?.channel).toBe('stable');
    expect(updates.failed).toBe(true);
    dispose();
  });

  it('serializes frontend commands without changing committed preferences and permits retry', async () => {
    const f = fixture();
    const updates = new DesktopUpdates();
    const dispose = updates.start(f.bridge);
    await expect.poll(() => updates.available).toBe(true);
    let reject: (reason: Error) => void = () => {};
    f.bridge.setPreference = vi.fn(
      () =>
        new Promise((_resolve, fail) => {
          reject = fail;
        })
    );
    const pending = updates.setChannel('beta');
    expect(updates.busy).toBe(true);
    expect(updates.status?.channel).toBe('stable');
    await updates.check();
    expect(f.bridge.check).not.toHaveBeenCalled();
    reject(new Error('private path'));
    await pending;
    expect(updates.failed).toBe(true);
    expect(updates.busy).toBe(false);
    await updates.check();
    expect(f.bridge.check).toHaveBeenCalledOnce();
    dispose();
  });
});
