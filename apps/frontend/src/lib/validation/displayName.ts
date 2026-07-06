/**
 * Display name validation matching the backend rules.
 *
 * Allowed: letters (any script), digits, marks (diacritics), emoji/symbols,
 * space, hyphen, apostrophe, period, underscore.
 *
 * Disallowed: control characters, zero-width characters, consecutive spaces.
 */

/** Maximum display name length in characters (matching backend) */
export const MAX_DISPLAY_NAME_LENGTH = 32;

/** Zero-width characters that should be rejected */
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
 * Check if a character is a zero-width/invisible formatting character.
 */
function isZeroWidthChar(char: string): boolean {
  return ZERO_WIDTH_CHARS.has(char);
}

/**
 * Check if a character is a control character.
 * Control characters are in Unicode categories Cc (control) and Cf (format).
 * We check Cc here; zero-width chars (Cf) are handled separately.
 */
function isControlChar(char: string): boolean {
  const code = char.charCodeAt(0);
  // C0 controls (0x00-0x1F) and DEL (0x7F)
  if (code <= 0x1f || code === 0x7f) return true;
  // C1 controls (0x80-0x9F)
  if (code >= 0x80 && code <= 0x9f) return true;
  return false;
}

/**
 * Check if a character is allowed in a display name.
 *
 * We use a Unicode-aware approach:
 * - Letters: Unicode category L (includes all scripts)
 * - Numbers: Unicode category N
 * - Marks: Unicode category M (diacritics, combining marks)
 * - Symbols: Unicode category S (includes emoji)
 * - Specific punctuation: hyphen, apostrophe, period, underscore, space
 *
 * JavaScript regex with Unicode property escapes handles this.
 */
const ALLOWED_CHAR_PATTERN = /^[\p{L}\p{N}\p{M}\p{S}\-'._\s]$/u;

function isAllowedChar(char: string): boolean {
  // First check for zero-width and control characters (explicit blocklist)
  if (isZeroWidthChar(char) || isControlChar(char)) {
    return false;
  }
  // Then check against the allowed pattern
  return ALLOWED_CHAR_PATTERN.test(char);
}

export interface ValidationResult {
  valid: boolean;
  error?: string;
}

/**
 * Validate a display name.
 *
 * Returns { valid: true } if valid, or { valid: false, error: "message" } if invalid.
 *
 * Note: This function expects the name to already be trimmed.
 * Use normalizeDisplayName() before validating.
 */
export function validateDisplayName(name: string): ValidationResult {
  if (name === '') {
    return { valid: false, error: 'Display name cannot be empty' };
  }

  // Check length in characters (matching backend which uses utf8.RuneCountInString() in Go)
  if ([...name].length > MAX_DISPLAY_NAME_LENGTH) {
    return { valid: false, error: `Display name cannot exceed ${MAX_DISPLAY_NAME_LENGTH} characters` };
  }

  // Check for consecutive spaces
  if (/\s{2,}/.test(name)) {
    return { valid: false, error: 'Display name cannot contain consecutive spaces' };
  }

  // Must start with a letter or digit (the avatar placeholder uses the first
  // character, so leading symbols/punctuation/emoji render badly).
  const firstChar = [...name][0];
  if (!/^[\p{L}\p{N}]$/u.test(firstChar)) {
    return { valid: false, error: 'Display name must start with a letter or digit' };
  }

  // Check each character
  for (const char of name) {
    if (!isAllowedChar(char)) {
      // Provide a helpful error message
      if (isControlChar(char)) {
        return { valid: false, error: 'Display name cannot contain control characters' };
      }
      if (isZeroWidthChar(char)) {
        return { valid: false, error: 'Display name cannot contain invisible characters' };
      }
      return {
        valid: false,
        error:
          "Display name can only contain letters, numbers, emoji, and basic punctuation (- ' . _)"
      };
    }
  }

  return { valid: true };
}

/**
 * Normalize a display name by trimming whitespace.
 */
export function normalizeDisplayName(name: string): string {
  return name.trim();
}

/**
 * Validate and normalize a display name.
 * Returns the validation result. If valid, the normalized name is in the result.
 */
export function validateAndNormalizeDisplayName(
  name: string
): ValidationResult & { normalized?: string } {
  const normalized = normalizeDisplayName(name);
  const result = validateDisplayName(normalized);
  if (result.valid) {
    return { ...result, normalized };
  }
  return result;
}
