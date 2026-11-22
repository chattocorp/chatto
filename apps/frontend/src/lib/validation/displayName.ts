/** Display-name validation messages. The rules live in `@chatto/client/util/displayName`. */
import { m } from '$lib/i18n/messages';
import {
  DISPLAY_NAME_MAX_LENGTH,
  displayNameValidationError,
  normalizeDisplayName
} from '@chatto/client/util/displayName';

export { normalizeDisplayName };

/** Maximum display-name length in Unicode code points. */
export const MAX_DISPLAY_NAME_LENGTH = DISPLAY_NAME_MAX_LENGTH;

/** Result of validating presentation text before an API request. */
export interface ValidationResult {
  valid: boolean;
  error?: string;
}

/**
 * Validate an already trimmed display name with the server's rules.
 * Call normalizeDisplayName first when validating user input.
 */
export function validateDisplayName(name: string): ValidationResult {
  switch (displayNameValidationError(name)) {
    case undefined:
      return { valid: true };
    case 'empty':
      return { valid: false, error: 'Display name cannot be empty' };
    case 'too_long':
      return {
        valid: false,
        error: `Display name cannot exceed ${MAX_DISPLAY_NAME_LENGTH} characters`
      };
    case 'invalid':
      return { valid: false, error: m('settings.profile.display_name.invalid') };
  }
}

/** Validate user input and return its trimmed spelling only when valid. */
export function validateAndNormalizeDisplayName(
  name: string
): ValidationResult & { normalized?: string } {
  const normalized = normalizeDisplayName(name);
  const result = validateDisplayName(normalized);
  return result.valid ? { ...result, normalized } : result;
}
