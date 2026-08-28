import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  threadPaneWidthSlot,
  THREAD_PANE_DEFAULT_WIDTH,
  THREAD_PANE_MAX_WIDTH,
  THREAD_PANE_MIN_WIDTH
} from './threadPaneWidth';

const storage = new Map<string, string>();
vi.stubGlobal('localStorage', {
  getItem: (key: string) => storage.get(key) ?? null,
  setItem: (key: string, value: string) => storage.set(key, value),
  removeItem: (key: string) => storage.delete(key),
  clear: () => storage.clear(),
  get length() {
    return storage.size;
  },
  key: (index: number) => [...storage.keys()][index] ?? null
} satisfies Storage);

describe('thread pane width storage', () => {
  beforeEach(() => localStorage.clear());

  afterAll(() => vi.unstubAllGlobals());

  it('uses the default for missing or unsupported stored widths', () => {
    expect(threadPaneWidthSlot.get()).toBe(THREAD_PANE_DEFAULT_WIDTH);

    localStorage.setItem('chatto:threadPaneWidth', String(THREAD_PANE_MAX_WIDTH + 1));
    expect(threadPaneWidthSlot.get()).toBe(THREAD_PANE_DEFAULT_WIDTH);
  });

  it('clamps persisted widths to the supported range', () => {
    threadPaneWidthSlot.set(THREAD_PANE_MIN_WIDTH - 100);
    expect(threadPaneWidthSlot.get()).toBe(THREAD_PANE_MIN_WIDTH);

    threadPaneWidthSlot.set(THREAD_PANE_MAX_WIDTH + 100);
    expect(threadPaneWidthSlot.get()).toBe(THREAD_PANE_MAX_WIDTH);
  });
});
