/**
 * Recent reactions state.
 *
 * Tracks the user's most recently used reaction emojis and surfaces them
 * as the quick reaction list (hover bar, context menu, mobile action sheet).
 * Persisted to localStorage so preferences survive page reloads.
 *
 * When a user reacts with any emoji, it moves to the front of the list.
 * The list is backfilled with defaults so there are always 6 quick reactions.
 */

import { QUICK_REACTIONS } from '$lib/emoji';

const STORAGE_KEY = 'chatto:recentReactions';
const MAX_RECENT = QUICK_REACTIONS.length;

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
   * The quick reactions list: user's recent emojis first,
   * backfilled with defaults to always return exactly 6.
   */
  get quickReactions(): readonly string[] {
    const result = [...this.recent];
    for (const emoji of QUICK_REACTIONS) {
      if (result.length >= MAX_RECENT) break;
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
