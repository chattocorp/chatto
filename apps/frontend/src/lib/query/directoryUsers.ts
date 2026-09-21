import type { DirectoryMember } from '$lib/api-client/memberDirectory';
import { queryClient } from './client';
import { registerDirectoryUserCache } from './cacheRegistry';
import { createSubscriber } from 'svelte/reactivity';

const trackCachedUsers = createSubscriber((update) =>
  queryClient.getQueryCache().subscribe((event) => {
    if (event.query.queryKey[4] === 'directory-user') update();
  })
);

/** Read already-loaded public profiles for one connection session without fetching.
 * Null entries preserve removal markers; callers must not revive them from older state.
 * Reactive consumers also observe profile replacements and session cache removal. */
export function readCachedDirectoryUsers(
  serverId: string,
  scope: string
): Array<[string, DirectoryMember | null]> {
  trackCachedUsers();
  return queryClient.getQueryCache().findAll({
    queryKey: ['server', serverId, 'session', scope, 'directory-user']
  }).flatMap((query) => {
    const user = query.state.data as DirectoryMember | null | undefined;
    return user === undefined ? [] : [[String(query.queryKey[5]), user] as [string, DirectoryMember | null]];
  });
}

/** Session-scoped profiles shared by room directories. Realtime reads replace
 * these entries; navigation and window focus do not refetch them. */
export function directoryUserKey(serverId: string, scope: string, userId: string) {
  return ['server', serverId, 'session', scope, 'directory-user', userId] as const;
}

export function primeDirectoryUsers(
  serverId: string,
  scope: string,
  users: DirectoryMember[],
  overwrite = true
): void {
  for (const user of users) {
    const queryKey = directoryUserKey(serverId, scope, user.id);
    // Incidental timeline reads can seed missing profiles, but must not replace
    // a newer realtime value, deletion marker, or an in-flight directory read.
    if (!overwrite && queryClient.getQueryState(queryKey)) continue;
    void queryClient.cancelQueries({ queryKey, exact: true }, { revert: true });
    queryClient.setQueryData(queryKey, user);
  }
}

export function removeDirectoryUser(serverId: string, scope: string, userId: string): void {
  const queryKey = directoryUserKey(serverId, scope, userId);
  void queryClient.cancelQueries({ queryKey, exact: true }, { revert: true });
  queryClient.setQueryData(queryKey, null);
}

registerDirectoryUserCache({
  prime: primeDirectoryUsers,
  remove: removeDirectoryUser,
  reset: (serverId) =>
    queryClient.removeQueries({
      predicate: (query) =>
        query.queryKey[0] === 'server' &&
        query.queryKey[1] === serverId &&
        query.queryKey[4] === 'directory-user'
    })
});

/** Coalesce cache misses from concurrent consumers into batches of at most 100.
 * TanStack owns per-user deduplication, cancellation, and session cleanup. */
export function createDirectoryUserLoader(
  serverId: string,
  scope: string,
  load: (ids: string[]) => Promise<DirectoryMember[]>
) {
  let pending: Array<{
    id: string;
    resolve: (user: DirectoryMember | null) => void;
    reject: (error: unknown) => void;
  }> = [];

  async function flush(): Promise<void> {
    const batch = pending;
    pending = [];
    // A fixed batch size also bounds the server's hydration work per request.
    for (let offset = 0; offset < batch.length; offset += 100) {
      const chunk = batch.slice(offset, offset + 100);
      try {
        const users = new Map(
          (await load(chunk.map((entry) => entry.id))).map((user) => [user.id, user])
        );
        for (const entry of chunk) entry.resolve(users.get(entry.id) ?? null);
      } catch (error) {
        for (const entry of chunk) entry.reject(error);
      }
    }
  }

  return async (ids: string[]): Promise<DirectoryMember[]> => {
    const users = await Promise.all(
      [...new Set(ids)].map(async (id) => {
        const queryKey = directoryUserKey(serverId, scope, id);
        try {
          return await queryClient.fetchQuery({
            queryKey,
            staleTime: Infinity,
            gcTime: Infinity,
            retry: false,
            queryFn: () =>
              new Promise<DirectoryMember | null>((resolve, reject) => {
                pending.push({ id, resolve, reject });
                if (pending.length === 1)
                  queueMicrotask(() => {
                    void flush();
                  });
              })
          });
        } catch (error) {
          // A canonical realtime read can supersede an in-flight hydration.
          const current = queryClient.getQueryData<DirectoryMember | null>(queryKey);
          if (current !== undefined) return current;
          throw error;
        }
      })
    );
    return users.filter((user): user is DirectoryMember => user !== null && !user.deleted);
  };
}
