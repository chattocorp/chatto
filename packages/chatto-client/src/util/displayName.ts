/**
 * Display-name rules that mirror the server (`cli/internal/core/validation.go`).
 * Hosts use them to check input before a request. The transport uses
 * {@link presentableDisplayName} to show names from older servers that do not
 * apply the rules at read time.
 */

/** Maximum display-name length in Unicode code points. */
export const DISPLAY_NAME_MAX_LENGTH = 32;

/** Why a display name is not valid. */
export type DisplayNameValidationError = 'empty' | 'too_long' | 'invalid';

/**
 * Block mosaics: Block Elements, Symbols for Legacy Computing, and its
 * Supplement. They fill the complete character cell and form pixel lettering
 * that is about twice as tall as the surrounding text.
 */
const MOSAIC_GLYPH = /[▀-▟\u{1FB00}-\u{1FBFF}\u{1CC00}-\u{1CEBF}]/u;

/** Joiners and the emoji tag alphabet are the only permitted format characters. */
function isDisplayNameFormatChar(char: string): boolean {
  const code = char.codePointAt(0)!;
  return code === 0x200c || code === 0x200d || (code >= 0xe0020 && code <= 0xe007f);
}

/** Trim surrounding whitespace without changing the spelling or internal spaces. */
export function normalizeDisplayName(name: string): string {
  // Unicode White_Space matches Go strings.TrimSpace. JavaScript trim also
  // removes BOM, which the server correctly rejects as an invisible format.
  return name.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '');
}

/**
 * Validate an already normalized display name with the server's rules.
 * Letters, numbers, punctuation, and symbols supply visible content; combining
 * marks and format characters alone do not. Returns `undefined` when valid.
 */
export function displayNameValidationError(name: string): DisplayNameValidationError | undefined {
  if (name === '') return 'empty';
  if ([...name].length > DISPLAY_NAME_MAX_LENGTH) return 'too_long';
  let visible = false;
  for (const char of name) {
    if (/[\p{Cc}\p{Zl}\p{Zp}]/u.test(char) || MOSAIC_GLYPH.test(char)) return 'invalid';
    if (/\p{Cf}/u.test(char) && !isDisplayNameFormatChar(char)) return 'invalid';
    visible ||=
      /[\p{L}\p{N}\p{P}\p{S}]/u.test(char) &&
      !/\p{Default_Ignorable_Code_Point}/u.test(char) &&
      char !== '⠀';
  }
  return visible ? undefined : 'invalid';
}

/**
 * Return the display name to show for a user. A name that the current rules
 * do not permit, such as one stored before a rule existed, is replaced by the
 * login. An empty name stays empty: deleted and placeholder users have none.
 */
export function presentableDisplayName(displayName: string, login: string): string {
  if (displayName === '' || login === '') return displayName;
  const normalized = normalizeDisplayName(displayName);
  return displayNameValidationError(normalized) ? login : normalized;
}
