import { beforeEach, describe, expect, it } from 'vitest';
import { paneWidthSlot } from '$lib/storage/paneWidth';
import { PaneWidthState } from './paneWidth.svelte';

const SLOT_KEY = 'chatto:paneWidthStateSpec';

function specSlot() {
  return paneWidthSlot('paneWidthStateSpec', {
    defaultValue: 300,
    minWidth: 200,
    maxWidth: 400
  });
}

describe('pane width state', () => {
  beforeEach(() => localStorage.removeItem(SLOT_KEY));

  it('starts from the stored or default width', () => {
    expect(new PaneWidthState(specSlot()).value).toBe(300);

    localStorage.setItem(SLOT_KEY, '360');
    expect(new PaneWidthState(specSlot()).value).toBe(360);
  });

  it('mirrors the clamped width in memory and persists best-effort', () => {
    const state = new PaneWidthState(specSlot());
    state.set(900);
    expect(state.value).toBe(400);
    expect(localStorage.getItem(SLOT_KEY)).toBe('400');

    state.set(100);
    expect(state.value).toBe(200);
    expect(localStorage.getItem(SLOT_KEY)).toBe('200');
  });

  it('keeps the clamped value when browser storage is unavailable', () => {
    const descriptor = Object.getOwnPropertyDescriptor(window, 'localStorage');
    expect(descriptor).toBeDefined();
    Object.defineProperty(window, 'localStorage', { configurable: true, value: undefined });
    try {
      const state = new PaneWidthState(specSlot());
      state.set(380);
      expect(state.value).toBe(380);
      state.reset();
      expect(state.value).toBe(300);
    } finally {
      Object.defineProperty(window, 'localStorage', descriptor as PropertyDescriptor);
    }
  });
});
