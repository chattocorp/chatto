import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { paneWidthSlot } from './paneWidth';

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

describe('pane width slot', () => {
  beforeEach(() => localStorage.clear());

  afterAll(() => vi.unstubAllGlobals());

  it('clamps writes into bounds and returns the clamped value', () => {
    const slot = paneWidthSlot('paneWidthSlotSpecClamp', {
      defaultValue: 300,
      minWidth: 200,
      maxWidth: 400
    });

    expect(slot.set(100)).toBe(200);
    expect(slot.get()).toBe(200);
    expect(slot.set(900)).toBe(400);
    expect(slot.get()).toBe(400);
    expect(slot.set(320)).toBe(320);
  });

  it('uses the default for missing or out-of-range stored payloads', () => {
    const slot = paneWidthSlot('paneWidthSlotSpecCorrupt', {
      defaultValue: 300,
      minWidth: 200,
      maxWidth: 400
    });

    expect(slot.get()).toBe(300);

    localStorage.setItem('chatto:paneWidthSlotSpecCorrupt', '9999');
    expect(slot.get()).toBe(300);

    localStorage.setItem('chatto:paneWidthSlotSpecCorrupt', 'not-a-number');
    expect(slot.get()).toBe(300);
  });
});
