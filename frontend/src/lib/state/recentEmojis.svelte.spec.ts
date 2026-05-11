import { describe, it, expect, beforeEach } from 'vitest';
import { flushSync } from 'svelte';
import {
  RecentEmojisStore,
  MAX_RECENT_EMOJIS,
  getRecentEmojis,
  __resetRecentEmojisForTests
} from './recentEmojis.svelte';
import {
  PINNED_REACTIONS,
  QUICK_REACTIONS_COUNT,
  RECENT_REACTION_FALLBACKS
} from '$lib/emoji';
import { serverStorageKey } from '$lib/storage/serverStorage';

const PINNED_COUNT = PINNED_REACTIONS.length;
const TRAILING_SLOTS = QUICK_REACTIONS_COUNT - PINNED_COUNT;

const SERVER_A = 'server-a';
const SERVER_B = 'server-b';

describe('RecentEmojisStore', () => {
  beforeEach(() => {
    localStorage.clear();
    __resetRecentEmojisForTests();
  });

  describe('initial state', () => {
    it('starts empty when storage is empty', () => {
      const store = new RecentEmojisStore(SERVER_A);
      expect(store.recent).toEqual([]);
    });

    it('hydrates recents from per-server localStorage', () => {
      localStorage.setItem(
        serverStorageKey(SERVER_A, 'recentEmojis'),
        JSON.stringify(['🚀', '🔥'])
      );
      const store = new RecentEmojisStore(SERVER_A);
      expect([...store.recent]).toEqual(['🚀', '🔥']);
    });

    it('ignores corrupt JSON without throwing', () => {
      localStorage.setItem(serverStorageKey(SERVER_A, 'recentEmojis'), 'not-json');
      const store = new RecentEmojisStore(SERVER_A);
      expect([...store.recent]).toEqual([]);
    });

    it('ignores non-array payloads', () => {
      localStorage.setItem(
        serverStorageKey(SERVER_A, 'recentEmojis'),
        JSON.stringify({ not: 'array' })
      );
      const store = new RecentEmojisStore(SERVER_A);
      expect([...store.recent]).toEqual([]);
    });

    it('filters non-string entries', () => {
      localStorage.setItem(
        serverStorageKey(SERVER_A, 'recentEmojis'),
        JSON.stringify(['🚀', 42, null, '🔥'])
      );
      const store = new RecentEmojisStore(SERVER_A);
      expect([...store.recent]).toEqual(['🚀', '🔥']);
    });

    it('truncates stored data to MAX_RECENT_EMOJIS', () => {
      const tooMany = Array.from({ length: MAX_RECENT_EMOJIS + 5 }, (_, i) => `e${i}`);
      localStorage.setItem(serverStorageKey(SERVER_A, 'recentEmojis'), JSON.stringify(tooMany));
      const store = new RecentEmojisStore(SERVER_A);
      expect(store.recent.length).toBe(MAX_RECENT_EMOJIS);
    });
  });

  describe('record', () => {
    it('places the recorded emoji at the front', () => {
      const store = new RecentEmojisStore(SERVER_A);
      store.record('🚀');
      store.record('🔥');
      expect([...store.recent]).toEqual(['🔥', '🚀']);
    });

    it('deduplicates: re-recording moves the emoji to the front without duplicates', () => {
      const store = new RecentEmojisStore(SERVER_A);
      store.record('🚀');
      store.record('🔥');
      store.record('🚀');
      expect([...store.recent]).toEqual(['🚀', '🔥']);
    });

    it(`caps the list at MAX_RECENT_EMOJIS (${MAX_RECENT_EMOJIS})`, () => {
      const store = new RecentEmojisStore(SERVER_A);
      for (let i = 0; i < MAX_RECENT_EMOJIS + 5; i++) {
        store.record(`e${i}`);
      }
      expect(store.recent.length).toBe(MAX_RECENT_EMOJIS);
      // Most recent first
      expect(store.recent[0]).toBe(`e${MAX_RECENT_EMOJIS + 4}`);
    });

    it('persists to per-server localStorage', () => {
      const store = new RecentEmojisStore(SERVER_A);
      store.record('🚀');
      const stored = JSON.parse(
        localStorage.getItem(serverStorageKey(SERVER_A, 'recentEmojis')) ?? '[]'
      );
      expect(stored).toEqual(['🚀']);
    });
  });

  describe('per-server isolation', () => {
    it('does not leak recents between servers', () => {
      const a = new RecentEmojisStore(SERVER_A);
      const b = new RecentEmojisStore(SERVER_B);
      a.record('🚀');
      expect([...a.recent]).toEqual(['🚀']);
      expect([...b.recent]).toEqual([]);
    });
  });

  describe('quickReactions reactivity', () => {
    // Regression: previously `quickReactions` was a plain JS getter on the
    // class. Reading it inside a $derived/$effect tracked correctly in
    // isolation, but cross-component consumers (the message hover bar) didn't
    // re-fire after `record()`, so the toolbar showed stale recents until a
    // page reload. Switching to a $derived.by class field fixed it. This test
    // pins the reactive contract so a future refactor can't quietly regress.
    it('an external $effect sees quickReactions updates after record()', () => {
      const store = getRecentEmojis(SERVER_A);
      let captured: readonly string[] = [];
      const cleanup = $effect.root(() => {
        $effect(() => {
          captured = store.quickReactions;
        });
      });
      flushSync();
      const before = [...captured];

      store.record('🚀');
      flushSync();

      expect(captured).not.toEqual(before);
      expect(captured).toContain('🚀');
      cleanup();
    });
  });

  describe('quickReactions', () => {
    it('returns pinned + fallbacks when there are no recents', () => {
      const store = new RecentEmojisStore(SERVER_A);
      expect([...store.quickReactions]).toEqual([
        ...PINNED_REACTIONS,
        ...RECENT_REACTION_FALLBACKS.slice(0, TRAILING_SLOTS)
      ]);
    });

    it('always returns exactly QUICK_REACTIONS_COUNT items', () => {
      const store = new RecentEmojisStore(SERVER_A);
      expect(store.quickReactions.length).toBe(QUICK_REACTIONS_COUNT);
    });

    it('puts the most recent non-pinned emojis into the trailing slots', () => {
      const store = new RecentEmojisStore(SERVER_A);
      store.record('🚀');
      store.record('🔥');
      const list = [...store.quickReactions];
      expect(list.slice(0, PINNED_COUNT)).toEqual([...PINNED_REACTIONS]);
      expect(list[PINNED_COUNT]).toBe('🔥');
      expect(list[PINNED_COUNT + 1]).toBe('🚀');
    });

    it('does not duplicate when a recorded emoji is already pinned', () => {
      const store = new RecentEmojisStore(SERVER_A);
      store.record(PINNED_REACTIONS[0]);
      const list = [...store.quickReactions];
      expect(list.filter((e) => e === PINNED_REACTIONS[0]).length).toBe(1);
      expect(list.slice(0, PINNED_COUNT)).toEqual([...PINNED_REACTIONS]);
    });
  });

  describe('getRecentEmojis', () => {
    it('returns the same store for the same server', () => {
      const a1 = getRecentEmojis(SERVER_A);
      const a2 = getRecentEmojis(SERVER_A);
      expect(a1).toBe(a2);
    });

    it('returns distinct stores per server', () => {
      const a = getRecentEmojis(SERVER_A);
      const b = getRecentEmojis(SERVER_B);
      expect(a).not.toBe(b);
    });
  });
});
