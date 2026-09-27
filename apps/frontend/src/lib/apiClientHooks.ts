import { configureApiClientHooks } from '$lib/api-client/hooks';
import { isExplicitSignOutRedirectInProgress } from '$lib/auth/signOut';
import { serverRegistry } from '$lib/state/server/registry.svelte';

configureApiClientHooks({
  onAuthenticationRequired(serverId, source) {
    if (isExplicitSignOutRedirectInProgress() && serverRegistry.isOriginServer(serverId)) {
      return;
    }
    serverRegistry.confirmAuthenticationRequired(serverId, source).catch((error) => {
      console.warn('[auth] could not confirm the rejected session', { serverId, source }, error);
    });
  }
});
