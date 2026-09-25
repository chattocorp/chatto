/** Route adapters for the per-server account owner. These helpers keep no user cache. */
import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import { browser } from '$app/environment';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import type { CurrentUser } from '$lib/api-client/viewer';
import { saveReturnUrl } from './returnNavigation';
import { isExplicitSignOutRedirectInProgress } from './signOut';

export type { CurrentUser };

/** Refresh the origin account through the same owner used by startup and recovery. */
export async function loadCurrentUser(): Promise<CurrentUser | null> {
  if (!browser) return null;
  if (isExplicitSignOutRedirectInProgress()) {
    serverRegistry.clearOriginAuthentication();
    return null;
  }
  const origin = serverRegistry.originServer;
  if (!origin) return null;
  await serverRegistry.getStore(origin.id).currentUser.load();
  if (isExplicitSignOutRedirectInProgress()) {
    serverRegistry.clearOriginAuthentication();
    return null;
  }
  // A changed account can replace the store while the request is in flight.
  return serverRegistry.tryGetStore(origin.id)?.currentUser.user ?? null;
}

/** Require an account in a route loader and preserve its return URL on redirect. */
export async function requireAuth(returnUrl?: string): Promise<CurrentUser> {
  return requireUser(await loadCurrentUser(), returnUrl);
}

/** Require account data from a parent loader. Session validity is owned by the server store. */
export function requireUser(user: CurrentUser | null, returnUrl?: string): CurrentUser {
  if (!user) {
    if (returnUrl && browser) saveReturnUrl(returnUrl);
    redirect(302, resolve('/'));
  }
  return user;
}
