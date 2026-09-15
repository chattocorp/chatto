import { beforeEach, describe, expect, it, vi } from 'vitest';
import { serverStorageKey } from './serverStorage';
import {
  getRoomSidebarPanelState,
  roomSidebarPanelStorageSuffix,
  setRoomSidebarPanelState
} from './roomSidebarPanel';

const storage = new Map<string, string>();
const localStorageMock: Storage = {
  getItem: (key) => storage.get(key) ?? null,
  setItem: (key, value) => storage.set(key, value),
  removeItem: (key) => storage.delete(key),
  clear: () => storage.clear(),
  get length() {
    return storage.size;
  },
  key: (index) => [...storage.keys()][index] ?? null
};
vi.stubGlobal('localStorage', localStorageMock);

beforeEach(() => {
  storage.clear();
});

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

  it('persists closed state across sessions', () => {
    setRoomSidebarPanelState('server-a', 'room-1', 'files');
    setRoomSidebarPanelState('server-a', 'room-1', null);

    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    expect(localStorage.getItem(key)).toBe('closed');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeNull();
  });

  it('accepts legacy closed values', () => {
    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    localStorage.setItem(key, 'closed');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeNull();
  });

  it('treats unknown stored values as no choice', () => {
    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    localStorage.setItem(key, 'calendar');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBeUndefined();
  });
});

describe('profile preferences', () => {
  it('round-trips profiles with and without a previous panel', () => {
    for (const previousPanel of ['files', null] as const) {
      const preference = { view: 'profile', previousPanel } as const;
      setRoomSidebarPanelState('server-a', 'dm-1', preference);
      expect(getRoomSidebarPanelState('server-a', 'dm-1')).toEqual(preference);
    }
  });

  it.each(['{', '{}', 'null', '{"view":"profile","previousPanel":"unknown"}'])(
    'ignores malformed profile state: %s',
    (raw) => {
      localStorage.setItem(
        serverStorageKey('server-a', roomSidebarPanelStorageSuffix('dm-1')),
        raw
      );
      expect(getRoomSidebarPanelState('server-a', 'dm-1')).toBeUndefined();
    }
  );
});
