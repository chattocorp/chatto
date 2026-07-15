import { createContext, untrack } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import type { PresenceStatus } from '$lib/render/types';

export type PresenceCacheScope = {
  serverId: string;
  userId: string;
};

/**
 * Server-scoped cache for live user presence updates.
 *
 * Presence is server-wide, not global across Chatto servers. A user can have
 * different ids on different servers, and two servers can also contain the
 * same user id string. Cache entries therefore include both server id and user
 * id so avatar dots always read the presence for the active server.
 */
export class PresenceCache {
  #entries = new SvelteMap<string, PresenceStatus>();
  #version = $state(0);

  update(scope: PresenceCacheScope, status: PresenceStatus) {
    untrack(() => {
      this.#entries.set(presenceCacheKey(scope), status);
      this.#version++;
    });
  }

  clear(retainedEntries: Iterable<readonly [PresenceCacheScope, PresenceStatus]> = []) {
    untrack(() => {
      this.#entries.clear();
      for (const [scope, status] of retainedEntries) {
        this.#entries.set(presenceCacheKey(scope), status);
      }
      this.#version++;
    });
  }

  get(scope: PresenceCacheScope, fallback: PresenceStatus): PresenceStatus {
    void this.#version;
    return this.#entries.get(presenceCacheKey(scope)) ?? fallback;
  }

  get version() {
    return this.#version;
  }
}

export type PresenceCacheCurrentUserStore = {
  serverId: string;
  isAuthenticated: boolean;
  currentUser: {
    user?: { id: string } | null;
  };
};

export function authenticatedCurrentUserPresenceEntries(
  stores: Iterable<PresenceCacheCurrentUserStore | undefined | null>,
  status: PresenceStatus
): Array<readonly [PresenceCacheScope, PresenceStatus]> {
  const entries: Array<readonly [PresenceCacheScope, PresenceStatus]> = [];
  for (const store of stores) {
    if (!store?.isAuthenticated || !store.currentUser.user) continue;
    entries.push([{ serverId: store.serverId, userId: store.currentUser.user.id }, status]);
  }
  return entries;
}

export function updateAuthenticatedCurrentUserPresenceEntries(
  presenceCache: PresenceCache,
  stores: Iterable<PresenceCacheCurrentUserStore | undefined | null>,
  status: PresenceStatus
) {
  for (const [scope, retainedStatus] of authenticatedCurrentUserPresenceEntries(stores, status)) {
    presenceCache.update(scope, retainedStatus);
  }
}

function presenceCacheKey({ serverId, userId }: PresenceCacheScope): string {
  return `${serverId}\u0000${userId}`;
}

const [getCache, setCache] = createContext<PresenceCache>();

/**
 * Creates and sets the presence cache context.
 * Must be called synchronously during component initialization (chat layout).
 */
export function createPresenceCache(): PresenceCache {
  const cache = new PresenceCache();
  setCache(cache);
  return cache;
}

/**
 * Get the presence cache from context.
 * Must be called during component initialization (captures context).
 */
export function getPresenceCache(): PresenceCache {
  return getCache();
}
