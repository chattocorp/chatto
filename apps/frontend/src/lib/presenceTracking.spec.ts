import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import { PresencePreference, PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import { presencePreferences } from '$lib/state/server/presencePreference.svelte';
import {
  initPresenceTracking,
  refreshPresencePreference,
  setPresenceStatus,
  type PresenceReporter
} from './presenceTracking';

const origin = { serverId: 'origin', userId: 'user' };
const remote = { serverId: 'remote', userId: 'user' };
const choice = (mode: PresenceStatus, revision = 'one') =>
  new PresencePreference({ status: mode, revision });
function reporter(
  scope = origin,
  initial: PresencePreference | null = choice(PresenceStatus.ONLINE)
) {
  let saved: PresencePreference | undefined = initial ?? undefined;
  return {
    ...scope,
    getPreference: vi.fn(async (): Promise<PresencePreference | undefined> => saved),
    setPreference: vi.fn(async (mode: PresenceStatus, revision: string) => {
      if (revision !== (saved?.revision ?? '')) throw new Error('conflict');
      saved = choice(mode, `${revision}-next`);
      return saved;
    }),
    refreshPresence: vi.fn(async () => saved)
  };
}
let reporters: PresenceReporter[];
let tracking: ReturnType<typeof initPresenceTracking>;
async function settle() {
  for (let i = 0; i < 12; i++) await Promise.resolve();
}
async function start() {
  tracking = initPresenceTracking(() => reporters);
  tracking.sync();
  await settle();
}

describe('shared account presence', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    const storage = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key)
    });
    presencePreferences.clear();
  });
  afterEach(() => {
    tracking?.stop();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('loads shared DND before refreshing and does not write on heartbeats', async () => {
    const api = reporter(origin, choice(PresenceStatus.DO_NOT_DISTURB));
    reporters = [api];
    await start();
    expect(presencePreferences.get(origin).status).toBe(PresenceStatus.DO_NOT_DISTURB);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(api.setPreference).not.toHaveBeenCalled();
    expect(api.refreshPresence).toHaveBeenCalledTimes(3);
    expect(api.getPreference).toHaveBeenCalledOnce();
  });

  it('recovers a missed device update from a heartbeat without another read', async () => {
    const api = reporter();
    reporters = [api];
    await start();
    api.refreshPresence.mockResolvedValue(choice(PresenceStatus.OFFLINE, 'other-device'));
    await vi.advanceTimersByTimeAsync(30_000);
    expect(presencePreferences.get(origin).status).toBe(PresenceStatus.OFFLINE);
    expect(api.getPreference).toHaveBeenCalledOnce();
    expect(api.setPreference).not.toHaveBeenCalled();
  });

  it('reads before heartbeats when an authenticated account reconnects', async () => {
    const api = reporter();
    reporters = [api];
    await start();
    reporters = [];
    tracking.sync();
    api.getPreference.mockRejectedValue(new Error('unavailable'));
    api.refreshPresence.mockClear();
    reporters = [api];
    tracking.sync();
    await settle();
    expect(api.getPreference).toHaveBeenCalledTimes(2);
    expect(api.refreshPresence).not.toHaveBeenCalled();
    expect(presencePreferences.get(origin).ready).toBe(false);
  });

  it('reinitializes through a fresh read if a heartbeat finds no saved choice', async () => {
    const api = reporter();
    reporters = [api];
    await start();
    api.refreshPresence.mockResolvedValueOnce(undefined);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(presencePreferences.get(origin).ready).toBe(false);
    await expect(setPresenceStatus(origin, PresenceStatus.ONLINE)).rejects.toThrow('not ready');
    api.getPreference.mockResolvedValueOnce(undefined);
    api.setPreference.mockResolvedValueOnce(choice(PresenceStatus.ONLINE, 'restored'));
    await vi.advanceTimersByTimeAsync(30_000);
    expect(api.getPreference).toHaveBeenCalledTimes(2);
    expect(api.setPreference).toHaveBeenCalledWith(PresenceStatus.ONLINE, '');
    expect(presencePreferences.get(origin).ready).toBe(true);
  });

  it.each([
    ['online', PresenceStatus.ONLINE],
    ['away', PresenceStatus.AWAY],
    ['doNotDisturb', PresenceStatus.DO_NOT_DISTURB],
    ['invisible', PresenceStatus.OFFLINE],
    ['auto', PresenceStatus.ONLINE],
    ['invalid', PresenceStatus.OFFLINE]
  ] as const)('migrates the legacy %s value before the first heartbeat', async (raw, status) => {
    localStorage.setItem('chatto.presence.mode', raw);
    const api = reporter(origin, null);
    reporters = [api];
    await start();
    expect(presencePreferences.get(origin).status).toBe(status);
    expect(api.setPreference).toHaveBeenCalledWith(status, '');
    expect(api.refreshPresence).toHaveBeenCalledOnce();
    expect(api.setPreference.mock.invocationCallOrder[0]).toBeLessThan(
      api.refreshPresence.mock.invocationCallOrder[0]
    );
  });

  it('changes only the selected server account', async () => {
    const a = reporter();
    const b = reporter(remote, choice(PresenceStatus.OFFLINE));
    reporters = [a, b];
    await start();
    await setPresenceStatus(origin, PresenceStatus.DO_NOT_DISTURB);
    await settle();
    expect(a.setPreference).toHaveBeenCalledWith(PresenceStatus.DO_NOT_DISTURB, 'one');
    expect(b.setPreference).not.toHaveBeenCalled();
    expect(presencePreferences.get(remote).status).toBe(PresenceStatus.OFFLINE);
  });

  it('initializes an absent choice from the local invisible preference', async () => {
    const key = presencePreferences.get(origin).slot.key;
    localStorage.setItem(key, 'invisible');
    presencePreferences.clear();
    const api = reporter(origin, null);
    reporters = [api];
    await start();
    expect(api.setPreference).toHaveBeenCalledWith(PresenceStatus.OFFLINE, '');
    expect(presencePreferences.get(origin).status).toBe(PresenceStatus.OFFLINE);
  });

  it('preserves legacy local invisible on first migration, then follows other devices', async () => {
    localStorage.setItem('chatto.presence.mode', 'invisible');
    const api = reporter();
    reporters = [api];
    await start();
    expect(api.setPreference).toHaveBeenCalledWith(PresenceStatus.OFFLINE, 'one');
    api.getPreference.mockResolvedValue(choice(PresenceStatus.ONLINE, 'other-device'));
    api.refreshPresence.mockResolvedValue(choice(PresenceStatus.ONLINE, 'other-device'));
    refreshPresencePreference(origin);
    await settle();
    expect(presencePreferences.get(origin).status).toBe(PresenceStatus.ONLINE);
    expect(api.setPreference).toHaveBeenCalledTimes(1);
  });

  it('never refreshes or initializes after a failed private read', async () => {
    const api = reporter();
    api.getPreference.mockRejectedValue(new Error('offline'));
    reporters = [api];
    await start();
    expect(api.setPreference).not.toHaveBeenCalled();
    expect(api.refreshPresence).not.toHaveBeenCalled();
    await expect(setPresenceStatus(origin, PresenceStatus.ONLINE)).rejects.toThrow('not ready');
  });

  it('follows another device after migration even when browser storage cannot save', async () => {
    localStorage.setItem('chatto.presence.mode', 'invisible');
    vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
      throw new Error('storage unavailable');
    });
    const api = reporter();
    reporters = [api];
    await start();
    api.getPreference.mockResolvedValue(choice(PresenceStatus.ONLINE, 'other-device'));
    api.refreshPresence.mockResolvedValue(choice(PresenceStatus.ONLINE, 'other-device'));
    refreshPresencePreference(origin);
    await settle();
    expect(presencePreferences.get(origin).status).toBe(PresenceStatus.ONLINE);
    expect(api.setPreference).toHaveBeenCalledTimes(1);
  });

  it('rereads promptly when permission recovery discards an initial response', async () => {
    const api = reporter();
    api.getPreference.mockRejectedValueOnce(new ConnectError('permission reset', Code.Canceled));
    reporters = [api];
    await start();
    expect(presencePreferences.get(origin).ready).toBe(true);
    expect(api.refreshPresence).toHaveBeenCalledOnce();
  });

  it('recovers immediately when another device initializes the choice first', async () => {
    const api = reporter();
    api.getPreference.mockResolvedValueOnce(undefined);
    api.setPreference.mockRejectedValueOnce(new ConnectError('conflict', Code.Aborted));
    reporters = [api];
    await start();
    expect(presencePreferences.get(origin).ready).toBe(true);
    expect(api.refreshPresence).toHaveBeenCalledOnce();
  });

  it('rejects failed saves without claiming success and recovers the shared choice', async () => {
    const api = reporter();
    reporters = [api];
    await start();
    api.setPreference.mockRejectedValue(new Error('conflict'));
    await expect(setPresenceStatus(origin, PresenceStatus.OFFLINE)).rejects.toThrow('conflict');
    await settle();
    expect(presencePreferences.get(origin).status).toBe(PresenceStatus.ONLINE);
  });

  it('discards a late reply after authentication is removed', async () => {
    const api = reporter();
    let finish!: (p: PresencePreference) => void;
    api.getPreference.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    reporters = [api];
    await start();
    reporters = [];
    tracking.sync();
    finish(choice(PresenceStatus.ONLINE));
    await settle();
    expect(api.refreshPresence).not.toHaveBeenCalled();
    expect(presencePreferences.get(origin).ready).toBe(false);
  });

  it('discards an old refresh after a newer explicit choice', async () => {
    const api = reporter();
    reporters = [api];
    await start();
    let finish!: (p: PresencePreference) => void;
    api.refreshPresence.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        })
    );
    refreshPresencePreference(origin);
    await settle();
    await setPresenceStatus(origin, PresenceStatus.OFFLINE);
    await settle();
    finish(choice(PresenceStatus.ONLINE));
    await settle();
    expect(presencePreferences.get(origin).status).toBe(PresenceStatus.OFFLINE);
  });
});
