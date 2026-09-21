import { mapDirectoryMember, type DirectoryMember } from '$lib/api-client/directoryMemberView';
import { getUserStore, memberFromSummary } from '$lib/state/server/users.svelte';

/** Compatibility ingestion adapter. Public profiles have one connection-scoped owner. */
export function primeDirectoryUsers(serverId: string, scope: string, users: DirectoryMember[], overwrite = true): void {
  const store = getUserStore(serverId, scope);
  for (const user of users) {
    const member = memberFromSummary(user, store.get(user.id));
    if (overwrite) store.set(user.id, member);
    else store.seed(member);
  }
}

export function removeDirectoryUser(serverId: string, scope: string, userId: string): void {
  getUserStore(serverId, scope).delete(userId);
}

/** Room membership and timeline consumers share profile reads and invalidation fences. */
export function createDirectoryUserLoader(
  serverId: string, scope: string, load: (ids: string[]) => Promise<DirectoryMember[]>
) {
  const store = getUserStore(serverId, scope);
  return async (ids: string[]): Promise<DirectoryMember[]> =>
    (await store.resolve(ids, async (batch) =>
      (await load(batch)).map((user) => memberFromSummary(user)))).map(mapDirectoryMember);
}
