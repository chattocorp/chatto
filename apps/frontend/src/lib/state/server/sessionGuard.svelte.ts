import {
  registerQueryCacheRemovalListener,
  registerServerQueryCacheRemovalListener
} from '$lib/query/cacheRegistry';
import type { ServerScope } from './scope.svelte';
import type { ServerConnection } from './serverConnection.svelte';

/** The session in which a mutation started. */
export type SessionSnapshot = {
  readonly serverId: string;
  readonly connection: ServerConnection;
  readonly generation: number;
};

export type SessionGuard = ReturnType<typeof createSessionGuard>;

/**
 * Tells whether a mutation response still belongs to the screen that sent it.
 * Take a `snapshot()` when the mutation starts, and check it with `isCurrent()`
 * before the response changes the screen or its caches.
 *
 * A snapshot is stale after the component is destroyed, after `invalidate()`,
 * and after the server removes or rechecks its private query data. Account
 * settings use the `'server-session'` fence, which ignores changes to admin
 * data only. A change of server or session remounts the route subtree (see
 * {@link ServerScope}), so it destroys the component.
 *
 * Call it while a component initializes.
 */
export function createSessionGuard(
  scope: ServerScope,
  fence: 'private-data' | 'server-session' = 'private-data'
) {
  let generation = 0;
  const register =
    fence === 'server-session'
      ? registerServerQueryCacheRemovalListener
      : registerQueryCacheRemovalListener;
  const unregister = register((serverId) => {
    if (serverId === scope.serverId) generation++;
  });
  $effect(() => () => {
    generation++;
    unregister();
  });

  return {
    snapshot: (): SessionSnapshot => ({
      serverId: scope.serverId,
      connection: scope.connection,
      generation
    }),
    isCurrent: <T extends SessionSnapshot>(snapshot: T | null | undefined): snapshot is T =>
      snapshot != null && scope.isCurrent() && snapshot.generation === generation,
    invalidate: () => {
      generation++;
    }
  };
}
