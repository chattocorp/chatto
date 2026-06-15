import { beforeEach, describe, expect, it, vi } from 'vitest';
import { globalSlot } from './slot';
import {
  getMainSidebarOpen,
  MAIN_SIDEBAR_DEFAULT_OPEN,
  setMainSidebarOpen
} from './mainSidebarOpen';

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

describe('main sidebar open storage', () => {
  it('defaults to open', () => {
    expect(getMainSidebarOpen()).toBe(MAIN_SIDEBAR_DEFAULT_OPEN);
  });

  it('persists both states globally', () => {
    setMainSidebarOpen(false);
    expect(getMainSidebarOpen()).toBe(false);

    setMainSidebarOpen(true);
    expect(getMainSidebarOpen()).toBe(true);
  });

  it('falls back to open for unknown stored values', () => {
    const key = globalSlot('mainSidebarOpen', true, {
      serialize: String,
      parse: (raw) => raw
    }).key;

    localStorage.setItem(key, 'sometimes');

    expect(getMainSidebarOpen()).toBe(true);
  });
});
