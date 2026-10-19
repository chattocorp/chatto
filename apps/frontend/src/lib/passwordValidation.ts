import { validatePassword } from '@chatto/client/auth/passwordPolicy';
import { m } from '$lib/i18n/messages';

/** Translate the shared new-password policy result for form validation. */
export function passwordValidationError(password: string): string | undefined {
  const error = validatePassword(password);
  if (error === 'too_short') return m('common.validation.password_min_bytes');
  if (error === 'too_long') return m('common.validation.password_max');
  return undefined;
}
