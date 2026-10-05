import { authorizationLaunchStorageKey } from '$lib/oauth/authorizationWindow';

/**
 * Return the target URL that the latest `window.open` call's authorization
 * window received through its launch record, or `null` before navigation.
 */
export function authorizationLaunchTarget(open: {
  mock: { lastCall?: readonly unknown[] };
}): string | null {
  const launchUrl = open.mock.lastCall?.[0];
  if (typeof launchUrl !== 'string') return null;
  const launchId = new URL(launchUrl).hash.slice(1);
  const raw = localStorage.getItem(authorizationLaunchStorageKey(launchId));
  return raw === null ? null : (JSON.parse(raw) as { url: string }).url;
}
