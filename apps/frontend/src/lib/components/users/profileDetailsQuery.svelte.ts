import { createUserAPI } from '@chatto/client/api/users';
import { createQuery } from '$lib/query/client';
import { profileDetailsKey } from '$lib/query/keys';
import { useServerScope } from '$lib/state/server/scope.svelte';

/**
 * Shared read of a profile's roles, account age, and verified identities. The
 * Profile Card and Profile View use one cache entry per user and session, and
 * stale data stays visible during a refresh.
 */
export function useProfileDetails(getUserId: () => string) {
  const scope = useServerScope();
  return createQuery(() => {
    const userId = getUserId();
    return {
      queryKey: profileDetailsKey(scope, userId),
      queryFn: async ({ signal }) => {
        const details = await scope.connection
          .getAPI(createUserAPI)
          .getProfileDetails(userId, signal);
        // Role names come from the shared catalogue; load it once roles need labels.
        if (details.roles.length) void scope.store.mentionRoles.load();
        return details;
      },
      enabled: !!userId,
      retry: false
    };
  });
}
