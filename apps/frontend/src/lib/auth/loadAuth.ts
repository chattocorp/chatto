/** Route adapters for the per-server account owner. These helpers keep no user cache. */
import type { ServerRegistry } from '@chatto/client/server/registry';
import type { CurrentUser } from '@chatto/client/api/viewer';
import { isExplicitSignOutRedirectInProgress } from '@chatto/client/auth/signOut';

export type { CurrentUser };

/**
 * Refresh the origin account through the same owner used by startup and
 * recovery. `registry` is the application's client registry.
 */
export async function loadCurrentUser(registry: ServerRegistry): Promise<CurrentUser | null> {
  if (typeof window === 'undefined') return null;
  if (isExplicitSignOutRedirectInProgress()) {
    registry.clearOriginAuthentication();
    return null;
  }
  const origin = registry.originServer;
  if (!origin) return null;
  await registry.getStore(origin.id).currentUser.load();
  if (isExplicitSignOutRedirectInProgress()) {
    registry.clearOriginAuthentication();
    return null;
  }
  // A changed account can replace the store while the request is in flight.
  return registry.tryGetStore(origin.id)?.currentUser.user ?? null;
}
