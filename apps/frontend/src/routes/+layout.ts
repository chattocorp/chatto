import '$lib/apiClientHooks';
import { loadCurrentUser } from '$lib/auth/loadAuth';
import { getPublicServerInfo } from '$lib/api-client/server';
import { preloadActiveLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

export const load: LayoutLoad = async ({ route, url }) => {
  const [, serverInfo, user] = await Promise.all([
    preloadActiveLocaleMessages(route.id),
    getPublicServerInfo(url.origin).catch(() => null),
    loadCurrentUser()
  ]);

  return {
    serverInfo,
    serverInfoLoaded: true,
    user
  };
};
