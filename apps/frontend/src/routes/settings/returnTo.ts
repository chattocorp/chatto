/** Restrict the settings back link to an absolute path on the current origin. */
export function safeSettingsReturnTo(
  candidate: string | null,
  currentURL: URL,
  fallback: string
): string {
  if (!candidate?.startsWith('/')) return fallback;
  try {
    const target = new URL(candidate, currentURL);
    if (target.origin !== currentURL.origin) return fallback;
    return `${target.pathname}${target.search}${target.hash}`;
  } catch {
    return fallback;
  }
}
