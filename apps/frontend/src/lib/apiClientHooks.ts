import { configureApiClientHooks } from '$lib/api-client/hooks';
import { isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
import { serverRegistry } from '$lib/state/server/registry.svelte';

configureApiClientHooks({
  onAuthenticationRequired(serverId) {
    if (isExplicitSignOutRedirectInProgress() && serverRegistry.isOriginServer(serverId)) {
      return;
    }
    serverRegistry.handleAuthenticationRequired(serverId);
  }
});
