import { goto } from '$app/navigation';
import { isSafeInternalPath } from '$lib/navigation/safeInternalPath';

const RETURN_URL_KEY = 'returnUrl';
const RETURN_NAVIGATION_KEY = 'returnUrl:navigating';

function readSafePath(key: string): string | null {
  const value = sessionStorage.getItem(key);
  if (!value) return null;
  if (isSafeInternalPath(value)) return value;

  sessionStorage.removeItem(key);
  return null;
}

/** Store a safe app path to resume after authentication. */
export function saveReturnUrl(path: string): void {
  sessionStorage.removeItem(RETURN_NAVIGATION_KEY);
  if (isSafeInternalPath(path)) {
    sessionStorage.setItem(RETURN_URL_KEY, path);
  } else {
    sessionStorage.removeItem(RETURN_URL_KEY);
  }
}

/** Whether authentication return navigation is queued or already underway. */
export function hasPendingReturnNavigation(): boolean {
  return readSafePath(RETURN_NAVIGATION_KEY) !== null || readSafePath(RETURN_URL_KEY) !== null;
}

/** Navigate to a safe frontend or backend path after authentication. */
export async function navigateAfterAuthentication(path: string): Promise<void> {
  const target = isSafeInternalPath(path) ? path : '/';
  if (target.startsWith('/oauth/')) {
    window.location.href = target;
    return;
  }
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- validated concrete URL, not a route pattern
  await goto(target);
}

/**
 * Claim and resume a stored return path exactly once.
 *
 * The in-progress marker prevents the authenticated chat landing route from
 * racing its default redirect against this navigation.
 */
export async function resumeReturnNavigation(): Promise<boolean> {
  if (readSafePath(RETURN_NAVIGATION_KEY)) return true;

  const returnUrl = readSafePath(RETURN_URL_KEY);
  if (!returnUrl) return false;
  sessionStorage.removeItem(RETURN_URL_KEY);

  const currentUrl = window.location.pathname + window.location.search + window.location.hash;
  if (returnUrl === currentUrl) return true;
  if (returnUrl.startsWith('/oauth/')) {
    await navigateAfterAuthentication(returnUrl);
    return true;
  }

  sessionStorage.setItem(RETURN_NAVIGATION_KEY, returnUrl);
  try {
    await navigateAfterAuthentication(returnUrl);
  } catch (error) {
    console.warn('Return URL navigation failed:', error);
  } finally {
    if (sessionStorage.getItem(RETURN_NAVIGATION_KEY) === returnUrl) {
      sessionStorage.removeItem(RETURN_NAVIGATION_KEY);
    }
  }
  return true;
}
