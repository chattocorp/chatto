import { createContext } from 'svelte';
import type { UserStore } from './server/users.svelte';
import { mapUserSummary, mapUserPresenceView } from '$lib/api-client/userSummary';

export type CustomUserStatus = {
  emoji: string;
  text: string;
  expiresAt?: string | null;
};

type ProfileView = {
  displayName?: string;
  avatarUrl?: string | null;
  login?: string;
  customStatus?: CustomUserStatus | null;
  bio?: string | null;
  timezone?: string | null;
};

const [getUsers, setUsers] = createContext<() => UserStore | undefined>();

/** Install a read-only view of the connection's profiles during component
 * initialization. Standalone renderers can omit the getter and use row fallbacks. */
export function provideUserProfiles(users: () => UserStore | undefined = () => undefined): void {
  setUsers(users);
}

function readProfile(userId: string): ProfileView | undefined {
  const store = getUsers()();
  if (store?.isDeleted(userId)) return {
    displayName: '', login: '', avatarUrl: null, customStatus: null, bio: null, timezone: null
  };
  const user = store?.get(userId)?.user;
  return user ? { ...mapUserSummary(user), ...mapUserPresenceView(user) } : undefined;
}

export function isCustomStatusActive(
  status: CustomUserStatus | null | undefined
): status is CustomUserStatus {
  if (!status) return false;
  if (!status.expiresAt) return true;
  return Date.parse(status.expiresAt) > Date.now();
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
