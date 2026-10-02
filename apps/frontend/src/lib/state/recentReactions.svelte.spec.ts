import { beforeEach, describe, expect, it } from 'vitest';
import { PINNED_REACTIONS } from '$lib/emoji';
import {
  RecentEmojisStore,
  getRecentEmojis,
  __resetRecentEmojisForTests
} from './recentEmojis.svelte';
import { serverStorageKey } from '@chatto/client/storage/serverStorage';

const SERVER_ID = 'recent-reactions-server';
const storageKey = serverStorageKey(SERVER_ID, 'recentReactions');

beforeEach(() => {
  localStorage.clear();
  __resetRecentEmojisForTests();
});

describe('RecentEmojisStore reaction history', () => {
  it('starts with only the pinned reactions and ignores general emoji history', () => {
    localStorage.setItem(serverStorageKey(SERVER_ID, 'recentEmojis'), JSON.stringify(['🌿']));
    const store = getRecentEmojis(SERVER_ID);
    expect(store.recent).toEqual(['🌿']);
    expect(store.quickReactions).toEqual([...PINNED_REACTIONS]);
    store.record('🚀');
    expect(store.quickReactions).toEqual([...PINNED_REACTIONS]);
  });

  it('shows zero, one, or two distinct custom choices, newest first', () => {
    const store = getRecentEmojis(SERVER_ID);
    store.recordReaction('🚀');
    expect(store.quickReactions).toEqual([...PINNED_REACTIONS, '🚀']);
    store.recordReaction('🔥');
    expect(store.quickReactions).toEqual([...PINNED_REACTIONS, '🔥', '🚀']);
    store.recordReaction('✅');
    expect(store.quickReactions).toEqual([...PINNED_REACTIONS, '✅', '🔥']);
    store.recordReaction('🔥');
    expect(store.quickReactions).toEqual([...PINNED_REACTIONS, '🔥', '✅']);
  });

  it('does not displace custom choices when a pinned emoji is selected', () => {
    const store = getRecentEmojis(SERVER_ID);
    store.recordReaction('🚀');
    store.recordReaction('🔥');
    for (const pinned of PINNED_REACTIONS) store.recordReaction(pinned);
    expect(store.quickReactions).toEqual([...PINNED_REACTIONS, '🔥', '🚀']);
  });

  it('persists custom choices under the new key and restores them after reload', () => {
    const store = getRecentEmojis(SERVER_ID);
    store.recordReaction('🚀');
    store.recordReaction('🔥');
    expect(JSON.parse(localStorage.getItem(storageKey)!)).toEqual(['🔥', '🚀']);
    __resetRecentEmojisForTests();
    expect(getRecentEmojis(SERVER_ID).quickReactions).toEqual([...PINNED_REACTIONS, '🔥', '🚀']);
  });

  it.each(['not-json', '{}', 'null'])('ignores malformed saved history: %s', (value) => {
    localStorage.setItem(storageKey, value);
    expect(new RecentEmojisStore(SERVER_ID).quickReactions).toEqual([...PINNED_REACTIONS]);
  });

  it('filters invalid, pinned, and duplicate saved entries before limiting the history', () => {
    localStorage.setItem(storageKey, JSON.stringify([42, null, '', '👍', '🔥', '🔥', '🚀', '✅']));
    expect(new RecentEmojisStore(SERVER_ID).quickReactions).toEqual([
      ...PINNED_REACTIONS,
      '🔥',
      '🚀'
    ]);
  });

  it('shares one store per server and keeps other servers isolated', () => {
    const store = getRecentEmojis(SERVER_ID);
    expect(getRecentEmojis(SERVER_ID)).toBe(store);
    const other = getRecentEmojis('other-server');
    expect(other).not.toBe(store);
    store.recordReaction('🚀');
    expect(other.quickReactions).toEqual([...PINNED_REACTIONS]);
  });

  it('updates in memory when browser storage is full', () => {
    const store = getRecentEmojis(SERVER_ID);
    const original = Storage.prototype.setItem;
    Storage.prototype.setItem = () => {
      throw new DOMException('Full', 'QuotaExceededError');
    };
    try {
      store.recordReaction('🚀');
      expect(store.quickReactions).toEqual([...PINNED_REACTIONS, '🚀']);
    } finally {
      Storage.prototype.setItem = original;
    }
  });
});
