import { describe, it, expect, beforeEach } from 'vitest';
import {
  PINNED_REACTIONS,
  QUICK_REACTIONS_COUNT,
  RECENT_REACTION_FALLBACKS
} from '$lib/emoji';
import { RecentReactionsState } from './recentReactions.svelte';

const STORAGE_KEY = 'chatto:recentReactions';

const PINNED_COUNT = PINNED_REACTIONS.length;
const TRAILING_SLOTS = QUICK_REACTIONS_COUNT - PINNED_COUNT;

describe('RecentReactionsState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('initial state', () => {
    it('returns the pinned set followed by fallbacks when storage is empty', () => {
      const state = new RecentReactionsState();
      expect([...state.quickReactions]).toEqual([
        ...PINNED_REACTIONS,
        ...RECENT_REACTION_FALLBACKS.slice(0, TRAILING_SLOTS)
      ]);
    });

    it('always returns exactly QUICK_REACTIONS_COUNT items', () => {
      const state = new RecentReactionsState();
      expect(state.quickReactions.length).toBe(QUICK_REACTIONS_COUNT);
    });

    it('hydrates non-pinned recents into the trailing slots', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(['🚀', '🔥']));
      const state = new RecentReactionsState();
      const list = [...state.quickReactions];
      expect(list.slice(0, PINNED_COUNT)).toEqual([...PINNED_REACTIONS]);
      expect(list[PINNED_COUNT]).toBe('🚀');
      expect(list[PINNED_COUNT + 1]).toBe('🔥');
    });

    it('ignores corrupt JSON without throwing', () => {
      localStorage.setItem(STORAGE_KEY, 'not-json');
      const state = new RecentReactionsState();
      expect([...state.quickReactions]).toEqual([
        ...PINNED_REACTIONS,
        ...RECENT_REACTION_FALLBACKS.slice(0, TRAILING_SLOTS)
      ]);
    });

    it('ignores non-array payloads', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ not: 'an array' }));
      const state = new RecentReactionsState();
      expect([...state.quickReactions]).toEqual([
        ...PINNED_REACTIONS,
        ...RECENT_REACTION_FALLBACKS.slice(0, TRAILING_SLOTS)
      ]);
    });

    it('filters non-string entries on load', () => {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(['🚀', 42, null, '🔥']));
      const state = new RecentReactionsState();
      const list = [...state.quickReactions];
      expect(list[PINNED_COUNT]).toBe('🚀');
      expect(list[PINNED_COUNT + 1]).toBe('🔥');
    });
  });

  describe('record', () => {
    it('surfaces a recorded non-pinned emoji at the first trailing slot', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      expect(state.quickReactions[PINNED_COUNT]).toBe('🚀');
    });

    it('does not shift the pinned set', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      state.record('🔥');
      state.record('✨');
      expect(state.quickReactions.slice(0, PINNED_COUNT)).toEqual([...PINNED_REACTIONS]);
    });

    it('recording a pinned emoji does not duplicate it or push out non-pinned recents', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      state.record('🔥');
      const before = [...state.quickReactions];

      // Record one of the pinned emojis
      state.record(PINNED_REACTIONS[0]);

      const after = [...state.quickReactions];
      expect(after).toEqual(before);
      // Pinned emoji should appear exactly once (in its pinned slot)
      expect(after.filter((e) => e === PINNED_REACTIONS[0]).length).toBe(1);
    });

    it('only the most recent TRAILING_SLOTS non-pinned emojis are shown', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      state.record('🔥');
      state.record('✨');
      state.record('🌟');

      const list = [...state.quickReactions];
      // Slots 4-5 should be the two most recently recorded
      expect(list[PINNED_COUNT]).toBe('🌟');
      expect(list[PINNED_COUNT + 1]).toBe('✨');
    });

    it('deduplicates: re-recording the same emoji keeps it at the first trailing slot', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      state.record('🔥');
      state.record('🚀');
      expect(state.quickReactions[PINNED_COUNT]).toBe('🚀');
      expect(state.quickReactions[PINNED_COUNT + 1]).toBe('🔥');
    });

    it('caps internal recent history at 10 entries', () => {
      const state = new RecentReactionsState();
      const many = ['🚀', '🔥', '✨', '🌟', '💫', '⭐', '🌈', '🎯', '🦄', '🍕', '🎸'];
      for (const e of many) state.record(e);
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]');
      expect(stored.length).toBe(10);
      expect(stored[0]).toBe('🎸');
    });

    it('backfills with fallback defaults when fewer than TRAILING_SLOTS non-pinned recents exist', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      const list = [...state.quickReactions];
      expect(list.length).toBe(QUICK_REACTIONS_COUNT);
      expect(list.slice(0, PINNED_COUNT)).toEqual([...PINNED_REACTIONS]);
      expect(list[PINNED_COUNT]).toBe('🚀');
      // Remaining trailing slots come from the fallback list
      expect(RECENT_REACTION_FALLBACKS).toContain(list[PINNED_COUNT + 1]);
    });

    it('persists to localStorage', () => {
      const state = new RecentReactionsState();
      state.record('🚀');
      const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]');
      expect(stored[0]).toBe('🚀');
    });
  });
});
