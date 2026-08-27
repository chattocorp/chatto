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
const expiryCleanups = new SvelteMap<string, () => void>();
const MAX_TIMEOUT_DELAY_MS = 2_147_483_647;

export function isCustomStatusActive(
  status: CustomUserStatus | null | undefined
): status is CustomUserStatus {
  if (!status) return false;
  if (!status.expiresAt) return true;
  return Date.parse(status.expiresAt) > Date.now();
}

export function scheduleCustomStatusExpiry(
  status: CustomUserStatus | null | undefined,
  onExpire: () => void
): () => void {
  const expiresAt = status?.expiresAt;
  if (!expiresAt) return () => {};

  let timeout: ReturnType<typeof setTimeout> | undefined;
  let cancelled = false;

  const schedule = (fromTimer = false) => {
    if (cancelled) return;
    const expiresAtMs = Date.parse(expiresAt);
    if (Number.isNaN(expiresAtMs)) return;
    const delay = expiresAtMs - Date.now();
    if (delay <= 0) {
      if (fromTimer) {
        onExpire();
      } else {
        timeout = setTimeout(() => {
          if (!cancelled) onExpire();
        }, 0);
      }
      return;
    }
    timeout = setTimeout(() => schedule(true), Math.min(delay, MAX_TIMEOUT_DELAY_MS));
  };

  schedule();

  return () => {
    cancelled = true;
    if (timeout) clearTimeout(timeout);
  };
}

function scheduleExpiry(
  userId: string,
  status: CustomUserStatus | null | undefined,
  cache: SvelteMap<string, ProfileUpdate>
) {
  const existing = expiryCleanups.get(userId);
  if (existing) {
    existing();
    expiryCleanups.delete(userId);
  }
  if (!status?.expiresAt) return;
  const cleanup = scheduleCustomStatusExpiry(status, () => {
    const current = cache.get(userId);
    if (current?.customStatus?.expiresAt === status?.expiresAt) {
      cache.set(userId, { ...current, customStatus: null });
    }
    expiryCleanups.delete(userId);
  });
  expiryCleanups.set(userId, cleanup);
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
    },
    remove: (userId: string) => {
      expiryCleanups.get(userId)?.();
      expiryCleanups.delete(userId);
      state.current.delete(userId);
    },
    clear: () => {
      for (const userId of state.current.keys()) {
        expiryCleanups.get(userId)?.();
        expiryCleanups.delete(userId);
      }
      state.current.clear();
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
  return isCustomStatusActive(status) ? status : null;
}
