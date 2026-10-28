/**
 * Emoji utilities for autocomplete and display.
 * Uses the shared client catalog as the data source.
 */

import { builtInEmoji } from '@chatto/client/messaging/emojiShortcodes';
import { resolveEmoji, emojiNameFromUnicode } from '@chatto/client/messaging/emojiShortcodes';
import type { EmojiTone } from '$lib/emojiTones';

/** Apply the picker default only to unmodified names with a supported variant. */
export function emojiWithTone(name: string, tone: EmojiTone): { name: string; emoji: string } {
  const variant =
    tone && !name.includes('::') ? resolveEmoji(`${name}::skin-tone-${tone}`) : undefined;
  if (variant?.kind === 'unicode') return variant;
  const base = resolveEmoji(name);
  return { name, emoji: base.kind === 'unicode' ? base.emoji : `:${name}:` };
}

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
  const resolved = resolveEmoji(name);
  return resolved.kind === 'unicode' ? resolved.emoji : undefined;
}

/** Display a reaction glyph or its literal shortcode until the name is known. */
export function getReactionDisplay(name: string): string {
  return getEmojiByName(name) ?? `:${name}:`;
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

  for (const e of builtInEmoji) {
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
 * Pinned emojis that always occupy the leading slots of the quick-reaction toolbar,
 * in display order. Eventually intended to be user-configurable.
 */
export const PINNED_REACTIONS = ['👍', '👋', '🤣', '🙏'] as const;

/**
 * Convert a Unicode emoji to its shortcode name for sending to the server.
 * Returns undefined if the emoji is not recognized.
 * @example emojiToName('👍') // 'thumbsup'
 * @example emojiToName('❤️') // 'heart'
 */
export function emojiToName(emoji: string): string | undefined {
  return emojiNameFromUnicode(emoji);
}

const EMOJI_DISPLAY_NAME_OVERRIDES: Record<string, string> = {
  thumbsup: 'Thumbs up',
  thumbsdown: 'Thumbs down'
};

/**
 * Human-readable base label for a reaction emoji or shortcode. Supported skin
 * tones stay visible in the glyph rather than leaking suffix syntax into labels.
 * @example getEmojiDisplayName('thumbsup') // 'Thumbs up'
 * @example getEmojiDisplayName('🚀') // 'Rocket'
 */
export function getEmojiDisplayName(emojiOrName: string): string {
  const emoji = getEmojiByName(emojiOrName) ?? emojiOrName;
  const canonicalName = emojiToName(emoji);
  const name = canonicalName ? canonicalName.split('::')[0] : emojiOrName;
  const override = Object.hasOwn(EMOJI_DISPLAY_NAME_OVERRIDES, name)
    ? EMOJI_DISPLAY_NAME_OVERRIDES[name]
    : undefined;
  if (override) return override;

  const readable = name
    .replace(/^:+|:+$/g, '')
    .replace(/[+_-]+/g, ' ')
    .trim();

  if (!readable) return emojiOrName;

  return readable
    .split(/\s+/)
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
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
  for (const e of builtInEmoji) {
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
