/**
 * Content validation for messages.
 *
 * Validates that message content has at least one visible character,
 * preventing messages with only invisible Unicode characters.
 *
 * NOTE: This must stay in sync with HasVisibleContent() in
 * cli/internal/core/validation.go. The backend is authoritative;
 * this frontend check is for UX (disabling submit button).
 */

/** Zero-width characters that should be treated as invisible */
const ZERO_WIDTH_CHARS = new Set([
  '\u200B', // Zero Width Space
  '\u200C', // Zero Width Non-Joiner
  '\u200D', // Zero Width Joiner
  '\u200E', // Left-to-Right Mark
  '\u200F', // Right-to-Left Mark
  '\u2060', // Word Joiner
  '\u2061', // Function Application
  '\u2062', // Invisible Times
  '\u2063', // Invisible Separator
  '\u2064', // Invisible Plus
  '\uFEFF' // Byte Order Mark / Zero Width No-Break Space
]);

/**
 * Additional format characters from Unicode Cf category.
 *
 * This is a best-effort subset of common invisible characters.
 * The Go backend uses unicode.Is(unicode.Cf, r) which covers the
 * entire Cf category. Missing characters here will still be
 * rejected by the backend.
 */
const FORMAT_CHARS = new Set([
  '\u00AD', // Soft Hyphen
  '\u034F', // Combining Grapheme Joiner
  '\u061C', // Arabic Letter Mark
  '\u115F', // Hangul Choseong Filler
  '\u1160', // Hangul Jungseong Filler
  '\u17B4', // Khmer Vowel Inherent Aq
  '\u17B5', // Khmer Vowel Inherent Aa
  '\u180B', // Mongolian Free Variation Selector One
  '\u180C', // Mongolian Free Variation Selector Two
  '\u180D', // Mongolian Free Variation Selector Three
  '\u180E', // Mongolian Vowel Separator
  '\u180F' // Mongolian Free Variation Selector Four
]);

/**
 * Check if a character is invisible (whitespace, control, or format character).
 */
function isInvisibleChar(char: string): boolean {
  // Check zero-width characters
  if (ZERO_WIDTH_CHARS.has(char)) return true;

  // Check format characters
  if (FORMAT_CHARS.has(char)) return true;

  // Check whitespace (using Unicode pattern with u flag for full Unicode support)
  if (/^\s$/u.test(char)) return true;

  // Check control characters (C0/C1)
  const code = char.charCodeAt(0);
  if (code <= 0x1f || code === 0x7f) return true;
  if (code >= 0x80 && code <= 0x9f) return true;

  return false;
}

/**
 * Check if a string contains at least one visible character.
 *
 * A visible character is one that is not:
 * - Whitespace
 * - Control character (Unicode Cc)
 * - Format character (Unicode Cf), including zero-width chars
 *
 * This matches the backend HasVisibleContent() function in Go.
 *
 * @param content The string to check
 * @returns true if the string contains at least one visible character
 */
export function hasVisibleContent(content: string): boolean {
  for (const char of content) {
    if (!isInvisibleChar(char)) {
      return true;
    }
  }
  return false;
}
