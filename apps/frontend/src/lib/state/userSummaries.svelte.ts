import type { UserAPI, UserSummary } from '$lib/api-client/users';
import { mapUserSummary } from '$lib/api-client/userSummary';
import type { UserSummaryReader } from '$lib/api-client/hooks';
import { clearUserStores, getUserStore, memberFromSummary, resetUserStoresForTests, type UserStore } from './server/users.svelte';

/** Summary-shaped adapter over the shared user store. It owns no profile data. */
export class UserSummaryCache {
  readonly store: UserStore;
  constructor(readonly serverId: string, readonly scope?: string) {
    this.store = getUserStore(serverId, scope);
  }
  prime(users: Iterable<UserSummary>): void {
    for (const user of users) this.store.set(user.id, memberFromSummary(user, this.store.get(user.id)));
  }
  remove(id: string): void { this.store.invalidate(id); }
  clear(): void { this.store.clear(); }
  get(id: string): UserSummary | null {
    const user = this.store.get(id)?.user;
    return user ? mapUserSummary(user) : null;
  }
  missing(ids: Iterable<string>): string[] { return this.store.missing(ids); }
  async loadMissing(api: Pick<UserAPI, 'batchGetUsers'>, ids: Iterable<string>): Promise<void> {
    const missing = this.missing(ids);
    if (missing.length) await this.store.readSnapshot(async () =>
      (await api.batchGetUsers(missing)).map((user) => memberFromSummary(user)));
  }
  async resolve(ids: string[], read: UserSummaryReader, cursor?: string): Promise<UserSummary[]> {
    const members = await this.store.resolve(ids, async (batch, boundary) =>
      (await read(batch, boundary)).map((user) => memberFromSummary(user)), cursor);
    return members.flatMap((member) => member.user ? [mapUserSummary(member.user)] : []);
  }
}

export function getUserSummaryCache(serverId: string, scope?: string): UserSummaryCache {
  return new UserSummaryCache(serverId, scope);
}
export function primeUserSummaryCache(serverId: string | undefined, users: Iterable<UserSummary>): void {
  if (serverId) getUserSummaryCache(serverId).prime(users);
}
export function removeUserSummaryCacheEntry(serverId: string | undefined, id: string): void {
  if (serverId) getUserSummaryCache(serverId).remove(id);
}
export function clearUserSummaryCache(serverId: string | undefined): void {
  if (serverId) clearUserStores(serverId);
}
export const __resetUserSummaryCachesForTests = resetUserStoresForTests;
