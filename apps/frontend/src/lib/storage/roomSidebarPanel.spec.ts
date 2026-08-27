import { beforeEach, describe, expect, it, vi } from 'vitest';
import { serverStorageKey } from './serverStorage';
import {
  getRoomSidebarPanelState,
  ROOM_SIDEBAR_DEFAULT_PANEL,
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
  it('defaults to members', () => {
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe(ROOM_SIDEBAR_DEFAULT_PANEL);
  });

  it('persists the selected panel per server and room', () => {
    setRoomSidebarPanelState('server-a', 'room-1', 'files');
    setRoomSidebarPanelState('server-a', 'room-2', 'search');
    setRoomSidebarPanelState('server-b', 'room-1', 'call');

    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');
    expect(getRoomSidebarPanelState('server-a', 'room-2')).toBe('search');
    expect(getRoomSidebarPanelState('server-b', 'room-1')).toBe('call');
  });

  it('does not persist closed state across sessions', () => {
    setRoomSidebarPanelState('server-a', 'room-1', 'files');
    setRoomSidebarPanelState('server-a', 'room-1', null);

    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    expect(localStorage.getItem(key)).toBe('files');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('files');
  });

  it('falls back to members for legacy closed values', () => {
    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    localStorage.setItem(key, 'closed');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('members');
  });

  it('falls back to members for unknown stored values', () => {
    const key = serverStorageKey('server-a', roomSidebarPanelStorageSuffix('room-1'));

    localStorage.setItem(key, 'calendar');
    expect(getRoomSidebarPanelState('server-a', 'room-1')).toBe('members');
  });
});
