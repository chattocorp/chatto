/**
 * Authentication utilities for SvelteKit load functions.
 *
 * These functions can be used in +layout.ts and +page.ts files to check
 * authentication status and redirect unauthenticated users before components render.
 */

import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { browser } from '$app/environment';
import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { getCurrentUserViaConnect, type CurrentUser } from '$lib/api/viewer';
import { isAuthenticationRequiredError } from './errors';

export type { CurrentUser };

// Module-level cache for the current user. Root load re-checks the server on
// navigation, but keeps this value as a fallback when the check itself fails.
let cachedUser: CurrentUser | null = null;

/**
 * Load the current user from the ConnectRPC API.
 * Returns null if not authenticated.
 *
 * On transient network errors (e.g., slow CI, server still warming up after reload),
 * retries once. If the query still fails and we previously had a user, keep the
 * cached user rather than rendering the app as unauthenticated.
 */
export async function loadCurrentUser(): Promise<CurrentUser | null> {
  if (!browser) {
    // In SPA mode, load functions only run in the browser.
    // If somehow called on server, return null (will trigger redirect).
    return null;
  }

  for (let attempt = 0; attempt < 2; attempt++) {
    try {
      cachedUser = await getCurrentUserViaConnect({
        baseUrl: serverConnectionManager.originClient.connectBaseUrl,
        bearerToken: serverConnectionManager.originClient.bearerToken
      });
      return cachedUser;
    } catch (err) {
      if (isAuthenticationRequiredError(err)) {
        cachedUser = null;
        serverRegistry.clearOriginAuthentication();
        return null;
      }
      if (attempt === 0) {
        await new Promise((r) => setTimeout(r, 200));
        continue;
      }
      return cachedUser;
    }
  }

  return cachedUser;
}

/**
 * Clear the cached user. Call this when the user logs out.
 */
export function clearCachedUser(): void {
  cachedUser = null;
}

/**
 * Require authentication in a load function.
 * If not authenticated, stores the return URL and redirects to the home page.
 *
 * @param returnUrl - The URL to return to after login. Stored in sessionStorage.
 * @returns The authenticated user.
 * @throws Redirect to '/' if not authenticated.
 *
 * @example
 * // In +layout.ts or +page.ts
 * export const load: LayoutLoad = async ({ url }) => {
 *   const user = await requireAuth(url.pathname + url.search);
 *   return { user };
 * };
 */
export async function requireAuth(returnUrl?: string): Promise<CurrentUser> {
  const user = await loadCurrentUser();
  return requireUser(user, returnUrl);
}

/**
 * Require that a user is authenticated, redirecting to home if not.
 * Use this when you already have the user from a parent load function.
 *
 * @param user - The user from parent layout data (may be null)
 * @param returnUrl - The URL to return to after login. Stored in sessionStorage.
 * @returns The authenticated user.
 * @throws Redirect to '/' if not authenticated.
 *
 * @example
 * // In +layout.ts or +page.ts
 * export const load: LayoutLoad = async ({ url, parent }) => {
 *   const { user } = await parent();
 *   return { user: requireUser(user, url.pathname + url.search) };
 * };
 */
export function requireUser(user: CurrentUser | null, returnUrl?: string): CurrentUser {
  if (!user) {
    if (returnUrl && browser) {
      sessionStorage.setItem('returnUrl', returnUrl);
    }
    redirect(302, resolve('/'));
  }

  return user;
}
