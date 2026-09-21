import { createContext } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import type { UserStore } from './server/users.svelte';
import { mapUserSummary, mapUserPresenceView } from '$lib/api-client/userSummary';
import { scheduleCustomStatusExpiry } from '$lib/utils/customStatusExpiry';
export { scheduleCustomStatusExpiry } from '$lib/utils/customStatusExpiry';

/**
 * Context-bound profile views. Production contexts read the connection's shared
 * user store. Standalone renderers and stories can install a local profile map.
 * The getLive* helpers preserve render fallbacks for profiles not loaded yet.
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
  bio?: string | null;
  timezone?: string | null;
};

const [getCache, setCache] = createContext<{
  current: SvelteMap<string, ProfileUpdate>;
  users?: () => UserStore | undefined;
}>();

/** Production contexts read the shared connection owner. The local map supports
 * standalone renderers and stories that have no authenticated server scope. */
function readProfile(userId: string): ProfileUpdate | undefined {
  const cache = getCache();
  if (!cache.users) return cache.current.get(userId);
  const store = cache.users();
  if (store?.isDeleted(userId)) return { displayName: '', login: '', avatarUrl: null, customStatus: null, bio: null, timezone: null };
  const user = store?.get(userId)?.user;
  return user ? { ...mapUserSummary(user), ...mapUserPresenceView(user) } : undefined;
}
const expiryCleanups = new SvelteMap<string, () => void>();

export function isCustomStatusActive(
  status: CustomUserStatus | null | undefined
): status is CustomUserStatus {
  if (!status) return false;
  if (!status.expiresAt) return true;
  return Date.parse(status.expiresAt) > Date.now();
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
export function createUserProfileCache(users?: () => UserStore | undefined) {
  const state = $state<{ current: SvelteMap<string, ProfileUpdate>; users?: () => UserStore | undefined }>({
    current: new SvelteMap(), users
  });
  setCache(state);

  return {
    update: (
      userId: string,
      displayName: string,
      avatarUrl: string | null,
      login: string,
      customStatus?: CustomUserStatus | null,
      extras?: { bio?: string | null; timezone?: string | null }
    ) => {
      if (users) return; // The server reducer owns profile writes.
      const update: ProfileUpdate = { displayName, avatarUrl, login };
      if (customStatus !== undefined) update.customStatus = customStatus;
      if (extras) {
        if ('bio' in extras) update.bio = extras.bio ?? null;
        if ('timezone' in extras) update.timezone = extras.timezone ?? null;
      }
      mergeProfileUpdate(state.current, userId, update);
    },
    updateStatus: (userId: string, customStatus: CustomUserStatus | null) => {
      if (users) return;
      mergeProfileUpdate(state.current, userId, { customStatus });
    },
    remove: (userId: string) => {
      if (users) return;
      expiryCleanups.get(userId)?.();
      expiryCleanups.delete(userId);
      state.current.delete(userId);
    },
    clear: () => {
      if (users) return;
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
  const update = readProfile(userId);
  return update && 'displayName' in update ? (update.displayName ?? fallback) : fallback;
}

/**
 * Get live avatar URL if available, otherwise return fallback.
 */
export function getLiveAvatarUrl(userId: string, fallback: string | null): string | null {
  const update = readProfile(userId);
  return update && 'avatarUrl' in update ? (update.avatarUrl ?? null) : fallback;
}

/**
 * Get live login if available, otherwise return fallback.
 */
export function getLiveLogin(userId: string, fallback: string): string {
  const update = readProfile(userId);
  return update && 'login' in update ? (update.login ?? fallback) : fallback;
}

/**
 * Get live custom status if available and active, otherwise return fallback.
 */
export function getLiveCustomStatus(
  userId: string,
  fallback: CustomUserStatus | null | undefined
): CustomUserStatus | null {
  const update = readProfile(userId);
  const status = update && 'customStatus' in update ? update.customStatus : fallback;
  return isCustomStatusActive(status) ? status : null;
}

/**
 * Get the live public bio if available, otherwise return fallback.
 */
export function getLiveBio(userId: string, fallback: string | null): string | null {
  const update = readProfile(userId);
  return update && 'bio' in update ? (update.bio ?? null) : fallback;
}

/**
 * Get the live public time zone if available, otherwise return fallback.
 */
export function getLiveTimezone(userId: string, fallback: string | null): string | null {
  const update = readProfile(userId);
  return update && 'timezone' in update ? (update.timezone ?? null) : fallback;
}
