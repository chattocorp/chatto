/**
 * Recently used emojis from the emoji picker, per-server.
 *
 * Distinct from `recentReactions` (which tracks the most-recent message-reaction
 * emojis used to populate the quick-reaction toolbar). This store tracks
 * everything selected from the full emoji picker so the picker can lead with a
 * "Recently Used" section.
 *
 * State is keyed by server ID via {@link serverStorageKey}; switching servers
 * shows a different recent list.
 */

import { serverStorageKey } from '$lib/storage/serverStorage';

const STORAGE_SUFFIX = 'recentEmojis';
export const MAX_RECENT_EMOJIS = 16;

export class RecentEmojisStore {
  recent = $state<string[]>([]);
  private storageKey: string;

  constructor(serverId: string) {
    this.storageKey = serverStorageKey(serverId, STORAGE_SUFFIX);
    if (typeof window === 'undefined') return;
    try {
      const stored = localStorage.getItem(this.storageKey);
      if (!stored) return;
      const parsed = JSON.parse(stored);
      if (Array.isArray(parsed)) {
        this.recent = parsed
          .filter((e): e is string => typeof e === 'string')
          .slice(0, MAX_RECENT_EMOJIS);
      }
    } catch {
      // Ignore corrupt localStorage
    }
  }

  record(emoji: string) {
    const filtered = this.recent.filter((e) => e !== emoji);
    this.recent = [emoji, ...filtered].slice(0, MAX_RECENT_EMOJIS);
    try {
      localStorage.setItem(this.storageKey, JSON.stringify(this.recent));
    } catch {
      // localStorage full or unavailable
    }
  }
}

const stores = new Map<string, RecentEmojisStore>();

/** Get (or lazily create) the recent-emojis store for a given server. */
export function getRecentEmojis(serverId: string): RecentEmojisStore {
  let store = stores.get(serverId);
  if (!store) {
    store = new RecentEmojisStore(serverId);
    stores.set(serverId, store);
  }
  return store;
}

/** Test-only: clear the store cache so a fresh instance is built per test. */
export function __resetRecentEmojisForTests() {
  stores.clear();
}
