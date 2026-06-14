import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';

/**
 * Global cache for live user profile updates (display name, avatar URL, login).
 *
 * This store centralizes subscription to profile update events, avoiding
 * duplicate subscriptions across components. Components use getLiveDisplayName(),
 * getLiveAvatarUrl(), and getLiveLogin() to get the most recent values.
 *
 * Pattern: The chat layout calls createUserProfileCache() during initialization,
 * which returns an update function. The layout then subscribes to onUserProfileUpdate()
 * and calls the returned update function to populate the cache.
 * Components read from the cache via getLiveDisplayName(), getLiveAvatarUrl(), and getLiveLogin().
 */

type ProfileUpdate = {
  displayName: string;
  avatarUrl: string | null;
  login: string;
};

const [getCache, setCache] = createContext<{ current: SvelteMap<string, ProfileUpdate> }>();

/**
 * Creates and sets the user profile cache context.
 * Must be called synchronously during component initialization (chat layout).
 * Returns an update function that can be safely called from event handlers.
 */
export function createUserProfileCache() {
  const state = $state<{ current: SvelteMap<string, ProfileUpdate> }>({
    current: new SvelteMap()
  });
  setCache(state);

  // Return update function with closure over state (no getContext() call needed)
  return {
    update: (userId: string, displayName: string, avatarUrl: string | null, login: string) => {
      state.current.set(userId, { displayName, avatarUrl, login });
    }
  };
}

/**
 * Get live display name if available, otherwise return fallback.
 * Use with $derived for reactive updates:
 *
 * ```svelte
 * const displayName = $derived(getLiveDisplayName(user.id, user.displayName));
 * ```
 */
export function getLiveDisplayName(userId: string, fallback: string): string {
  const cache = getCache();
  return cache.current.get(userId)?.displayName ?? fallback;
}

/**
 * Get live avatar URL if available, otherwise return fallback.
 * Use with $derived for reactive updates:
 *
 * ```svelte
 * const avatarUrl = $derived(getLiveAvatarUrl(user.id, user.avatarUrl));
 * ```
 */
export function getLiveAvatarUrl(userId: string, fallback: string | null): string | null {
  const cache = getCache();
  return cache.current.get(userId)?.avatarUrl ?? fallback;
}

/**
 * Get live login if available, otherwise return fallback.
 * Use with $derived for reactive updates:
 *
 * ```svelte
 * const login = $derived(getLiveLogin(user.id, user.login));
 * ```
 */
export function getLiveLogin(userId: string, fallback: string): string {
  const cache = getCache();
  return cache.current.get(userId)?.login ?? fallback;
}
