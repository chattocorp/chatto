/**
 * Recent reactions state.
 *
 * Tracks the user's most recently used reaction emojis and surfaces them
 * as the quick reaction list (hover bar, context menu, mobile action sheet).
 * Persisted to localStorage so preferences survive page reloads.
 *
 * The quick reaction list is a fixed-size array of QUICK_REACTIONS_COUNT
 * slots: the first PINNED_REACTIONS.length slots are always the pinned
 * emojis, and the remaining slots are filled with the user's most recently
 * used non-pinned emojis (backfilled from RECENT_REACTION_FALLBACKS so the
 * list always returns exactly QUICK_REACTIONS_COUNT entries).
 */

import {
  PINNED_REACTIONS,
  QUICK_REACTIONS_COUNT,
  RECENT_REACTION_FALLBACKS
} from '$lib/emoji';

const STORAGE_KEY = 'chatto:recentReactions';
const MAX_RECENT = 10;

export class RecentReactionsState {
  private recent = $state<string[]>([]);

  constructor() {
    if (typeof window !== 'undefined') {
      try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (stored) {
          const parsed = JSON.parse(stored);
          if (Array.isArray(parsed)) {
            this.recent = parsed.filter((e): e is string => typeof e === 'string');
          }
        }
      } catch {
        // Ignore corrupt localStorage
      }
    }
  }

  /**
   * The quick reactions list: pinned emojis followed by the user's most
   * recent non-pinned emojis, backfilled with fallback defaults so the
   * list always has exactly QUICK_REACTIONS_COUNT entries.
   */
  get quickReactions(): readonly string[] {
    const pinned = PINNED_REACTIONS as readonly string[];
    const result: string[] = [...pinned];

    for (const emoji of this.recent) {
      if (result.length >= QUICK_REACTIONS_COUNT) break;
      if (!result.includes(emoji)) {
        result.push(emoji);
      }
    }

    for (const emoji of RECENT_REACTION_FALLBACKS) {
      if (result.length >= QUICK_REACTIONS_COUNT) break;
      if (!result.includes(emoji)) {
        result.push(emoji);
      }
    }

    return result;
  }

  /** Record an emoji as the most recently used reaction. */
  record(emoji: string) {
    const filtered = this.recent.filter((e) => e !== emoji);
    this.recent = [emoji, ...filtered].slice(0, MAX_RECENT);
    this.persist();
  }

  private persist() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(this.recent));
    } catch {
      // localStorage full or unavailable
    }
  }
}

export const recentReactions = new RecentReactionsState();
