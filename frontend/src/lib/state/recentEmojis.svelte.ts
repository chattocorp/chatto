/**
 * Recently used emojis, per-server.
 *
 * Single source of truth for "what emojis has this user picked lately" on a
 * given server. Used by:
 * - The full emoji picker, which leads with a "Recently Used" section.
 * - The message quick-reaction bar / context menu / mobile action sheet,
 *   which fill the trailing (non-pinned) slots with these recents.
 *
 * State is keyed by server ID via {@link serverStorageKey}; switching servers
 * shows a different recent list.
 */

import {
  PINNED_REACTIONS,
  QUICK_REACTIONS_COUNT,
  RECENT_REACTION_FALLBACKS
} from '$lib/emoji';
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

  /**
   * The quick-reactions list shown on the message hover bar / context menu /
   * mobile action sheet: pinned emojis followed by the user's most recent
   * non-pinned emojis on this server, backfilled with fallback defaults so
   * the list always has exactly {@link QUICK_REACTIONS_COUNT} entries.
   *
   * Declared as a $derived class field rather than a JS getter so consumers
   * across the app share one memoised computation that re-fires on `recent`
   * mutations — the getter form silently lost reactivity for some consumers.
   */
  quickReactions: readonly string[] = $derived.by(() => {
    const pinned = PINNED_REACTIONS as readonly string[];
    const result: string[] = [...pinned];
    const recent = [...this.recent];

    for (const emoji of recent) {
      if (result.length >= QUICK_REACTIONS_COUNT) break;
      if (!result.includes(emoji)) result.push(emoji);
    }

    for (const emoji of RECENT_REACTION_FALLBACKS) {
      if (result.length >= QUICK_REACTIONS_COUNT) break;
      if (!result.includes(emoji)) result.push(emoji);
    }

    return result;
  });
}

// Private singleton registry. Reactivity comes from each store's $state.recent
// field; the Map itself is just an identity cache so the same serverId always
// returns the same store instance. A SvelteMap would invalidate readers on
// every first-access (mutate-during-derived), which is not the intent.
// eslint-disable-next-line svelte/prefer-svelte-reactivity
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
