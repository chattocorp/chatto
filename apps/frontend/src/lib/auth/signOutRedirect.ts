/**
 * Leave the app after an explicit sign-out with a full document navigation,
 * so no state of the signed-out session survives. The client stops publishing
 * origin viewer responses while the redirect is in progress.
 */

import { beginExplicitSignOutRedirect } from '@chatto/client/auth/signOut';

/**
 * Replace the current document with `href`. A same-origin target keeps only
 * its path, query, and fragment; an unreadable target navigates as given.
 */
export function hardRedirectAfterSignOut(href = '/'): void {
  beginExplicitSignOutRedirect();
  try {
    const target = new URL(href, window.location.href);
    if (target.origin === window.location.origin) {
      window.setTimeout(() => {
        window.location.replace(target.pathname + target.search + target.hash);
      }, 0);
      return;
    }
  } catch {
    // Fall back to a regular document navigation below.
  }
  window.setTimeout(() => {
    window.location.replace(href);
  }, 0);
}
