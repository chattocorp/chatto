import '$lib/apiClientHooks';
import { loadCurrentUser } from '$lib/auth/loadAuth';
import { getPublicServerInfo } from '$lib/api-client/server';
import { preloadPublicLocaleMessages } from '$lib/i18n/messages';
import { isBackendCapableOrigin } from '$lib/runtimeOrigin';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

export const load: LayoutLoad = async ({ url }) => {
  const originHasBackend = isBackendCapableOrigin(url);
  const [, serverInfo, user] = await Promise.all([
    preloadPublicLocaleMessages(),
    originHasBackend ? getPublicServerInfo(url.origin).catch(() => null) : null,
    originHasBackend ? loadCurrentUser() : null
  ]);

  return {
    serverInfo,
    serverInfoLoaded: true,
    user
  };
};
