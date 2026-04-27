import { describe, it, expect, beforeEach } from 'vitest';
import { QUICK_REACTIONS } from '$lib/emoji';
import { RecentReactionsState } from './recentReactions.svelte';

const STORAGE_KEY = 'chatto:recentReactions';

describe('RecentReactionsState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('initial state', () => {
    it('returns the default QUICK_REACTIONS list when storage is empty', () => {
      const state = new RecentReactionsState();
      expect([...state.quickReactions]).toEqual([...QUICK_REACTIONS]);
    });

    it('always returns exactly QUICK_REACTIONS.length items', () => {
      const state = new RecentReactionsState();
      expect(state.quickReactions.length).toBe(QUICK_REACTIONS.length);
    });

    it('hydrates the persisted recent list', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(['🚀', '🔥']));
      const state = new RecentReactionsState();
      expect(state.quickReactions.slice(0, 2)).toEqual(['🚀', '🔥']);
    });

    it('ignores corrupt JSON without throwing', () => {
      localStorage.setItem(STORAGE_KEY, 'not-json');
      const state = new RecentReactionsState();
      expect([...state.quickReactions]).toEqual([...QUICK_REACTIONS]);
    });

    it('ignores non-array payloads', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ not: 'an array' }));
      const state = new RecentReactionsState();
      expect([...state.quickReactions]).toEqual([...QUICK_REACTIONS]);
    });

    it('filters non-string entries on load', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(['🚀', 42, null, '🔥']));
      const state = new RecentReactionsState();
      expect(state.quickReactions.slice(0, 2)).toEqual(['🚀', '🔥']);
    });
  });

  describe('record', () => {
    it('moves a recorded emoji to the front', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      expect(state.quickReactions[0]).toBe('🚀');
    });

    it('deduplicates: re-recording the same emoji keeps it at the front', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      state.record('🔥');
      state.record('🚀');
      expect(state.quickReactions[0]).toBe('🚀');
      expect(state.quickReactions[1]).toBe('🔥');
    });

    it('caps the list at QUICK_REACTIONS.length', () => {
      const state = new RecentReactionsState();
      const many = ['🚀', '🔥', '✨', '🌟', '💫', '⭐', '🌈', '🎯'];
      for (const e of many) state.record(e);
      expect(state.quickReactions.length).toBe(QUICK_REACTIONS.length);
      expect(state.quickReactions[0]).toBe('🎯');
    });

    it('backfills with defaults when fewer than max have been recorded', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      const list = [...state.quickReactions];
      expect(list.length).toBe(QUICK_REACTIONS.length);
      expect(list[0]).toBe('🚀');
      // Remaining slots are defaults (QUICK_REACTIONS may include the recorded emoji,
      // which is fine — it just means it gets surfaced once at position 0).
      for (let i = 1; i < list.length; i++) {
        expect(QUICK_REACTIONS).toContain(list[i]);
      }
    });

    it('persists to localStorage', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]');
      expect(stored[0]).toBe('🚀');
    });
  });
});
