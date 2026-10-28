/** Shared emoji names and shortcode syntax, independent of Markdown and UI. */
import { nameToEmoji } from 'gemoji';

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
 * Call this only on prose: the host parser must exclude code and URLs first.
 */
export function* emojiShortcodes(text: string): Generator<EmojiShortcode> {
  for (const match of text.matchAll(/:([a-zA-Z0-9_+-]+):/g)) {
    yield {
      start: match.index,
      end: match.index + match[0].length,
      resolution: resolveEmoji(match[1]!)
    };
  }
}
