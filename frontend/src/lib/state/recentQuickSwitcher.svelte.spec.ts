import { describe, it, expect, beforeEach } from 'vitest';
import { RecentQuickSwitcherState } from './recentQuickSwitcher.svelte';

const STORAGE_KEY = 'chatto:quickSwitcherRecents';
const MAX_RECENTS = 15;

describe('RecentQuickSwitcherState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('initial state', () => {
    it('starts empty', () => {
      const state = new RecentQuickSwitcherState();
      expect([...state.urls]).toEqual([]);
    });

    it('hydrates persisted urls', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(['/a', '/b']));
      const state = new RecentQuickSwitcherState();
      expect([...state.urls]).toEqual(['/a', '/b']);
    });

    it('ignores corrupt JSON', () => {
      localStorage.setItem(STORAGE_KEY, '{not json');
      const state = new RecentQuickSwitcherState();
      expect([...state.urls]).toEqual([]);
    });

    it('filters non-string entries', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(['/a', 42, null, '/b']));
      const state = new RecentQuickSwitcherState();
      expect([...state.urls]).toEqual(['/a', '/b']);
    });
  });

  describe('record', () => {
    it('adds a new url at the front', () => {
      const state = new RecentQuickSwitcherState();
      state.record('/a');
      state.record('/b');
      expect([...state.urls]).toEqual(['/b', '/a']);
    });

    it('moves an existing url to the front (no duplicates)', () => {
      const state = new RecentQuickSwitcherState();
      state.record('/a');
      state.record('/b');
      state.record('/a');
      expect([...state.urls]).toEqual(['/a', '/b']);
    });

    it('caps the list at MAX_RECENTS', () => {
      const state = new RecentQuickSwitcherState();
      for (let i = 0; i < MAX_RECENTS + 5; i++) {
        state.record('/url-' + i);
      }
      expect(state.urls.length).toBe(MAX_RECENTS);
      expect(state.urls[0]).toBe('/url-' + (MAX_RECENTS + 4));
    });

    it('persists to localStorage', () => {
      const state = new RecentQuickSwitcherState();
      state.record('/x');
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]');
      expect(stored).toEqual(['/x']);
    });
  });

  describe('indexOf', () => {
    it('returns -1 for unknown urls', () => {
      const state = new RecentQuickSwitcherState();
      expect(state.indexOf('/missing')).toBe(-1);
    });

    it('returns 0 for the most recent', () => {
      const state = new RecentQuickSwitcherState();
      state.record('/a');
      state.record('/b');
      expect(state.indexOf('/b')).toBe(0);
      expect(state.indexOf('/a')).toBe(1);
    });
  });
});
