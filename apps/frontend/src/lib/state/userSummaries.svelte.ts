import { SvelteMap } from 'svelte/reactivity';
import type { UserAPI, UserSummary } from '$lib/api-client/users';

export class UserSummaryCache {
  readonly serverId: string;
  #entries = new SvelteMap<string, UserSummary>();
  #version = $state(0);

  constructor(serverId: string) {
    this.serverId = serverId;
  }

  prime(users: Iterable<UserSummary>): void {
    let changed = false;
    for (const user of users) {
      if (!user.id) continue;
      this.#entries.set(user.id, user);
      changed = true;
    }
    if (changed) this.#version++;
  }

  remove(userId: string): void {
    if (this.#entries.delete(userId)) this.#version++;
  }

  clear(): void {
    if (this.#entries.size === 0) return;
    this.#entries.clear();
    this.#version++;
  }

  get(userId: string): UserSummary | null {
    void this.#version;
    return this.#entries.get(userId) ?? null;
  }

  missing(userIds: Iterable<string>): string[] {
    void this.#version;
    const missing: string[] = [];
    const seen: string[] = [];
    for (const userId of userIds) {
      if (!userId || seen.includes(userId)) continue;
      seen.push(userId);
      if (!this.#entries.has(userId)) missing.push(userId);
    }
    return missing;
  }

  async loadMissing(api: Pick<UserAPI, 'batchGetUsers'>, userIds: Iterable<string>): Promise<void> {
    const missing = this.missing(userIds);
    if (missing.length === 0) return;
    this.prime(await api.batchGetUsers(missing));
  }
}

// Private singleton registry. Reactivity lives inside each cache instance.
// eslint-disable-next-line svelte/prefer-svelte-reactivity
const caches = new Map<string, UserSummaryCache>();

export function getUserSummaryCache(serverId: string): UserSummaryCache {
  let cache = caches.get(serverId);
  if (!cache) {
    cache = new UserSummaryCache(serverId);
    caches.set(serverId, cache);
  }
  return cache;
}

export function primeUserSummaryCache(serverId: string | undefined, users: Iterable<UserSummary>) {
  if (!serverId) return;
  getUserSummaryCache(serverId).prime(users);
}

export function removeUserSummaryCacheEntry(serverId: string | undefined, userId: string): void {
  if (!serverId) return;
  getUserSummaryCache(serverId).remove(userId);
}

export function clearUserSummaryCache(serverId: string | undefined): void {
  if (!serverId) return;
  getUserSummaryCache(serverId).clear();
}

export function __resetUserSummaryCachesForTests() {
  caches.clear();
}
