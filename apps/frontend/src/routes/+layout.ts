import '$lib/apiClientHooks';
import { loadCurrentUser } from '$lib/auth/loadAuth';
import { getPublicServerInfo } from '@chatto/api-client/server';
import { preloadActiveLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

export const load: LayoutLoad = async ({ url }) => {
  await preloadActiveLocaleMessages();

  const [serverInfo, user] = await Promise.all([
    getPublicServerInfo(url.origin).catch(() => null),
    loadCurrentUser()
  ]);

  return {
    serverInfo,
    serverInfoLoaded: true,
    user
  };
};
