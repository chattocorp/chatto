import '$lib/apiClientHooks';
import { loadCurrentUser } from '$lib/auth/loadAuth';
import { getPublicServerInfo } from '$lib/api-client/server';
import { preloadPublicLocaleMessages } from '$lib/i18n/messages';
import { isBackendCapableOrigin } from '$lib/runtimeOrigin';
import { serverRegistry } from '$lib/state/server/registry.svelte';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

export const load: LayoutLoad = async ({ url }) => {
  const originHasBackend = isBackendCapableOrigin(url);
  // Initialise persisted remote sessions before child route loads read them.
  // This is idempotent across SPA navigations.
  serverRegistry.init();
  const [, serverInfo, user] = await Promise.all([
    preloadPublicLocaleMessages(),
    originHasBackend ? getPublicServerInfo(url.origin).catch(() => null) : null,
    originHasBackend ? loadCurrentUser() : null
  ]);

  // Child route loads need a settled origin registry to resolve the "-" URL
  // segment and make authentication decisions before components render.
  await serverRegistry.probeOrigin(user !== null, undefined, serverInfo ?? undefined);
  if (!user) serverRegistry.settleOriginUnauthenticated();

  return {
    serverInfo,
    serverInfoLoaded: true,
    user
  };
};
