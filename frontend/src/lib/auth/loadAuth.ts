/**
 * Authentication utilities for SvelteKit load functions.
 *
 * These functions can be used in +layout.ts and +page.ts files to check
 * authentication status and redirect unauthenticated users before components render.
 */

import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { browser } from '$app/environment';
import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
import { graphql } from '$lib/gql';
import type { LoadCurrentUserQuery } from '$lib/gql/graphql';

export const LoadCurrentUserDocument = graphql(`
  query LoadCurrentUser {
    me {
      id
      login
      displayName
      avatarUrl
      presenceStatus
      hasVerifiedEmail
      settings {
        timezone
        timeFormat
      }
    }
  }
`);

// Re-export the CurrentUser type for use in load function return types
export type CurrentUser = NonNullable<LoadCurrentUserQuery['me']>;

// Module-level cache for the current user.
// Since we're in an SPA, this persists across navigations and prevents
// redundant network requests on every route change.
let cachedUser: CurrentUser | null = null;

/**
 * Load the current user from the GraphQL API.
 * Returns null if not authenticated.
 *
 * Uses a module-level cache to avoid redundant requests during navigation.
 * The cache is cleared when the user logs out (via clearCachedUser).
 *
 * On transient network errors (e.g., slow CI, server still warming up after reload),
 * retries once before giving up. Without this, a single GraphQL hiccup makes the SPA
 * render as unauthenticated and redirect away from authenticated routes.
 */
export async function loadCurrentUser(): Promise<CurrentUser | null> {
  if (!browser) {
    // In SPA mode, load functions only run in the browser.
    // If somehow called on server, return null (will trigger redirect).
    return null;
  }

  // Return cached user if available - avoids network request on every navigation
  if (cachedUser) {
    return cachedUser;
  }

  for (let attempt = 0; attempt < 2; attempt++) {
    const resp = await graphqlClientManager.originClient.client.query(
      LoadCurrentUserDocument,
      {},
      { requestPolicy: 'network-only' }
    );

    if (resp.error?.networkError && attempt === 0) {
      await new Promise((r) => setTimeout(r, 200));
      continue;
    }

    cachedUser = resp.data?.me ?? null;
    return cachedUser;
  }

  return null;
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
