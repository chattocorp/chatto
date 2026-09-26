/**
 * Profile bio validation matching the backend rules.
 *
 * The bio is Markdown source. The server trims surrounding whitespace and
 * limits the result to 1,000 Unicode characters. An empty bio clears it.
 */

import { m } from '$lib/i18n/messages';
import type { ValidationResult } from './displayName';

/** Maximum bio length in Unicode characters (matching backend) */
export const MAX_BIO_LENGTH = 1000;

/** Trim a bio draft the same way the server does before it stores it. */
export function normalizeBio(bio: string): string {
  return bio.trim();
}

/** Validate and normalize a bio draft. Returns the normalized value if valid. */
export function validateAndNormalizeBio(bio: string): ValidationResult & { normalized?: string } {
  const normalized = normalizeBio(bio);
  if ([...normalized].length > MAX_BIO_LENGTH) {
    return { valid: false, error: m('settings.profile.bio.too_long', { max: MAX_BIO_LENGTH }) };
  }
  return { valid: true, normalized };
}
