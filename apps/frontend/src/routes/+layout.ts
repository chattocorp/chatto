import '$lib/apiClientHooks';
import { loadViewerState } from '$lib/auth/loadAuth';
import { getPublicServerInfo } from '$lib/api-client/server';
import { preloadActiveLocaleMessages } from '$lib/i18n/messages';
import type { LayoutLoad } from './$types';

// SPA mode - no server-side rendering
export const ssr = false;

export const load: LayoutLoad = async ({ url }) => {
  await preloadActiveLocaleMessages();

  const [serverInfo, viewer] = await Promise.all([
    getPublicServerInfo(url.origin).catch(() => null),
    loadViewerState()
  ]);

  return {
    serverInfo,
    serverInfoLoaded: true,
    viewer,
    user: viewer?.user ?? null
  };
};
