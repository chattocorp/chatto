/** Minimum UTF-8 byte length for a new Chatto password. */
export const MIN_PASSWORD_BYTES = 8;
/** Maximum UTF-8 byte length accepted by the server's bcrypt implementation. */
export const MAX_PASSWORD_BYTES = 72;

/**
 * Check a new password without changing or truncating it.
 * These limits mirror core.ValidatePassword. Do not apply them to sign-in.
 */
export function validatePassword(password: string): 'too_short' | 'too_long' | undefined {
  const bytes = new TextEncoder().encode(password).length;
  if (bytes < MIN_PASSWORD_BYTES) return 'too_short';
  if (bytes > MAX_PASSWORD_BYTES) return 'too_long';
  return undefined;
}
