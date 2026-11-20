/**
 * The AT Protocol handle page continues a provider sign-in that started
 * without a handle (FDR-058). The server sends the original start request as
 * `next`; the page adds the handle as `login_hint`.
 */

const PROVIDER_START_PREFIX = '/auth/providers/';

/**
 * The URL that continues `next` with `handle`, or `null` when `next` is not a
 * provider start on this origin. The check keeps the page from sending the
 * handle, or the browser, anywhere else.
 */
export function atprotoHandleContinuationUrl(
  next: string | null,
  handle: string,
  origin: string
): string | null {
  if (!next?.startsWith(PROVIDER_START_PREFIX)) return null;
  const url = new URL(next, origin);
  if (url.origin !== origin || !url.pathname.startsWith(PROVIDER_START_PREFIX)) return null;
  url.searchParams.set('login_hint', handle.trim().replace(/^@/, ''));
  return url.pathname + url.search;
}
