/**
 * Login/username validation matching the backend rules.
 *
 * Allowed: ASCII letters, digits, periods, underscores, hyphens.
 * Must start with a letter or digit.
 * Length: 2-32 characters.
 * Mixed case is preserved; uniqueness and login are case-insensitive.
 */

import type { ValidationResult } from './displayName';

/** Maximum login length in characters (matching backend) */
export const MAX_LOGIN_LENGTH = 32;

/** Minimum login length in characters (matching backend) */
export const MIN_LOGIN_LENGTH = 2;

/** Cooldown duration in milliseconds (30 days, matching backend) */
export const LOGIN_CHANGE_COOLDOWN_MS = 30 * 24 * 60 * 60 * 1000;

/** Pattern: must start with letter/digit, followed by letters/digits/periods/underscores/hyphens */
const LOGIN_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9._-]*$/;

/**
 * Validate a login/username.
 *
 * Returns { valid: true } if valid, or { valid: false, error: "message" } if invalid.
 * The login should be trimmed before validating (use normalizeLogin).
 */
export function validateLogin(login: string): ValidationResult {
  if (login === '') {
    return { valid: false, error: 'Username cannot be empty' };
  }

  if (login.length < MIN_LOGIN_LENGTH) {
    return { valid: false, error: `Username must be at least ${MIN_LOGIN_LENGTH} characters` };
  }

  if (login.length > MAX_LOGIN_LENGTH) {
    return { valid: false, error: `Username cannot exceed ${MAX_LOGIN_LENGTH} characters` };
  }

  if (!LOGIN_PATTERN.test(login)) {
    const firstChar = login[0];
    if (firstChar === '.' || firstChar === '_' || firstChar === '-') {
      return { valid: false, error: 'Username must start with a letter or number' };
    }
    return {
      valid: false,
      error: 'Username can only contain letters, numbers, periods, underscores, and hyphens'
    };
  }

  return { valid: true };
}

/**
 * Normalize a login by trimming whitespace. Casing is preserved.
 */
export function normalizeLogin(login: string): string {
  return login.trim();
}

/**
 * Validate and normalize a login.
 * Returns the validation result. If valid, the normalized login is in the result.
 */
export function validateAndNormalizeLogin(
  login: string
): ValidationResult & { normalized?: string } {
  const normalized = normalizeLogin(login);
  const result = validateLogin(normalized);
  if (result.valid) {
    return { ...result, normalized };
  }
  return result;
}

/**
 * Get the remaining cooldown time in milliseconds.
 * Returns 0 if no cooldown is active.
 */
export function getLoginChangeCooldownRemaining(lastChangeDate: Date | null): number {
  if (!lastChangeDate) return 0;
  const elapsed = Date.now() - lastChangeDate.getTime();
  const remaining = LOGIN_CHANGE_COOLDOWN_MS - elapsed;
  return remaining > 0 ? remaining : 0;
}

/**
 * Format a cooldown duration in milliseconds into a human-readable string.
 */
export function formatCooldownRemaining(ms: number): string {
  if (ms <= 0) return '';

  const days = Math.ceil(ms / (24 * 60 * 60 * 1000));
  if (days > 1) return `${days} days`;

  const hours = Math.ceil(ms / (60 * 60 * 1000));
  if (hours > 1) return `${hours} hours`;

  const minutes = Math.ceil(ms / (60 * 1000));
  return `${minutes} minute${minutes !== 1 ? 's' : ''}`;
}
