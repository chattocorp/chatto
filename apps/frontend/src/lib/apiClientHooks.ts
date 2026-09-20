import { configureApiClientHooks } from '$lib/api-client/hooks';
import { isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { getUserSummaryCache, primeUserSummaryCache } from '$lib/state/userSummaries.svelte';

configureApiClientHooks({
  resolveUserSummaries: (serverId, ids, read, cursor) =>
    getUserSummaryCache(serverId).resolve(ids, read, cursor),
  onAuthenticationRequired(serverId) {
    if (isExplicitSignOutRedirectInProgress() && serverRegistry.isOriginServer(serverId)) {
      return;
    }
    serverRegistry.handleAuthenticationRequired(serverId);
  },
  onUserSummaries(serverId, users) {
    primeUserSummaryCache(serverId, users);
  }
});
