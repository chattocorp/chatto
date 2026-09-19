import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Code, ConnectError } from '@connectrpc/connect';
import {
  PresenceMode,
  PresencePreference,
  PresenceStatus
} from '@chatto/api-types/api/v1/presence_pb';
import { presencePreferences } from '$lib/state/server/presencePreference.svelte';
import {
  initPresenceTracking,
  refreshPresencePreference,
  setPresenceMode,
  type PresenceReporter
} from './presenceTracking';

const origin = { serverId: 'origin', userId: 'user' };
const remote = { serverId: 'remote', userId: 'user' };
const choice = (mode: PresenceMode, revision = 'one') => new PresencePreference({ mode, revision });
function reporter(
  scope = origin,
  initial: PresencePreference | null = choice(PresenceMode.ONLINE)
) {
  let saved: PresencePreference | undefined = initial ?? undefined;
  return {
    ...scope,
    getPreference: vi.fn(async (): Promise<PresencePreference | undefined> => saved),
    setPreference: vi.fn(async (mode: PresenceMode, revision: string) => {
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
    const api = reporter(origin, choice(PresenceMode.DO_NOT_DISTURB));
    reporters = [api];
    await start();
    expect(presencePreferences.get(origin).mode).toBe('doNotDisturb');
    expect(presencePreferences.get(origin).effectiveStatus).toBe(PresenceStatus.DO_NOT_DISTURB);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(api.setPreference).not.toHaveBeenCalled();
    expect(api.refreshPresence).toHaveBeenCalledTimes(3);
  });

  it('changes only the selected server account', async () => {
    const a = reporter();
    const b = reporter(remote, choice(PresenceMode.INVISIBLE));
    reporters = [a, b];
    await start();
    await setPresenceMode(origin, 'doNotDisturb');
    await settle();
    expect(a.setPreference).toHaveBeenCalledWith(PresenceMode.DO_NOT_DISTURB, 'one');
    expect(b.setPreference).not.toHaveBeenCalled();
    expect(presencePreferences.get(remote).mode).toBe('invisible');
  });

  it('initializes an absent choice from the local invisible preference', async () => {
    presencePreferences.get(origin).select('invisible');
    const api = reporter(origin, null);
    reporters = [api];
    await start();
    expect(api.setPreference).toHaveBeenCalledWith(PresenceMode.INVISIBLE, '');
    expect(presencePreferences.get(origin).mode).toBe('invisible');
  });

  it('preserves legacy local invisible on first migration, then follows other devices', async () => {
    presencePreferences.get(origin).select('invisible');
    const api = reporter();
    reporters = [api];
    await start();
    expect(api.setPreference).toHaveBeenCalledWith(PresenceMode.INVISIBLE, 'one');
    api.getPreference.mockResolvedValue(choice(PresenceMode.ONLINE, 'other-device'));
    api.refreshPresence.mockResolvedValue(choice(PresenceMode.ONLINE, 'other-device'));
    refreshPresencePreference(origin);
    await settle();
    expect(presencePreferences.get(origin).mode).toBe('online');
    expect(api.setPreference).toHaveBeenCalledTimes(1);
  });

  it('never refreshes or initializes after a failed private read', async () => {
    const api = reporter();
    api.getPreference.mockRejectedValue(new Error('offline'));
    reporters = [api];
    await start();
    expect(api.setPreference).not.toHaveBeenCalled();
    expect(api.refreshPresence).not.toHaveBeenCalled();
    await expect(setPresenceMode(origin, 'online')).rejects.toThrow('not ready');
  });

  it('follows another device after migration even when browser storage cannot save', async () => {
    presencePreferences.get(origin).select('invisible');
    vi.spyOn(localStorage, 'setItem').mockImplementation(() => { throw new Error('storage unavailable'); });
    const api = reporter(); reporters = [api]; await start();
    api.getPreference.mockResolvedValue(choice(PresenceMode.ONLINE, 'other-device'));
    api.refreshPresence.mockResolvedValue(choice(PresenceMode.ONLINE, 'other-device'));
    refreshPresencePreference(origin); await settle();
    expect(presencePreferences.get(origin).mode).toBe('online');
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
    await expect(setPresenceMode(origin, 'invisible')).rejects.toThrow('conflict');
    await settle();
    expect(presencePreferences.get(origin).mode).toBe('online');
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
    finish(choice(PresenceMode.ONLINE));
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
    await setPresenceMode(origin, 'invisible');
    await settle();
    finish(choice(PresenceMode.ONLINE));
    await settle();
    expect(presencePreferences.get(origin).mode).toBe('invisible');
  });
});
