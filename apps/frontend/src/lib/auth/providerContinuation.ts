/** Browser routes that must finish provider authorization before app navigation. */
import { resolve } from '$app/paths';
import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';

/** Recognize an explicit provider callback or existing-account continuation. */
export function isProviderContinuation(path: string): boolean {
  if (!isSafeInternalPath(path)) return false;
  const url = new URL(path, 'https://chatto.invalid');
  return (
    (url.pathname === resolve('/sso/confirm') &&
      url.searchParams.get('existing') === '1' &&
      !!url.searchParams.get('token')) ||
    isProviderCallback(url)
  );
}

/** Identify the cookie-session callback used by the waiting opening window. */
export function isProviderCallback(url: URL): boolean {
  return (
    url.pathname === resolve('/servers/callback') &&
    url.searchParams.get('mode') === 'provider' &&
    !!url.searchParams.get('state')
  );
}
