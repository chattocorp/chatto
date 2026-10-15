/** Host configuration helpers. Chat messages never select these values. */
import type { ThinkingLevel } from 'runling';

/** A host configuration value is missing or invalid. Its message names settings, never their values. */
export class ConfigurationError extends Error {
  override name = 'ConfigurationError';
}

/** Reasoning effort levels that agents accept. */
export const THINKING_LEVELS: readonly ThinkingLevel[] = [
  'off',
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max'
];

/** Read an agent's reasoning effort setting, or `fallback` when it is unset. Throws a
 * ConfigurationError for an unknown level. */
export function thinkingSetting(name: string, fallback: ThinkingLevel): ThinkingLevel {
  const value = setting(name);
  if (value === undefined) return fallback;
  if (!(THINKING_LEVELS as readonly string[]).includes(value))
    throw new ConfigurationError(`${name} must be one of: ${THINKING_LEVELS.join(', ')}`);
  return value as ThinkingLevel;
}

/** Read an environment setting. Surrounding whitespace is removed; an empty value counts as unset. */
export function setting(name: string): string | undefined {
  return process.env[name]?.trim() || undefined;
}

/** Optional Chatto login for maintainer handoffs. This contact never grants authorization.
 * Accept one user handle, not arbitrary prompt text or a room-wide broadcast. */
export function maintainerLoginSetting(): string | undefined {
  const value = setting('CHATTO_MAINTAINER_NAME');
  if (!value) return undefined;
  const login = value.replace(/^@/, '');
  if (
    !/^[A-Za-z0-9][A-Za-z0-9._-]{1,31}$/.test(login) ||
    login.endsWith('.') ||
    ['all', 'here', 'everyone'].includes(login.toLowerCase())
  )
    throw new ConfigurationError(
      'CHATTO_MAINTAINER_NAME must be one Chatto user login, not a display name or broadcast handle'
    );
  return login;
}
