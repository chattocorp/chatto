import { configureApiClientHooks } from '@chatto/api-client/hooks';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import { primeUserSummaryCache } from '$lib/state/userSummaries.svelte';

configureApiClientHooks({
  onAuthenticationRequired(serverId) {
    serverRegistry.handleAuthenticationRequired(serverId);
  },
  onUserSummaries(serverId, users) {
    primeUserSummaryCache(serverId, users);
  }
});
