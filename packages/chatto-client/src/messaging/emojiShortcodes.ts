/** Shared emoji names and shortcode syntax, independent of Markdown and UI. */
import { nameToEmoji } from 'gemoji';

// Complete tags and autocomplete share the same opening boundary and name syntax.
const wordCharacter = String.raw`[\p{L}\p{N}\p{M}_]`;
const shortcodeStart = String.raw`(?<!${wordCharacter}):([a-zA-Z0-9_+-]+)`;
const shortcodePattern = new RegExp(`${shortcodeStart}:(?!${wordCharacter})`, 'gu');
const shortcodeQueryPattern = new RegExp(`${shortcodeStart}$`, 'u');

/** A resolved built-in emoji, or a name that the client does not recognise. */
export type EmojiResolution =
  { kind: 'unicode'; name: string; emoji: string } | { kind: 'unknown'; name: string };

/**
 * Resolves a case-sensitive gemoji name without colons, including aliases.
 * Unknown names remain distinct from Unicode so hosts can retain their source.
 * Custom emoji resolution will extend this boundary when their model exists.
 */
export function resolveEmoji(name: string): EmojiResolution {
  const emoji = Object.hasOwn(nameToEmoji, name) ? nameToEmoji[name] : undefined;
  return emoji ? { kind: 'unicode', name, emoji } : { kind: 'unknown', name };
}

/** One complete shortcode and its UTF-16 source offsets (end is exclusive). */
export type EmojiShortcode = {
  start: number;
  end: number;
  resolution: EmojiResolution;
};

/**
 * Finds non-overlapping `:name:` tags, including unknown names and adjacent tags.
 * Names accept ASCII letters, digits, underscores, plus signs, and hyphens.
 * Unicode letters, numbers, combining marks, and underscores directly beside
 * a tag prevent conversion. Punctuation and whitespace are boundaries.
 * Call this only on prose: the host parser must exclude code and URLs first.
 */
export function* emojiShortcodes(text: string): Generator<EmojiShortcode> {
  for (const match of text.matchAll(shortcodePattern)) {
    yield {
      start: match.index,
      end: match.index + match[0].length,
      resolution: resolveEmoji(match[1]!)
    };
  }
}

/** A partial shortcode name and the UTF-16 offset of its opening colon. */
export type EmojiShortcodeQuery = { name: string; start: number };

/**
 * Reads an unfinished shortcode at the end of text before the cursor.
 * Uses the same opening boundary and name characters as complete shortcodes.
 * Hosts choose a suggestion threshold; a completed tag is not a query.
 */
export function emojiShortcodeQuery(textBeforeCursor: string): EmojiShortcodeQuery | null {
  const match = shortcodeQueryPattern.exec(textBeforeCursor);
  return match ? { name: match[1]!, start: match.index } : null;
}
