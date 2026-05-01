/**
 * Emoji utilities for autocomplete and display.
 * Uses gemoji (GitHub's emoji shortcodes) as the data source.
 */

import { gemoji, nameToEmoji } from 'gemoji';

export type EmojiResult = {
  name: string;
  emoji: string;
  tags: string[];
};

/**
 * Get an emoji by its shortcode name.
 * @example getEmojiByName('heart') // '❤️'
 */
export function getEmojiByName(name: string): string | undefined {
  return nameToEmoji[name];
}

/**
 * Search emojis by query string.
 * Matches against both shortcode names and tags.
 * Results are scored: exact match > prefix match > substring match.
 * @param query - Search query (e.g., 'hea' matches 'heart', 'headphones')
 * @param limit - Maximum number of results to return
 * @returns Array of matching emojis with name, emoji character, and tags
 */
export function searchEmojis(query: string, limit = 10): EmojiResult[] {
  const q = query.toLowerCase();

  type ScoredEmoji = {
    emoji: string;
    name: string;
    tags: string[];
    score: number;
  };

  const scored: ScoredEmoji[] = [];

  for (const e of gemoji) {
    let bestScore = 0;

    // Check names for matches
    for (const name of e.names) {
      if (name === q) {
        bestScore = Math.max(bestScore, 1000); // Exact match
      } else if (name.startsWith(q)) {
        bestScore = Math.max(bestScore, 500 - name.length); // Prefix (shorter = better)
      } else if (name.includes(q)) {
        bestScore = Math.max(bestScore, 100 - name.length); // Substring (shorter = better)
      }
    }

    // Check tags for matches (lower priority than names)
    for (const tag of e.tags) {
      if (tag === q) {
        bestScore = Math.max(bestScore, 50);
      } else if (tag.startsWith(q)) {
        bestScore = Math.max(bestScore, 25);
      } else if (tag.includes(q)) {
        bestScore = Math.max(bestScore, 10);
      }
    }

    if (bestScore > 0) {
      scored.push({
        emoji: e.emoji,
        name: e.names[0],
        tags: e.tags,
        score: bestScore
      });
    }
  }

  // Sort by score (descending) and take top results
  scored.sort((a, b) => b.score - a.score);

  return scored.slice(0, limit).map((e) => ({
    name: e.name,
    emoji: e.emoji,
    tags: e.tags
  }));
}

/**
 * Total number of slots shown in the quick-reaction toolbar.
 * The first PINNED_REACTIONS.length slots are stable; the rest track recent usage.
 */
export const QUICK_REACTIONS_COUNT = 6;

/**
 * Pinned emojis that always occupy the leading slots of the quick-reaction toolbar,
 * in display order. Eventually intended to be user-configurable.
 */
export const PINNED_REACTIONS = ['👍', '👋', '🤣', '🙏'] as const;

/**
 * Fallback emojis used to fill the trailing (non-pinned) slots of the
 * quick-reaction toolbar when the user has not yet accumulated enough
 * non-pinned recent reactions. Listed in priority order.
 */
export const RECENT_REACTION_FALLBACKS = ['❤️', '😂', '😮', '😢', '🎉'] as const;

/**
 * Reverse lookup: Unicode emoji → shortcode name.
 * Built from gemoji data at module load. Uses names[0] (primary name)
 * except for +1/-1 which use their aliases (thumbsup/thumbsdown)
 * since +/- characters are problematic in NATS KV keys.
 */
const emojiToNameMap: Record<string, string> = (() => {
  const map: Record<string, string> = {};
  for (const e of gemoji) {
    const primary = e.names[0];
    if (primary === '+1' || primary === '-1') {
      // Use the alias that avoids + and - characters
      map[e.emoji] = e.names[1]; // thumbsup / thumbsdown
    } else {
      map[e.emoji] = primary;
    }
  }
  return map;
})();

/**
 * Convert a Unicode emoji to its shortcode name for sending to the server.
 * Returns undefined if the emoji is not recognized.
 * @example emojiToName('👍') // 'thumbsup'
 * @example emojiToName('❤️') // 'heart'
 */
export function emojiToName(emoji: string): string | undefined {
  return emojiToNameMap[emoji];
}

/**
 * Emoji categories with representative icons for tab rendering.
 * Order matches the standard Unicode emoji category order.
 */
const CATEGORY_ICONS: Record<string, string> = {
  'Smileys & Emotion': '😀',
  'People & Body': '👋',
  'Animals & Nature': '🐾',
  'Food & Drink': '🍔',
  'Travel & Places': '✈️',
  Activities: '⚽',
  Objects: '💡',
  Symbols: '💛',
  Flags: '🏁'
};

export type EmojiCategory = {
  name: string;
  icon: string;
  emojis: { emoji: string; name: string }[];
};

/**
 * All emojis grouped by category. Computed once at module load.
 */
export const EMOJI_BY_CATEGORY: EmojiCategory[] = (() => {
  const map = new Map<string, { emoji: string; name: string }[]>();
  for (const e of gemoji) {
    let list = map.get(e.category);
    if (!list) {
      list = [];
      map.set(e.category, list);
    }
    list.push({ emoji: e.emoji, name: e.names[0] });
  }
  return Array.from(map.entries()).map(([name, emojis]) => ({
    name,
    icon: CATEGORY_ICONS[name] ?? '❓',
    emojis
  }));
})();
