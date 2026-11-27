/**
 * Builds the query string for an auth link that returns to a same-origin path.
 * The default destination needs no query string.
 */
export function returnPathQuery(redirectUrl: string): '' | `?${string}` {
  if (redirectUrl === '/') return '';
  return `?${new URLSearchParams({ redirect: redirectUrl })}`;
}
