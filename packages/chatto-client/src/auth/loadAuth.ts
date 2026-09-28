/** Route adapters for the per-server account owner. These helpers keep no user cache. */
import { serverRegistry } from '../server/registry.js';
import type { CurrentUser } from '../api/viewer.js';
import { isExplicitSignOutRedirectInProgress } from './signOut.js';

export type { CurrentUser };

/** Refresh the origin account through the same owner used by startup and recovery. */
export async function loadCurrentUser(): Promise<CurrentUser | null> {
  if (typeof window === 'undefined') return null;
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
