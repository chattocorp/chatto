import { describe, it, expect, beforeEach } from 'vitest';
import {
  RecentEmojisStore,
  MAX_RECENT_EMOJIS,
  getRecentEmojis,
  __resetRecentEmojisForTests
} from './recentEmojis.svelte';
import { serverStorageKey } from '$lib/storage/serverStorage';

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
