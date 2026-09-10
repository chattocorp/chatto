import '$lib/apiClientHooks';
import { redirect } from '@sveltejs/kit';
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

  if (serverInfo?.setupRequired && (url.pathname === '/' || url.pathname === '/login' || url.pathname.startsWith('/register'))) {
    redirect(302, '/setup');
  }

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
