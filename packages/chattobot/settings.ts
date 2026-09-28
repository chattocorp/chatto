/** Host configuration helpers. Chat messages never select these values. */

/** A host configuration value is missing or invalid. Its message names settings, never their values. */
export class ConfigurationError extends Error {
  override name = 'ConfigurationError';
}

/** Read an environment setting. Surrounding whitespace is removed; an empty value counts as unset. */
export function setting(name: string): string | undefined {
  return process.env[name]?.trim() || undefined;
}
