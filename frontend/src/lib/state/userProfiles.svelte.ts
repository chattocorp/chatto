import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';

/**
 * Global cache for live user profile updates (display name, avatar URL, login,
 * custom status).
 *
 * This store centralizes subscription to profile update events, avoiding
 * duplicate subscriptions across components. Components use the getLive*()
 * helpers to get the most recent values.
 */

export type CustomUserStatus = {
  emoji: string;
  text: string;
  expiresAt?: string | null;
};

type ProfileUpdate = {
  displayName?: string;
  avatarUrl?: string | null;
  login?: string;
  customStatus?: CustomUserStatus | null;
};

const [getCache, setCache] = createContext<{ current: SvelteMap<string, ProfileUpdate> }>();
const expiryTimers = new SvelteMap<string, ReturnType<typeof setTimeout>>();
const MAX_TIMEOUT_DELAY_MS = 2_147_483_647;

function isStatusActive(status: CustomUserStatus | null | undefined): status is CustomUserStatus {
  if (!status) return false;
  if (!status.expiresAt) return true;
  return new Date(status.expiresAt).getTime() > Date.now();
}

function scheduleExpiry(
  userId: string,
  status: CustomUserStatus | null | undefined,
  cache: SvelteMap<string, ProfileUpdate>
) {
  const existing = expiryTimers.get(userId);
  if (existing) {
    clearTimeout(existing);
    expiryTimers.delete(userId);
  }
  if (!status?.expiresAt) return;

  const expiresAtMs = new Date(status.expiresAt).getTime();
  if (Number.isNaN(expiresAtMs)) return;
  const delay = expiresAtMs - Date.now();
  if (delay <= 0) {
    const current = cache.get(userId);
    if (current) cache.set(userId, { ...current, customStatus: null });
    return;
  }

  const timeoutDelay = Math.min(delay, MAX_TIMEOUT_DELAY_MS);
  expiryTimers.set(
    userId,
    setTimeout(() => {
      const current = cache.get(userId);
      expiryTimers.delete(userId);
      if (current?.customStatus?.expiresAt === status.expiresAt) {
        if (expiresAtMs <= Date.now()) {
          cache.set(userId, { ...current, customStatus: null });
        } else {
          scheduleExpiry(userId, status, cache);
        }
      }
    }, timeoutDelay)
  );
}

function mergeProfileUpdate(
  cache: SvelteMap<string, ProfileUpdate>,
  userId: string,
  update: ProfileUpdate
) {
  const next = { ...(cache.get(userId) ?? {}), ...update };
  cache.set(userId, next);
  if ('customStatus' in update) {
    scheduleExpiry(userId, update.customStatus, cache);
  }
}

/**
 * Creates and sets the user profile cache context.
 * Must be called synchronously during component initialization (chat layout).
 * Returns update functions that can be safely called from event handlers.
 */
export function createUserProfileCache() {
  const state = $state<{ current: SvelteMap<string, ProfileUpdate> }>({
    current: new SvelteMap()
  });
  setCache(state);

  return {
    update: (
      userId: string,
      displayName: string,
      avatarUrl: string | null,
      login: string,
      customStatus?: CustomUserStatus | null
    ) => {
      const update: ProfileUpdate = { displayName, avatarUrl, login };
      if (customStatus !== undefined) update.customStatus = customStatus;
      mergeProfileUpdate(state.current, userId, update);
    },
    updateStatus: (userId: string, customStatus: CustomUserStatus | null) => {
      mergeProfileUpdate(state.current, userId, { customStatus });
    }
  };
}

/**
 * Get live display name if available, otherwise return fallback.
 */
export function getLiveDisplayName(userId: string, fallback: string): string {
  const cache = getCache();
  const update = cache.current.get(userId);
  return update && 'displayName' in update ? (update.displayName ?? fallback) : fallback;
}

/**
 * Get live avatar URL if available, otherwise return fallback.
 */
export function getLiveAvatarUrl(userId: string, fallback: string | null): string | null {
  const cache = getCache();
  const update = cache.current.get(userId);
  return update && 'avatarUrl' in update ? (update.avatarUrl ?? null) : fallback;
}

/**
 * Get live login if available, otherwise return fallback.
 */
export function getLiveLogin(userId: string, fallback: string): string {
  const cache = getCache();
  const update = cache.current.get(userId);
  return update && 'login' in update ? (update.login ?? fallback) : fallback;
}

/**
 * Get live custom status if available and active, otherwise return fallback.
 */
export function getLiveCustomStatus(
  userId: string,
  fallback: CustomUserStatus | null | undefined
): CustomUserStatus | null {
  const cache = getCache();
  const update = cache.current.get(userId);
  const status = update && 'customStatus' in update ? update.customStatus : fallback;
  return isStatusActive(status) ? status : null;
}
