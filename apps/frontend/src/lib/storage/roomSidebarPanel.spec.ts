import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { serverStorageKey } from './serverStorage';
import {
  getRoomSidebarPanelState,
  roomSidebarPanelStorageSuffix,
  setRoomSidebarPanelState
} from './roomSidebarPanel';

const storage = new Map<string, string>();
const sessionStorageMock: Storage = {
  getItem: (key) => storage.get(key) ?? null,
  setItem: (key, value) => storage.set(key, value),
  removeItem: (key) => storage.delete(key),
  clear: () => storage.clear(),
  get length() {
    return storage.size;
  },
  key: (index) => [...storage.keys()][index] ?? null
};
beforeEach(() => {
  storage.clear();
  vi.stubGlobal('sessionStorage', sessionStorageMock);
});

afterEach(() => vi.unstubAllGlobals());

describe('room sidebar panel storage', () => {
  it('distinguishes no choice from closed', () => {
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeUndefined();
  });

  it('persists the selected panel per server and room', () => {
    setRoomSidebarPanelState('server-a', 'room-1', 'files');
    setRoomSidebarPanelState('server-a', 'room-2', 'search');
    setRoomSidebarPanelState('server-b', 'room-1', 'call');

    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');
    expect(getRoomSidebarPanelState('server-a', 'room-2')).toBe('search');
    expect(getRoomSidebarPanelState('server-b', 'room-1')).toBe('call');
  });

  it('retains closed state in the page session', () => {
    setRoomSidebarPanelState('server-a', 'room-1', 'files');
    setRoomSidebarPanelState('server-a', 'room-1', null);

    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    expect(sessionStorage.getItem(key)).toBe('closed');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeNull();
  });

  it('ignores legacy local storage choices', () => {
    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    const getItem = vi.fn(() => 'closed');
    vi.stubGlobal('localStorage', { getItem });
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeUndefined();
    expect(getItem).not.toHaveBeenCalled();
    expect(sessionStorage.getItem(key)).toBeNull();
  });

  it('returns to no choice in a fresh page session', () => {
    setRoomSidebarPanelState('server-a', 'room-1', null);
    sessionStorage.clear();
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeUndefined();
  });

  it('handles missing session storage', () => {
    vi.stubGlobal('sessionStorage', undefined);
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeUndefined();
    expect(() => setRoomSidebarPanelState('server-a', 'room-1', 'files')).not.toThrow();
  });

  it('handles storage access and quota failures', () => {
    vi.stubGlobal('sessionStorage', {
      getItem: () => {
        throw new Error('Storage access denied');
      },
      setItem: () => {
        throw new Error('Storage full');
      }
    });
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeUndefined();
    expect(() => setRoomSidebarPanelState('server-a', 'room-1', 'files')).not.toThrow();
  });

  it('treats unknown stored values as no choice', () => {
    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    sessionStorage.setItem(key, 'calendar');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeUndefined();
  });
});

describe('profile preferences', () => {
  it('round-trips profiles with and without a previous panel', () => {
    for (const previousPanel of ['files', null] as const) {
      const preference = { previousPanel } as const;
      setRoomSidebarPanelState('server-a', 'dm-1', preference);
      expect(getRoomSidebarPanelState('server-a', 'dm-1')).toEqual(preference);
    }
  });

  it.each(['{', '{}', 'null', 'profile:', 'profile:unknown', 'profile:files:extra'])(
    'ignores malformed profile state: %s',
    (raw) => {
      sessionStorage.setItem(
        serverStorageKey('server-a', roomSidebarPanelStorageSuffix('dm-1')),
        raw
      );
      expect(getRoomSidebarPanelState('server-a', 'dm-1')).toBeUndefined();
    }
  );
});
