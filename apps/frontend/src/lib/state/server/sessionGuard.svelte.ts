import {
  registerQueryCacheRemovalListener,
  registerServerQueryCacheRemovalListener
} from '$lib/query/cacheRegistry';
import type { ServerScope } from './scope.svelte';
import type { ServerConnection } from './serverConnection.svelte';

/** The server session in which a mutation started. Check it with {@link SessionGuard.isCurrent}. */
export type SessionSnapshot = {
  readonly serverId: string;
  readonly connection: ServerConnection;
  readonly generation: number;
};

/**
 * Which cache removal ends the session:
 * - `private-data`: the server removes its private or admin query data, for
 *   example at sign-out or when the viewer loses admin rights.
 * - `server-session`: the complete server session is disposed. Account
 *   settings use this, because they stay valid when admin data is removed.
 */
export type SessionFence = 'private-data' | 'server-session';

/**
 * Tells whether the response to a mutation still belongs to the screen that
 * sent it. Take a {@link snapshot} when the mutation starts, and check it with
 * {@link isCurrent} before the response changes the screen or its caches.
 *
 * A snapshot becomes stale when the route leaves its server, the connection
 * changes its query scope, the component is destroyed, the fence removes the
 * server's query data, or the owner calls {@link invalidate}.
 *
 * Create the guard while a component initializes.
 */
export class SessionGuard {
  readonly #scope: ServerScope;
  /**
   * The session generation. A plain field is the source, because an effect
   * teardown reads the old value of `$state`. `#reactiveGeneration` mirrors it
   * so that derived values that call {@link isCurrent} update.
   */
  #generation = 0;
  #reactiveGeneration = $state(0);

  constructor(scope: ServerScope, fence: SessionFence = 'private-data') {
    this.#scope = scope;
    const register =
      fence === 'server-session'
        ? registerServerQueryCacheRemovalListener
        : registerQueryCacheRemovalListener;
    const unregister = register((serverId) => {
      if (serverId === scope.serverId) this.invalidate();
    });
    // The teardown runs when the owning component is destroyed.
    $effect(() => () => {
      this.invalidate();
      unregister();
    });
  }

  /** The current session, to attach to mutation variables. */
  snapshot(): SessionSnapshot {
    return {
      serverId: this.#scope.serverId,
      connection: this.#scope.connection,
      generation: this.#generation
    };
  }

  /** Whether a snapshot still belongs to the current session. Reactive in derived values. */
  isCurrent<T extends SessionSnapshot>(snapshot: T | null | undefined): snapshot is T {
    return (
      snapshot != null &&
      this.#scope.isCurrent() &&
      snapshot.serverId === this.#scope.serverId &&
      snapshot.connection.queryScope === this.#scope.connection.queryScope &&
      snapshot.generation === this.#reactiveGeneration
    );
  }

  /** Make every earlier snapshot stale, for example when the page changes its subject. */
  invalidate(): void {
    this.#reactiveGeneration = ++this.#generation;
  }
}
