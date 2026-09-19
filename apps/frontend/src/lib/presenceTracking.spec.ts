import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { APIPresenceStatus } from '$lib/api-client/presence';
import { PresenceStatus } from '@chatto/api-types/api/v1/presence_pb';
import {
  LEGACY_PRESENCE_MODE_STORAGE_KEY,
  presencePreferences
} from '$lib/state/server/presencePreference.svelte';
import { initPresenceTracking, setPresenceMode, type PresenceReporter } from './presenceTracking';

const origin = { serverId: 'origin', userId: 'same-user-id' };
const remote = { serverId: 'remote', userId: 'same-user-id' };
const report = () => vi.fn((status: APIPresenceStatus) => Promise.resolve(status));
let originReport: ReturnType<typeof report>;
let remoteReport: ReturnType<typeof report>;
let reporters: PresenceReporter[];
let tracking: ReturnType<typeof initPresenceTracking>;
let windowTarget: EventTarget;

function start() {
  tracking = initPresenceTracking(() => reporters);
  tracking.sync();
}

function storageSelection(scope: typeof origin, mode: string) {
  const key = presencePreferences.get(scope).slot.key;
  localStorage.setItem(key, mode);
  const event = new Event('storage');
  Object.defineProperties(event, { key: { value: key }, newValue: { value: mode } });
  windowTarget.dispatchEvent(event);
}

describe('per-account presence tracking', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    presencePreferences.clear();
    const storage = new Map<string, string>();
    vi.stubGlobal('localStorage', {
      getItem: vi.fn((key: string) => storage.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => {
        storage.set(key, value);
      }),
      removeItem: vi.fn((key: string) => {
        storage.delete(key);
      })
    });
    windowTarget = new EventTarget();
    vi.stubGlobal('window', {
      addEventListener: windowTarget.addEventListener.bind(windowTarget),
      removeEventListener: windowTarget.removeEventListener.bind(windowTarget)
    });
    originReport = report();
    remoteReport = report();
    reporters = [
      { ...origin, setPresence: originReport },
      { ...remote, setPresence: remoteReport }
    ];
  });

  afterEach(() => {
    tracking?.stop();
    presencePreferences.clear();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('reports each default and refreshes explicit choices without idle changes', () => {
    start();
    expect(originReport).toHaveBeenCalledWith(APIPresenceStatus.ONLINE, true);
    expect(remoteReport).toHaveBeenCalledWith(APIPresenceStatus.ONLINE, true);
    setPresenceMode(origin, 'away');
    vi.advanceTimersByTime(60_000);
    expect(originReport.mock.calls.map(([status]) => status)).toEqual([
      APIPresenceStatus.ONLINE,
      APIPresenceStatus.AWAY,
      APIPresenceStatus.AWAY,
      APIPresenceStatus.AWAY
    ]);
    expect(remoteReport.mock.calls.map(([status]) => status)).toEqual([
      APIPresenceStatus.ONLINE,
      APIPresenceStatus.ONLINE,
      APIPresenceStatus.ONLINE
    ]);
  });

  it('never reports to an invisible server when another server changes, including after reload', () => {
    setPresenceMode(remote, 'invisible');
    presencePreferences.clear();
    start();
    setPresenceMode(origin, 'doNotDisturb');
    vi.advanceTimersByTime(60_000);
    expect(remoteReport).not.toHaveBeenCalled();
    expect(presencePreferences.get(remote).mode).toBe('invisible');
    expect(presencePreferences.get(remote).effectiveStatus).toBe(PresenceStatus.OFFLINE);
    expect(originReport).toHaveBeenLastCalledWith(APIPresenceStatus.DO_NOT_DISTURB, true);

    tracking.stop();
    start();
    expect(remoteReport).not.toHaveBeenCalled();
    expect(originReport).toHaveBeenLastCalledWith(APIPresenceStatus.DO_NOT_DISTURB, true);
  });

  it.each(['invisible', 'away', 'doNotDisturb'] as const)(
    'migrates the legacy %s choice independently',
    (mode) => {
      localStorage.setItem(LEGACY_PRESENCE_MODE_STORAGE_KEY, mode);
      start();
      expect(presencePreferences.get(origin).mode).toBe(mode);
      expect(presencePreferences.get(remote).mode).toBe(mode);
      if (mode === 'invisible') {
        expect(originReport).not.toHaveBeenCalled();
        expect(remoteReport).not.toHaveBeenCalled();
      }
      setPresenceMode(origin, 'online');
      expect(presencePreferences.get(remote).mode).toBe(mode);
      expect(localStorage.getItem(LEGACY_PRESENCE_MODE_STORAGE_KEY)).toBe(mode);
      tracking.stop();
      start();
      expect(presencePreferences.get(origin).mode).toBe('online');
      expect(presencePreferences.get(remote).mode).toBe(mode);
    }
  );

  it('normalizes the retired auto choice to online', () => {
    localStorage.setItem(LEGACY_PRESENCE_MODE_STORAGE_KEY, 'auto');
    start();
    expect(originReport).toHaveBeenCalledWith(APIPresenceStatus.ONLINE, true);
  });

  it('does not expose accounts when saved preferences cannot be read', () => {
    vi.mocked(localStorage.getItem).mockImplementation(() => {
      throw new Error('Unavailable');
    });
    start();
    vi.advanceTimersByTime(60_000);
    expect(originReport).not.toHaveBeenCalled();
    expect(remoteReport).not.toHaveBeenCalled();
  });

  it('does not turn a corrupt account preference into an online report', () => {
    const key = presencePreferences.get(remote).slot.key;
    localStorage.setItem(key, 'invalid');
    presencePreferences.clear();
    start();
    expect(remoteReport).not.toHaveBeenCalled();
    expect(originReport).toHaveBeenCalledWith(APIPresenceStatus.ONLINE, true);
  });

  it('applies cross-tab choices only to the matching account without echo writes', () => {
    start();
    originReport.mockClear();
    remoteReport.mockClear();
    vi.mocked(localStorage.setItem).mockClear();
    storageSelection(remote, 'away');
    expect(originReport).not.toHaveBeenCalled();
    expect(remoteReport).toHaveBeenCalledWith(APIPresenceStatus.AWAY, true);
    expect(localStorage.setItem).toHaveBeenCalledTimes(1);
    storageSelection(remote, 'invisible');
    vi.advanceTimersByTime(60_000);
    expect(remoteReport).toHaveBeenCalledTimes(1);
    expect(presencePreferences.get(origin).mode).toBe('online');
  });

  it('keeps different accounts on the same server separate and restores a returning account', () => {
    start();
    setPresenceMode(origin, 'invisible');
    const other = { ...origin, userId: 'other-user' };
    reporters = [{ ...other, setPresence: originReport }];
    tracking.sync();
    expect(presencePreferences.get(other).mode).toBe('online');
    originReport.mockClear();
    reporters = [{ ...origin, setPresence: originReport }];
    tracking.sync();
    vi.advanceTimersByTime(30_000);
    expect(originReport).not.toHaveBeenCalled();
  });

  it('hydrates accounts added later before their first report and stops signed-out reporters', () => {
    setPresenceMode(remote, 'invisible');
    reporters = [{ ...origin, setPresence: originReport }];
    start();
    reporters.push({ ...remote, setPresence: remoteReport });
    tracking.sync();
    expect(remoteReport).not.toHaveBeenCalled();
    reporters = [];
    originReport.mockClear();
    vi.advanceTimersByTime(60_000);
    expect(originReport).not.toHaveBeenCalled();
  });

  it('reloads a cross-tab preference changed while the account was signed out', () => {
    start();
    reporters = [];
    tracking.sync();
    storageSelection(remote, 'invisible');
    reporters = [{ ...remote, setPresence: remoteReport }];
    remoteReport.mockClear();
    tracking.sync();
    expect(remoteReport).not.toHaveBeenCalled();
  });

  it('isolates accepted statuses and ignores responses to superseded choices', async () => {
    const pending = Promise.withResolvers<APIPresenceStatus>();
    originReport.mockImplementationOnce(() => pending.promise);
    remoteReport.mockResolvedValueOnce(APIPresenceStatus.AWAY);
    start();
    await Promise.resolve();
    expect(presencePreferences.get(remote).effectiveStatus).toBe(PresenceStatus.AWAY);
    expect(presencePreferences.get(origin).effectiveStatus).toBe(PresenceStatus.ONLINE);
    setPresenceMode(origin, 'invisible');
    pending.resolve(APIPresenceStatus.DO_NOT_DISTURB);
    await Promise.resolve();
    expect(presencePreferences.get(origin).effectiveStatus).toBe(PresenceStatus.OFFLINE);
    expect(presencePreferences.get(remote).effectiveStatus).toBe(PresenceStatus.AWAY);
  });

  it('ignores late responses after an account is removed or tracking stops', async () => {
    const pending = Promise.withResolvers<APIPresenceStatus>();
    originReport.mockImplementationOnce(() => pending.promise);
    start();
    const preference = presencePreferences.get(origin);
    reporters = [];
    pending.resolve(APIPresenceStatus.AWAY);
    await Promise.resolve();
    expect(preference.effectiveStatus).toBe(PresenceStatus.ONLINE);
    tracking.stop();
    originReport.mockClear();
    vi.advanceTimersByTime(60_000);
    expect(originReport).not.toHaveBeenCalled();
  });
});
