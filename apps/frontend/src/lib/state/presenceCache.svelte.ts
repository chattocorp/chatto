import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import type { PresenceStatus } from '$lib/render/types';

/**
 * Global cache for live user presence updates.
 *
 * Solves the problem where newly-mounted UserAvatar components (e.g., in popovers)
 * show stale presence from the initial server snapshot because they missed earlier
 * PresenceChangedEvents. By writing all presence updates to a shared cache,
 * any component can read the most recently observed status at mount time.
 *
 * The chat layout calls createPresenceCache() during initialization,
 * ServerEventProvider populates it, and components read via get().
 */
export class PresenceCache {
  #entries = new SvelteMap<string, PresenceStatus>();
  #version = $state(0);

  update(userId: string, status: PresenceStatus) {
    this.#entries.set(userId, status);
    this.#version++;
  }

  clear() {
    this.#entries.clear();
    this.#version++;
  }

  get(userId: string, fallback: PresenceStatus): PresenceStatus {
    void this.#version;
    return this.#entries.get(userId) ?? fallback;
  }

  get version() {
    return this.#version;
  }
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
