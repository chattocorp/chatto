/** Display-name validation for single-line Unicode presentation text, matching the server. */
import { m } from '$lib/i18n/messages';

/** Maximum display-name length in Unicode code points. */
export const MAX_DISPLAY_NAME_LENGTH = 32;

/** Result of validating presentation text before an API request. */
export interface ValidationResult {
  valid: boolean;
  error?: string;
}

/** Joiners and the emoji tag alphabet are the only permitted format characters. */
function isDisplayNameFormatChar(char: string): boolean {
  const code = char.codePointAt(0)!;
  return code === 0x200c || code === 0x200d || (code >= 0xe0020 && code <= 0xe007f);
}

/**
 * Validate an already trimmed display name. Letters, numbers, punctuation, and
 * symbols supply visible content; combining marks and format characters alone do
 * not. Call normalizeDisplayName first when validating user input.
 */
export function validateDisplayName(name: string): ValidationResult {
  if (name === '') {
    return { valid: false, error: 'Display name cannot be empty' };
  }
  if ([...name].length > MAX_DISPLAY_NAME_LENGTH) {
    return {
      valid: false,
      error: `Display name cannot exceed ${MAX_DISPLAY_NAME_LENGTH} characters`
    };
  }
  let visible = false;
  for (const char of name) {
    if (/[\p{Cc}\p{Zl}\p{Zp}]/u.test(char)) {
      return {
        valid: false,
        error: m('settings.profile.display_name.invalid')
      };
    }
    if (/\p{Cf}/u.test(char) && !isDisplayNameFormatChar(char)) {
      return { valid: false, error: m('settings.profile.display_name.invalid') };
    }
    visible ||=
      /[\p{L}\p{N}\p{P}\p{S}]/u.test(char) &&
      !/\p{Default_Ignorable_Code_Point}/u.test(char) &&
      char !== '\u2800';
  }
  if (!visible) {
    return { valid: false, error: m('settings.profile.display_name.invalid') };
  }
  return { valid: true };
}

/** Trim surrounding whitespace without changing the spelling or internal spaces. */
export function normalizeDisplayName(name: string): string {
  // Unicode White_Space matches Go strings.TrimSpace. JavaScript trim also
  // removes BOM, which the server correctly rejects as an invisible format.
  return name.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '');
}

/** Validate user input and return its trimmed spelling only when valid. */
export function validateAndNormalizeDisplayName(
  name: string
): ValidationResult & { normalized?: string } {
  const normalized = normalizeDisplayName(name);
  const result = validateDisplayName(normalized);
  return result.valid ? { ...result, normalized } : result;
}
