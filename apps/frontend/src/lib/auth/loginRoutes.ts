/**
 * Appends the post-sign-in return path to an internal auth route, such as
 * `/register` or `/login/password`. The default return path `/` is omitted.
 *
 * @param path Resolved internal route path without a query string.
 * @param redirectUrl Same-origin path to open after sign-in.
 */
export function withReturnPath(path: string, redirectUrl: string): string {
  if (redirectUrl === '/') return path;
  return `${path}?${new URLSearchParams({ redirect: redirectUrl })}`;
}
