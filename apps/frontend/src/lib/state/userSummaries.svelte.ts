import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import type { UserAPI, UserSummary } from '$lib/api-client/users';
import type { UserSummaryReader } from '$lib/api-client/hooks';
import { StaleResponseError } from '$lib/api-client/connect';

export class UserSummaryCache {
  readonly serverId: string;
  #entries = new SvelteMap<string, UserSummary>();
  #version = $state(0);
  #generation = 0;
  readonly #pending = new SvelteMap<string, { completion: Promise<void>; cursor?: string }>();

  constructor(serverId: string) {
    this.serverId = serverId;
  }

  prime(users: Iterable<UserSummary>): void {
    let changed = false;
    for (const user of users) {
      if (!user.id) continue;
      this.#entries.set(user.id, user);
      this.#pending.delete(user.id);
      changed = true;
    }
    if (changed) this.#version++;
  }

  remove(userId: string): void {
    this.#pending.delete(userId);
    if (this.#entries.delete(userId)) this.#version++;
  }

  clear(): void {
    this.#generation++;
    this.#pending.clear();
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
    await this.resolve([...userIds], (ids) => api.batchGetUsers(ids));
  }

  /**
   * Share missing-user reads across message consumers. A cached profile remains
   * valid until a profile event invalidates it; message cursors do not invalidate
   * unchanged users. Removal and reset fence reads already in flight.
   */
  async resolve(ids: string[], read: UserSummaryReader, minimumCursor?: string): Promise<UserSummary[]> {
    const generation = this.#generation;
    const weakerReads = ids.filter((id) => {
      const pending = this.#pending.get(id);
      return pending && pending.cursor !== minimumCursor;
    });
    const missing = this.missing(ids).filter((id) => !this.#pending.has(id));
    for (let offset = 0; offset < missing.length; offset += 100) {
      const batch = missing.slice(offset, offset + 100);
      const request = Promise.resolve().then(() => read(batch, minimumCursor)).then((users) => {
        if (generation !== this.#generation) throw new StaleResponseError(false);
        const byId = new SvelteMap(users.map((user) => [user.id, user]));
        for (const id of batch) {
          if (this.#pending.get(id)?.completion !== request) {
            if (!this.#entries.has(id)) throw new StaleResponseError(false);
            continue;
          }
          const user = byId.get(id);
          if (user) this.prime([user]);
        }
      }).finally(() => {
        for (const id of batch) if (this.#pending.get(id)?.completion === request) this.#pending.delete(id);
      });
      for (const id of batch) this.#pending.set(id, { completion: request, cursor: minimumCursor });
    }
    await Promise.all([...new SvelteSet(ids.flatMap((id) => this.#pending.get(id)?.completion ?? []))]);
    if (generation !== this.#generation) throw new StaleResponseError(false);
    // A missing result at a different opaque boundary is not evidence that the
    // user is absent at this caller's boundary. Retry only those unresolved IDs.
    if (this.missing(weakerReads).length > 0) return this.resolve(ids, read, minimumCursor);
    return [...new SvelteSet(ids)].flatMap((id) => this.get(id) ?? []);
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
